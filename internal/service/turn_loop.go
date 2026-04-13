package service

import (
	"context"
	"fmt"
	"log"
	"slices"
	"time"

	"github.com/sageil/kodacode/v1/internal/config"
	"github.com/sageil/kodacode/v1/internal/permission"
	"github.com/sageil/kodacode/v1/internal/pipeline"
	"github.com/sageil/kodacode/v1/internal/provider"
	"github.com/sageil/kodacode/v1/internal/repository"
	"github.com/sageil/kodacode/v1/internal/sandbox"
	"github.com/sageil/kodacode/v1/internal/snapshot"
	"github.com/sageil/kodacode/v1/internal/tool"
)

type turnLoop struct {
	ctx              context.Context
	req              *pipeline.TurnRequest
	prov             provider.Provider
	modelID          string
	publish          func(sessionID string, ev SSEEvent)
	sndbx            *sandbox.Sandbox
	msgs             repository.MessageRepo
	askPerm          AskPermissionFunc
	askUser          AskUserFunc
	spawnSubagent    SpawnSubagentFunc
	globalCfg        *config.Config
	cfg              *config.SessionConfig
	utility          utilityProvider
	utilityHealth    *utilityHealthTracker
	snapshotSvc      *snapshot.Service
	sc               *SessionCost
	budgetStatus     func(context.Context, string, *config.SessionConfig) BudgetStatus
	toolCache        *toolResultCache
	lspDiagnoser     tool.LSPDiagnoser
	taskStore        *tool.TaskStore
	sessionTraces    *SessionTraces
	turnTrace        []StepTrace
	primaryModelID   string
	requestCostModel provider.Model
	workflowIntent   *workflowIntentResult
	intentChecked    bool

	// reviewFindings stores the reviewer findings from the most recent review
	// for the current task. When the engineer workflow calls the reviewer
	// again, these are automatically appended to the reviewer prompt so it
	// performs a targeted re-review instead of a full re-check.
	reviewFindings               string
	totalBudgetWarnPublished     bool
	totalBudgetExceededPublished bool
}

const (
	defaultPrimaryMaxToolLoopSteps  = 250
	defaultSubagentMaxToolLoopSteps = 30
)

func (tl *turnLoop) init() {
	if !slices.Contains(tl.req.Agent.Tools, "subagent") {
		tl.spawnSubagent = nil
	}
	if len(tl.req.Agent.Permission) > 0 && tl.sndbx != nil {
		tl.sndbx = tl.sndbx.WithPermission(permission.Merge(tl.sndbx.Permission(), tl.req.Agent.Permission))
	}
}

