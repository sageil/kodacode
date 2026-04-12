package service

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/sageil/kodacode/v1/internal/pipeline"
	"github.com/sageil/kodacode/v1/internal/provider"
	"github.com/sageil/kodacode/v1/internal/tool"
)

func (tl *turnLoop) publishUsageUpdate(activeModel string) bool {
	if tl.req.Usage == nil {
		return false
	}
	if tl.sc != nil {
		tl.sc.Add(tl.req.Usage, tl.costModelFor(activeModel))
	}

	displayContextSize := tl.req.Model.ContextSize
	if displayContextSize == 0 {
		displayContextSize = defaultContextSize
	}

	sessionCost := 0.0
	subagentCost := 0.0
	budgetWarn := false
	budgetExceeded := false
	var aggregate BudgetStatus
	var costSnap *CostSnapshot
	if tl.sc != nil {
		s := tl.sc.Snapshot()
		costSnap = &s
		sessionCost = s.TotalCost
		subagentCost = s.SubagentCost
	}
	if tl.budgetStatus != nil {
		aggregate = tl.budgetStatus(tl.ctx, tl.req.SessionID, tl.cfg)
		if aggregate.SessionCost > 0 || tl.sc == nil {
			sessionCost = aggregate.SessionCost
		}
		budgetWarn = aggregate.SessionWarn || aggregate.TotalWarn
		budgetExceeded = aggregate.SessionExceeded || aggregate.TotalExceeded
	} else if tl.cfg != nil && tl.cfg.Budget > 0 {
		budgetExceeded = sessionCost >= tl.cfg.Budget
		if tl.cfg.BudgetWarn > 0 {
			budgetWarn = sessionCost >= tl.cfg.Budget*tl.cfg.BudgetWarn
		}
	}

	tl.publish(tl.req.SessionID, SSEEvent{
		Type: "usage",
		Data: SSEDoneData{
			Usage:           tl.req.Usage,
			ContextSize:     displayContextSize,
			MaxInputTokens:  tl.req.Model.MaxInputTokens,
			MaxOutputTokens: tl.req.Agent.MaxTokens,
			SessionCost:     sessionCost,
			SubagentCost:    subagentCost,
			BudgetWarn:      budgetWarn,
			CostSnapshot:    costSnap,
		},
	})

	if aggregate.TotalWarn && !tl.totalBudgetWarnPublished && tl.cfg != nil && tl.cfg.TotalBudget > 0 {
		tl.totalBudgetWarnPublished = true
		tl.publish(tl.req.SessionID, SSEEvent{
			Type: "warning",
			Data: map[string]string{
				"message": fmt.Sprintf("Cross-session budget warning: $%.4f of $%.4f used", aggregate.TotalCost, tl.cfg.TotalBudget),
			},
		})
	}
	if aggregate.TotalExceeded && !tl.totalBudgetExceededPublished && tl.cfg != nil && tl.cfg.TotalBudget > 0 {
		tl.totalBudgetExceededPublished = true
		tl.publish(tl.req.SessionID, SSEEvent{
			Type: "warning",
			Data: map[string]string{
				"message": fmt.Sprintf("Cross-session budget reached: $%.4f of $%.4f used", aggregate.TotalCost, tl.cfg.TotalBudget),
			},
		})
	}

	if !budgetExceeded {
		return false
	}
	if aggregate.TotalExceeded && tl.cfg != nil && tl.cfg.TotalBudget > 0 {
		log.Printf("llm: cross-session budget exceeded for session %s (%.4f >= %.4f), stopping tool loop", tl.req.SessionID, aggregate.TotalCost, tl.cfg.TotalBudget)
		return true
	}
	if tl.cfg == nil || tl.cfg.Budget <= 0 {
		return false
	}
	log.Printf("llm: session %s budget exceeded (%.4f >= %.4f), stopping tool loop", tl.req.SessionID, sessionCost, tl.cfg.Budget)
	return true
}

func (tl *turnLoop) recordToolStepTrace(activeModel string, retryCount int, loopVerdict loopVerdict, executions []toolExecution, segBytes *SegmentBytes, stepStart time.Time) {
	if tl.sessionTraces == nil {
		return
	}
	st := StepTrace{
		Step:         tl.req.Step,
		ModelID:      activeModel,
		Usage:        tl.req.Usage,
		RetryCount:   retryCount,
		FallbackUsed: activeModel != tl.primaryModelID,
		LoopVerdict:  int(loopVerdict),
		WallClock:    time.Since(stepStart),
	}
	if tl.req.Usage != nil {
		st.CostMicroUSD = usageMicroUSD(tl.req.Usage, tl.costModelFor(activeModel))
	}
	for _, ex := range executions {
		tt := StepToolTrace{Name: ex.call.Name, Elapsed: ex.elapsed}
		if ex.errStr != nil {
			tt.Error = *ex.errStr
		}
		st.Tools = append(st.Tools, tt)
	}
	st.Segments = segBytes
	finalizeStepTrace(&st)
	tl.turnTrace = append(tl.turnTrace, st)
}

