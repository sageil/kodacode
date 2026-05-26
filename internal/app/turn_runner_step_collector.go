package app

import (
	"strings"

	"github.com/sageil/kodacode/internal/provider"
)

type stepToolCallCollector struct {
	calls                         map[string]*toolCallAccumulator
	captureOpenAIReasoningContent bool
	openAIReasoningContent        strings.Builder
	openAIReasoningAttached       bool
}

func newStepToolCallCollector(captureOpenAIReasoningContent bool) *stepToolCallCollector {
	return &stepToolCallCollector{
		calls:                         make(map[string]*toolCallAccumulator),
		captureOpenAIReasoningContent: captureOpenAIReasoningContent,
	}
}

func (c *stepToolCallCollector) appendOpenAIReasoningDelta(delta string) {
	if c == nil || !c.captureOpenAIReasoningContent {
		return
	}
	c.openAIReasoningContent.WriteString(delta)
}

func (c *stepToolCallCollector) appendToolCallDelta(event provider.Event) {
	if c == nil {
		return
	}
	call := ensureToolCallAccumulator(c.calls, event.ToolCallID, event.ToolName)
	call.ToolName = event.ToolName
	call.ToolKind = event.ToolKind
	call.Input.WriteString(event.InputDelta)
}

func (c *stepToolCallCollector) completeToolCall(event provider.Event) stepToolCall {
	if c == nil {
		return stepToolCall{}
	}
	call := ensureToolCallAccumulator(c.calls, event.ToolCallID, event.ToolName)
	call.ToolName = event.ToolName
	if event.ToolKind != "" {
		call.ToolKind = event.ToolKind
	}
	toolKind := normalizeStepToolKind(call.ToolKind)
	arguments := call.Input.String()
	if toolKind != provider.ToolKindCustom {
		arguments = strings.TrimSpace(arguments)
	}
	if arguments == "" && toolKind != provider.ToolKindCustom {
		arguments = "{}"
	}
	openAIReasoningContent := ""
	if c.captureOpenAIReasoningContent && !c.openAIReasoningAttached {
		openAIReasoningContent = c.openAIReasoningContent.String()
		c.openAIReasoningAttached = true
	}
	return stepToolCall{
		CallID:                 event.ToolCallID,
		ToolName:               event.ToolName,
		ToolKind:               toolKind,
		Arguments:              arguments,
		GoogleThoughtSignature: append([]byte(nil), event.GoogleThoughtSignature...),
		OpenAIReasoningContent: openAIReasoningContent,
	}
}
