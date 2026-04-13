package service

import (
	"container/list"
	"context"
	"fmt"
	"log"
	"sync"

	"github.com/sageil/kodacode/v1/internal/config"
	"github.com/sageil/kodacode/v1/internal/message"
	"github.com/sageil/kodacode/v1/internal/pipeline"
	"github.com/sageil/kodacode/v1/internal/provider"
	"github.com/sageil/kodacode/v1/internal/repository"
	"github.com/sageil/kodacode/v1/internal/tool"
)

type compactionConfig struct {
	threshold       float64
	keepTurns       int
	pruneProtect    int
	pruneMinSavings int
	contextLimit    float64
}

const firstTurnSessionLimit = 4096

type boundedSessionSet struct {
	mu         sync.Mutex
	maxEntries int
	order      *list.List
	entries    map[string]*list.Element
}

func newBoundedSessionSet(maxEntries int) *boundedSessionSet {
	if maxEntries <= 0 {
		maxEntries = firstTurnSessionLimit
	}
	return &boundedSessionSet{
		maxEntries: maxEntries,
		order:      list.New(),
		entries:    make(map[string]*list.Element, maxEntries),
	}
}

func (s *boundedSessionSet) markSeen(sessionID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if sessionID == "" {
		return false
	}
	if elem, ok := s.entries[sessionID]; ok {
		s.order.MoveToFront(elem)
		return false
	}
	s.entries[sessionID] = s.order.PushFront(sessionID)
	if len(s.entries) > s.maxEntries {
		oldest := s.order.Back()
		if oldest != nil {
			s.order.Remove(oldest)
			if sid, ok := oldest.Value.(string); ok {
				delete(s.entries, sid)
			}
		}
	}
	return true
}

func derefFloat(p *float64) float64 {
	if p == nil {
		return 0
	}
	return *p
}

func derefInt(p *int) int {
	if p == nil {
		return 0
	}
	return *p
}

func resolveCompactionConfig(cfg *config.SessionConfig, providerID, modelID string, contextSize int) compactionConfig {
	cc := compactionConfig{
		threshold:       derefFloat(cfg.CompactionThreshold),
		keepTurns:       derefInt(cfg.CompactionKeepTurns),
		pruneProtect:    derefInt(cfg.PruneProtectTokens),
		pruneMinSavings: derefInt(cfg.PruneMinSavings),
		contextLimit:    derefFloat(cfg.ContextLimit),
	}
	if mc, ok := cfg.ModelConfig(providerID, modelID); ok {
		if mc.CompactionThreshold != nil {
			cc.threshold = *mc.CompactionThreshold
		}
		if mc.CompactionKeepTurns != nil {
			cc.keepTurns = *mc.CompactionKeepTurns
		}
		if mc.PruneProtectTokens != nil {
			cc.pruneProtect = *mc.PruneProtectTokens
		}
		if mc.PruneMinSavings != nil {
			cc.pruneMinSavings = *mc.PruneMinSavings
		}
		if mc.ContextLimit != nil {
			cc.contextLimit = *mc.ContextLimit
		}
	}
	if cc.threshold > 0 && contextSize > 0 && contextSize < 256000 {
		minThreshold := 0.60
		if contextSize < 64000 {
			minThreshold = 0.70
		}
		if cc.threshold < minThreshold {
			log.Printf("compaction: raising threshold from %.2f to %.2f (minimum for %d-token context)", cc.threshold, minThreshold, contextSize)
			cc.threshold = minThreshold
		}
	}
	return cc
}