func (tl *turnLoop) recordFinalStepTrace(activeModel string, retryCount int, segBytes *SegmentBytes, stepStart time.Time) {
	if tl.sessionTraces == nil {
		return
	}
	st := StepTrace{
		Step:         tl.req.Step,
		ModelID:      activeModel,
		Usage:        tl.req.Usage,
		RetryCount:   retryCount,
		FallbackUsed: activeModel != tl.primaryModelID,
		WallClock:    time.Since(stepStart),
	}
	if tl.req.Usage != nil {
		st.CostMicroUSD = usageMicroUSD(tl.req.Usage, tl.costModelFor(activeModel))
	}
	st.Segments = segBytes
	finalizeStepTrace(&st)
	tl.turnTrace = append(tl.turnTrace, st)

	tl.sessionTraces.CommitTurn(tl.turnTrace)
	tl.publish(tl.req.SessionID, SSEEvent{
		Type: "step_trace",
		Data: SSETraceData{
			TurnIndex: tl.sessionTraces.TurnCount() - 1,
			Steps:     tl.turnTrace,
		},
	})
}

func estimateSystemPartTokens(parts []string) int {
	total := 0
	for _, sp := range parts {
		total += (len(sp) + 3) / 4
	}
	return total
}

func (tl *turnLoop) applyMidTurnPressure(msgCountBeforeIteration, systemTokensBeforeIteration int, budgetExceeded, loopToolsDisabled bool) bool {
	if tl.req.Usage == nil || budgetExceeded || loopToolsDisabled || tl.req.Ephemeral {
		return loopToolsDisabled
	}

	contextSize := tl.req.Model.EffectiveContextSize()
	if contextSize <= 0 {
		contextSize = 128000
	}
	cc := resolveCompactionConfig(tl.cfg, tl.req.ProviderID, tl.req.Model.ID, contextSize)
	baseTokens := tl.req.Usage.InputTokens + tl.req.Usage.CacheWriteTokens
	appendedTokens := estimateProviderMessages(tl.req.Messages[msgCountBeforeIteration:])
	systemDelta := estimateSystemPartTokens(tl.req.SystemParts) - systemTokensBeforeIteration
	estimatedTotal := baseTokens + appendedTokens + systemDelta

	if float64(estimatedTotal) >= float64(contextSize)*cc.threshold {
		log.Printf("compaction: mid-turn threshold reached (%d+%d tokens, %.1f%% of %d), compacting",
			baseTokens, appendedTokens, float64(estimatedTotal)/float64(contextSize)*100, contextSize)
		var isReadOnly func(string) bool
		if tl.sndbx != nil {
			isReadOnly = tl.sndbx.IsReadOnly
		}
		if err := maybeCompact(tl.ctx, tl.cfg, tl.msgs, isReadOnly, tl.utility, tl.publish, tl.req, estimatedTotal, tl.sc); err != nil {
			log.Printf("compaction: mid-turn compaction failed: %v", err)
		}
		estimatedTotal = estimateProviderMessages(tl.req.Messages)
		for _, sp := range tl.req.SystemParts {
			estimatedTotal += (len(sp) + 3) / 4
		}
	}

	if float64(estimatedTotal) <= float64(contextSize)*cc.contextLimit {
		return loopToolsDisabled
	}

	log.Printf("compaction: mid-turn context limit (%d tokens, %.0f%% of %d), stopping tools",
		estimatedTotal, cc.contextLimit*100, contextSize)
	setWorkflowRuntimeDirective(tl.req, "The context window is nearly full. Please provide your response now based on the information you have already gathered. Do not attempt to call any more tools.")
	return true
}

func (tl *turnLoop) appendDiagnostics(modifiedFiles []string, lastDiagOutput *string, diagRepeatCount *int) {
	if len(modifiedFiles) == 0 || tl.req.Ephemeral {
		return
	}
	diag := tool.RunDiagnostics(tl.ctx, tl.sndbx.ProjectDir(), modifiedFiles, tl.lspDiagnoser)
	if diag == "" {
		*lastDiagOutput = ""
		*diagRepeatCount = 0
		return
	}

	if diag == *lastDiagOutput {
		*diagRepeatCount++
	} else {
		*diagRepeatCount = 0
		*lastDiagOutput = diag
	}

	var diagMsg string
	switch *diagRepeatCount {
	case 0:
		diagMsg = "[DIAGNOSTICS]\n" + diag + "\n[/DIAGNOSTICS]\nFix the issues above before proceeding."
	case 1:
		diagMsg = "[DIAGNOSTICS]\n" + diag + "\n[/DIAGNOSTICS]\nThese diagnostics persist after your fix attempt. They may be false positives or stale LSP cache. Continue with your task if you cannot resolve them."
	default:
		log.Printf("auto-diagnostics: suppressed (same output repeated %d times)", *diagRepeatCount+1)
	}
	if diagMsg == "" {
		return
	}

	log.Printf("auto-diagnostics: found issues in %d files (repeat=%d)", len(modifiedFiles), *diagRepeatCount)
	setWorkflowRuntimeDirective(tl.req, diagMsg)
}

func (tl *turnLoop) shouldNudgeVerification(filesModified, hasRunVerification, verifyNudgeSent bool) bool {
	if verifyNudgeSent || !filesModified || hasRunVerification || tl.req.Ephemeral {
		return false
	}
	for _, t := range tl.req.Tools {
		if t.Name == "test" {
			return true
		}
	}
	return false
}

func (tl *turnLoop) shouldRetryIncompletePrebuild(prebuildRetrySent bool, finishReason string, toolCalls []provider.ToolCall) bool {
	if prebuildRetrySent || tl.req == nil || tl.req.Ephemeral || !isEngineerWorkflowAgent(tl.req.AgentID) {
		return false
	}
	if finishReason != "stop" || len(toolCalls) > 0 {
		return false
	}
	if !hasTool(tl.req.Tools, "test") {
		return false
	}
	workflow := ensureWorkflowState(tl.req)
	return workflow.Phase == pipeline.WorkflowPhasePrebuild && !workflow.HasCalledTest
}

