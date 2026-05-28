package tui

import (
	"encoding/json"
	"regexp"
	"strings"

	"github.com/sageil/kodacode/internal/events"
)

type taskToolViewInput struct {
	Action        string `json:"action"`
	TaskID        string `json:"task_id"`
	Title         string `json:"title"`
	Kind          string `json:"kind"`
	Status        string `json:"status"`
	Notes         string `json:"notes"`
	Progress      string `json:"progress"`
	BlockReason   string `json:"block_reason"`
	Summary       string `json:"summary"`
	ReviewStatus  string `json:"review_status"`
	ReviewSummary string `json:"review_summary"`
}

type taskToolViewRecord struct {
	TaskID        string `json:"task_id"`
	Title         string `json:"title"`
	Kind          string `json:"kind"`
	Status        string `json:"status"`
	Notes         string `json:"notes"`
	Progress      string `json:"progress"`
	BlockReason   string `json:"block_reason"`
	ReviewStatus  string `json:"review_status"`
	ReviewSummary string `json:"review_summary"`
}

type taskToolListOutput struct {
	Tasks []taskToolViewRecord `json:"tasks"`
}

type taskToolErrorInfo struct {
	Action  string
	Reason  string
	Hint    string
	Detail  string
	HasTask bool
}

var (
	taskToolQuotedFieldPattern         = regexp.MustCompile(`"([a-z_]+)"\s*:\s*"([^"]*)"`)
	taskToolLikelyUnquotedValuePattern = regexp.MustCompile(`"(action|task_id|title|kind|status|notes|progress|block_reason|summary|review_status|review_summary)"\s*:\s*([^"\[{0-9tfn-][^,}\]]*)`)
)

func normalizeTaskToolViewRecord(record *taskToolViewRecord) {
	record.TaskID = strings.TrimSpace(record.TaskID)
	record.Title = strings.TrimSpace(record.Title)
	record.Kind = strings.TrimSpace(record.Kind)
	record.Status = strings.TrimSpace(record.Status)
	record.Notes = strings.TrimSpace(record.Notes)
	record.Progress = strings.TrimSpace(record.Progress)
	record.BlockReason = strings.TrimSpace(record.BlockReason)
	record.ReviewStatus = strings.TrimSpace(record.ReviewStatus)
	record.ReviewSummary = strings.TrimSpace(record.ReviewSummary)
}

func isTaskToolCall(call *events.ToolCallState) bool {
	if call == nil {
		return false
	}
	switch strings.TrimSpace(call.ToolName) {
	case "task", "task_workflow", "task_review":
		return true
	default:
		return false
	}
}

func parseTaskToolViewInput(raw string) (taskToolViewInput, bool) {
	var input taskToolViewInput
	if json.Unmarshal([]byte(raw), &input) != nil {
		return input, false
	}
	input.Action = strings.TrimSpace(input.Action)
	if input.Action == "" {
		return input, false
	}
	input.TaskID = strings.TrimSpace(input.TaskID)
	input.Title = strings.TrimSpace(input.Title)
	return input, true
}

func parseTaskToolViewOutput(raw string) (taskToolViewRecord, bool) {
	var output struct {
		Task taskToolViewRecord `json:"task"`
	}
	if json.Unmarshal([]byte(raw), &output) != nil {
		return taskToolViewRecord{}, false
	}
	normalizeTaskToolViewRecord(&output.Task)
	if output.Task.TaskID == "" &&
		output.Task.Title == "" &&
		output.Task.Kind == "" &&
		output.Task.Status == "" &&
		output.Task.Notes == "" &&
		output.Task.Progress == "" &&
		output.Task.BlockReason == "" &&
		output.Task.ReviewStatus == "" &&
		output.Task.ReviewSummary == "" {
		return taskToolViewRecord{}, false
	}
	return output.Task, true
}