// NewCompactionMiddleware returns a TurnMiddleware that:
//   - Pre-turn: checks token count; runs structured compaction if above threshold;
//     injects summary text into req.SystemParts[2].
//   - Post-turn: prunes old tool output parts.
func NewCompactionMiddleware(
	sessionCfg *config.SessionConfig,
	msgs repository.MessageRepo,
	registry *provider.Registry,
	toolRegistry *tool.Registry,
	appCfg *config.Config,
	publish func(sessionID string, ev SSEEvent),
	getCost func(ctx context.Context, sessionID string) *SessionCost,
	utilityHealth *utilityHealthTracker,
) pipeline.TurnMiddleware {
	// Track recently-seen sessions for one-time crash-orphan cleanup without
	// retaining unbounded process-lifetime state.
	firstTurnDone := newBoundedSessionSet(firstTurnSessionLimit)

	return func(ctx context.Context, req *pipeline.TurnRequest, next pipeline.TurnHandler) error {
		if req.Ephemeral {
			return next(ctx, req)
		}
		var isReadOnly func(string) bool
		if toolRegistry != nil {
			isReadOnly = toolRegistry.IsReadOnly
		}
		var lastInputTokens int
		if getCost != nil {
			if sc := getCost(ctx, req.SessionID); sc != nil {
				lastInputTokens = sc.LastInputTokens()
			}
		}

		// On the first turn of each session, clean up any orphaned tool_call
		// parts left by a previous crash or kill.
		needsCleanup := firstTurnDone.markSeen(req.SessionID)
		if needsCleanup {
			if err := cleanOrphanedToolCalls(ctx, msgs, req.SessionID); err != nil {
				log.Printf("compaction: startup orphan cleanup failed: %v", err)
			}
		}

		utility := resolveUtility(registry, appCfg, req, utilityHealth)
		var sc *SessionCost
		if getCost != nil {
			sc = getCost(ctx, req.SessionID)
		}
		if err := maybeCompact(ctx, sessionCfg, msgs, isReadOnly, utility, publish, req, lastInputTokens, sc, utilityHealth); err != nil {
			log.Printf("compaction: pre-turn compaction failed: %v", err)
		}
		if err := next(ctx, req); err != nil {
			return err
		}
		contextSize := req.Model.EffectiveContextSize()
		if contextSize <= 0 {
			contextSize = 128000
		}
		postCC := resolveCompactionConfig(sessionCfg, req.ProviderID, req.Model.ID, contextSize)
		if postCC.threshold > 0 && len(req.Messages) > postCC.keepTurns*2+4 {
			if err := pruneToolOutputs(ctx, sessionCfg, msgs, isReadOnly, req); err != nil {
				log.Printf("compaction: tool output pruning failed: %v", err)
			}
		}
		if req.StreamInterrupted {
			if err := cleanOrphanedToolCalls(ctx, msgs, req.SessionID); err != nil {
				log.Printf("compaction: orphan cleanup failed: %v", err)
			}
		}
		return nil
	}
}

