package service

import (
	"context"
	"encoding/json"
	"log"

	"github.com/sageil/kodacode/v1/internal/message"
	"github.com/sageil/kodacode/v1/internal/provider"
	"github.com/sageil/kodacode/v1/internal/repository"
)

// safeTruncateMessages keeps at most keepTurns recent user turns from msgs,
// ensuring the result forms a valid conversation:
//   - starts with a "user" message (not assistant)
//   - never has tool_results without their matching tool_calls
//
// A "turn" starts at a user message containing text or file content and
// includes all subsequent messages until the next such user message.
// Tool-heavy turns (with many tool_call/tool_result pairs) are preserved
// as complete units.
//
// When there are fewer turn boundaries than keepTurns (e.g., a single turn
// with many tool_call/result pairs), it falls back to keeping the first
// user prompt plus the last keepTurns*2 messages to avoid returning the
// entire unbounded conversation.
func safeTruncateMessages(msgs []provider.Message, keepTurns int) []provider.Message {
	if keepTurns <= 0 {
		return msgs
	}

	// Find all user turn boundaries (text or file parts).
	var boundaries []int
	for i, m := range msgs {
		if m.Role != "user" {
			continue
		}
		for _, p := range m.Parts {
			switch p.(type) {
			case provider.TextPart, provider.FilePart:
				boundaries = append(boundaries, i)
				goto nextMsg
			}
		}
	nextMsg:
	}

	if len(boundaries) > keepTurns {
		// Normal multi-turn case: cut at the Nth boundary from the end.
		cutIdx := boundaries[len(boundaries)-keepTurns]
		if cutIdx > 0 {
			return msgs[cutIdx:]
		}
		return msgs
	}

	// If there are fewer turns than keepTurns, all existing turns fit within budget.
	// For multi-turn sessions (≥2 boundaries), keep everything since we
	// want all the turns we have.
	if len(boundaries) > 1 {
		return msgs
	}

	// Single-turn fallback: one user prompt followed by many
	// tool_call/result pairs. Use message-count truncation to avoid
	// returning an unbounded conversation.
	keep := keepTurns * 2
	if len(msgs) <= keep {
		return msgs
	}

	// Keep the first user prompt + the last `keep` messages, starting
	// from an assistant message so tool pairs stay complete.
	start := len(msgs) - keep
	for start < len(msgs) && msgs[start].Role != "assistant" {
		start++
	}
	if start >= len(msgs) {
		return msgs
	}
	var result []provider.Message
	if len(boundaries) > 0 && boundaries[0] < start {
		result = append(result, msgs[boundaries[0]])
	}
	result = append(result, msgs[start:]...)
	return result
}

