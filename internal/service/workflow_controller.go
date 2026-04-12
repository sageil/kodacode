package service

import (
	"encoding/json"
	"fmt"
	"log"
	"regexp"
	"strconv"
	"strings"

	"github.com/sageil/kodacode/v1/internal/pipeline"
	"github.com/sageil/kodacode/v1/internal/provider"
	"github.com/sageil/kodacode/v1/internal/tool"
)

func (tl *turnLoop) maybeBootstrapWorkflowController() (bool, error) {
	if tl.req == nil || tl.req.Ephemeral || !isEngineerWorkflowAgent(tl.req.AgentID) || tl.taskStore == nil || tl.spawnSubagent == nil {
		return false, nil
	}

	workflow := ensureWorkflowState(tl.req)
	if workflow.Plan.EffectiveStatus == pipeline.WorkflowApprovalApproved || workflow.HasCalledPlanner {
		return false, nil
	}
	if !workflow.HasCalledTest {
		return false, nil
	}
	if len(tl.taskStore.GetTasks(tl.req.SessionID)) > 0 {
		return false, nil
	}

	intent := tl.workflowIntentForTurn()
	if intent == nil || !shouldBootstrapWorkflowIntent(intent.Kind) {
		return false, nil
	}
	userText := initialUserText(tl.req.Messages)

	log.Printf(
		"workflow-controller: bootstrapping explorer/planner for session %s (intent=%s confidence=%.2f)",
		tl.req.SessionID,
		intent.Kind,
		intent.Confidence,
	)

	explorerOutput := strings.TrimSpace(strings.Join(completedSubagentOutputs(tl.req.Messages, "explorer"), "\n\n---\n\n"))
	if explorerOutput == "" {
		explorerTask := fmt.Sprintf(
			"Explore the codebase for the following user request and gather the concrete context needed to act on it:\n\n%s\n\nFocus on the relevant architecture, files, constraints, and likely change surfaces. Return concise findings and end with exactly one scope marker on its own line: [SCOPE: focused] or [SCOPE: broad].",
			userText,
		)
		explorerCall, err := tl.startWorkflowControllerSubagent("explorer", explorerTask, "workflow-controller-explorer")
		if err != nil {
			return false, err
		}
		explorerOutput, err = tl.spawnSubagent(tl.ctx, tl.req.SessionID, "explorer", explorerTask, nil)
		tl.finishWorkflowControllerSubagent(explorerCall, explorerOutput, err)
		if err != nil {
			return false, fmt.Errorf("workflow controller: explorer failed: %w", err)
		}
	}

	plannerTask := fmt.Sprintf(
		"Using the user request and explorer findings below, create an actionable implementation plan.\n\nUser request:\n%s\n\nExplorer findings:\n%s\n\nReturn ONLY a JSON object with this exact schema:\n{\"summary\":\"short user-facing plan summary\",\"tasks\":[{\"title\":\"task title\",\"kind\":\"implementation|analysis|report\",\"notes\":\"durable task details followed by an explicit Acceptance criteria section\"}]}\nRules:\n- The task tool is unavailable in this run.\n- Do not call any tools.\n- Do not wrap the JSON in markdown fences.\n- Produce 2-7 ordered tasks.\n- Keep titles concise and imperative.\n- kind is REQUIRED for every task.\n- Use kind=implementation for code changes, kind=analysis for audit/research tasks, and kind=report for final report/synthesis tasks.\n- notes is REQUIRED for every task.\n- Every task notes field MUST include an explicit `Acceptance criteria:` section.\n- Format notes like: `Short durable details. Acceptance criteria: 1. First criterion. 2. Second criterion.`\n- Include 2-5 concrete acceptance criteria per task.\n- Keep criteria specific and verifiable.",
		userText,
		explorerOutput,
	)

	for attempt := 0; attempt < 2; attempt++ {
		plannerCall, err := tl.startWorkflowControllerSubagent("planner", plannerTask, fmt.Sprintf("workflow-controller-planner-%d", attempt+1))
		if err != nil {
			return false, err
		}
		plannerOutput, err := tl.spawnSubagent(withStructuredPlanner(tl.ctx), tl.req.SessionID, "planner", plannerTask, nil)
		if err != nil {
			tl.finishWorkflowControllerSubagent(plannerCall, plannerOutput, err)
			return false, fmt.Errorf("workflow controller: planner failed: %w", err)
		}

		plan, err := parseStructuredPlan(plannerOutput)
		if err == nil && len(plan.Tasks) > 0 {
			createdTasks := make([]*tool.Task, 0, len(plan.Tasks))
			for _, t := range plan.Tasks {
				created, _, err := tl.taskStore.CreateTaskWithKind(tl.ctx, tl.req.SessionID, t.Title, t.Kind, "pending", t.Notes)
				if err != nil {
					tl.finishWorkflowControllerSubagent(plannerCall, plannerOutput, fmt.Errorf("create task: %w", err))
					return false, fmt.Errorf("workflow controller: create task: %w", err)
				}
				createdTasks = append(createdTasks, created)
			}

			renderedPlan := renderStructuredPlan(plan.Summary, createdTasks)
			plannerExec := tl.finishWorkflowControllerSubagent(plannerCall, renderedPlan, nil)
			retry, err := tl.maybeHandlePlannerApproval([]toolExecution{plannerExec}, nil)
			if err != nil {
				return false, err
			}
			if retry {
				return false, fmt.Errorf("workflow controller: unexpected planner retry after structured plan parse")
			}
			ApplyPhaseRules(tl.req)
			return true, nil
		}

		tl.finishWorkflowControllerSubagent(
			plannerCall,
			invalidStructuredPlanRetryOutput(err),
			fmt.Errorf("planner returned invalid structured output"),
		)
		plannerTask += "\n\nIMPORTANT: Your previous response was not valid. Return ONLY a JSON object matching the requested schema, with a non-empty tasks array. Do not call any tools."
	}

	return false, fmt.Errorf("workflow controller: planner returned invalid structured plan")
}

