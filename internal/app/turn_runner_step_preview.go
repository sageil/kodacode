package app

import (
	"context"
	"strings"

	"github.com/sageil/kodacode/internal/provider"
	"github.com/sageil/kodacode/internal/tool"
)

type stepAssistantPreview struct {
	r                   *TurnRunner
	ctx                 context.Context
	sessionID           string
	turnID              string
	state               *turnLoopState
	segment             *string
	hasToolCalls        *bool
	trimOverlap         bool
	markDurableProgress func()
	commitStepState     func()
}

type stepAssistantPreviewInput struct {
	Runner              *TurnRunner
	Context             context.Context
	SessionID           string
	TurnID              string
	State               *turnLoopState
	Segment             *string
	HasToolCalls        *bool
	TrimOverlap         bool
	MarkDurableProgress func()
	CommitStepState     func()
}

func newStepAssistantPreview(input stepAssistantPreviewInput) stepAssistantPreview {
	return stepAssistantPreview{
		r:                   input.Runner,
		ctx:                 input.Context,
		sessionID:           input.SessionID,
		turnID:              input.TurnID,
		state:               input.State,
		segment:             input.Segment,
		hasToolCalls:        input.HasToolCalls,
		trimOverlap:         input.TrimOverlap,
		markDurableProgress: input.MarkDurableProgress,
		commitStepState:     input.CommitStepState,
	}
}

func (p stepAssistantPreview) CommitAssistant() error {
	if p.segment == nil || *p.segment == "" {
		return nil
	}
	segment := *p.segment
	if p.trimOverlap {
		segment = trimAssistantContinuationOverlap(p.state.AssistantText, segment)
	}
	nextAssistantText := p.state.AssistantText + segment
	if strings.TrimSpace(nextAssistantText) == "" {
		*p.segment = ""
		return nil
	}
	if err := p.r.appendAssistantCommit(p.ctx, p.sessionID, p.turnID, nextAssistantText); err != nil {
		return err
	}
	p.state.AssistantText = nextAssistantText
	if len(p.state.Conversation) > 0 && p.state.Conversation[len(p.state.Conversation)-1].Kind == provider.InputKindAssistantMessage {
		p.state.Conversation[len(p.state.Conversation)-1].Content += segment
	} else {
		p.state.Conversation = append(p.state.Conversation, provider.Input{
			Kind:    provider.InputKindAssistantMessage,
			Content: segment,
		})
	}
	p.r.logAssistantCommit(p.sessionID, p.turnID, "assistant", segment)
	*p.segment = ""
	if p.markDurableProgress != nil {
		p.markDurableProgress()
	}
	if p.commitStepState != nil {
		p.commitStepState()
	}
	return nil
}

func trimAssistantContinuationOverlap(existing, segment string) string {
	if existing == "" || segment == "" {
		return segment
	}
	maxOverlap := min(len(existing), len(segment), 2048)
	for overlap := maxOverlap; overlap > 0; overlap-- {
		if strings.HasSuffix(existing, segment[:overlap]) {
			return segment[overlap:]
		}
	}
	return segment
}

func (p stepAssistantPreview) CommitWorklog() error {
	if p.segment == nil || strings.TrimSpace(*p.segment) == "" {
		return nil
	}
	p.r.logAssistantCommit(p.sessionID, p.turnID, "worklog", *p.segment)
	return p.r.appendAssistantWorklogCommit(p.ctx, p.sessionID, p.turnID, *p.segment)
}

func (p stepAssistantPreview) Discard() error {
	if p.segment == nil || *p.segment == "" {
		return nil
	}
	if err := p.CommitWorklog(); err != nil {
		return err
	}
	if p.markDurableProgress != nil {
		p.markDurableProgress()
	}
	*p.segment = ""
	return p.r.appendAssistantPreviewReset(p.sessionID, p.turnID)
}

func (p stepAssistantPreview) StartToolStep(toolName string) error {
	if p.hasToolCalls == nil {
		return nil
	}
	if *p.hasToolCalls {
		return nil
	}
	*p.hasToolCalls = true
	if strings.TrimSpace(toolName) == tool.QuestionToolName {
		return nil
	}
	return p.Discard()
}