func maybeCompact(
	ctx context.Context,
	cfg *config.SessionConfig,
	msgs repository.MessageRepo,
	isReadOnly func(string) bool,
	utility utilityProvider,
	publish func(sessionID string, ev SSEEvent),
	req *pipeline.TurnRequest,
	lastInputTokens int,
	sc *SessionCost,
	utilityHealth *utilityHealthTracker,
) error {
	contextSize := req.Model.EffectiveContextSize()
	if contextSize <= 0 {
		contextSize = 128000
	}
	cc := resolveCompactionConfig(cfg, req.ProviderID, req.Model.ID, contextSize)
	if cc.threshold <= 0 {
		return nil
	}

	// Estimate the full current request: all messages + system prompt.
	// char/4 underestimates (misses tool schemas, framing, tokenizer
	// differences), so take the higher of our estimate and the provider-
	// reported count from the last API call. This works correctly for
	// both pre-turn calls (where lastInputTokens is stale) and mid-turn
	// calls (where lastInputTokens includes appended messages).
	msgEstimate := estimateProviderMessages(req.Messages)
	for _, sp := range req.SystemParts {
		msgEstimate += (len(sp) + 3) / 4
	}
	totalTokens := max(msgEstimate, lastInputTokens)

	limit := float64(contextSize) * cc.threshold
	log.Printf("compaction: model=%s/%s contextSize=%d threshold=%.3f totalTokens=%d limit=%.0f needsCompaction=%v",
		req.ProviderID, req.Model.ID, contextSize, cc.threshold, totalTokens, limit, float64(totalTokens) >= limit)
	if float64(totalTokens) < limit {
		return nil
	}

	// Preserve workflow/approval state from the full pre-compaction transcript.
	// The rebuilt message slice may intentionally drop older workflow markers.
	ensureWorkflowState(req)

	if publish != nil {
		publish(req.SessionID, SSEEvent{Type: "compaction_start"})
	}

	// Load the session snapshot once (2 queries). All subsequent operations
	// (pruning, recounting, summary generation, cutoff computation) reuse it.
	allMsgs, err := msgs.ListBySession(ctx, req.SessionID)
	if err != nil {
		return err
	}
	allParts, err := msgs.ListPartsBySession(ctx, req.SessionID)
	if err != nil {
		return err
	}
	// Populate Parts on messages for buildProtectedMessageSet → isUserTurnStart.
	{
		byMsg := groupPartsByMessage(allParts)
		for i := range allMsgs {
			allMsgs[i].Parts = byMsg[allMsgs[i].ID]
		}
	}

	prunedParts, pruneErr := pruneToolOutputsFromSnapshot(ctx, cfg, msgs, isReadOnly, req, allMsgs, allParts)
	if pruneErr != nil {
		log.Printf("compaction: tool output pruning failed: %v", pruneErr)
	}

	// Apply prune results to the in-memory snapshot so we can rebuild
	// req.Messages without a DB reload.
	if len(prunedParts) > 0 {
		for i := range allParts {
			if updated, ok := prunedParts[allParts[i].ID]; ok {
				allParts[i] = updated
			}
		}
	}
	partsByMsg := groupPartsByMessage(allParts)
	req.Messages, req.SummaryText = buildTurnMessages(allMsgs, partsByMsg)

	// Recount after pruning to see if summarization can be skipped.
	// Re-estimate from req.Messages (which reflects pruned state) plus
	// system prompt overhead, with a 1.3x safety margin when provider-
	// reported tokens triggered the compaction.
	{
		estimated := estimateProviderMessages(req.Messages)
		for _, sp := range req.SystemParts {
			estimated += (len(sp) + 3) / 4
		}
		safeEstimate := float64(estimated)
		if lastInputTokens > 0 {
			safeEstimate *= 1.3
		}
		if safeEstimate < limit {
			log.Printf("compaction: pruning sufficient (estimated=%d, safe=%.0f, limit=%.0f), skipping summarization", estimated, safeEstimate, limit)
			if publish != nil {
				publish(req.SessionID, SSEEvent{Type: "compaction"})
			}
			return nil
		}
	}

	var (
		summaryText  string
		summaryUsage *provider.Usage
		summaryErr   error
		usedUtility  utilityProvider
	)
	for _, candidate := range utilityCandidates(utility) {
		log.Printf("compaction: generating summary with model=%s via provider=%s (contextSize=%d)", candidate.modelID, candidate.prov.ID(), candidate.contextSize)
		summaryText, summaryUsage, summaryErr = generateSummary(ctx, candidate.prov, candidate.modelID, candidate.contextSize, req)
		if summaryErr != nil {
			log.Printf("compaction: summary generation failed via %s/%s: %v", candidate.prov.ID(), candidate.modelID, summaryErr)
			if isUtilityPermanentUnavailable(summaryErr) {
				utilityHealth.markUnavailable(candidate)
			}
			continue
		}
		if summaryText == "" {
			log.Printf("compaction: summary generation returned empty text via %s/%s", candidate.prov.ID(), candidate.modelID)
			continue
		}
		// A good summary should be at least 200 chars. Shorter output means the
		// utility model didn't follow the structured format. Treat as failure.
		const minSummaryLen = 200
		if len(summaryText) < minSummaryLen {
			log.Printf("compaction: summary too short (%d chars < %d min) via %s/%s, trying next candidate", len(summaryText), minSummaryLen, candidate.prov.ID(), candidate.modelID)
			summaryText = ""
			continue
		}
		utilityHealth.markAvailable(candidate)
		usedUtility = candidate
		log.Printf("compaction: summary generated (%d chars) via %s/%s", len(summaryText), candidate.prov.ID(), candidate.modelID)
		if sc != nil && summaryUsage != nil {
			sc.Add(summaryUsage, provider.Model{CostInput: candidate.costIn, CostOutput: candidate.costOut})
		}
		break
	}
	// Both the success and fallback paths need durable truncation:
	// truncate in-memory, compute the DB cutoff, and persist a summary
	// message so buildTurnMessages filters correctly on future reloads.
	// Compute cutoff from pre-sanitization count so orphan removal by
	// sanitizeToolPairs doesn't shift the DB exclusion boundary.
	truncated := safeTruncateMessages(req.Messages, cc.keepTurns)
	cutoffID := compactionCutoff(allMsgs, len(truncated))
	req.Messages = sanitizeToolPairs(truncated)

	if summaryText == "" {
		// Summary generation failed. Persist a cutoff-only summary so the
		// truncation is durable across reloads (prevents emergency retry
		// from rebuilding the same bloated message set).
		fallbackText := req.SummaryText // preserve previous summary if any
		if storeErr := storeSummary(ctx, msgs, req, cutoffID, fallbackText); storeErr != nil {
			log.Printf("compaction: failed to persist fallback cutoff: %v", storeErr)
		}
		ensureSummaryInSystemParts(req)
		if publish != nil {
			publish(req.SessionID, SSEEvent{Type: "compaction", Data: map[string]string{"summary": "(static truncation — summary generation failed)"}})
		}
		return nil
	}

	if storeErr := storeSummary(ctx, msgs, req, cutoffID, summaryText); storeErr != nil {
		return storeErr
	}

	if publish != nil {
		publish(req.SessionID, SSEEvent{
			Type: "compaction",
			Data: SSECompactionData{Summary: summaryText, ModelID: usedUtility.modelID},
		})
	}

	injectSummary(req, summaryText)
	return nil
}