func shouldBootstrapWorkflowIntent(kind workflowIntentKind) bool {
	switch kind {
	case workflowIntentBroadReviewExecute, workflowIntentPlanOnly:
		return true
	default:
		return false
	}
}

func completedSubagentOutputs(msgs []provider.Message, agentID string) []string {
	if agentID == "" {
		return nil
	}
	callIDs := make(map[string]struct{})
	var outputs []string
	for _, m := range msgs {
		switch m.Role {
		case "assistant":
			for _, p := range m.Parts {
				tc, ok := p.(provider.ToolCallPart)
				if !ok || tc.Name != "subagent" || plannerAgentIDFromArgs(tc.Arguments) != agentID {
					continue
				}
				callIDs[tc.ID] = struct{}{}
			}
		case "user":
			for _, p := range m.Parts {
				tr, ok := p.(provider.ToolResultPart)
				if !ok || tr.Error != nil {
					continue
				}
				if _, ok := callIDs[tr.ToolCallID]; !ok {
					continue
				}
				if out := strings.TrimSpace(tr.Output); out != "" {
					outputs = append(outputs, out)
				}
			}
		}
	}
	return outputs
}

func (tl *turnLoop) startWorkflowControllerSubagent(agentID, task, callID string) (provider.ToolCall, error) {
	callArgs, err := workflowControllerSubagentArgs(agentID, task)
	if err != nil {
		return provider.ToolCall{}, err
	}
	call := provider.ToolCall{
		ID:        callID,
		Name:      "subagent",
		Arguments: callArgs,
	}
	if tl.publish != nil {
		tl.publish(tl.req.SessionID, SSEEvent{
			Type: "tool_start",
			Data: SSEToolStartData{Tool: call.Name, Input: call.Arguments, CallID: call.ID},
		})
	}
	tl.persistAssistantMessage("", nil, []provider.ToolCall{call})
	tl.req.Messages = append(tl.req.Messages, provider.Message{
		Role:  "assistant",
		Parts: []provider.MessagePart{provider.ToolCallPart(call)},
	})
	noteWorkflowToolCalls(tl.req, []provider.ToolCall{call})
	return call, nil
}

func (tl *turnLoop) finishWorkflowControllerSubagent(call provider.ToolCall, output string, runErr error) toolExecution {
	output = strings.TrimSpace(output)
	if output == "" && runErr == nil {
		output = "Subagent produced no response."
	}
	var errStr *string
	if runErr != nil {
		msg := runErr.Error()
		errStr = &msg
		if output == "" {
			output = fmt.Sprintf("tool error [%s]: %s", call.Name, runErr)
		}
	}
	if tl.publish != nil {
		tl.publish(tl.req.SessionID, SSEEvent{
			Type: "tool_end",
			Data: SSEToolEndData{Tool: call.Name, Output: output, Error: errStr, CallID: call.ID},
		})
	}
	exec := toolExecution{
		call:   call,
		output: output,
		errStr: errStr,
	}
	tl.persistToolResults([]toolExecution{exec})
	tl.req.Messages = append(tl.req.Messages, provider.Message{
		Role: "user",
		Parts: []provider.MessagePart{provider.ToolResultPart{
			ToolCallID: call.ID,
			Output:     output,
			Error:      errStr,
		}},
	})
	return exec
}