func parseTaskToolViewListOutput(raw string) ([]taskToolViewRecord, bool) {
	var envelope map[string]json.RawMessage
	if json.Unmarshal([]byte(raw), &envelope) != nil {
		return nil, false
	}
	body, ok := envelope["tasks"]
	if !ok {
		return nil, false
	}
	var output taskToolListOutput
	output.Tasks = []taskToolViewRecord{}
	if len(body) > 0 && string(body) != "null" {
		if json.Unmarshal(body, &output.Tasks) != nil {
			return nil, false
		}
	}
	for idx := range output.Tasks {
		normalizeTaskToolViewRecord(&output.Tasks[idx])
	}
	return output.Tasks, true
}

func taskToolAction(call *events.ToolCallState) string {
	if call == nil {
		return ""
	}
	if input, ok := parseTaskToolViewInput(call.Input); ok {
		return strings.TrimSpace(input.Action)
	}
	return strings.TrimSpace(taskToolActionHint(call.Input))
}

func isTaskToolListCall(call *events.ToolCallState) bool {
	return strings.EqualFold(taskToolAction(call), "list")
}

func taskToolQuotedField(raw, field string) string {
	raw = strings.TrimSpace(raw)
	field = strings.TrimSpace(field)
	if raw == "" || field == "" {
		return ""
	}
	for _, match := range taskToolQuotedFieldPattern.FindAllStringSubmatch(raw, -1) {
		if len(match) != 3 || strings.TrimSpace(match[1]) != field {
			continue
		}
		return strings.TrimSpace(match[2])
	}
	return ""
}

func taskToolActionHint(raw string) string {
	return taskToolQuotedField(raw, "action")
}

func taskToolLooksBatched(raw string) bool {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return false
	}
	return strings.HasPrefix(raw, "[") ||
		strings.Count(raw, `"action"`) > 1 ||
		strings.Count(raw, `"title"`) > 1
}

func taskToolLikelyHasUnquotedStringValue(raw string) bool {
	return taskToolLikelyUnquotedValuePattern.MatchString(strings.TrimSpace(raw))
}

func taskToolUnderlyingErrorDetail(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	idx := strings.LastIndex(raw, "Details:")
	if idx >= 0 {
		detail := strings.TrimSpace(raw[idx+len("Details:"):])
		return strings.TrimSuffix(detail, ".")
	}
	if idx := strings.Index(strings.ToLower(raw), " failed."); idx >= 0 {
		detail := strings.TrimSpace(raw[idx+len(" failed."):])
		if cut := strings.Index(detail, " Example:"); cut >= 0 {
			detail = strings.TrimSpace(detail[:cut])
		}
		return strings.TrimSuffix(detail, ".")
	}
	idx = strings.Index(raw, ":")
	if idx < 0 {
		return ""
	}
	detail := strings.TrimSpace(raw[idx+1:])
	if cut := strings.Index(detail, " Example:"); cut >= 0 {
		detail = strings.TrimSpace(detail[:cut])
	}
	return strings.TrimSuffix(detail, ".")
}

func taskToolSummaryReason(info taskToolErrorInfo) string {
	reason := strings.TrimSpace(info.Reason)
	if reason == "" {
		return ""
	}
	if info.HasTask {
		return reason
	}
	action := strings.TrimSpace(info.Action)
	if action == "" {
		return reason
	}
	return action + " · " + reason
}

func taskToolDisplayReason(info taskToolErrorInfo) string {
	reason := strings.TrimSpace(info.Reason)
	if reason == "" {
		return ""
	}
	action := strings.TrimSpace(info.Action)
	if action == "" {
		return reason
	}
	return action + " call " + reason
}

