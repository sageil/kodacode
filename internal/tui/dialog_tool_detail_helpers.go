package tui

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/sageil/kodacode/internal/events"
)

func toolDetailArgumentsBlock(call *events.ToolCallState) (string, string, bool) {
	if call == nil {
		return "", "", false
	}
	raw := strings.TrimSpace(call.Input)
	if raw == "" {
		return "", "", false
	}
	if pretty, ok := toolDetailPrettyJSON(raw); ok {
		return "json", pretty, true
	}
	return "text", raw, true
}

func toolDetailPrettyJSON(raw string) (string, bool) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || !json.Valid([]byte(trimmed)) {
		return "", false
	}
	var buf bytes.Buffer
	if err := json.Indent(&buf, []byte(trimmed), "", "  "); err != nil {
		return "", false
	}
	return buf.String(), true
}

func toolDetailCommandText(call *events.ToolCallState) string {
	switch {
	case call == nil:
		return ""
	case isBashToolCall(call):
		if input, ok := parseBashToolViewInput(call.Input); ok {
			return strings.TrimSpace(input.Command)
		}
	case isTestToolCall(call):
		if call.Execution != nil && strings.TrimSpace(call.Execution.CommandPreview) != "" {
			return strings.TrimSpace(call.Execution.CommandPreview)
		}
		if input, ok := parseTestToolViewInput(call.Input); ok {
			return strings.TrimSpace(input.Command)
		}
	}
	return ""
}

func toolDetailCommandWorkingDirectory(call *events.ToolCallState) string {
	if call == nil {
		return ""
	}
	if input, ok := parseBashToolViewInput(call.Input); ok {
		return strings.TrimSpace(input.WorkingDirectory)
	}
	return ""
}

func toolDetailOutputLanguage(call *events.ToolCallState, body string) string {
	if strings.TrimSpace(body) == "" {
		return "text"
	}
	if toolDetailLooksLikeJSON(body) {
		return "json"
	}
	if path := toolDetailPrimaryPath(call); path != "" {
		if lang := toolDetailLanguageForPath(path); lang != "" {
			return lang
		}
	}
	return "text"
}

func toolDetailPrimaryPath(call *events.ToolCallState) string {
	if call == nil {
		return ""
	}
	switch strings.TrimSpace(call.ToolName) {
	case "read":
		if input, ok := parseReadToolViewInput(call.Input); ok && len(input.Paths) == 1 {
			return input.Paths[0]
		}
	case "write", "edit", "mkdir":
		if params := toolInspectorParams(call); len(params) > 0 {
			for _, param := range params {
				if strings.EqualFold(strings.TrimSpace(param.Label), "Path") && strings.TrimSpace(param.Value) != "" {
					return strings.TrimSpace(param.Value)
				}
			}
		}
	}
	return ""
}

func toolDetailLanguageForPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	base := strings.ToLower(filepath.Base(path))
	switch base {
	case "dockerfile":
		return "dockerfile"
	case "makefile":
		return "makefile"
	}

	switch strings.ToLower(filepath.Ext(path)) {
	case ".go":
		return "go"
	case ".ts":
		return "typescript"
	case ".tsx":
		return "tsx"
	case ".vue":
		return "vue"
	case ".js":
		return "javascript"
	case ".jsx":
		return "jsx"
	case ".json":
		return "json"
	case ".yaml", ".yml":
		return "yaml"
	case ".md":
		return "markdown"
	case ".sh", ".bash", ".zsh":
		return "bash"
	case ".css":
		return "css"
	case ".scss":
		return "scss"
	case ".html":
		return "html"
	case ".xml":
		return "xml"
	case ".sql":
		return "sql"
	case ".py":
		return "python"
	case ".rb":
		return "ruby"
	case ".rs":
		return "rust"
	case ".java":
		return "java"
	case ".toml":
		return "toml"
	case ".proto":
		return "protobuf"
	default:
		return ""
	}
}