func workflowControllerSubagentArgs(agentID, task string) (string, error) {
	payload, err := json.Marshal(map[string]string{
		"agent_id": agentID,
		"task":     task,
	})
	if err != nil {
		return "", fmt.Errorf("marshal %s subagent args: %w", agentID, err)
	}
	return string(payload), nil
}

func renderStructuredPlan(summary string, tasks []*tool.Task) string {
	summary = strings.TrimSpace(summary)
	var sb strings.Builder
	if summary != "" {
		sb.WriteString(summary)
		sb.WriteString("\n\n")
	}
	sb.WriteString("## Tasks\n\n")
	firstTask := true
	for _, task := range tasks {
		if task == nil {
			continue
		}
		if !firstTask {
			sb.WriteString("\n\n")
		}
		firstTask = false
		fmt.Fprintf(&sb, "### %s: %s", task.ID, strings.TrimSpace(task.Title))
		renderStructuredTaskNotes(&sb, strings.TrimSpace(task.Notes))
	}
	return strings.TrimSpace(sb.String())
}

func renderStructuredTaskNotes(sb *strings.Builder, notes string) {
	if notes == "" {
		return
	}
	description, criteria := splitAcceptanceCriteria(notes)
	if description != "" {
		sb.WriteString("\n")
		sb.WriteString(description)
	}
	if len(criteria) == 0 {
		return
	}
	sb.WriteString("\n\nAcceptance criteria\n")
	for _, item := range criteria {
		if item == "" {
			continue
		}
		sb.WriteString("- ")
		sb.WriteString(item)
		sb.WriteString("\n")
	}
}

func splitAcceptanceCriteria(notes string) (string, []string) {
	re := regexp.MustCompile(`(?is)\bacceptance criteria\b\s*:?\s*`)
	loc := re.FindStringIndex(notes)
	if loc == nil {
		return strings.TrimSpace(notes), nil
	}
	description := strings.TrimSpace(notes[:loc[0]])
	criteriaText := strings.TrimSpace(notes[loc[1]:])
	if criteriaText == "" {
		return description, nil
	}
	items := parseAcceptanceCriteriaItems(criteriaText)
	if len(items) == 0 {
		return description, []string{criteriaText}
	}
	return description, items
}

func parseAcceptanceCriteriaItems(raw string) []string {
	raw = strings.ReplaceAll(raw, "\r\n", "\n")
	if strings.Contains(raw, "\n") {
		lines := strings.Split(raw, "\n")
		items := make([]string, 0, len(lines))
		for _, line := range lines {
			line = strings.TrimSpace(line)
			line = strings.TrimLeft(line, "-* ")
			line = strings.TrimSpace(regexp.MustCompile(`^\d+[.)]\s*`).ReplaceAllString(line, ""))
			if line == "" {
				continue
			}
			items = append(items, line)
		}
		return items
	}
	re := regexp.MustCompile(`(?:^|\s+)\d+[.)]\s+`)
	locs := re.FindAllStringIndex(raw, -1)
	if len(locs) == 0 {
		return nil
	}
	items := make([]string, 0, len(locs))
	for i, loc := range locs {
		start := loc[1]
		end := len(raw)
		if i+1 < len(locs) {
			end = locs[i+1][0]
		}
		item := strings.TrimSpace(raw[start:end])
		if item != "" {
			items = append(items, item)
		}
	}
	return items
}

func invalidStructuredPlanRetryOutput(err error) string {
	msg := "Planner output did not match the required structured task schema and was not adopted. Retrying with stricter instructions."
	if err == nil {
		return msg
	}
	return msg + "\nReason: " + err.Error()
}

type structuredPlan struct {
	Summary string               `json:"summary"`
	Tasks   []structuredPlanTask `json:"tasks"`
}

type structuredPlanTask struct {
	Title string `json:"title"`
	Kind  string `json:"kind,omitempty"`
	Notes string `json:"notes"`
}

