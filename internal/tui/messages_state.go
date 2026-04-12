package tui

import (
	"strings"
	"time"

	"github.com/sageil/kodacode/v1/internal/logging"
)

func (m *Messages) SetMessages(msgs []Message) {
	m.messages = msgs
	m.invalidateFrom(0)
	m.needsRender = true
}

// SetSearch activates or deactivates search highlighting.
// A non-empty query enables highlighting; an empty query clears it.
func (m *Messages) SetSearch(query string) {
	if query == m.searchQuery {
		return
	}
	m.searchQuery = query
	m.searchActive = query != ""
	m.invalidateFrom(0)
	m.needsRender = true
}

func (m *Messages) AppendReasoningDelta(delta string) {
	last := len(m.messages) - 1
	if last >= 0 && m.messages[last].Streaming && !m.messages[last].ReasoningDone {
		m.messages[last].Reasoning += delta
		logging.Debugf("[7-messages] AppendReasoningDelta: appended %d chars to msg[%d], total reasoning=%d", len(delta), last, len(m.messages[last].Reasoning))
		m.invalidateFrom(last)
	} else {
		logging.Debugf("[7-messages] AppendReasoningDelta: new assistant msg, last=%d streaming=%v reasoningDone=%v", last, last >= 0 && m.messages[last].Streaming, last >= 0 && m.messages[last].ReasoningDone)
		m.messages = append(m.messages, Message{
			Role: "assistant", Reasoning: delta, Streaming: true, Timestamp: time.Now(),
			ReasoningStartTime: time.Now(),
		})
		m.invalidateFrom(len(m.messages) - 1)
	}
	if !m.userScrolled {
		m.autoScroll = true
	}
	m.needsRender = true
}

func (m *Messages) FinishReasoning() {
	// Search backwards for the last assistant message with active reasoning.
	// The last message may be a tool_call if reasoning_done arrives after
	// tool_start, so we can't assume m.messages[last] is the target.
	for i := len(m.messages) - 1; i >= 0; i-- {
		msg := &m.messages[i]
		if msg.Role == "assistant" && msg.Streaming && msg.Reasoning != "" && !msg.ReasoningDone {
			msg.ReasoningDone = true
			msg.ReasoningCollapsed = true
			if !msg.ReasoningStartTime.IsZero() {
				msg.ReasoningElapsed = time.Since(msg.ReasoningStartTime)
			}
			m.invalidateFrom(i)
			break
		}
	}
	m.needsRender = true
}

func (m *Messages) AppendDelta(delta string) {
	if len(m.messages) > 0 && m.messages[len(m.messages)-1].Streaming {
		m.messages[len(m.messages)-1].Content += delta
		m.invalidateFrom(len(m.messages) - 1)
	} else {
		m.messages = append(m.messages, Message{Role: "assistant", Content: delta, Streaming: true, Timestamp: time.Now()})
		m.invalidateFrom(len(m.messages) - 1)
	}
	if !m.userScrolled {
		m.autoScroll = true
	}
	m.needsRender = true
}

// AppendUserMessage appends a completed (non-streaming) user message.
// Resets userScrolled so the viewport follows the new response.
func (m *Messages) AppendUserMessage(text string) {
	m.messages = append(m.messages, Message{Role: "user", Content: text, Timestamp: time.Now()})
	m.invalidateFrom(len(m.messages) - 1)
	m.userScrolled = false
	m.autoScroll = true
	m.needsRender = true
	m.lastRender = time.Time{} // bypass throttle for user messages
}

func (m *Messages) AppendAssistantMessage(text string) {
	m.messages = append(m.messages, Message{Role: "assistant", Content: text, Timestamp: time.Now()})
	m.invalidateFrom(len(m.messages) - 1)
	if !m.userScrolled {
		m.autoScroll = true
	}
	m.needsRender = true
	m.lastRender = time.Time{} // bypass throttle for completed assistant messages
}

func (m *Messages) FinishStreaming() {
	for i := range m.messages {
		if m.messages[i].Streaming {
			m.messages[i].Streaming = false
			m.messages[i].ReasoningDone = true
			if m.messages[i].Reasoning != "" {
				m.messages[i].ReasoningCollapsed = true
				if m.messages[i].ReasoningElapsed == 0 && !m.messages[i].ReasoningStartTime.IsZero() {
					m.messages[i].ReasoningElapsed = time.Since(m.messages[i].ReasoningStartTime)
				}
			}
			m.invalidateFrom(i)
		}
		if m.messages[i].Role == "tool_call" && !m.messages[i].ToolDone {
			m.messages[i].ToolDone = true
			m.messages[i].ToolError = "interrupted"
			m.messages[i].ToolElapsed = time.Since(m.messages[i].ToolStartTime)
			m.invalidateFrom(i)
		}
	}
	if !m.userScrolled {
		m.autoScroll = true
	}
	m.needsRender = true
	m.lastRender = time.Time{} // bypass throttle
}