const (
	executionStallQuestionText     = "Execution is stalled on the current task. How should KodaCode proceed?"
	executionStallPurpose          = "execution_stall_resolution"
	executionStallContinueOption   = "Keep working on the current task"
	executionStallBlockOption      = "Mark the current task blocked and stop"
	executionStallStopOption       = "Stop and wait for me"
	defaultToolCallArgumentTimeout = 5 * time.Minute
)

type executionStallDecision string

const (
	executionStallDecisionContinue executionStallDecision = "continue"
	executionStallDecisionBlock    executionStallDecision = "block"
	executionStallDecisionStop     executionStallDecision = "stop"
)

func (tl *turnLoop) builderReviewLimit() int {
	if tl != nil && tl.cfg != nil && tl.cfg.EngineerReviewLimit > 0 {
		return tl.cfg.EngineerReviewLimit
	}
	return 3
}

func (tl *turnLoop) toolCallArgumentTimeout(providerID, modelID string) time.Duration {
	if tl != nil && tl.cfg != nil {
		if mc, ok := tl.cfg.ModelConfig(providerID, modelID); ok && mc.ToolCallArgumentTimeout > 0 {
			return time.Duration(mc.ToolCallArgumentTimeout) * time.Second
		}
		if tl.cfg.ToolCallArgumentTimeout > 0 {
			return time.Duration(tl.cfg.ToolCallArgumentTimeout) * time.Second
		}
	}
	return defaultToolCallArgumentTimeout
}

func (tl *turnLoop) incompleteTaskExecutionRetry(finishReason string, toolCalls []provider.ToolCall, toolsDisabled bool) string {
	if tl.req == nil || tl.req.Ephemeral || !isEngineerWorkflowAgent(tl.req.AgentID) || tl.taskStore == nil || toolsDisabled {
		return ""
	}
	if finishReason != "stop" || len(toolCalls) > 0 {
		return ""
	}
	if ensureWorkflowState(tl.req).Phase != pipeline.WorkflowPhaseApproved {
		return ""
	}

	active, next := tl.incompleteTaskExecutionTasks()

	switch {
	case active != nil && next != nil:
		return fmt.Sprintf("[SYSTEM: Task execution is not complete. The current active task is %s: %s. Before stopping, either continue working on this task with tools, or call task.update to mark it completed or blocked. If you complete it, immediately set %s (%s) to in_progress and continue. Do not stop yet.]", active.ID, active.Title, next.ID, next.Title)
	case active != nil:
		return fmt.Sprintf("[SYSTEM: Task execution is not complete. The current active task is %s: %s. Before stopping, either continue working on this task with tools, or call task.update to mark it completed or blocked. Do not stop yet.]", active.ID, active.Title)
	case next != nil:
		return fmt.Sprintf("[SYSTEM: Task execution is not complete. Start the next pending task now: %s (%s). Call task.update to set it to in_progress, then continue with tools. Do not stop yet.]", next.ID, next.Title)
	default:
		return ""
	}
}

func (tl *turnLoop) incompleteTaskExecutionTasks() (*tool.Task, *tool.Task) {
	if tl.req == nil || tl.taskStore == nil {
		return nil, nil
	}
	tasks := tl.taskStore.GetTasks(tl.req.SessionID)
	var active *tool.Task
	var next *tool.Task
	for _, t := range tasks {
		if t == nil {
			continue
		}
		if active == nil && t.Status == "in_progress" {
			active = t
		}
		if next == nil && t.Status == "pending" {
			next = t
		}
	}
	return active, next
}

func (tl *turnLoop) incompleteTaskExecutionState() string {
	active, next := tl.incompleteTaskExecutionTasks()
	switch {
	case active != nil && next != nil:
		return fmt.Sprintf("active:%s:%s|next:%s:%s", active.ID, active.Status, next.ID, next.Status)
	case active != nil:
		return fmt.Sprintf("active:%s:%s", active.ID, active.Status)
	case next != nil:
		return fmt.Sprintf("next:%s:%s", next.ID, next.Status)
	default:
		return ""
	}
}

func (tl *turnLoop) blockStalledExecutionTask(directiveFmt string) (bool, error) {
	active, next := tl.incompleteTaskExecutionTasks()
	target := active
	if target == nil {
		target = next
	}
	if target == nil {
		return false, nil
	}
	updated, err := tl.taskStore.UpdateTaskWorkflowState(tl.ctx, tl.req.SessionID, target.ID, "blocked", "", tool.TaskBlockReasonExecutionStall, target.LastReviewSummary)
	if err != nil {
		return false, err
	}
	tl.publishTaskSync(updated.ID)
	tl.publishWorkflowStatusMessage(fmt.Sprintf("%s (%s) is blocked: execution stalled.", updated.ID, updated.Title))
	setWorkflowRuntimeDirective(tl.req, fmt.Sprintf(directiveFmt, updated.ID, updated.Title))
	return true, nil
}