func (tl *turnLoop) run() error {
	req := tl.req
	cfg := tl.cfg
	publish := tl.publish
	modelID := tl.modelID
	sndbx := tl.sndbx

	if _, err := tl.maybeBootstrapWorkflowController(); err != nil {
		return err
	}

	maxRetries := defaultMaxRetries
	if cfg != nil && cfg.MaxRetries > 0 {
		maxRetries = cfg.MaxRetries
	}

	maxToolLoopSteps := 0
	if req.Ephemeral {
		maxToolLoopSteps = defaultSubagentMaxToolLoopSteps
		if cfg != nil && cfg.SubagentMaxSteps > 0 {
			maxToolLoopSteps = cfg.SubagentMaxSteps
		}
	} else {
		maxToolLoopSteps = defaultPrimaryMaxToolLoopSteps
		if cfg != nil && cfg.PrimaryMaxSteps > 0 {
			maxToolLoopSteps = cfg.PrimaryMaxSteps
		}
	}

	filesModified := false
	budgetExceeded := false
	loopToolsDisabled := false
	loopDet := newLoopDetector()
	lastDiagOutput := ""
	diagRepeatCount := 0
	verifyNudgeSent := false
	prebuildRetrySent := false
	plannerTaskRetrySent := false
	taskExecutionRetryCount := 0
	lastTaskExecutionState := ""
	stallContinueState := ""
	stallContinueAwaitingProgress := false
	fileMutationRetrySent := false
	reviewedTasks := make(map[string]bool)
	reviewSinceIdx := 0 // message index from which to count reviewer calls for current task
	reviewTaskID := ""
	hasRunVerification := false

	// Pre-mark tasks that were already completed before this turn so the
	// review nudge only fires for tasks completed during this turn.
	if tl.taskStore != nil {
		for _, t := range tl.taskStore.GetTasks(tl.req.SessionID) {
			if t.Status == "completed" {
				reviewedTasks[t.ID] = true
			}
		}
	}

	for {
		req.Step++

		toolsSuppressedThisIteration := false
		if maxToolLoopSteps > 0 && req.Step > maxToolLoopSteps {
			log.Printf("llm: tool loop hit max steps (%d), forcing response", maxToolLoopSteps)
			setWorkflowRuntimeDirective(req, fmt.Sprintf("You have executed %d tool calls. Stop calling tools and provide your response now.", maxToolLoopSteps))
			toolsSuppressedThisIteration = true
		}

		activeProv, activeModel := tl.prov, modelID

		tools := req.Tools
		if tools != nil && (req.Ephemeral || !providerSupportsCaching(tl.prov.ID())) {
			tools = compactToolSchemas(tools)
		}
		if loopToolsDisabled {
			tools = nil
			toolsSuppressedThisIteration = true
		}
		if (maxToolLoopSteps > 0 && req.Step > maxToolLoopSteps) || budgetExceeded {
			tools = nil
			toolsSuppressedThisIteration = true
		}
		if req.Step > 1 && req.Usage != nil {
			contextSize := req.Model.EffectiveContextSize()
			if contextSize <= 0 {
				contextSize = 128000
			}

			actualTokens := req.Usage.InputTokens + req.Usage.CacheReadTokens + req.Usage.CacheWriteTokens
			newInputTokens := req.Usage.InputTokens + req.Usage.CacheWriteTokens
			cc := resolveCompactionConfig(cfg, req.ProviderID, req.Model.ID, contextSize)
			cacheInfo := ""
			if req.Usage.CacheReadTokens > 0 || req.Usage.CacheWriteTokens > 0 {
				cacheInfo = fmt.Sprintf(" cache_read=%d cache_write=%d", req.Usage.CacheReadTokens, req.Usage.CacheWriteTokens)
			}
			log.Printf("llm: step=%d tokens=%d (new=%d) contextLimit=%.0f (%.1f%% of %d)%s",
				req.Step, actualTokens, newInputTokens, float64(contextSize)*cc.contextLimit,
				float64(actualTokens)/float64(contextSize)*100, contextSize, cacheInfo)

			usage := float64(actualTokens) / float64(contextSize)
			req.ContextUsage = usage
			if usage > 0.6 && tools != nil {
				tools = compactToolSchemas(tools)
			}
			if float64(actualTokens) > float64(contextSize)*cc.contextLimit {
				log.Printf("compaction: context limit reached (%d tokens, %.0f%% of %d), stopping tool loop",
					actualTokens, float64(actualTokens)/float64(contextSize)*100, contextSize)
				tools = nil
				toolsSuppressedThisIteration = true
				setWorkflowRuntimeDirective(req, "The context window is nearly full. Please provide your response now based on the information you have already gathered. Do not attempt to call any more tools.")
			}
		}

		systemTokensBeforeIteration := estimateSystemPartTokens(req.SystemParts)

		var segBytes *SegmentBytes
		if tl.sessionTraces != nil {
			sb := captureSegmentBytes(req.SystemParts, req.Messages, tools)
			segBytes = &sb
		}

		stepStart := time.Now()
		sr, err := tl.streamWithRetry(streamParams{
			activeProv:  activeProv,
			activeModel: activeModel,
			tools:       tools,
			maxRetries:  maxRetries,
		})
		if err != nil {
			return err
		}
		setWorkflowRuntimeDirective(req, "")
		// Capture message count after streaming (which may rebuild
		// req.Messages via emergency compaction) so the mid-turn
		// estimate slices only messages appended from this point on.
		msgCountBeforeIteration := len(req.Messages)

		if tl.publishUsageUpdate(activeModel) {
			budgetExceeded = true
		}

		if sr.finishReason == "tool_calls" && len(sr.toolCalls) > 0 && tools != nil {
			if filtered, blocked := tl.filterWorkflowCompletionCalls(sr.toolCalls); blocked {
				sr.toolCalls = filtered
				setWorkflowRuntimeDirective(req, "[SYSTEM: Complete at most one task per response. Wait for the runtime to process that completion before completing the next task.]")
			}
			if filtered, blocked := tl.filterPhaseBlockedCalls(sr.toolCalls); blocked {
				sr.toolCalls = filtered
			}
			log.Printf("llm: step=%d dispatching %d tool calls in parallel", req.Step, len(sr.toolCalls))
			for _, tc := range sr.toolCalls {
				log.Printf("llm:   tool=%s id=%s", tc.Name, tc.ID)
				if tc.Name == "subagent" {
					log.Printf("workflow: model delegating to subagent, args=%s", truncateForLog(tc.Arguments, 300))
				}
			}
			if sr.text != "" {
				log.Printf("workflow: model text before tool calls: %s", truncateForLog(sr.text, 300))
			}
			executions := tl.dispatchToolCalls(sr.toolCalls)
			historyRejectedCallIDs := historyRejectedToolCallIDs(executions, req.Tools, req.FullTools)
			if len(historyRejectedCallIDs) > 0 {
				log.Printf("llm: stripped %d rejected tool calls from history at step %d", len(historyRejectedCallIDs), req.Step)
			}

			var allowedCalls []provider.ToolCall
			for _, tc := range sr.toolCalls {
				if !historyRejectedCallIDs[tc.ID] {
					allowedCalls = append(allowedCalls, tc)
				}
			}

			assistantText := sr.text
			if tl.hasSideEffectingToolFailure(executions) {
				assistantText = ""
			}
			tl.persistAssistantMessage(assistantText, sr.reasoning, allowedCalls)

			assistantParts := make([]provider.MessagePart, 0)
			if assistantText != "" {
				assistantParts = append(assistantParts, provider.TextPart{Text: assistantText})
			}
			for _, tc := range allowedCalls {
				assistantParts = append(assistantParts, provider.ToolCallPart(tc))
			}
			if len(assistantParts) > 0 {
				req.Messages = append(req.Messages, provider.Message{
					Role: "assistant", Parts: assistantParts,
				})
			}
			noteWorkflowToolCalls(req, allowedCalls)

			loopVerdict := loopDet.recordBatch(executions)
			if loopVerdict >= loopWarn {
				log.Printf("loop: detected at step %d (verdict=%d)", req.Step, loopVerdict)
				publish(req.SessionID, SSEEvent{
					Type: "loop_detected",
					Data: map[string]any{"verdict": int(loopVerdict), "step": req.Step},
				})
			}

			tl.recordToolStepTrace(activeModel, sr.retryCount, loopVerdict, executions, segBytes, stepStart)

			var modifiedFiles []string
			if sndbx != nil {
				for _, tc := range sr.toolCalls {
					if !sndbx.IsReadOnly(tc.Name) {
						filesModified = true
						if fp := extractFilePath(tc.Arguments); fp != "" {
							modifiedFiles = append(modifiedFiles, fp)
						}
					}
					if tc.Name == "test" {
						hasRunVerification = true
					}
				}
			}

			tl.persistToolResults(executions)
			toolResultParts := make([]provider.MessagePart, 0, len(executions))
			for _, ex := range executions {
				if historyRejectedCallIDs[ex.call.ID] {
					continue
				}
				if executionCountsAsVerification(ex) {
					hasRunVerification = true
				}
				if ex.call.Name == "subagent" {
					log.Printf("workflow: subagent result returned, output=%d chars", len(ex.output))
				}
				var metadata map[string]any
				if ex.result != nil {
					metadata = ex.result.Metadata
				}
				toolResultParts = append(toolResultParts, provider.ToolResultPart{
					ToolCallID: ex.call.ID,
					Output:     ex.output,
					Error:      ex.errStr,
					Metadata:   metadata,
				})
			}
			if len(toolResultParts) > 0 {
				req.Messages = append(req.Messages, provider.Message{
					Role: "user", Parts: toolResultParts,
				})
			}
			if len(executions) > 0 && stallContinueAwaitingProgress {
				stallContinueAwaitingProgress = false
				stallContinueState = ""
			}
			noteWorkflowExecutions(req, executions)

			retryPlanner, err := tl.maybeHandlePlannerApproval(executions, &plannerTaskRetrySent)
			if err != nil {
				return err
			}
			if retryPlanner {
				ApplyPhaseRules(req)
				continue
			}
			bootstrapped, err := tl.maybeBootstrapWorkflowController()
			if err != nil {
				return err
			}
			if bootstrapped {
				ApplyPhaseRules(req)
				continue
			}
			if retry := tl.failedFileMutationRetry(fileMutationRetrySent, executions); retry != "" {
				fileMutationRetrySent = true
				setWorkflowRuntimeDirective(req, retry)
				log.Printf("file-mutation: retry injected at step %d", req.Step)
				ApplyPhaseRules(req)
				continue
			}

			tl.appendDiagnostics(modifiedFiles, &lastDiagOutput, &diagRepeatCount)

			if reviewed, disableTools, err := tl.maybeHandleCompletedTaskReview(executions, reviewedTasks, &reviewSinceIdx, &reviewTaskID, loopToolsDisabled, budgetExceeded); err != nil {
				return err
			} else if reviewed {
				if disableTools {
					loopToolsDisabled = true
				}
				setReviewRuntimeDirective(req, "")
				ApplyPhaseRules(req)
				continue
			}
			setReviewRuntimeDirective(req, "")

			if loopVerdict >= loopStop {
				loopToolsDisabled = true
				setWorkflowRuntimeDirective(req, "[SYSTEM: Repeated identical tool calls detected. Tool use is disabled for the rest of this turn. Do not call more tools. Provide your best answer to the user based on the information you already gathered.]")
				log.Printf("loop: disabled tools for remainder of turn at step %d", req.Step)
			} else if loopVerdict >= loopNudge {
				nudge := "[SYSTEM: Loop detected. You are repeating the same tool calls with identical results. " +
					"Stop repeating and either present your findings to the user or try a different approach.]"
				setWorkflowRuntimeDirective(req, nudge)
				log.Printf("loop: injected nudge message at step %d", req.Step)
			}

			ApplyPhaseRules(req)

			for _, ex := range executions {
				if ex.call.Name == "subagent" {
					log.Printf("workflow: step=%d subagent result injected, model will now generate next response", req.Step)
				}
			}

			// Mid-turn compaction: estimate tokens including messages appended
			// this iteration (tool results, diagnostics, nudges). The
			// top-of-loop check uses stale req.Usage and would miss growth.
			// If over the compaction threshold, run actual compaction (prune
			// + summarize). If still over contextLimit after, disable tools.
			loopToolsDisabled = tl.applyMidTurnPressure(msgCountBeforeIteration, systemTokensBeforeIteration, budgetExceeded, loopToolsDisabled)

			continue
		}

		if sr.finishReason == "stop" && req.Step > 0 {
			log.Printf("workflow: step=%d model finished with text response (no tool calls), finishReason=%s, textLen=%d",
				req.Step, sr.finishReason, len(sr.text))
		}

		if !toolsSuppressedThisIteration && tl.shouldRetryIncompletePrebuild(prebuildRetrySent, sr.finishReason, sr.toolCalls) {
			prebuildRetrySent = true
			setWorkflowRuntimeDirective(req, "[SYSTEM: Prebuild is incomplete. Run the single best verification or build command now using test or bash. Do not stop yet.]")
			log.Printf("prebuild: retry injected at step %d", req.Step)
			continue
		}

		if !toolsSuppressedThisIteration && !budgetExceeded && tl.shouldNudgeVerification(filesModified, hasRunVerification, verifyNudgeSent) {
			verifyNudgeSent = true
			setWorkflowRuntimeDirective(req, "[SYSTEM: You modified files but have not used the test tool to verify your changes. Run the test tool before providing your final response.]")
			log.Printf("verification: nudge injected at step %d", req.Step)
			continue
		}

		if retry := tl.incompleteTaskExecutionRetry(sr.finishReason, sr.toolCalls, toolsSuppressedThisIteration || budgetExceeded); retry != "" {
			currentTaskState := tl.incompleteTaskExecutionState()
			if stallContinueAwaitingProgress && currentTaskState != stallContinueState {
				stallContinueAwaitingProgress = false
				stallContinueState = ""
			}
			switch currentTaskState {
			case "":
				taskExecutionRetryCount = 0
				lastTaskExecutionState = ""
			case lastTaskExecutionState:
				taskExecutionRetryCount++
			default:
				taskExecutionRetryCount = 1
				lastTaskExecutionState = currentTaskState
			}
			if taskExecutionRetryCount > tl.builderReviewLimit() {
				if stallContinueAwaitingProgress && currentTaskState == stallContinueState {
					disableTools, err := tl.blockStalledExecutionTask("[SYSTEM: Execution stalled again on %s (%s) after the user chose to keep working. The task has been marked blocked. Do not call more tools. Respond briefly that execution is paused and wait for the user's next message.]")
					if err != nil {
						return err
					}
					if disableTools {
						loopToolsDisabled = true
					}
					taskExecutionRetryCount = 0
					lastTaskExecutionState = tl.incompleteTaskExecutionState()
					stallContinueAwaitingProgress = false
					stallContinueState = ""
					continue
				}
				decision, disableTools, err := tl.handleExecutionStallDecision()
				if err != nil {
					return err
				}
				if disableTools {
					loopToolsDisabled = true
				}
				if decision == executionStallDecisionContinue {
					stallContinueState = tl.incompleteTaskExecutionState()
					stallContinueAwaitingProgress = true
				} else {
					stallContinueAwaitingProgress = false
					stallContinueState = ""
				}
				taskExecutionRetryCount = 0
				lastTaskExecutionState = tl.incompleteTaskExecutionState()
				continue
			}
			setWorkflowRuntimeDirective(req, retry)
			log.Printf("task: execution retry injected at step %d (attempt %d)", req.Step, taskExecutionRetryCount)
			continue
		}

		tl.persistAssistantMessage(sr.text, sr.reasoning, nil)
		if sr.text != "" {
			req.Messages = append(req.Messages, provider.Message{
				Role:  "assistant",
				Parts: []provider.MessagePart{provider.TextPart{Text: sr.text}},
			})
		}
		if err := tl.maybeHandleDirectPlannerApproval(sr.text); err != nil {
			return err
		}
		tl.recordFinalStepTrace(activeModel, sr.retryCount, segBytes, stepStart)
		tl.publishDone()
		tl.snapshotStep(filesModified)
		break
	}

	return nil
}

func (tl *turnLoop) costModelFor(activeModel string) provider.Model {
	m := tl.requestCostModel
	if m.ID == "" {
		m = tl.req.Model
	}
	if activeModel != tl.primaryModelID && tl.utility.costIn > 0 {
		m.CostInput = tl.utility.costIn
		m.CostOutput = tl.utility.costOut
	}
	return m
}