var (
	structuredPlanHeadingRE       = regexp.MustCompile(`(?i)^\s*(?:#+\s*)?(?:[-*]\s*)?(?:(?:task|part)\s*\d+\s*[:.)-]?\s*|\d+[.)]\s+)(.+?)\s*$`)
	structuredPlanInlineLabelRE   = regexp.MustCompile(`(?i)(\b(?:tasks|parts):)\s+((?:#+\s*)?(?:(?:task|part)\s*\d+\s*[:.)-]?\s*|\d+[.)]\s+))`)
	structuredPlanInlineAfterPunc = regexp.MustCompile(`(?i)([.!?])\s+((?:#+\s*)?(?:(?:task|part)\s*\d+\s*[:.)-]?\s*|\d+[.)]\s+))`)
)

func parseStructuredPlan(raw string) (structuredPlan, error) {
	payload := strings.TrimSpace(raw)
	if strings.HasPrefix(payload, "```") {
		payload = strings.TrimPrefix(payload, "```json")
		payload = strings.TrimPrefix(payload, "```")
		payload = strings.TrimSpace(strings.TrimSuffix(payload, "```"))
	}

	plan, err := parseStructuredPlanJSON(payload)
	if err == nil {
		return plan, nil
	}
	fallback, ferr := parseStructuredPlanJSONLike(payload)
	if ferr == nil {
		return fallback, nil
	}
	prose, perr := parseStructuredPlanText(payload)
	if perr == nil {
		return prose, nil
	}
	return structuredPlan{}, fmt.Errorf("%v; %v; %v", err, ferr, perr)
}

func parseStructuredPlanJSON(payload string) (structuredPlan, error) {
	start := strings.Index(payload, "{")
	end := strings.LastIndex(payload, "}")
	if start < 0 || end < start {
		return structuredPlan{}, fmt.Errorf("no json object found")
	}
	payload = payload[start : end+1]

	var plan structuredPlan
	if err := json.Unmarshal([]byte(payload), &plan); err != nil {
		return structuredPlan{}, fmt.Errorf("invalid structured plan json: %w", err)
	}
	return cleanStructuredPlan(plan)
}

func parseStructuredPlanJSONLike(payload string) (structuredPlan, error) {
	plan := structuredPlan{}
	if summaries := extractStructuredPlanKeyValues(payload, "summary"); len(summaries) > 0 {
		plan.Summary = summaries[0]
	}
	titles := extractStructuredPlanKeyValues(payload, "title")
	if len(titles) == 0 {
		return structuredPlan{}, fmt.Errorf("no task titles found in json-like planner output")
	}
	kinds := extractStructuredPlanKeyValues(payload, "kind")
	notes := extractStructuredPlanKeyValues(payload, "notes")
	if len(kinds) > 0 && len(titles) > len(kinds) {
		titles = titles[:len(kinds)]
	}
	for i, title := range titles {
		kind := ""
		if i < len(kinds) {
			kind = kinds[i]
		}
		note := ""
		if i < len(notes) {
			note = notes[i]
		}
		plan.Tasks = append(plan.Tasks, structuredPlanTask{
			Title: title,
			Kind:  strings.TrimSpace(kind),
			Notes: note,
		})
	}
	return cleanStructuredPlan(plan)
}

func parseStructuredPlanText(payload string) (structuredPlan, error) {
	lines := strings.Split(normalizeStructuredPlanText(payload), "\n")
	var (
		plan         structuredPlan
		summaryLines []string
		noteLines    []string
		currentTitle string
		sawTask      bool
	)

	flushTask := func() {
		if strings.TrimSpace(currentTitle) == "" {
			return
		}
		kind, strippedNotes := extractStructuredTaskKind(noteLines)
		plan.Tasks = append(plan.Tasks, structuredPlanTask{
			Title: strings.TrimSpace(currentTitle),
			Kind:  kind,
			Notes: strings.TrimSpace(strings.Join(trimBlankEdges(strippedNotes), "\n")),
		})
		noteLines = nil
	}

	for _, line := range lines {
		if title, ok := parseStructuredPlanHeading(line); ok {
			flushTask()
			currentTitle = title
			sawTask = true
			noteLines = nil
			continue
		}
		trimmed := strings.TrimSpace(line)
		if !sawTask {
			if strings.EqualFold(trimmed, "tasks:") {
				continue
			}
			summaryLines = append(summaryLines, line)
			continue
		}
		noteLines = append(noteLines, line)
	}
	flushTask()
	plan.Summary = strings.TrimSpace(strings.Join(trimBlankEdges(summaryLines), "\n"))
	if len(plan.Tasks) == 0 {
		return structuredPlan{}, fmt.Errorf("no task headings found in planner text")
	}
	return cleanStructuredPlan(plan)
}