func (tl *turnLoop) handleExecutionStallDecision() (executionStallDecision, bool, error) {
	active, next := tl.incompleteTaskExecutionTasks()
	if tl.askUser == nil {
		disableTools, err := tl.blockStalledExecutionTask("[SYSTEM: Execution stalled on %s (%s). The task has been marked blocked. Do not call more tools. Respond briefly that execution is paused and wait for the user's next message.]")
		return executionStallDecisionBlock, disableTools, err
	}

	answer, err := tl.askUser(tl.ctx, tl.req.SessionID)(
		executionStallQuestionText,
		[]string{executionStallContinueOption, executionStallBlockOption, executionStallStopOption},
		false,
		executionStallPurpose,
	)
	if err != nil {
		return "", false, err
	}

	switch strings.TrimSpace(answer) {
	case executionStallContinueOption:
		if active == nil && next != nil {
			updated, err := tl.taskStore.UpdateTask(tl.ctx, tl.req.SessionID, next.ID, "in_progress", "", "", false)
			if err != nil {
				return "", false, err
			}
			tl.publishTaskSync(updated.ID)
			setWorkflowRuntimeDirective(tl.req, fmt.Sprintf("[SYSTEM: The user asked you to keep working. %s (%s) is now the active task. Use tools on this task now and do not stop until its status changes.]", updated.ID, updated.Title))
			return executionStallDecisionContinue, false, nil
		}
		setWorkflowRuntimeDirective(tl.req, fmt.Sprintf("[SYSTEM: The user asked you to keep working on %s (%s). Use tools on this task now and do not stop until its status changes.]", active.ID, active.Title))
		return executionStallDecisionContinue, false, nil
	case executionStallBlockOption:
		disableTools, err := tl.blockStalledExecutionTask("[SYSTEM: The user chose to block %s (%s). Do not call more tools. Respond briefly that execution is paused and wait for the next message.]")
		return executionStallDecisionBlock, disableTools, err
	default:
		setWorkflowRuntimeDirective(tl.req, "[SYSTEM: The user chose to stop and wait. Do not call more tools. Respond briefly that execution is paused and wait for the next message.]")
		return executionStallDecisionStop, true, nil
	}
}

func (tl *turnLoop) failedFileMutationRetry(retrySent bool, executions []toolExecution) string {
	if retrySent || tl.req == nil || tl.req.Ephemeral {
		return ""
	}

	var toolsUsed []string
	var paths []string
	for _, ex := range executions {
		if !executionFailed(ex) || !isFileMutationTool(ex.call.Name) {
			continue
		}
		toolsUsed = appendUnique(toolsUsed, ex.call.Name)
		if path := extractFilePath(ex.call.Arguments); path != "" {
			paths = appendUnique(paths, path)
		}
	}
	if len(toolsUsed) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("[SYSTEM: One or more file mutation tools failed (")
	sb.WriteString(strings.Join(toolsUsed, ", "))
	sb.WriteString("). Do not stop yet. Inspect the failed tool results, reread the affected file")
	if len(paths) != 1 {
		sb.WriteString("s")
	}
	if len(paths) > 0 {
		sb.WriteString(" first")
	}
	sb.WriteString(", and retry with a more exact edit or patch.")
	if len(paths) > 0 {
		sb.WriteString(" Affected path")
		if len(paths) != 1 {
			sb.WriteString("s")
		}
		sb.WriteString(": ")
		sb.WriteString(strings.Join(paths, ", "))
		sb.WriteString(".")
	}
	sb.WriteString(" If the exact text no longer matches, use read/read_files to capture the current content before trying again.]")
	return sb.String()
}

func initialUserText(msgs []provider.Message) string {
	for _, m := range msgs {
		if m.Role != "user" {
			continue
		}
		for _, p := range m.Parts {
			tp, ok := p.(provider.TextPart)
			if !ok {
				continue
			}
			text := strings.TrimSpace(tp.Text)
			if text == "" || strings.HasPrefix(text, "[SYSTEM:") {
				continue
			}
			return text
		}
	}
	return ""
}

func (tl *turnLoop) hasSideEffectingToolFailure(executions []toolExecution) bool {
	for _, ex := range executions {
		if ex.errStr != nil && tl.isSideEffectingTool(ex.call.Name) {
			return true
		}
	}
	return false
}

func executionFailed(ex toolExecution) bool {
	if ex.errStr != nil {
		return true
	}
	return ex.result != nil && ex.result.ErrorCode != ""
}

func isFileMutationTool(name string) bool {
	switch name {
	case "write", "edit", "patch":
		return true
	default:
		return false
	}
}

