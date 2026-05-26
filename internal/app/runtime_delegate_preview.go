package app

import (
	"context"
	"strings"
	"time"

	"github.com/sageil/kodacode/internal/events"
)

const (
	delegatedPreviewPublishInterval = 100 * time.Millisecond
	delegatedPreviewMaxSnippetRunes = 120
	delegatedPreviewBufferRunes     = 2048
)

type delegatedHandoffPreviewObserver struct {
	runtime         *Runtime
	handoff         events.AgentHandoffPayload
	assistantBuffer string
	active          bool
	toolName        string
	action          string
	assistantText   string
	lastPublished   events.AgentHandoffPreviewPayload
	lastPublishedAt time.Time
}

func (r *Runtime) startDelegatedHandoffPreview(ctx context.Context, handoff events.AgentHandoffPayload, initialAction string) (func(), error) {
	childState, err := r.Sessions.Snapshot(ctx, handoff.ChildSessionID)
	if err != nil {
		return nil, err
	}

	watchCtx, cancel := context.WithCancel(ctx)
	stream, err := r.Sessions.Watch(watchCtx, handoff.ChildSessionID, childState.LastSequence)
	if err != nil {
		cancel()
		return nil, err
	}

	observer := &delegatedHandoffPreviewObserver{
		runtime: r,
		handoff: handoff,
		active:  true,
		action:  strings.TrimSpace(initialAction),
	}
	if err := observer.publish(true); err != nil {
		cancel()
		return nil, err
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		observer.run(stream)
	}()

	return func() {
		cancel()
		<-done
	}, nil
}

func (o *delegatedHandoffPreviewObserver) run(stream <-chan events.Event) {
	for event := range stream {
		if strings.TrimSpace(event.TurnID) != strings.TrimSpace(o.handoff.ChildTurnID) {
			continue
		}
		o.handleEvent(event)
	}
}

func (o *delegatedHandoffPreviewObserver) handleEvent(event events.Event) {
	switch payload := event.Payload.(type) {
	case events.AssistantPreviewResetPayload:
		o.assistantBuffer = ""
		o.assistantText = ""
		if strings.TrimSpace(o.toolName) == "" {
			o.action = "drafting response"
		}
		o.publishNow()
	case events.AssistantPreviewDeltaPayload:
		o.appendAssistantPreview(payload.Content)
		if strings.TrimSpace(o.toolName) == "" {
			o.action = "drafting response"
		}
		o.publishAssistantPreview()
	case events.AssistantCommitPayload:
		o.assistantBuffer = strings.TrimSpace(payload.Content)
		o.assistantText = delegatedPreviewSnippet(o.assistantBuffer)
		if strings.TrimSpace(o.toolName) == "" {
			o.action = "finalizing response"
		}
		o.publishNow()
	case events.ToolCallDeclaredPayload:
		o.toolName = strings.TrimSpace(payload.ToolName)
		o.action = "preparing " + o.toolName
		o.publishNow()
	case events.ToolExecStartPayload:
		o.toolName = strings.TrimSpace(payload.ToolName)
		o.action = "running " + o.toolName
		o.publishNow()
	case events.ToolExecEndPayload:
		o.toolName = ""
		if payload.Successful() {
			o.action = "processing " + strings.TrimSpace(payload.ToolName) + " result"
		} else {
			o.action = "handling " + strings.TrimSpace(payload.ToolName) + " error"
		}
		o.publishNow()
	case events.PermissionRequestedPayload:
		o.toolName = strings.TrimSpace(payload.ToolName)
		o.action = "awaiting permission for " + o.toolName
		o.publishNow()
	case events.ExecutionApprovalRequestedPayload:
		o.toolName = strings.TrimSpace(payload.ToolName)
		o.action = "awaiting permission for " + o.toolName
		o.publishNow()
	case events.QuestionRequestedPayload:
		o.toolName = strings.TrimSpace(payload.ToolName)
		if o.toolName == "" {
			o.action = "awaiting user input"
		} else {
			o.action = "awaiting input for " + o.toolName
		}
		o.publishNow()
	case events.AgentHandoffPayload:
		childAgent := strings.TrimSpace(payload.ChildAgentID)
		if childAgent == "" {
			childAgent = "subagent"
		}
		o.toolName = ""
		o.action = "delegating to " + childAgent
		o.publishNow()
	}
}

func (o *delegatedHandoffPreviewObserver) appendAssistantPreview(content string) {
	if strings.TrimSpace(content) == "" {
		return
	}
	o.assistantBuffer += content
	o.assistantBuffer = delegatedPreviewTail(o.assistantBuffer, delegatedPreviewBufferRunes)
	o.assistantText = delegatedPreviewSnippet(o.assistantBuffer)
}

func (o *delegatedHandoffPreviewObserver) publishAssistantPreview() {
	if o == nil {
		return
	}
	if o.lastPublished.AssistantText == "" {
		o.publishNow()
		return
	}
	if time.Since(o.lastPublishedAt) >= delegatedPreviewPublishInterval {
		o.publishNow()
		return
	}
	if previewHasBoundary(o.assistantText) {
		o.publishNow()
		return
	}
	if previewRuneDelta(o.lastPublished.AssistantText, o.assistantText) >= 24 {
		o.publishNow()
	}
}

func (o *delegatedHandoffPreviewObserver) publishNow() {
	if o == nil {
		return
	}
	if err := o.publish(false); err != nil {
		o.runtime.log("runtime").Debug("delegated handoff preview publish skipped",
			"handoff_id", o.handoff.HandoffID,
			"error", err.Error(),
		)
	}
}

func (o *delegatedHandoffPreviewObserver) publish(force bool) error {
	if o == nil || o.runtime == nil {
		return nil
	}
	payload := events.AgentHandoffPreviewPayload{
		HandoffID:      o.handoff.HandoffID,
		ChildSessionID: o.handoff.ChildSessionID,
		ChildTurnID:    o.handoff.ChildTurnID,
		Active:         o.active,
		ToolName:       strings.TrimSpace(o.toolName),
		Action:         strings.TrimSpace(o.action),
		AssistantText:  strings.TrimSpace(o.assistantText),
	}
	if !force && payload == o.lastPublished {
		return nil
	}
	if err := o.runtime.Sessions.publishEphemeral(o.handoff.ParentSessionID, o.handoff.ParentTurnID, events.TypeAgentHandoffPreview, payload); err != nil {
		return err
	}
	o.lastPublished = payload
	o.lastPublishedAt = time.Now()
	return nil
}

func delegatedPreviewSnippet(raw string) string {
	line := strings.Join(strings.Fields(strings.TrimSpace(raw)), " ")
	if line == "" {
		return ""
	}
	runes := []rune(line)
	if len(runes) <= delegatedPreviewMaxSnippetRunes {
		return line
	}
	return "..." + string(runes[len(runes)-delegatedPreviewMaxSnippetRunes+3:])
}

func delegatedPreviewTail(raw string, limit int) string {
	if limit <= 0 {
		return ""
	}
	runes := []rune(raw)
	if len(runes) <= limit {
		return raw
	}
	return string(runes[len(runes)-limit:])
}

func previewHasBoundary(text string) bool {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return false
	}
	switch trimmed[len(trimmed)-1] {
	case '.', '!', '?', ';', ':':
		return true
	default:
		return false
	}
}

func previewRuneDelta(previous, current string) int {
	prev := len([]rune(strings.TrimSpace(previous)))
	cur := len([]rune(strings.TrimSpace(current)))
	if cur <= prev {
		return 0
	}
	return cur - prev
}