// sanitizeToolPairs ensures every assistant message with ToolCallParts has
// corresponding ToolResultPart messages following it. Orphaned tool_calls
// (e.g. from truncation or stream recovery) are stripped so the message
// history is valid for strict OpenAI-compatible providers.
func sanitizeToolPairs(msgs []provider.Message) []provider.Message {
	// Log tool_call/tool_result counts for debugging.
	var tcCount, trCount int
	for _, m := range msgs {
		for _, p := range m.Parts {
			switch p.(type) {
			case provider.ToolCallPart:
				tcCount++
			case provider.ToolResultPart:
				trCount++
			}
		}
	}
	if tcCount != trCount {
		log.Printf("sanitizeToolPairs: MISMATCH tool_calls=%d tool_results=%d (msgs=%d)", tcCount, trCount, len(msgs))
	}

	resultIDs := make(map[string]bool)
	invalidCallIDs := make(map[string]bool)
	for _, m := range msgs {
		for _, p := range m.Parts {
			if tr, ok := p.(provider.ToolResultPart); ok {
				resultIDs[tr.ToolCallID] = true
			}
			if tc, ok := p.(provider.ToolCallPart); ok {
				if !validToolCallArguments(tc.Arguments) {
					invalidCallIDs[tc.ID] = true
				}
			}
		}
	}
	if len(invalidCallIDs) > 0 {
		log.Printf("sanitizeToolPairs: stripping %d malformed tool_calls", len(invalidCallIDs))
	}

	out := make([]provider.Message, 0, len(msgs))
	for _, m := range msgs {
		if m.Role != "assistant" {
			out = append(out, m)
			continue
		}
		var cleaned []provider.MessagePart
		hasOrphans := false
		for _, p := range m.Parts {
			if tc, ok := p.(provider.ToolCallPart); ok {
				if invalidCallIDs[tc.ID] || !resultIDs[tc.ID] {
					hasOrphans = true
					continue // drop orphaned tool_call
				}
			}
			cleaned = append(cleaned, p)
		}
		if !hasOrphans {
			out = append(out, m)
			continue
		}
		// If all parts were tool_calls and all were orphaned, drop the message.
		if len(cleaned) == 0 {
			continue
		}
		out = append(out, provider.Message{Role: m.Role, Parts: cleaned})
	}

	// Also drop tool_result messages whose tool_call was removed.
	keepCallIDs := make(map[string]bool)
	for _, m := range out {
		for _, p := range m.Parts {
			if tc, ok := p.(provider.ToolCallPart); ok {
				keepCallIDs[tc.ID] = true
			}
		}
	}
	final := make([]provider.Message, 0, len(out))
	for _, m := range out {
		var cleaned []provider.MessagePart
		for _, p := range m.Parts {
			if tr, ok := p.(provider.ToolResultPart); ok {
				if !keepCallIDs[tr.ToolCallID] {
					continue // drop orphaned tool_result
				}
			}
			cleaned = append(cleaned, p)
		}
		if len(cleaned) == 0 {
			continue
		}
		final = append(final, provider.Message{Role: m.Role, Parts: cleaned})
	}

	// Final pass: verify every assistant message with tool_calls has
	// matching tool_results in the IMMEDIATELY FOLLOWING message (required
	// by Anthropic and some other providers). If a user message or any
	// non-tool-result message appears between tool_calls and tool_results,
	// the tool_calls are stripped.
	for i := len(final) - 1; i >= 0; i-- {
		m := final[i]
		if m.Role != "assistant" {
			continue
		}
		var tcIDs []string
		for _, p := range m.Parts {
			if tc, ok := p.(provider.ToolCallPart); ok {
				tcIDs = append(tcIDs, tc.ID)
			}
		}
		if len(tcIDs) == 0 {
			continue
		}
		// Tool results must be in the immediately following message.
		answered := make(map[string]bool)
		if i+1 < len(final) {
			next := final[i+1]
			for _, p := range next.Parts {
				if tr, ok := p.(provider.ToolResultPart); ok {
					answered[tr.ToolCallID] = true
				}
			}
		}
		allAnswered := true
		for _, id := range tcIDs {
			if !answered[id] {
				allAnswered = false
				break
			}
		}
		if allAnswered {
			continue
		}
		// Strip unanswered tool_calls from this assistant message.
		log.Printf("sanitizeToolPairs: stripping %d unanswered tool_calls from assistant message %d", len(tcIDs)-len(answered), i)
		var stripped []provider.MessagePart
		for _, p := range m.Parts {
			if tc, ok := p.(provider.ToolCallPart); ok {
				if !answered[tc.ID] {
					continue
				}
			}
			stripped = append(stripped, p)
		}
		if len(stripped) == 0 {
			// Drop the assistant message and its orphaned tool_results.
			// Also drop the next message if it only contained tool_results
			// for the removed tool_calls.
			final = append(final[:i], final[i+1:]...)
		} else {
			final[i] = provider.Message{Role: m.Role, Parts: stripped}
		}
	}

	// One more pass: drop any tool_result parts whose tool_call was removed.
	keepIDs := make(map[string]bool)
	for _, m := range final {
		for _, p := range m.Parts {
			if tc, ok := p.(provider.ToolCallPart); ok {
				keepIDs[tc.ID] = true
			}
		}
	}
	cleaned2 := make([]provider.Message, 0, len(final))
	for _, m := range final {
		var parts []provider.MessagePart
		for _, p := range m.Parts {
			if tr, ok := p.(provider.ToolResultPart); ok {
				if !keepIDs[tr.ToolCallID] {
					continue
				}
			}
			parts = append(parts, p)
		}
		if len(parts) > 0 {
			cleaned2 = append(cleaned2, provider.Message{Role: m.Role, Parts: parts})
		}
	}

	return cleaned2
}

func validToolCallArguments(args string) bool {
	if args == "" {
		return false
	}
	var obj map[string]json.RawMessage
	return json.Unmarshal([]byte(args), &obj) == nil
}