func appendUnique(values []string, value string) []string {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

func (tl *turnLoop) isSideEffectingTool(name string) bool {
	if tl.sndbx != nil {
		return !tl.sndbx.IsReadOnly(name)
	}
	switch name {
	case "write", "edit", "patch", "bash", "git", "task":
		return true
	default:
		return false
	}
}

// tasksNeedingReview returns completed tasks that haven't been reviewed yet.
func (tl *turnLoop) tasksNeedingReview(reviewedTasks map[string]bool) []*tool.Task {
	if tl.req.Ephemeral || !isEngineerWorkflowAgent(tl.req.AgentID) {
		return nil
	}
	if !hasTool(tl.req.Tools, "subagent") {
		return nil
	}
	if tl.taskStore == nil {
		return nil
	}
	tasks := tl.taskStore.GetTasks(tl.req.SessionID)
	var needsReview []*tool.Task
	for _, t := range tasks {
		if taskRequiresReview(t) && !reviewedTasks[t.ID] {
			needsReview = append(needsReview, t)
		}
	}
	return needsReview
}

func taskRequiresReview(task *tool.Task) bool {
	return task != nil && task.Status == "completed"
}

func (tl *turnLoop) filterWorkflowCompletionCalls(calls []provider.ToolCall) ([]provider.ToolCall, bool) {
	if tl.req == nil || tl.req.Ephemeral || !isEngineerWorkflowAgent(tl.req.AgentID) || len(calls) < 2 {
		return calls, false
	}
	filtered := make([]provider.ToolCall, 0, len(calls))
	completionSeen := false
	blocked := false
	for _, call := range calls {
		switch {
		case isTaskCompletionCall(call):
			if !completionSeen {
				completionSeen = true
				filtered = append(filtered, call)
				continue
			}
			blocked = true
			log.Printf("workflow: blocked extra task completion call %s in same response", call.ID)
		case completionSeen && isTaskStartCall(call):
			blocked = true
			log.Printf("workflow: blocked next-task start call %s until review finishes", call.ID)
		default:
			filtered = append(filtered, call)
			continue
		}
	}
	return filtered, blocked
}

func (tl *turnLoop) filterPhaseBlockedCalls(calls []provider.ToolCall) ([]provider.ToolCall, bool) {
	if tl.req == nil || !tl.req.PhaseFilterActive || len(calls) < 2 {
		return calls, false
	}

	filtered := make([]provider.ToolCall, 0, len(calls))
	blockedIncluded := false
	blockedAny := false
	for _, call := range calls {
		if isToolAllowed(call.Name, tl.req.Tools) {
			filtered = append(filtered, call)
			continue
		}
		blockedAny = true
		if blockedIncluded {
			continue
		}
		filtered = append(filtered, call)
		blockedIncluded = true
	}
	if !blockedAny || len(filtered) == len(calls) {
		return calls, false
	}
	return filtered, true
}

func isTaskCompletionCall(call provider.ToolCall) bool {
	if call.Name != "task" {
		return false
	}
	var args struct {
		Action string `json:"action"`
		Status string `json:"status"`
	}
	if err := json.Unmarshal([]byte(call.Arguments), &args); err != nil {
		return false
	}
	return args.Action == "update" && args.Status == "completed"
}

func isTaskStartCall(call provider.ToolCall) bool {
	if call.Name != "task" {
		return false
	}
	var args struct {
		Action string `json:"action"`
		Status string `json:"status"`
	}
	if err := json.Unmarshal([]byte(call.Arguments), &args); err != nil {
		return false
	}
	return args.Action == "update" && args.Status == "in_progress"
}

// countReviewerCallsSince counts reviewer subagent calls in the message
// history starting from index `since`. This allows per-task counting by
// resetting `since` each time a new task enters review.
func countReviewerCallsSince(msgs []provider.Message, since int) int {
	count := 0
	for i := since; i < len(msgs); i++ {
		m := msgs[i]
		if m.Role != "assistant" {
			continue
		}
		for _, p := range m.Parts {
			tc, ok := p.(provider.ToolCallPart)
			if !ok || tc.Name != "subagent" {
				continue
			}
			if plannerAgentIDFromArgs(tc.Arguments) == "reviewer" {
				count++
			}
		}
	}
	return count
}

const (
	reviewCapQuestionText          = "Reviewer retry limit reached. How should KodaCode proceed?"
	reviewCapPurpose               = "review_cap_resolution"
	reviewCapContinueFixOption     = "Continue fixing this task without more automatic reviews"
	reviewCapProceedOption         = "Accept unresolved reviewer findings and proceed"
	reviewCapStopOption            = "Stop execution and wait for me"
	reviewCapFallbackBlockedStatus = "blocked"
)

// extractReviewerFindings extracts FAIL/CONCERN lines from reviewer output.
// Uses case-insensitive Contains (not HasPrefix) because different models
// format output differently — bullets, brackets, markdown bold, emoji, etc.
// Skips the "Overall:" summary line to avoid duplication.
func extractReviewerFindings(output string) string {
	var findings []string
	for _, line := range strings.Split(output, "\n") {
		trimmed := strings.TrimSpace(line)
		upper := strings.ToUpper(trimmed)
		if strings.HasPrefix(upper, "OVERALL") {
			continue
		}
		if strings.Contains(upper, "FAIL") || strings.Contains(upper, "CONCERN") {
			findings = append(findings, trimmed)
		}
	}
	if len(findings) == 0 {
		return ""
	}
	return strings.Join(findings, "\n")
}

type reviewerVerdict int

const (
	reviewerVerdictUnknown reviewerVerdict = iota
	reviewerVerdictPass
	reviewerVerdictConcern
	reviewerVerdictFail
)

func parseReviewerVerdict(output string) reviewerVerdict {
	for _, line := range strings.Split(output, "\n") {
		trimmed := strings.TrimSpace(line)
		upper := strings.ToUpper(trimmed)
		if !strings.HasPrefix(upper, "OVERALL:") && !strings.HasPrefix(upper, "OVERALL ") {
			continue
		}
		switch {
		case strings.Contains(upper, "PASS"):
			return reviewerVerdictPass
		case strings.Contains(upper, "CONCERN"):
			return reviewerVerdictConcern
		case strings.Contains(upper, "FAIL"):
			return reviewerVerdictFail
		}
	}
	upper := strings.ToUpper(output)
	switch {
	case strings.Contains(upper, "[FAIL]"):
		return reviewerVerdictFail
	case strings.Contains(upper, "[CONCERN]"):
		return reviewerVerdictConcern
	case strings.Contains(upper, "[PASS]"):
		return reviewerVerdictPass
	default:
		return reviewerVerdictUnknown
	}
}

func reviewerStatus(verdict reviewerVerdict) string {
	switch verdict {
	case reviewerVerdictPass:
		return tool.TaskReviewPass
	case reviewerVerdictConcern:
		return tool.TaskReviewConcern
	case reviewerVerdictFail:
		return tool.TaskReviewFail
	default:
		return ""
	}
}

func truncateWorkflowSummary(s string, max int) string {
	s = strings.TrimSpace(strings.Join(strings.Fields(s), " "))
	if max <= 0 || len(s) <= max {
		return s
	}
	if max <= 1 {
		return s[:max]
	}
	return strings.TrimSpace(s[:max-1]) + "…"
}

func reviewerSummary(output string) string {
	findings := extractReviewerFindings(output)
	if findings != "" {
		return truncateWorkflowSummary(strings.ReplaceAll(findings, "\n", "; "), 240)
	}
	switch parseReviewerVerdict(output) {
	case reviewerVerdictPass:
		return "Acceptance review passed."
	case reviewerVerdictConcern:
		return "Reviewer raised unresolved concerns."
	case reviewerVerdictFail:
		return "Reviewer found unmet acceptance criteria."
	default:
		return ""
	}
}

func buildReviewerTask(task *tool.Task, tasks []*tool.Task, previousFindings string) string {
	if task == nil {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("Review whether the current task satisfies its acceptance criteria.\n\n")
	fmt.Fprintf(&sb, "Task: %s: %s\n\n", task.ID, task.Title)
	fmt.Fprintf(&sb, "Task kind: %s\n\n", normalizedTaskKindForPrompt(task.Kind))
	if notes := strings.TrimSpace(task.Notes); notes != "" {
		sb.WriteString("Task notes:\n")
		sb.WriteString(notes)
		sb.WriteString("\n\n")
	}
	if progress := strings.TrimSpace(task.Progress); progress != "" {
		sb.WriteString("Completion summary:\n")
		sb.WriteString(progress)
		sb.WriteString("\n\n")
	}
	switch normalizedTaskKindForPrompt(task.Kind) {
	case tool.TaskKindAnalysis:
		sb.WriteString("Review mode: analysis verification\n")
		sb.WriteString("- This task may be read-only. Do NOT require git diff or changed files.\n")
		sb.WriteString("- Verify that the completion summary actually satisfies the acceptance criteria in the task notes.\n")
		sb.WriteString("- Validate material claims against the current repository state using read, read_files, and grep.\n")
		sb.WriteString("- Any claim that is unsupported, overly vague, or contradicted by the code should be marked FAIL.\n")
	case tool.TaskKindReport:
		sb.WriteString("Review mode: report verification\n")
		sb.WriteString("- This task may be read-only. Do NOT require git diff or changed files.\n")
		sb.WriteString("- Verify that the completion summary satisfies the task notes and accurately synthesizes prior completed work.\n")
		if summaries := completedTaskSummaries(tasks, task.ID); summaries != "" {
			sb.WriteString("- Use the completed task summaries below as source material for synthesis checks.\n\n")
			sb.WriteString("Completed task summaries:\n")
			sb.WriteString(summaries)
			sb.WriteString("\n")
		}
		sb.WriteString("- Validate any repository claims directly with read, read_files, and grep.\n")
	default:
		sb.WriteString("Review mode: implementation verification\n")
		sb.WriteString("- Use git changed_files with no args to list the current changed files.\n")
		sb.WriteString("- Use git diff with no args to inspect the current uncommitted changes.\n")
		sb.WriteString("- Review only those changed files, plus direct consumers when a criterion explicitly requires checking them.\n")
		sb.WriteString("- Verify correctness against the acceptance criteria only.\n")
	}
	if findings := strings.TrimSpace(previousFindings); findings != "" {
		sb.WriteString("\nPrevious reviewer findings for targeted re-review:\n")
		sb.WriteString(findings)
		sb.WriteString("\n")
	}
	return strings.TrimSpace(sb.String())
}

func (tl *turnLoop) publishWorkflowStatusMessage(content string) {
	if tl.publish == nil || tl.req == nil {
		return
	}
	content = strings.TrimSpace(content)
	if content == "" {
		return
	}
	tl.publish(tl.req.SessionID, SSEEvent{
		Type: "system_message",
		Data: SSESystemMessageData{Content: content},
	})
}

func reviewerExecution(executions []toolExecution) (toolExecution, bool) {
	for _, ex := range executions {
		if ex.call.Name == "subagent" && plannerAgentIDFromArgs(ex.call.Arguments) == "reviewer" {
			return ex, true
		}
	}
	return toolExecution{}, false
}

func (tl *turnLoop) maybeHandleCompletedTaskReview(executions []toolExecution, reviewedTasks map[string]bool, reviewSinceIdx *int, reviewTaskID *string, loopToolsDisabled, budgetExceeded bool) (bool, bool, error) {
	if loopToolsDisabled || budgetExceeded || tl.req == nil || tl.req.Ephemeral || !isEngineerWorkflowAgent(tl.req.AgentID) || tl.taskStore == nil || tl.spawnSubagent == nil {
		return false, false, nil
	}
	pending := tl.tasksNeedingReview(reviewedTasks)
	if len(pending) == 0 {
		return false, false, nil
	}
	task := pending[0]
	if reviewTaskID != nil && *reviewTaskID != task.ID {
		*reviewTaskID = task.ID
		*reviewSinceIdx = len(tl.req.Messages)
		tl.reviewFindings = ""
	}
	if *reviewSinceIdx == 0 {
		*reviewSinceIdx = len(tl.req.Messages)
	}
	if n := countReviewerCallsSince(tl.req.Messages, *reviewSinceIdx); n >= tl.builderReviewLimit() {
		log.Printf("review: cap reached for %s after %d reviewer calls", task.ID, n)
		disableTools, err := tl.handleReviewCapDecision(task, reviewedTasks)
		return true, disableTools, err
	}

	reviewExec, hasReviewer := reviewerExecution(executions)
	if !hasReviewer {
		taskPrompt := buildReviewerTask(task, tl.taskStore.GetTasks(tl.req.SessionID), tl.reviewFindings)
		callID := fmt.Sprintf("workflow-controller-reviewer-%s-%d", strings.ReplaceAll(task.ID, " ", "-"), countReviewerCallsSince(tl.req.Messages, *reviewSinceIdx)+1)
		call, err := tl.startWorkflowControllerSubagent("reviewer", taskPrompt, callID)
		if err != nil {
			return false, false, err
		}
		output, err := tl.spawnSubagent(tl.ctx, tl.req.SessionID, "reviewer", taskPrompt, nil)
		reviewExec = tl.finishWorkflowControllerSubagent(call, output, err)
		if err != nil {
			return false, false, fmt.Errorf("workflow controller: reviewer failed: %w", err)
		}
	}

	verdict := parseReviewerVerdict(reviewExec.output)
	findings := extractReviewerFindings(reviewExec.output)
	tl.reviewFindings = findings
	summary := reviewerSummary(reviewExec.output)

	switch verdict {
	case reviewerVerdictPass:
		updated, err := tl.taskStore.UpdateTaskWorkflowState(tl.ctx, tl.req.SessionID, task.ID, "completed", reviewerStatus(verdict), "", summary)
		if err != nil {
			return false, false, err
		}
		tl.publishTaskSync(updated.ID)
		reviewedTasks[task.ID] = true
		tl.reviewFindings = ""
		log.Printf("review: %s passed acceptance-criteria review", task.ID)
	default:
		updated, err := tl.taskStore.UpdateTaskWorkflowState(tl.ctx, tl.req.SessionID, task.ID, "in_progress", reviewerStatus(verdict), "", summary)
		if err != nil {
			return false, false, err
		}
		if tl.publish != nil {
			tl.publish(tl.req.SessionID, SSEEvent{
				Type: "task_sync",
				Data: SSETaskSyncData{ActiveTaskID: updated.ID},
			})
		}
		switch verdict {
		case reviewerVerdictConcern:
			tl.publishWorkflowStatusMessage(fmt.Sprintf("%s (%s) remains open. Reviewer verdict: CONCERN. %s", updated.ID, updated.Title, summary))
		default:
			tl.publishWorkflowStatusMessage(fmt.Sprintf("%s (%s) remains open. Reviewer verdict: FAIL. %s", updated.ID, updated.Title, summary))
		}
		setWorkflowRuntimeDirective(tl.req, fmt.Sprintf("[SYSTEM: %s (%s) did not pass acceptance-criteria review. Address the reviewer findings and only mark it completed again after the issues are fixed.]", updated.ID, updated.Title))
		log.Printf("review: %s failed or raised concerns; moved back to in_progress", updated.ID)
	}

	return true, false, nil
}

func (tl *turnLoop) handleReviewCapDecision(task *tool.Task, reviewedTasks map[string]bool) (bool, error) {
	if task == nil {
		return false, nil
	}
	if tl.askUser == nil {
		updated, err := tl.taskStore.UpdateTaskWorkflowState(tl.ctx, tl.req.SessionID, task.ID, reviewCapFallbackBlockedStatus, normalizeTaskReviewStatusForTask(task), tool.TaskBlockReasonReviewCap, strings.TrimSpace(task.LastReviewSummary))
		if err != nil {
			return false, err
		}
		reviewedTasks[task.ID] = true
		tl.reviewFindings = ""
		tl.publishTaskSync(updated.ID)
		tl.publishWorkflowStatusMessage(fmt.Sprintf("Reviewer retry limit reached for %s (%s). %s", updated.ID, updated.Title, strings.TrimSpace(updated.LastReviewSummary)))
		setWorkflowRuntimeDirective(tl.req, fmt.Sprintf("[SYSTEM: Reviewer retry limit reached for %s (%s). Execution is paused because no user question handler is available. Do not call more tools. Wait for the user's next message.]", updated.ID, updated.Title))
		return true, nil
	}

	questionText := reviewCapQuestionText
	if summary := strings.TrimSpace(task.LastReviewSummary); summary != "" {
		questionText += "\n\nOutstanding reviewer findings: " + summary
	}
	answer, err := tl.askUser(tl.ctx, tl.req.SessionID)(
		questionText,
		[]string{reviewCapContinueFixOption, reviewCapProceedOption, reviewCapStopOption},
		false,
		reviewCapPurpose,
	)
	if err != nil {
		return false, err
	}

	switch strings.TrimSpace(answer) {
	case reviewCapContinueFixOption:
		updated, err := tl.taskStore.UpdateTaskWorkflowState(tl.ctx, tl.req.SessionID, task.ID, "in_progress", normalizeTaskReviewStatusForTask(task), "", strings.TrimSpace(task.LastReviewSummary))
		if err != nil {
			return false, err
		}
		reviewedTasks[task.ID] = true
		tl.reviewFindings = ""
		tl.publishTaskSync(updated.ID)
		tl.publishWorkflowStatusMessage(fmt.Sprintf("Continuing %s (%s) without more automatic reviewer retries. Outstanding reviewer findings remain: %s", updated.ID, updated.Title, strings.TrimSpace(updated.LastReviewSummary)))
		setWorkflowRuntimeDirective(tl.req, fmt.Sprintf("[SYSTEM: The user chose to keep working on %s (%s) without further automatic reviewer retries. Continue only this task and do not start another task until it is ready.]", updated.ID, updated.Title))
		return false, nil
	case reviewCapProceedOption:
		updated, err := tl.taskStore.UpdateTaskWorkflowState(tl.ctx, tl.req.SessionID, task.ID, "completed", tool.TaskReviewAccepted, "", strings.TrimSpace(task.LastReviewSummary))
		if err != nil {
			return false, err
		}
		reviewedTasks[task.ID] = true
		tl.reviewFindings = ""
		tl.publishTaskSync(updated.ID)
		tl.publishWorkflowStatusMessage(fmt.Sprintf("The user accepted unresolved reviewer findings for %s (%s). %s", updated.ID, updated.Title, strings.TrimSpace(updated.LastReviewSummary)))
		setWorkflowRuntimeDirective(tl.req, fmt.Sprintf("[SYSTEM: The user accepted unresolved reviewer findings for %s (%s). You may proceed, but clearly report the unresolved findings in your final response.]", updated.ID, updated.Title))
		return false, nil
	default:
		updated, err := tl.taskStore.UpdateTaskWorkflowState(tl.ctx, tl.req.SessionID, task.ID, reviewCapFallbackBlockedStatus, normalizeTaskReviewStatusForTask(task), tool.TaskBlockReasonReviewCap, strings.TrimSpace(task.LastReviewSummary))
		if err != nil {
			return false, err
		}
		reviewedTasks[task.ID] = true
		tl.reviewFindings = ""
		tl.publishTaskSync(updated.ID)
		tl.publishWorkflowStatusMessage(fmt.Sprintf("%s (%s) is blocked: reviewer retry limit reached. %s", updated.ID, updated.Title, strings.TrimSpace(updated.LastReviewSummary)))
		setWorkflowRuntimeDirective(tl.req, fmt.Sprintf("[SYSTEM: The user chose to stop after reviewer retry limit was reached for %s (%s). Do not call more tools. Respond briefly that execution is paused and wait for the next message.]", updated.ID, updated.Title))
		return true, nil
	}
}

func normalizeTaskReviewStatusForTask(task *tool.Task) string {
	if task == nil {
		return ""
	}
	switch strings.TrimSpace(strings.ToLower(task.ReviewStatus)) {
	case tool.TaskReviewPass:
		return tool.TaskReviewPass
	case tool.TaskReviewConcern:
		return tool.TaskReviewConcern
	case tool.TaskReviewFail:
		return tool.TaskReviewFail
	case tool.TaskReviewAccepted:
		return tool.TaskReviewAccepted
	default:
		return ""
	}
}

func (tl *turnLoop) publishTaskSync(activeTaskID string) {
	if tl.publish == nil {
		return
	}
	tl.publish(tl.req.SessionID, SSEEvent{
		Type: "task_sync",
		Data: SSETaskSyncData{ActiveTaskID: activeTaskID},
	})
}

func normalizedTaskKindForPrompt(kind string) string {
	switch strings.TrimSpace(strings.ToLower(kind)) {
	case tool.TaskKindAnalysis:
		return tool.TaskKindAnalysis
	case tool.TaskKindReport:
		return tool.TaskKindReport
	default:
		return tool.TaskKindImplementation
	}
}

func completedTaskSummaries(tasks []*tool.Task, excludeID string) string {
	var sb strings.Builder
	for _, task := range tasks {
		if task == nil || task.ID == excludeID || task.Status != "completed" {
			continue
		}
		summary := strings.TrimSpace(task.Progress)
		if summary == "" {
			continue
		}
		fmt.Fprintf(&sb, "- %s (%s): %s\n", task.ID, task.Title, summary)
	}
	return strings.TrimSpace(sb.String())
}

func (tl *turnLoop) publishDone() {
	doneCost := 0.0
	doneSubagentCost := 0.0
	doneBudgetWarn := false
	var doneCostSnap *CostSnapshot
	if tl.sc != nil {
		s := tl.sc.Snapshot()
		doneCostSnap = &s
		doneCost = s.TotalCost
		doneSubagentCost = s.SubagentCost
	}
	if tl.budgetStatus != nil {
		status := tl.budgetStatus(tl.ctx, tl.req.SessionID, tl.cfg)
		doneBudgetWarn = status.SessionWarn || status.TotalWarn
	}
	doneContextSize := tl.req.Model.ContextSize
	if doneContextSize == 0 {
		doneContextSize = defaultContextSize
	}
	tl.publish(tl.req.SessionID, SSEEvent{
		Type: "done",
		Data: SSEDoneData{
			Usage:           tl.req.Usage,
			ContextSize:     doneContextSize,
			MaxInputTokens:  tl.req.Model.MaxInputTokens,
			MaxOutputTokens: tl.req.Agent.MaxTokens,
			SessionCost:     doneCost,
			SubagentCost:    doneSubagentCost,
			BudgetWarn:      doneBudgetWarn,
			CostSnapshot:    doneCostSnap,
		},
	})
}

func (tl *turnLoop) snapshotStep(filesModified bool) {
	if !filesModified || tl.snapshotSvc == nil || tl.req.Ephemeral {
		return
	}
	if err := tl.snapshotSvc.Create(tl.req.SessionID, tl.req.Step, fmt.Sprintf("step %d", tl.req.Step)); err != nil {
		log.Printf("snapshot: create failed: %v", err)
	}
}