// AppendToolStart adds a running tool-call block, or updates the input of the
// last undone block for the same tool.
func (m *Messages) AppendToolStart(toolName, input, callID string) {
	if input != "" {
		for i := len(m.messages) - 1; i >= 0; i-- {
			if m.messages[i].Role == "tool_call" && !m.messages[i].ToolDone && matchTool(m.messages[i], toolName, callID) {
				m.messages[i].ToolInput = input
				m.invalidateFrom(i)
				if !m.userScrolled {
					m.autoScroll = true
				}
				m.needsRender = true
				return
			}
		}
	}
	for i := range m.messages {
		if m.messages[i].Role == "tool_call" && m.messages[i].ToolDone && !m.messages[i].Collapsed && !m.messages[i].UserExpanded && shouldAutoCollapse(m.messages[i]) {
			m.messages[i].Collapsed = true
		}
	}
	m.messages = append(m.messages, Message{
		Role:          "tool_call",
		ToolCallID:    callID,
		ToolName:      toolName,
		ToolInput:     input,
		ToolDone:      false,
		ToolStartTime: time.Now(),
		Collapsed:     isReadOnlyToolCall(toolName, input),
	})
	m.invalidateFrom(0)
	if !m.userScrolled {
		m.autoScroll = true
	}
	m.needsRender = true
}

// TrimToTurns drops old messages, keeping only the last `n` user turns
// and their responses. A turn starts at a user message and includes all
// subsequent assistant/tool_call/system messages until the next user message.
// If n <= 0, no trimming is performed.
func (m *Messages) TrimToTurns(n int) {
	if n <= 0 {
		return
	}
	var userIndices []int
	for i, msg := range m.messages {
		if msg.Role == "user" {
			userIndices = append(userIndices, i)
		}
	}
	if len(userIndices) <= n {
		return // already within limit
	}
	cutAt := userIndices[len(userIndices)-n]
	for cutAt > 0 && m.messages[cutAt-1].Role == "system" {
		cutAt--
	}
	m.messages = m.messages[cutAt:]
	m.invalidateFrom(0)
	m.needsRender = true
}

// AppendBackgroundTaskDone adds a pre-completed tool-call block for a finished
// background bash task, rendered identically to a normal tool completion.
func (m *Messages) AppendBackgroundTaskDone(input, output, toolErr string, elapsed time.Duration) {
	now := time.Now()
	m.messages = append(m.messages, Message{
		Role:        "tool_call",
		ToolName:    "bash",
		ToolInput:   input,
		ToolOutput:  output,
		ToolError:   toolErr,
		ToolDone:    true,
		ToolElapsed: elapsed,
		ToolEndTime: now,
		Collapsed:   true,
	})
	m.invalidateFrom(len(m.messages) - 1)
	if !m.userScrolled {
		m.autoScroll = true
	}
	m.needsRender = true
}

func (m *Messages) AppendSystemMessage(content string) {
	m.messages = append(m.messages, Message{
		Role:    "system",
		Content: content,
	})
	m.invalidateFrom(len(m.messages) - 1)
	if !m.userScrolled {
		m.autoScroll = true
	}
	m.needsRender = true
}

func (m *Messages) AppendErrorMessage(content string) {
	m.messages = append(m.messages, Message{
		Role:    "error",
		Content: content,
	})
	m.invalidateFrom(len(m.messages) - 1)
	if !m.userScrolled {
		m.autoScroll = true
	}
	m.needsRender = true
}

// UpdateToolInputDelta appends streaming input to the last matching tool-call block.
func (m *Messages) UpdateToolInputDelta(toolName, delta, callID string) {
	for i := len(m.messages) - 1; i >= 0; i-- {
		if m.messages[i].Role == "tool_call" && !m.messages[i].ToolDone && matchTool(m.messages[i], toolName, callID) {
			m.messages[i].ToolInput += delta
			m.invalidateFrom(i)
			if !m.userScrolled {
				m.autoScroll = true
			}
			m.needsRender = true
			return
		}
	}
}