func taskToolErrorInfoFromText(call *events.ToolCallState, rawError string) (taskToolErrorInfo, bool) {
	rawError = strings.TrimSpace(rawError)
	if call == nil || rawError == "" {
		return taskToolErrorInfo{}, false
	}

	record := resolveTaskToolViewRecord(call)
	info := taskToolErrorInfo{
		Action:  taskToolActionHint(call.Input),
		Detail:  taskToolUnderlyingErrorDetail(rawError),
		HasTask: record.Title != "" || record.TaskID != "",
	}
	lowerError := strings.ToLower(rawError)
	lowerDetail := strings.ToLower(info.Detail)

	switch {
	case strings.Contains(lowerError, "malformed json"),
		strings.Contains(lowerDetail, "json ended before the object was complete"),
		strings.Contains(lowerDetail, "invalid character"),
		strings.Contains(lowerDetail, "unquoted string value"):
		info.Reason = "malformed JSON"
		switch {
		case taskToolLooksBatched(call.Input):
			info.Hint = "send one JSON object for one task"
		case taskToolLikelyHasUnquotedStringValue(call.Input) ||
			strings.Contains(lowerDetail, "invalid character") ||
			strings.Contains(lowerDetail, "unexpected end of json input"):
			info.Hint = "quote every property name and every string value"
		default:
			info.Hint = "send one JSON object with only this action's fields"
		}
	case strings.Contains(lowerDetail, "value must be an object"):
		info.Reason = "expected one JSON object"
		info.Hint = "send one task per call, not an array or prose"
	case strings.Contains(lowerDetail, "title is required"):
		if info.Action == "" {
			info.Action = "create"
		}
		info.Reason = "missing title"
		info.Hint = `include "title":"..."`
	case strings.Contains(lowerDetail, "task_id is required"):
		if info.Action == "" {
			info.Action = "update"
		}
		info.Reason = "missing task id"
		info.Hint = `include "task_id":"task-..."`
	case strings.Contains(lowerDetail, "status, progress, or notes is required"):
		if info.Action == "" {
			info.Action = "update"
		}
		info.Reason = "missing status, progress, or notes"
		info.Hint = `include at least one of "status", "progress", or "notes"`
	case strings.Contains(lowerDetail, "block_reason is required"):
		if info.Action == "" {
			info.Action = "block"
		}
		info.Reason = "missing block reason"
		info.Hint = `include "block_reason":"..."`
	case strings.Contains(lowerDetail, "review_summary is required"):
		if info.Action == "" {
			info.Action = "review"
		}
		info.Reason = "missing review summary"
		info.Hint = `include "review_summary":"..."`
	case strings.Contains(lowerDetail, "review_status must be"):
		if info.Action == "" {
			info.Action = "review"
		}
		info.Reason = "invalid review status"
		info.Hint = "use pass, concern, fail, or accepted"
	case strings.Contains(lowerDetail, "status must be pending or in_progress"):
		if info.Action == "" {
			info.Action = "create/update"
		}
		info.Reason = "invalid status"
		info.Hint = "use pending or in_progress"
	case strings.Contains(lowerDetail, "action must be list, create, update, block, or complete"):
		info.Reason = "invalid workflow action"
		info.Hint = "use list, create, update, block, or complete"
	case strings.Contains(lowerDetail, "action must be list or review"):
		info.Reason = "invalid review action"
		info.Hint = "use list or review"
	case strings.Contains(lowerError, "invalid for the selected action"),
		(strings.Contains(lowerError, " failed.") && strings.Contains(rawError, "Example:")):
		info.Reason = "invalid arguments"
		info.Hint = "send only the fields for this action"
	default:
		return taskToolErrorInfo{}, false
	}
	return info, true
}

func taskToolErrorSummary(call *events.ToolCallState, rawError string) string {
	info, ok := taskToolErrorInfoFromText(call, rawError)
	if !ok {
		return summarizeInlineValue(rawError)
	}
	summary := taskToolSummaryReason(info)
	hint := strings.TrimSpace(info.Hint)
	if hint == "" {
		return summary
	}
	switch {
	case strings.HasPrefix(hint, "quote every"):
		return summary + " · quote string values"
	case strings.HasPrefix(hint, "send one JSON object for one task"),
		strings.HasPrefix(hint, "send one task per call"):
		return summary + " · one task per call"
	default:
		return summary
	}
}