func cleanStructuredPlan(plan structuredPlan) (structuredPlan, error) {
	filtered := plan.Tasks[:0]
	for _, task := range plan.Tasks {
		task.Title = strings.TrimSpace(task.Title)
		task.Kind = normalizeStructuredTaskKind(task.Kind)
		task.Notes = strings.TrimSpace(task.Notes)
		if task.Title == "" {
			continue
		}
		if task.Kind == "" {
			return structuredPlan{}, fmt.Errorf("task %q is missing required kind", task.Title)
		}
		if _, criteria := splitAcceptanceCriteria(task.Notes); len(criteria) == 0 {
			return structuredPlan{}, fmt.Errorf("task %q is missing required acceptance criteria", task.Title)
		}
		filtered = append(filtered, task)
	}
	plan.Tasks = filtered
	if len(plan.Tasks) == 0 {
		return structuredPlan{}, fmt.Errorf("structured plan contains no tasks")
	}
	return plan, nil
}

func normalizeStructuredTaskKind(kind string) string {
	switch strings.TrimSpace(strings.ToLower(kind)) {
	case tool.TaskKindAnalysis:
		return tool.TaskKindAnalysis
	case tool.TaskKindReport:
		return tool.TaskKindReport
	case tool.TaskKindImplementation:
		return tool.TaskKindImplementation
	default:
		return ""
	}
}

func extractStructuredTaskKind(lines []string) (string, []string) {
	if len(lines) == 0 {
		return "", lines
	}
	out := make([]string, 0, len(lines))
	kind := ""
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			out = append(out, line)
			continue
		}
		lower := strings.ToLower(trimmed)
		switch {
		case strings.HasPrefix(lower, "kind:"):
			if parsed := normalizeStructuredTaskKind(strings.TrimSpace(trimmed[len("kind:"):])); parsed != "" {
				kind = parsed
				continue
			}
		case strings.HasPrefix(lower, "kind="):
			if parsed := normalizeStructuredTaskKind(strings.TrimSpace(trimmed[len("kind="):])); parsed != "" {
				kind = parsed
				continue
			}
		}
		out = append(out, line)
	}
	return kind, out
}

func normalizeStructuredPlanText(payload string) string {
	payload = strings.ReplaceAll(payload, "\r\n", "\n")
	payload = structuredPlanInlineLabelRE.ReplaceAllString(payload, "$1\n$2")
	payload = structuredPlanInlineAfterPunc.ReplaceAllString(payload, "$1\n$2")
	return payload
}

func extractStructuredPlanKeyValues(payload, key string) []string {
	re := regexp.MustCompile(fmt.Sprintf(`(?is)["']?%s["']?\s*:\s*(?:"((?:\\.|[^"\\])*)"|'((?:\\.|[^'\\])*)'|([^,\n}\]]+))`, regexp.QuoteMeta(key)))
	matches := re.FindAllStringSubmatch(payload, -1)
	values := make([]string, 0, len(matches))
	for _, match := range matches {
		if len(match) != 4 {
			continue
		}
		for i := 1; i < len(match); i++ {
			if strings.TrimSpace(match[i]) == "" {
				continue
			}
			values = append(values, strings.TrimSpace(jsonUnescape(match[i])))
			break
		}
	}
	return values
}

func jsonUnescape(s string) string {
	if unquoted, err := strconv.Unquote(`"` + s + `"`); err == nil {
		return unquoted
	}
	return s
}

func parseStructuredPlanHeading(line string) (string, bool) {
	match := structuredPlanHeadingRE.FindStringSubmatch(line)
	if len(match) != 2 {
		return "", false
	}
	title := strings.TrimSpace(match[1])
	if title == "" {
		return "", false
	}
	return title, true
}

func trimBlankEdges(lines []string) []string {
	start := 0
	for start < len(lines) && strings.TrimSpace(lines[start]) == "" {
		start++
	}
	end := len(lines)
	for end > start && strings.TrimSpace(lines[end-1]) == "" {
		end--
	}
	return lines[start:end]
}
