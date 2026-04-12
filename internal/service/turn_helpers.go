package service

import (
	"encoding/json"
	"log"
	"strings"

	"github.com/sageil/kodacode/v1/internal/message"
	"github.com/sageil/kodacode/v1/internal/provider"
	"github.com/sageil/kodacode/v1/internal/repository"
)

func (tl *turnLoop) persistAssistantMessage(text string, reasoning []message.ReasoningContent, toolCalls []provider.ToolCall) {
	if tl.msgs == nil || tl.req.Ephemeral {
		return
	}
	if text == "" && len(reasoning) == 0 && len(toolCalls) == 0 {
		return
	}
	var parts []repository.MessagePart
	if text != "" {
		content, _ := message.MarshalContent(message.TextContent{Text: text})
		parts = append(parts, repository.MessagePart{
			SessionID: tl.req.SessionID,
			Type:      "text", Content: content,
		})
	}
	for _, rc := range reasoning {
		content, _ := message.MarshalContent(rc)
		parts = append(parts, repository.MessagePart{
			SessionID: tl.req.SessionID,
			Type:      "reasoning", Content: content,
		})
	}
	for _, tc := range toolCalls {
		content, _ := message.MarshalContent(message.ToolCallContent{
			ID: tc.ID, Name: tc.Name, Arguments: tc.Arguments,
			ThoughtSignature: tc.ThoughtSignature,
		})
		parts = append(parts, repository.MessagePart{
			SessionID: tl.req.SessionID,
			Type:      "tool_call", Content: content,
		})
	}
	if _, err := tl.msgs.CreateWithParts(tl.ctx, repository.Message{
		SessionID: tl.req.SessionID,
		Role:      "assistant",
	}, parts); err != nil {
		log.Printf("middleware_llm: persist assistant message: %v", err)
		tl.warnPersist("Session history not saved")
	}
}

func (tl *turnLoop) persistTextMessage(role, text string, synthetic bool) {
	if tl.msgs == nil || tl.req.Ephemeral || text == "" {
		return
	}
	content, _ := message.MarshalContent(message.TextContent{Text: text})
	if _, err := tl.msgs.CreateWithParts(tl.ctx, repository.Message{
		SessionID: tl.req.SessionID,
		Role:      role,
	}, []repository.MessagePart{{
		SessionID: tl.req.SessionID,
		Type:      "text",
		Content:   content,
		Synthetic: synthetic,
	}}); err != nil {
		log.Printf("middleware_llm: persist %s message: %v", role, err)
		tl.warnPersist("Session history not saved")
	}
}

func (tl *turnLoop) persistToolResults(executions []toolExecution) {
	if tl.msgs == nil || tl.req.Ephemeral {
		return
	}
	var parts []repository.MessagePart
	for _, ex := range executions {
		trc := message.ToolResultContent{
			ToolCallID: ex.call.ID,
			Output:     ex.output,
			Error:      ex.errStr,
		}
		if ex.result != nil {
			trc.ErrorCode = ex.result.ErrorCode
			trc.Metadata = ex.result.Metadata
		}
		content, _ := message.MarshalContent(trc)
		parts = append(parts, repository.MessagePart{
			SessionID: tl.req.SessionID,
			Type:      "tool_result", Content: content,
		})
	}
	if len(parts) == 0 {
		return
	}
	if _, err := tl.msgs.CreateWithParts(tl.ctx, repository.Message{
		SessionID: tl.req.SessionID,
		Role:      "user",
	}, parts); err != nil {
		log.Printf("middleware_llm: persist tool result message: %v", err)
		tl.warnPersist("Tool results not saved")
	}
}

func (tl *turnLoop) warnPersist(msg string) {
	if tl.publish != nil {
		tl.publish(tl.req.SessionID, SSEEvent{
			Type: "warning",
			Data: map[string]string{"message": msg},
		})
	}
}

func truncateForLog(s string, max int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) > max {
		return s[:max] + "..."
	}
	return s
}

func extractFilePath(argsJSON string) string {
	var m struct {
		FilePath  string `json:"filePath"`
		FilePath2 string `json:"file_path"`
	}
	if json.Unmarshal([]byte(argsJSON), &m) == nil {
		if m.FilePath != "" {
			return m.FilePath
		}
		return m.FilePath2
	}
	return ""
}