func taskToolErrorDisplayText(call *events.ToolCallState, rawError string) string {
	info, ok := taskToolErrorInfoFromText(call, rawError)
	if !ok {
		return summarizeBlock(rawError)
	}
	lines := make([]string, 0, 3)
	if headline := strings.TrimSpace(taskToolDisplayReason(info)); headline != "" {
		lines = append(lines, headline)
	}
	if hint := strings.TrimSpace(info.Hint); hint != "" {
		lines = append(lines, hint)
	}
	if detail := strings.TrimSpace(info.Detail); detail != "" {
		lines = append(lines, "detail: "+detail)
	}
	return strings.Join(lines, "\n")
}

func renderTaskToolOutputMarkdown(raw string) (string, bool) {
	if tasks, ok := parseTaskToolViewListOutput(raw); ok {
		lines := []string{"## Output"}
		if len(tasks) == 0 {
			lines = append(lines, "", "- No tasks")
			return strings.Join(lines, "\n"), true
		}
		lines = append(lines, "", "**Tasks:**")
		for _, task := range tasks {
			lines = append(lines, "- "+taskToolListItemMarkdownLabel(task))
		}
		return strings.Join(lines, "\n"), true
	}
	record, ok := parseTaskToolViewOutput(raw)
	if !ok {
		return "", false
	}
	lines := []string{"## Output"}
	appendTaskToolMarkdownField(&lines, "Task", taskToolDisplayTitle(record))
	appendTaskToolMarkdownField(&lines, "Task ID", record.TaskID)
	appendTaskToolMarkdownField(&lines, "Kind", record.Kind)
	appendTaskToolMarkdownField(&lines, "Status", record.Status)
	appendTaskToolMarkdownField(&lines, "Progress", record.Progress)
	appendTaskToolMarkdownField(&lines, "Block Reason", record.BlockReason)
	appendTaskToolMarkdownField(&lines, "Review", record.ReviewStatus)
	appendTaskToolMarkdownField(&lines, "Review Summary", record.ReviewSummary)
	appendTaskToolMarkdownField(&lines, "Notes", record.Notes)

	return strings.Join(lines, "\n"), true
}

func renderTaskToolInputTranscript(call *events.ToolCallState, width int) (string, bool) {
	input, ok := parseTaskToolViewInput(call.Input)
	if !ok {
		return "", false
	}
	record := resolveTaskToolViewRecord(call)
	lines := make([]string, 0, 10)
	appendTaskToolTranscriptField(&lines, width, "Action", input.Action)
	appendTaskToolTranscriptField(&lines, width, "Task", taskToolDisplayTitle(record))
	appendTaskToolTranscriptField(&lines, width, "Task ID", record.TaskID)
	appendTaskToolTranscriptField(&lines, width, "Kind", record.Kind)
	appendTaskToolTranscriptField(&lines, width, "Status", record.Status)
	appendTaskToolTranscriptField(&lines, width, "Progress", record.Progress)
	appendTaskToolTranscriptField(&lines, width, "Summary", input.Summary)
	appendTaskToolTranscriptField(&lines, width, "Block Reason", record.BlockReason)
	appendTaskToolTranscriptField(&lines, width, "Review", record.ReviewStatus)
	appendTaskToolTranscriptField(&lines, width, "Review Summary", record.ReviewSummary)
	appendTaskToolTranscriptField(&lines, width, "Notes", record.Notes)
	return strings.Join(lines, "\n"), len(lines) > 0
}