// truncateMessagesToFit keeps the first and last turns of the conversation,
// dropping middle messages until the estimated token count fits within
// maxTokens. Uses prefix-sum token accounting for O(N) performance.
func truncateMessagesToFit(msgs []provider.Message, maxTokens int) []provider.Message {
	tokens := estimateProviderMessages(msgs)
	if tokens <= maxTokens {
		return msgs
	}

	// Find turn boundaries (user messages with text or file parts).
	var turnStarts []int
	for i, m := range msgs {
		if m.Role != "user" {
			continue
		}
		for _, p := range m.Parts {
			switch p.(type) {
			case provider.TextPart, provider.FilePart:
				turnStarts = append(turnStarts, i)
				goto nextMsg
			}
		}
	nextMsg:
	}
	if len(turnStarts) <= 2 {
		return msgs
	}

	// Build per-turn token costs. Turn i spans turnStarts[i]..turnStarts[i+1]-1
	// (or to end of msgs for the last turn).
	nTurns := len(turnStarts)
	turnTokens := make([]int, nTurns)
	for i := 0; i < nTurns; i++ {
		start := turnStarts[i]
		end := len(msgs)
		if i+1 < nTurns {
			end = turnStarts[i+1]
		}
		turnTokens[i] = estimateProviderMessages(msgs[start:end])
	}

	// Prefix sum over turns for O(1) range queries.
	prefix := make([]int, nTurns+1)
	for i, t := range turnTokens {
		prefix[i+1] = prefix[i] + t
	}
	totalTurnTokens := prefix[nTurns]

	// Greedy two-pass: first find the largest tail that fits with at least
	// 1 head turn (preserves the user's original goal for summary quality).
	// If no head+tail combo fits, fall back to tail-only.
	bestHead, bestTail := 0, 0
	bestFit := false
	// Pass 1: largest tail that leaves room for ≥1 head turn.
	for tail := 1; tail < nTurns-1; tail++ {
		tailCost := totalTurnTokens - prefix[tail]
		headCost := prefix[1] // cost of first turn
		if tailCost+headCost > maxTokens {
			continue
		}
		remaining := maxTokens - tailCost
		head := 0
		for head < tail && prefix[head+1] <= remaining {
			head++
		}
		if head > 0 {
			bestHead, bestTail = head, nTurns-tail
			bestFit = true
			break
		}
	}
	// Pass 2: if no head+tail fit, try tail-only (no head preservation).
	if !bestFit {
		for tail := 1; tail < nTurns; tail++ {
			tailCost := totalTurnTokens - prefix[tail]
			if tailCost <= maxTokens {
				bestHead, bestTail = 0, nTurns-tail
				bestFit = true
				break
			}
		}
	}

	if !bestFit {
		// Even one tail turn exceeds the budget, so fall back to tail-only truncation.
		result := msgs[turnStarts[nTurns-1]:]
		for len(result) > 1 && estimateProviderMessages(result) > maxTokens {
			result = result[1:]
		}
		log.Printf("compaction: truncated messages for utility model: %d→%d msgs, %d→%d tokens (limit=%d, tail-only fallback)",
			len(msgs), len(result), tokens, estimateProviderMessages(result), maxTokens)
		return result
	}

	var headEnd int
	if bestHead > 0 {
		headEnd = turnStarts[bestHead]
	}
	tailStart := turnStarts[nTurns-bestTail]

	var result []provider.Message
	if headEnd > 0 {
		result = append(result, msgs[:headEnd]...)
	}
	result = append(result, msgs[tailStart:]...)
	result = sanitizeToolPairs(result)

	est := estimateProviderMessages(result)
	log.Printf("compaction: truncated messages for utility model: %d→%d msgs, %d→%d tokens (limit=%d, head=%d tail=%d turns)",
		len(msgs), len(result), tokens, est, maxTokens, bestHead, bestTail)
	return result
}

// estimateProviderMessages returns a rough char/4 token estimate for a slice
// of provider.Message. Delegates to provider.EstimateMessageTokens to keep
// the estimation logic in one place.
func estimateProviderMessages(msgs []provider.Message) int {
	total := 0
	for _, m := range msgs {
		total += provider.EstimateMessageTokens(m)
	}
	return total
}

// stripFileParts removes FilePart entries from messages. Images and PDFs are
// not needed for conversation summarization and can massively inflate token
// counts beyond the utility model's context limit.
func stripFileParts(msgs []provider.Message) []provider.Message {
	out := make([]provider.Message, 0, len(msgs))
	for _, m := range msgs {
		var filtered []provider.MessagePart
		hasFile := false
		for _, p := range m.Parts {
			if _, ok := p.(provider.FilePart); ok {
				hasFile = true
				// Replace with a text note so the summary model knows a file was attached.
				filtered = append(filtered, provider.TextPart{Text: "[file attachment omitted]"})
				continue
			}
			filtered = append(filtered, p)
		}
		if hasFile {
			m = provider.Message{Role: m.Role, Parts: filtered}
		}
		out = append(out, m)
	}
	return out
}