// UpdateToolOutput appends a streaming output chunk to the last matching
// (still running) tool-call block.
func (m *Messages) UpdateToolOutput(toolName, chunk, callID string) {
	for i := len(m.messages) - 1; i >= 0; i-- {
		if m.messages[i].Role == "tool_call" && !m.messages[i].ToolDone && matchTool(m.messages[i], toolName, callID) {
			if isBinaryContent(chunk) || isMediaOutput(chunk) || isMediaOutput(m.messages[i].ToolOutput) {
				m.messages[i].ToolOutput = "[binary content]"
			} else {
				m.messages[i].ToolOutput += chunk
				if lines := strings.Split(m.messages[i].ToolOutput, "\n"); len(lines) > maxStreamingLines {
					m.messages[i].ToolOutput = strings.Join(lines[len(lines)-maxStreamingLines:], "\n")
				}
			}
			m.invalidateFrom(i)
			if !m.userScrolled {
				m.autoScroll = true
			}
			m.needsRender = true
			return
		}
	}
}

// UpdateToolEnd marks the last matching tool-call block as done with output.
// toolErr is non-empty when the tool call failed or was denied.
func (m *Messages) UpdateToolEnd(toolName, output, toolErr, callID string) {
	for i := len(m.messages) - 1; i >= 0; i-- {
		if m.messages[i].Role == "tool_call" && !m.messages[i].ToolDone && matchTool(m.messages[i], toolName, callID) {
			m.messages[i].ToolElapsed = time.Since(m.messages[i].ToolStartTime)
			if isBinaryContent(output) || isMediaOutput(output) {
				m.messages[i].ToolOutput = "[binary content]"
			} else {
				m.messages[i].ToolOutput = output
			}
			m.messages[i].ToolError = toolErr
			m.messages[i].ToolDone = true
			m.messages[i].ToolEndTime = time.Now()
			for j := range m.messages[i].SubagentActivities {
				a := &m.messages[i].SubagentActivities[j]
				if !a.Done {
					a.Done = true
					a.Elapsed = time.Since(a.StartTime)
				}
			}
			if shouldAutoCollapse(m.messages[i]) {
				m.messages[i].Collapsed = true
			}
			m.invalidateFrom(i)
			if !m.userScrolled {
				m.autoScroll = true
			}
			m.needsRender = true
			return
		}
	}
	now := time.Now()
	m.messages = append(m.messages, Message{
		Role:          "tool_call",
		ToolCallID:    callID,
		ToolName:      toolName,
		ToolOutput:    output,
		ToolError:     toolErr,
		ToolDone:      true,
		ToolStartTime: now,
		ToolEndTime:   now,
	})
	if len(m.messages) > 0 {
		last := &m.messages[len(m.messages)-1]
		last.ToolElapsed = last.ToolEndTime.Sub(last.ToolStartTime)
		if shouldAutoCollapse(*last) {
			last.Collapsed = true
		}
	}
	m.invalidateFrom(len(m.messages) - 1)
	if !m.userScrolled {
		m.autoScroll = true
	}
	m.needsRender = true
}

func matchTool(msg Message, toolName, callID string) bool {
	if callID != "" && msg.ToolCallID != "" {
		return msg.ToolCallID == callID
	}
	// When callID is provided but the message has no call ID, don't match —
	// falling back to name-only causes cross-contamination between parallel
	// tool calls of the same type (e.g. multiple concurrent globs).
	if callID != "" {
		return false
	}
	return msg.ToolName == toolName
}

func (m *Messages) UpdateSubagentActivity(tool, input, output string, done, hasError bool) {
	for i := len(m.messages) - 1; i >= 0; i-- {
		msg := &m.messages[i]
		if msg.Role != "tool_call" || msg.ToolName != "subagent" || msg.ToolDone {
			continue
		}
		if !done {
			ts := parseToolSummary(tool, input)
			msg.SubagentActivities = append(msg.SubagentActivities, SubagentActivity{
				Tool:      tool,
				Input:     input,
				Summary:   ts.summary,
				Args:      ts.args,
				StartTime: time.Now(),
			})
		} else {
			for j := range msg.SubagentActivities {
				a := &msg.SubagentActivities[j]
				if a.Tool == tool && !a.Done {
					a.Done = true
					a.Output = output
					a.Elapsed = time.Since(a.StartTime)
					a.Error = hasError
					break
				}
			}
		}
		m.invalidateFrom(i)
		m.needsRender = true
		if !m.userScrolled {
			m.autoScroll = true
		}
		return
	}
}

// expireReadOnlyGrace sets ToolEndTime to the past for all completed read-only
// tools, so the grace period check evaluates them as hidden. Used in tests.
func (m *Messages) expireReadOnlyGrace() {
	for i := range m.messages {
		msg := &m.messages[i]
		if msg.Role == "tool_call" && msg.ToolDone && !msg.UserExpanded && isReadOnlyToolCall(msg.ToolName, msg.ToolInput) {
			msg.Collapsed = true
		}
	}
	m.invalidateFrom(0)
	m.needsRender = true
}
