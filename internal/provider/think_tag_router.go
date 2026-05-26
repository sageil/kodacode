package provider

import (
	"fmt"
	"strings"
)

// thinkTagRouter normalizes providers that leak <think> markup through the
// assistant content stream instead of sending reasoning over a dedicated field.
type thinkTagRouter struct {
	reasoningMode          streamReasoningMode
	segmentPrefix          string
	reasoningSegmentID     string
	reasoningSegmentSerial int
	inThinkTag             bool
	contentRemainder       string
	emitAssistant          func(string)
	emitReasoning          func(string, string)
}

func newThinkTagRouter(
	reasoningMode streamReasoningMode,
	segmentPrefix string,
	emitAssistant func(string),
	emitReasoning func(string, string),
) thinkTagRouter {
	return thinkTagRouter{
		reasoningMode: reasoningMode,
		segmentPrefix: segmentPrefix,
		emitAssistant: emitAssistant,
		emitReasoning: emitReasoning,
	}
}

func (r *thinkTagRouter) resetReasoningSegment() {
	r.reasoningSegmentID = ""
}

func (r *thinkTagRouter) ensureReasoningSegmentID() string {
	if r.reasoningSegmentID != "" {
		return r.reasoningSegmentID
	}
	r.reasoningSegmentSerial++
	r.reasoningSegmentID = fmt.Sprintf("%s_%d", r.segmentPrefix, r.reasoningSegmentSerial)
	return r.reasoningSegmentID
}

func (r *thinkTagRouter) appendContentDelta(content string) {
	if content == "" {
		return
	}
	input := r.contentRemainder + content
	r.contentRemainder = ""
	for len(input) > 0 {
		if r.inThinkTag {
			if idx := strings.Index(input, "</think>"); idx >= 0 {
				r.emitReasoningChunk(input[:idx])
				input = input[idx+len("</think>"):]
				r.inThinkTag = false
				r.resetReasoningSegment()
				continue
			}
			keep := trailingTagPrefixLen(input, "</think>")
			r.emitReasoningChunk(input[:len(input)-keep])
			r.contentRemainder = input[len(input)-keep:]
			return
		}

		openIdx := strings.Index(input, "<think>")
		closeIdx := strings.Index(input, "</think>")
		switch {
		case closeIdx >= 0 && (openIdx < 0 || closeIdx < openIdx):
			r.emitAssistantChunk(input[:closeIdx])
			input = input[closeIdx+len("</think>"):]
			r.resetReasoningSegment()
			continue
		case openIdx >= 0:
			r.emitAssistantChunk(input[:openIdx])
			input = input[openIdx+len("<think>"):]
			r.inThinkTag = true
			continue
		default:
			keep := max(
				trailingTagPrefixLen(input, "<think>"),
				trailingTagPrefixLen(input, "</think>"),
			)
			r.emitAssistantChunk(input[:len(input)-keep])
			r.contentRemainder = input[len(input)-keep:]
			return
		}
	}
}

func (r *thinkTagRouter) emitAssistantChunk(content string) {
	if content == "" {
		return
	}
	r.resetReasoningSegment()
	if r.emitAssistant != nil {
		r.emitAssistant(content)
	}
}

func (r *thinkTagRouter) emitReasoningChunk(content string) {
	if content == "" {
		return
	}
	switch r.reasoningMode {
	case streamReasoningHidden:
		if r.emitReasoning != nil {
			r.emitReasoning(r.ensureReasoningSegmentID(), content)
		}
	default:
		return
	}
}

func trailingTagPrefixLen(content, tag string) int {
	maxKeep := min(len(content), len(tag)-1)
	for keep := maxKeep; keep > 0; keep-- {
		if strings.HasSuffix(content, tag[:keep]) {
			return keep
		}
	}
	return 0
}