func toolDetailLooksLikeJSON(body string) bool {
	trimmed := strings.TrimSpace(body)
	if trimmed == "" {
		return false
	}
	if !strings.HasPrefix(trimmed, "{") && !strings.HasPrefix(trimmed, "[") {
		return false
	}
	return json.Valid([]byte(trimmed))
}

func fencedToolDetailBlock(language, body string) string {
	body = strings.TrimRight(body, "\n")
	fence := markdownFence(body)
	language = strings.TrimSpace(language)
	if language == "" {
		return fence + "\n" + body + "\n" + fence
	}
	return fence + language + "\n" + body + "\n" + fence
}

func toolDetailErrorBlock(body string) string {
	return fencedToolDetailBlock("text-wrap", body)
}

func markdownFence(body string) string {
	maxRun := 0
	current := 0
	for _, r := range body {
		if r == '`' {
			current++
			if current > maxRun {
				maxRun = current
			}
			continue
		}
		current = 0
	}
	return strings.Repeat("`", max(maxRun+1, 3))
}

func parseQuestionToolViewInput(raw string) (struct {
	Question string   `json:"question"`
	Options  []string `json:"options"`
	Purpose  string   `json:"purpose"`
}, bool) {
	var input struct {
		Question string   `json:"question"`
		Options  []string `json:"options"`
		Purpose  string   `json:"purpose"`
	}
	if json.Unmarshal([]byte(raw), &input) != nil || strings.TrimSpace(input.Question) == "" {
		return input, false
	}
	options := make([]string, 0, len(input.Options))
	for _, option := range input.Options {
		if trimmed := strings.TrimSpace(option); trimmed != "" {
			options = append(options, trimmed)
		}
	}
	input.Options = options
	input.Question = strings.TrimSpace(input.Question)
	input.Purpose = strings.TrimSpace(input.Purpose)
	return input, true
}

func questionToolAnswer(state events.SessionState, ref sessionToolCallRef, call *events.ToolCallState) string {
	for _, answer := range state.QuestionAnswers {
		if answer == nil {
			continue
		}
		if answer.TurnID == ref.TurnID && answer.ToolCallID == ref.CallID && strings.TrimSpace(answer.Answer) != "" {
			return strings.TrimSpace(answer.Answer)
		}
	}
	var output struct {
		Answer string `json:"answer"`
	}
	if json.Unmarshal([]byte(call.Output), &output) == nil && strings.TrimSpace(output.Answer) != "" {
		return strings.TrimSpace(output.Answer)
	}
	for _, pending := range state.PendingQuestions {
		if pending == nil {
			continue
		}
		if pending.TurnID == ref.TurnID && pending.ToolCallID == ref.CallID {
			if pending.Multiple {
				return "Awaiting answers."
			}
			return "Awaiting answer."
		}
	}
	if call.Executing || !call.Completed {
		return "Awaiting answer."
	}
	return ""
}

func questionToolPrompt(call *events.ToolCallState) string {
	if call == nil {
		return ""
	}
	input, ok := parseQuestionToolViewInput(call.Input)
	if !ok {
		return ""
	}
	return strings.TrimSpace(input.Question)
}

func questionToolText(state events.SessionState, ref sessionToolCallRef, call *events.ToolCallState) string {
	for _, answer := range state.QuestionAnswers {
		if answer == nil {
			continue
		}
		if answer.TurnID == ref.TurnID && answer.ToolCallID == ref.CallID && strings.TrimSpace(answer.Question) != "" {
			return strings.TrimSpace(answer.Question)
		}
	}
	for _, pending := range state.PendingQuestions {
		if pending == nil {
			continue
		}
		if pending.TurnID == ref.TurnID && pending.ToolCallID == ref.CallID && strings.TrimSpace(pending.Question) != "" {
			return strings.TrimSpace(pending.Question)
		}
	}
	return questionToolPrompt(call)
}

func questionToolDisplayName(call *events.ToolCallState) string {
	input, ok := parseQuestionToolViewInput(call.Input)
	if !ok {
		return "question"
	}
	if len(input.Options) == 0 {
		return "question"
	}
	return "question · " + strconv.Itoa(len(input.Options)) + " options"
}