func renderTaskToolOutputTranscript(raw string, width int) (string, bool) {
	if tasks, ok := parseTaskToolViewListOutput(raw); ok {
		if len(tasks) == 0 {
			return "Tasks: none", true
		}
		lines := []string{"Tasks:"}
		for _, task := range tasks {
			lines = append(lines, "  "+taskToolListItemTranscriptLabel(task, width-2))
		}
		return strings.Join(lines, "\n"), true
	}
	record, ok := parseTaskToolViewOutput(raw)
	if !ok {
		return "", false
	}
	lines := make([]string, 0, 9)
	appendTaskToolTranscriptField(&lines, width, "Task", taskToolDisplayTitle(record))
	appendTaskToolTranscriptField(&lines, width, "Task ID", record.TaskID)
	appendTaskToolTranscriptField(&lines, width, "Kind", record.Kind)
	appendTaskToolTranscriptField(&lines, width, "Status", record.Status)
	appendTaskToolTranscriptField(&lines, width, "Progress", record.Progress)
	appendTaskToolTranscriptField(&lines, width, "Block Reason", record.BlockReason)
	appendTaskToolTranscriptField(&lines, width, "Review", record.ReviewStatus)
	appendTaskToolTranscriptField(&lines, width, "Review Summary", record.ReviewSummary)
	appendTaskToolTranscriptField(&lines, width, "Notes", record.Notes)
	return strings.Join(lines, "\n"), len(lines) > 0
}

func appendTaskToolMarkdownField(lines *[]string, label, value string) {
	value = strings.TrimSpace(value)
	if value == "" {
		return
	}
	if strings.Contains(value, "\n") {
		*lines = append(*lines, "", "**"+label+":**", "", value)
		return
	}
	*lines = append(*lines, "", "- "+label+": "+value)
}

func appendTaskToolTranscriptField(lines *[]string, width int, label, value string) {
	value = strings.TrimSpace(value)
	if value == "" {
		return
	}
	prefix := strings.TrimSpace(label) + ": "
	wrapWidth := max(width-len(prefix), 24)
	wrapped := wrapStructuredText(value, wrapWidth)
	if len(wrapped) == 0 {
		return
	}
	*lines = append(*lines, prefix+wrapped[0])
	for _, line := range wrapped[1:] {
		*lines = append(*lines, strings.Repeat(" ", len(prefix))+line)
	}
}

func taskToolDisplayTitle(record taskToolViewRecord) string {
	if title := strings.TrimSpace(record.Title); title != "" {
		return title
	}
	return strings.TrimSpace(record.TaskID)
}

func taskToolListItemMarkdownLabel(record taskToolViewRecord) string {
	label := taskToolListItemLabel(record)
	if status := strings.TrimSpace(record.Status); status != "" {
		label += " · " + status
	}
	return label
}

func taskToolListItemTranscriptLabel(record taskToolViewRecord, width int) string {
	return truncateEnd(taskToolListItemMarkdownLabel(record), max(width, 16))
}

func resolveTaskToolViewRecord(call *events.ToolCallState) taskToolViewRecord {
	var record taskToolViewRecord
	if call == nil {
		return record
	}
	if output, ok := parseTaskToolViewOutput(call.Output); ok {
		record = output
	}
	input, ok := parseTaskToolViewInput(call.Input)
	if !ok {
		if record.TaskID == "" {
			record.TaskID = taskToolQuotedField(call.Input, "task_id")
		}
		if record.Title == "" {
			record.Title = taskToolQuotedField(call.Input, "title")
		}
		if record.Kind == "" {
			record.Kind = taskToolQuotedField(call.Input, "kind")
		}
		if record.Status == "" {
			record.Status = taskToolQuotedField(call.Input, "status")
		}
		if record.Notes == "" {
			record.Notes = taskToolQuotedField(call.Input, "notes")
		}
		if record.Progress == "" {
			record.Progress = taskToolQuotedField(call.Input, "progress")
		}
		if record.BlockReason == "" {
			record.BlockReason = taskToolQuotedField(call.Input, "block_reason")
		}
		if record.ReviewStatus == "" {
			record.ReviewStatus = taskToolQuotedField(call.Input, "review_status")
		}
		if record.ReviewSummary == "" {
			record.ReviewSummary = taskToolQuotedField(call.Input, "review_summary")
		}
		normalizeTaskToolViewRecord(&record)
		return record
	}
	if record.TaskID == "" {
		record.TaskID = input.TaskID
	}
	if record.Title == "" {
		record.Title = input.Title
	}
	if record.Kind == "" {
		record.Kind = strings.TrimSpace(input.Kind)
	}
	if record.Status == "" {
		record.Status = strings.TrimSpace(input.Status)
	}
	if record.Notes == "" {
		record.Notes = strings.TrimSpace(input.Notes)
	}
	if record.Progress == "" {
		record.Progress = strings.TrimSpace(input.Progress)
	}
	if record.Progress == "" {
		record.Progress = strings.TrimSpace(input.Summary)
	}
	if record.BlockReason == "" {
		record.BlockReason = strings.TrimSpace(input.BlockReason)
	}
	if record.ReviewStatus == "" {
		record.ReviewStatus = strings.TrimSpace(input.ReviewStatus)
	}
	if record.ReviewSummary == "" {
		record.ReviewSummary = strings.TrimSpace(input.ReviewSummary)
	}
	return record
}