// compactionCutoff returns the ID of the last non-summary DB message that
// should be excluded when keptCount messages are preserved. Returns "" if
// nothing should be excluded.
func compactionCutoff(allMsgs []repository.Message, keptCount int) string {
	nonSummaryCount := 0
	for _, m := range allMsgs {
		if !m.Summary {
			nonSummaryCount++
		}
	}
	excludedCount := nonSummaryCount - keptCount
	if excludedCount <= 0 {
		return ""
	}
	idx := 0
	for _, m := range allMsgs {
		if m.Summary {
			continue
		}
		idx++
		if idx == excludedCount {
			return m.ID
		}
	}
	return ""
}

// storeSummary persists a compaction summary message with its cutoff and text.
func storeSummary(ctx context.Context, msgs repository.MessageRepo, req *pipeline.TurnRequest, cutoffID, summaryText string) error {
	var parts []repository.MessagePart
	if summaryText != "" {
		content, err := message.MarshalContent(message.TextContent{Text: summaryText})
		if err != nil {
			return fmt.Errorf("marshal summary content: %w", err)
		}
		parts = append(parts, repository.MessagePart{
			SessionID: req.SessionID,
			Type:      "text",
			Content:   content,
			Synthetic: true,
		})
	}
	_, err := msgs.CreateWithParts(ctx, repository.Message{
		SessionID:          req.SessionID,
		Role:               "assistant",
		Summary:            true,
		CompactionParentID: cutoffID,
	}, parts)
	if err != nil {
		return fmt.Errorf("store summary message: %w", err)
	}
	return nil
}
