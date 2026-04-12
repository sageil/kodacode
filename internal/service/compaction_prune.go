package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/sageil/kodacode/v1/internal/config"
	"github.com/sageil/kodacode/v1/internal/message"
	"github.com/sageil/kodacode/v1/internal/pipeline"
	"github.com/sageil/kodacode/v1/internal/repository"
)

// pruneToolOutputs loads messages and parts from DB (2 queries), then prunes.
// Used by post-turn cleanup where we don't have a pre-loaded snapshot.
func pruneToolOutputs(
	ctx context.Context,
	cfg *config.SessionConfig,
	msgs repository.MessageRepo,
	isReadOnly func(string) bool,
	req *pipeline.TurnRequest,
) error {
	allMsgs, err := msgs.ListBySession(ctx, req.SessionID)
	if err != nil {
		return err
	}
	allParts, err := msgs.ListPartsBySession(ctx, req.SessionID)
	if err != nil {
		return err
	}
	// Populate Parts on messages for buildProtectedMessageSet → isUserTurnStart.
	byMsg := groupPartsByMessage(allParts)
	for i := range allMsgs {
		allMsgs[i].Parts = byMsg[allMsgs[i].ID]
	}
	_, err = pruneToolOutputsFromSnapshot(ctx, cfg, msgs, isReadOnly, req, allMsgs, allParts)
	return err
}

// pruneToolOutputsFromSnapshot prunes old read-only tool outputs using
// pre-loaded messages and parts to avoid redundant DB queries.
// Returns a map of part ID → pruned part so callers can apply the changes
// to their in-memory snapshot without a DB reload.
func pruneToolOutputsFromSnapshot(
	ctx context.Context,
	cfg *config.SessionConfig,
	msgs repository.MessageRepo,
	isReadOnly func(string) bool,
	req *pipeline.TurnRequest,
	allMsgs []repository.Message,
	allParts []repository.MessagePart,
) (map[string]repository.MessagePart, error) {
	contextSize := req.Model.EffectiveContextSize()
	if contextSize <= 0 {
		contextSize = 128000
	}
	cc := resolveCompactionConfig(cfg, req.ProviderID, req.Model.ID, contextSize)

	toolNames := buildToolCallNameMap(allParts)
	protectedMessages := buildProtectedMessageSet(allMsgs, cc.keepTurns)

	accumulated := 0
	var toPrune []repository.MessagePart
	for i := len(allParts) - 1; i >= 0; i-- {
		p := allParts[i]
		tokens := (len(p.Content) + 3) / 4
		beyondTokenWindow := accumulated >= cc.pruneProtect
		beyondTurnWindow := !protectedMessages[p.MessageID]
		accumulated += tokens

		if !beyondTokenWindow || !beyondTurnWindow {
			continue
		}
		if p.Type == "tool_result" && p.CompactedAt == nil {
			callID := extractToolCallID(p.Content)
			toolName := toolNames[callID]
			if toolName == "" || isReadOnly == nil || !isReadOnly(toolName) {
				continue
			}
			toPrune = append(toPrune, p)
		}
	}
	savings := 0
	for _, p := range toPrune {
		savings += (len(p.Content) + 3) / 4
	}
	if savings < cc.pruneMinSavings {
		return nil, nil
	}
	now := time.Now().UTC()
	pruned := make(map[string]repository.MessagePart, len(toPrune))
	for i := range toPrune {
		p := &toPrune[i]
		callID := extractToolCallID(p.Content)
		toolName := toolNames[callID]
		summary := buildPruneSummary(toolName, p.Content)
		content, err := message.MarshalContent(message.ToolResultContent{
			ToolCallID: callID,
			Output:     summary,
		})
		if err != nil {
			return nil, fmt.Errorf("marshal pruned content: %w", err)
		}
		p.Content = content
		p.CompactedAt = &now
		pruned[p.ID] = *p
	}
	if err := msgs.BatchUpdateParts(ctx, toPrune); err != nil {
		return nil, fmt.Errorf("batch prune parts: %w", err)
	}
	return pruned, nil
}

func buildProtectedMessageSet(msgs []repository.Message, keepTurns int) map[string]bool {
	protected := make(map[string]bool, len(msgs))
	if len(msgs) == 0 || keepTurns <= 0 {
		return protected
	}
	start := 0
	turns := 0
	for i := len(msgs) - 1; i >= 0; i-- {
		if isUserTurnStart(msgs[i]) {
			turns++
			start = i
			if turns >= keepTurns {
				break
			}
		}
	}
	for i := start; i < len(msgs); i++ {
		protected[msgs[i].ID] = true
	}
	return protected
}

func isUserTurnStart(msg repository.Message) bool {
	if msg.Summary || msg.Role != "user" {
		return false
	}
	for _, part := range msg.Parts {
		switch part.Type {
		case "file":
			return true
		case "text":
			c, err := message.UnmarshalContent("text", part.Content)
			if err != nil {
				continue
			}
			tc, ok := c.(message.TextContent)
			if ok && strings.TrimSpace(tc.Text) != "" {
				return true
			}
		}
	}
	return false
}

func buildToolCallNameMap(parts []repository.MessagePart) map[string]string {
	m := make(map[string]string)
	for _, p := range parts {
		if p.Type == "tool_call" {
			c, err := message.UnmarshalContent("tool_call", p.Content)
			if err != nil {
				continue
			}
			if tc, ok := c.(message.ToolCallContent); ok {
				m[tc.ID] = tc.Name
			}
		}
	}
	return m
}

func buildPruneSummary(toolName, content string) string {
	c, err := message.UnmarshalContent("tool_result", content)
	if err != nil {
		return "[output pruned]"
	}
	tr, ok := c.(message.ToolResultContent)
	if !ok || tr.Output == "" {
		return "[output pruned]"
	}

	output := tr.Output
	lineCount := strings.Count(output, "\n") + 1
	charCount := len(output)

	switch toolName {
	case "read":
		return fmt.Sprintf("[pruned: %d lines, %d chars of file content]", lineCount, charCount)
	case "bash":
		return fmt.Sprintf("[pruned: %d lines of command output]", lineCount)
	case "glob":
		return fmt.Sprintf("[pruned: %d file matches]", lineCount)
	case "grep":
		return fmt.Sprintf("[pruned: %d lines of search results]", lineCount)
	case "write":
		return fmt.Sprintf("[pruned: file write confirmed, %d chars]", charCount)
	default:
		return fmt.Sprintf("[pruned: %d lines, %d chars of %s output]", lineCount, charCount, toolName)
	}
}