// extractToolCallID extracts the tool_call_id from a tool_result JSON content string.
func extractToolCallID(content string) string {
	c, err := message.UnmarshalContent("tool_result", content)
	if err != nil {
		return ""
	}
	if tr, ok := c.(message.ToolResultContent); ok {
		return tr.ToolCallID
	}
	return ""
}

func cleanOrphanedToolCalls(ctx context.Context, msgs repository.MessageRepo, sessionID string) error {
	parts, err := msgs.ListPartsBySession(ctx, sessionID)
	if err != nil {
		return err
	}

	resultIDs := make(map[string]bool)
	for _, p := range parts {
		if p.Type == "tool_result" {
			if id := extractToolCallID(p.Content); id != "" {
				resultIDs[id] = true
			}
		}
	}

	var orphans []repository.MessagePart
	for _, p := range parts {
		if p.Type != "tool_call" {
			continue
		}
		c, err := message.UnmarshalContent("tool_call", p.Content)
		if err != nil {
			continue
		}
		tc, ok := c.(message.ToolCallContent)
		if !ok {
			continue
		}
		if !resultIDs[tc.ID] {
			orphans = append(orphans, p)
		}
	}

	if len(orphans) == 0 {
		return nil
	}

	log.Printf("compaction: removing %d orphaned tool_call parts from repo", len(orphans))
	for _, p := range orphans {
		if err := msgs.DeletePart(ctx, p.ID); err != nil {
			log.Printf("compaction: failed to delete orphaned part %s: %v", p.ID, err)
		}
	}
	return nil
}

func rejectedToolCallIDs(executions []toolExecution, tools []provider.Tool) map[string]bool {
	toolSet := make(map[string]bool, len(tools))
	for _, t := range tools {
		toolSet[t.Name] = true
	}
	var rejected map[string]bool
	for _, ex := range executions {
		if !toolSet[ex.call.Name] {
			if rejected == nil {
				rejected = make(map[string]bool)
			}
			rejected[ex.call.ID] = true
		}
	}
	return rejected
}

func historyRejectedToolCallIDs(executions []toolExecution, tools []provider.Tool, fullTools []provider.Tool) map[string]bool {
	toolSet := make(map[string]bool, len(tools))
	for _, t := range tools {
		toolSet[t.Name] = true
	}
	historySet := toolSet
	if len(fullTools) > 0 {
		historySet = make(map[string]bool, len(fullTools))
		for _, t := range fullTools {
			historySet[t.Name] = true
		}
	}

	var rejected map[string]bool
	for _, ex := range executions {
		if toolSet[ex.call.Name] {
			continue
		}
		if historySet[ex.call.Name] {
			continue
		}
		if rejected == nil {
			rejected = make(map[string]bool)
		}
		rejected[ex.call.ID] = true
	}
	return rejected
}

func stripUnknownToolCalls(msgs []provider.Message, tools []provider.Tool) ([]provider.Message, int) {
	known := make(map[string]bool, len(tools))
	for _, t := range tools {
		known[t.Name] = true
	}

	removeIDs := make(map[string]bool)
	for _, m := range msgs {
		if m.Role != "assistant" {
			continue
		}
		for _, p := range m.Parts {
			if tc, ok := p.(provider.ToolCallPart); ok {
				if !known[tc.Name] {
					removeIDs[tc.ID] = true
				}
			}
		}
	}
	if len(removeIDs) == 0 {
		return msgs, 0
	}

	out := make([]provider.Message, 0, len(msgs))
	for _, m := range msgs {
		var cleaned []provider.MessagePart
		for _, p := range m.Parts {
			switch v := p.(type) {
			case provider.ToolCallPart:
				if removeIDs[v.ID] {
					continue
				}
			case provider.ToolResultPart:
				if removeIDs[v.ToolCallID] {
					continue
				}
			}
			cleaned = append(cleaned, p)
		}
		if len(cleaned) == 0 {
			continue
		}
		out = append(out, provider.Message{Role: m.Role, Parts: cleaned})
	}
	return out, len(removeIDs)
}
