package tui

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

type backgroundEventPayload struct {
	TaskID         string `json:"task_id"`
	Command        string `json:"command"`
	Description    string `json:"description,omitempty"`
	ExitCode       int    `json:"exit_code"`
	Output         string `json:"output"`
	ElapsedMS      int64  `json:"elapsed_ms"`
	Status         string `json:"status"`
	AutoReactState string `json:"auto_react_state,omitempty"`
	LastError      string `json:"last_error,omitempty"`
}

func parseBackgroundEvent(raw string) (backgroundEventPayload, error) {
	var p backgroundEventPayload
	err := json.Unmarshal([]byte(raw), &p)
	return p, err
}

func formatBackgroundEventMessage(p backgroundEventPayload) string {
	title := "Background task completed"
	if p.Status == "failed" || p.ExitCode != 0 {
		title = fmt.Sprintf("Background task failed (exit %d)", p.ExitCode)
	}

	subject := strings.TrimSpace(p.Description)
	if subject == "" {
		subject = strings.TrimSpace(p.Command)
	}

	var sb strings.Builder
	sb.WriteString(title)
	if subject != "" {
		sb.WriteString(": ")
		sb.WriteString(subject)
	}
	sb.WriteString("\n")

	if p.Command != "" && p.Command != subject {
		sb.WriteString("Command: `")
		sb.WriteString(p.Command)
		sb.WriteString("`\n")
	}
	if p.TaskID != "" {
		sb.WriteString("Task: `")
		sb.WriteString(p.TaskID)
		sb.WriteString("`\n")
	}
	if p.ElapsedMS > 0 {
		sb.WriteString("Duration: ")
		sb.WriteString((time.Duration(p.ElapsedMS) * time.Millisecond).Round(time.Millisecond).String())
		sb.WriteString("\n")
	}
	if state := formatBackgroundAutoReactState(p.AutoReactState, p.LastError); state != "" {
		sb.WriteString("Auto review: ")
		sb.WriteString(state)
		sb.WriteString("\n")
	}
	if excerpt := strings.TrimSpace(p.Output); excerpt != "" {
		sb.WriteString("\nOutput excerpt:\n```\n")
		sb.WriteString(excerpt)
		if !strings.HasSuffix(excerpt, "\n") {
			sb.WriteString("\n")
		}
		sb.WriteString("```")
	}

	return strings.TrimSpace(sb.String())
}

func formatBackgroundAutoReactState(state, lastError string) string {
	switch state {
	case "queued":
		return "queued"
	case "reviewing":
		return "in progress"
	case "reviewed":
		return "completed"
	case "failed":
		if lastError != "" {
			return "failed: " + lastError
		}
		return "failed"
	case "recorded", "":
		return ""
	default:
		return state
	}
}