func taskToolDisplayName(call *events.ToolCallState) string {
	if call == nil {
		return "Task"
	}
	if isTaskToolListCall(call) {
		return "List Tasks"
	}
	record := resolveTaskToolViewRecord(call)
	if record.Title != "" {
		return "Task: " + record.Title
	}
	if record.TaskID != "" {
		return "Task: " + record.TaskID
	}
	return "Task"
}

func taskToolListSummary(call *events.ToolCallState) string {
	action := taskToolAction(call)
	if action == "" {
		return ""
	}
	if action == "list" {
		tasks, ok := parseTaskToolViewListOutput(call.Output)
		if !ok {
			return "list tasks"
		}
		if len(tasks) == 0 {
			return "no tasks"
		}
		return pluralize(len(tasks), "task")
	}
	record := resolveTaskToolViewRecord(call)
	lines := []string{"action: " + action}
	if record.TaskID != "" {
		lines = append(lines, "task: "+record.TaskID)
	}
	if record.Status != "" {
		lines = append(lines, "status: "+record.Status)
	}
	if record.Progress != "" {
		lines = append(lines, "progress: "+record.Progress)
	}
	if record.BlockReason != "" {
		lines = append(lines, "block: "+record.BlockReason)
	}
	if record.ReviewStatus != "" {
		lines = append(lines, "review: "+record.ReviewStatus)
	}
	return strings.Join(lines, "\n")
}

func taskToolListItemLabel(record taskToolViewRecord) string {
	parts := make([]string, 0, 2)
	if strings.TrimSpace(record.TaskID) != "" {
		parts = append(parts, strings.TrimSpace(record.TaskID))
	}
	if strings.TrimSpace(record.Title) != "" {
		parts = append(parts, strings.TrimSpace(record.Title))
	}
	if len(parts) == 0 {
		return "task"
	}
	return strings.Join(parts, " · ")
}

func taskToolListItemStatus(record taskToolViewRecord) string {
	switch strings.TrimSpace(record.Status) {
	case events.TaskStatusCompleted:
		return "done"
	case events.TaskStatusBlocked:
		return "blocked"
	case events.TaskStatusInProgress:
		return "running"
	default:
		return "waiting"
	}
}

func taskInspectorParams(call *events.ToolCallState) []inspectorParam {
	action := ""
	if input, ok := parseTaskToolViewInput(call.Input); ok {
		action = input.Action
	} else {
		action = taskToolActionHint(call.Input)
	}
	if action == "" {
		return defaultInspectorParams(call)
	}
	record := resolveTaskToolViewRecord(call)
	params := []inspectorParam{{Label: "Action", Value: action}}
	if record.TaskID != "" {
		params = append(params, inspectorParam{Label: "Task", Value: record.TaskID})
	}
	if record.Title != "" {
		params = append(params, inspectorParam{Label: "Title", Value: record.Title})
	}
	if summary := strings.TrimSpace(taskToolListSummary(call)); summary != "" {
		params = append(params, inspectorParam{Label: "Details", Value: summary})
	}
	if errorText := strings.TrimSpace(taskToolErrorDisplayText(call, call.Error)); errorText != "" {
		params = append(params, inspectorParam{Label: "Error", Value: errorText, Error: true})
	}
	return params
}
