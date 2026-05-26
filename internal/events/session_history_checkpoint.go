package events

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

type SessionHistoryEntryPayload struct {
	Kind  string
	Index int
}

func (p SessionHistoryEntryPayload) validate() error {
	switch strings.TrimSpace(p.Kind) {
	case "user_message", "assistant_message", "anthropic_thinking", "openai_reasoning", "tool_call", "tool_result":
	default:
		return errors.New("kind is required")
	}
	if p.Index < 0 {
		return errors.New("index must be >= 0")
	}
	return nil
}

type SessionHistoryAssistantEntryPayload struct {
	Content string
	Reused  bool
}

func (p SessionHistoryAssistantEntryPayload) validate() error {
	if strings.TrimSpace(p.Content) == "" {
		return errors.New("content is required")
	}
	return nil
}

type SessionHistoryToolCallPayload struct {
	CallID                 string
	ToolName               string
	ToolKind               string
	Arguments              string
	GoogleThoughtSignature []byte
	OpenAIReasoningContent string
}

func (p SessionHistoryToolCallPayload) validate() error {
	if strings.TrimSpace(p.CallID) == "" {
		return errors.New("call_id is required")
	}
	if strings.TrimSpace(p.ToolName) == "" {
		return errors.New("tool_name is required")
	}
	if strings.TrimSpace(p.Arguments) == "" {
		return errors.New("arguments is required")
	}
	return nil
}

type SessionHistoryToolResultPayload struct {
	CallID              string
	ToolName            string
	ToolKind            string
	ReusedFromCallID    string
	ReusedFromSessionID string
	ReusedFromTurnID    string
	RetryOfCallID       string
	Succeeded           bool
	Output              string
	Error               string
	StructuredResult    json.RawMessage
	OutputBlob          *ToolResultBlobRef
	ErrorBlob           *ToolResultBlobRef
}

func (p SessionHistoryToolResultPayload) validate() error {
	if strings.TrimSpace(p.CallID) == "" {
		return errors.New("call_id is required")
	}
	if strings.TrimSpace(p.ToolName) == "" {
		return errors.New("tool_name is required")
	}
	if strings.TrimSpace(p.ReusedFromSessionID) != "" && strings.TrimSpace(p.ReusedFromCallID) == "" {
		return errors.New("reused_from_call_id is required when reused_from_session_id is set")
	}
	if strings.TrimSpace(p.ReusedFromTurnID) != "" && strings.TrimSpace(p.ReusedFromSessionID) == "" {
		return errors.New("reused_from_session_id is required when reused_from_turn_id is set")
	}
	if !validStructuredResult(p.StructuredResult) {
		return errors.New("structured_result must be valid JSON")
	}
	if !p.Succeeded && !toolResultPayloadHasOutputOrError(p.Output, p.Error, p.OutputBlob, p.ErrorBlob) {
		return errors.New("output or error is required")
	}
	return nil
}

func (p SessionHistoryToolResultPayload) Successful() bool {
	if p.Succeeded {
		return true
	}
	return strings.TrimSpace(p.Error) == "" && toolResultPayloadHasOutput(p.Output, p.OutputBlob)
}

type SessionHistoryAnthropicThinkingPayload struct {
	Type      string
	Thinking  string
	Signature string
	Data      string
}

type SessionHistoryOpenAIReasoningPayload struct {
	Item json.RawMessage
}

func (p SessionHistoryOpenAIReasoningPayload) validate() error {
	if !json.Valid(p.Item) {
		return errors.New("item must be valid JSON")
	}
	var item struct {
		Type             string `json:"type"`
		EncryptedContent string `json:"encrypted_content"`
	}
	if err := json.Unmarshal(p.Item, &item); err != nil {
		return errors.New("item must be valid JSON")
	}
	if strings.TrimSpace(item.Type) != "reasoning" {
		return errors.New("item.type must be reasoning")
	}
	if strings.TrimSpace(item.EncryptedContent) == "" {
		return errors.New("item.encrypted_content is required")
	}
	return nil
}

func (p SessionHistoryAnthropicThinkingPayload) validate() error {
	switch strings.TrimSpace(p.Type) {
	case "thinking":
		if strings.TrimSpace(p.Thinking) == "" {
			return errors.New("thinking is required")
		}
		if strings.TrimSpace(p.Signature) == "" {
			return errors.New("signature is required")
		}
	case "redacted_thinking":
		if strings.TrimSpace(p.Data) == "" {
			return errors.New("data is required")
		}
	default:
		return errors.New("type must be thinking or redacted_thinking")
	}
	return nil
}

type SessionHistoryExecutionPayload struct {
	CallID           string
	ToolName         string
	Intent           string
	Effect           string
	CommandPreview   string
	WorkingDirectory string
}

func (p SessionHistoryExecutionPayload) validate() error {
	if strings.TrimSpace(p.CallID) == "" {
		return errors.New("call_id is required")
	}
	return nil
}

type SessionHistoryRuntimeNotePayload struct {
	Sequence int64
	Content  string
}

func (p SessionHistoryRuntimeNotePayload) validate() error {
	if p.Sequence < 0 {
		return errors.New("sequence must be >= 0")
	}
	if strings.TrimSpace(p.Content) == "" {
		return errors.New("content is required")
	}
	return nil
}

type SessionHistoryAttachmentPayload struct {
	Name     string
	MIMEType string
	DataURL  string
}

func (p SessionHistoryAttachmentPayload) validate() error {
	if strings.TrimSpace(p.Name) == "" {
		return errors.New("name is required")
	}
	if strings.TrimSpace(p.MIMEType) == "" {
		return errors.New("mime_type is required")
	}
	prefix := "data:" + strings.TrimSpace(p.MIMEType) + ";base64,"
	if !strings.HasPrefix(strings.TrimSpace(p.DataURL), prefix) {
		return errors.New("data_url must be a base64 data URL matching mime_type")
	}
	return nil
}

type SessionHistoryTurnPayload struct {
	TurnID              string
	UserText            string
	UserAttachments     []SessionHistoryAttachmentPayload
	AssistantText       string
	ReasoningText       string
	WorkspacePaths      []string
	RuntimeNotes        []SessionHistoryRuntimeNotePayload
	AssistantEntries    []SessionHistoryAssistantEntryPayload
	AnthropicThinking   []SessionHistoryAnthropicThinkingPayload
	OpenAIReasoning     []SessionHistoryOpenAIReasoningPayload
	ToolCalls           []SessionHistoryToolCallPayload
	ToolResults         []SessionHistoryToolResultPayload
	Executions          []SessionHistoryExecutionPayload
	EntryOrder          []SessionHistoryEntryPayload
	ToolCallCount       int
	TerminalStatus      string
	TerminalSequence    int64
	TerminalError       string
	TerminalRetryable   bool
	SuccessfulToolCalls int
	FailedToolCalls     int
	ToolNames           []string
	FailedToolNames     []string
}

func (p SessionHistoryTurnPayload) validate() error {
	if strings.TrimSpace(p.TurnID) == "" {
		return errors.New("turn_id is required")
	}
	if p.ToolCallCount < 0 {
		return errors.New("tool_call_count must be >= 0")
	}
	if p.TerminalSequence < 0 {
		return errors.New("terminal_sequence must be >= 0")
	}
	switch strings.TrimSpace(p.TerminalStatus) {
	case "", "completed", "canceled", "failed":
	default:
		return errors.New("terminal_status must be empty, completed, canceled, or failed")
	}
	if p.TerminalStatus == "failed" && strings.TrimSpace(p.TerminalError) == "" {
		return errors.New("terminal_error is required for failed turns")
	}
	if p.SuccessfulToolCalls < 0 {
		return errors.New("successful_tool_calls must be >= 0")
	}
	if p.FailedToolCalls < 0 {
		return errors.New("failed_tool_calls must be >= 0")
	}
	for i, entry := range p.AssistantEntries {
		if err := entry.validate(); err != nil {
			return fmt.Errorf("assistant_entries[%d]: %w", i, err)
		}
	}
	for i, call := range p.ToolCalls {
		if err := call.validate(); err != nil {
			return fmt.Errorf("tool_calls[%d]: %w", i, err)
		}
	}
	for i, block := range p.AnthropicThinking {
		if err := block.validate(); err != nil {
			return fmt.Errorf("anthropic_thinking[%d]: %w", i, err)
		}
	}
	for i, item := range p.OpenAIReasoning {
		if err := item.validate(); err != nil {
			return fmt.Errorf("openai_reasoning[%d]: %w", i, err)
		}
	}
	for i, result := range p.ToolResults {
		if err := result.validate(); err != nil {
			return fmt.Errorf("tool_results[%d]: %w", i, err)
		}
	}
	for i, execution := range p.Executions {
		if err := execution.validate(); err != nil {
			return fmt.Errorf("executions[%d]: %w", i, err)
		}
	}
	for i, attachment := range p.UserAttachments {
		if err := attachment.validate(); err != nil {
			return fmt.Errorf("user_attachments[%d]: %w", i, err)
		}
	}
	for i, entry := range p.EntryOrder {
		if err := entry.validate(); err != nil {
			return fmt.Errorf("entry_order[%d]: %w", i, err)
		}
		switch entry.Kind {
		case "user_message":
			if strings.TrimSpace(p.UserText) == "" && len(p.UserAttachments) == 0 {
				return fmt.Errorf("entry_order[%d]: user_text or user_attachments is required when user_message is present", i)
			}
			if entry.Index != 0 {
				return fmt.Errorf("entry_order[%d]: user_message index must be 0", i)
			}
		case "assistant_message":
			if entry.Index >= len(p.AssistantEntries) {
				return fmt.Errorf("entry_order[%d]: assistant_message index out of range", i)
			}
		case "anthropic_thinking":
			if entry.Index >= len(p.AnthropicThinking) {
				return fmt.Errorf("entry_order[%d]: anthropic_thinking index out of range", i)
			}
		case "openai_reasoning":
			if entry.Index >= len(p.OpenAIReasoning) {
				return fmt.Errorf("entry_order[%d]: openai_reasoning index out of range", i)
			}
		case "tool_call":
			if entry.Index >= len(p.ToolCalls) {
				return fmt.Errorf("entry_order[%d]: tool_call index out of range", i)
			}
		case "tool_result":
			if entry.Index >= len(p.ToolResults) {
				return fmt.Errorf("entry_order[%d]: tool_result index out of range", i)
			}
		}
	}
	for _, toolName := range p.ToolNames {
		if strings.TrimSpace(toolName) == "" {
			return errors.New("tool_names must not contain empty values")
		}
	}
	for _, toolName := range p.FailedToolNames {
		if strings.TrimSpace(toolName) == "" {
			return errors.New("failed_tool_names must not contain empty values")
		}
	}
	for _, path := range p.WorkspacePaths {
		if strings.TrimSpace(path) == "" {
			return errors.New("workspace_paths must not contain empty values")
		}
	}
	for i, note := range p.RuntimeNotes {
		if err := note.validate(); err != nil {
			return fmt.Errorf("runtime_notes[%d]: %w", i, err)
		}
	}
	return nil
}

type SessionHistoryCheckpointPayload struct {
	ThroughSequence  int64
	Continuation     *SessionHistoryContinuationUpdatedPayload
	CompletedTurnIDs []string
	Turns            []SessionHistoryTurnPayload
}

func (SessionHistoryCheckpointPayload) eventType() Type { return TypeSessionHistoryCheckpoint }

func (p SessionHistoryCheckpointPayload) validate() error {
	if p.ThroughSequence < 0 {
		return errors.New("through_sequence must be >= 0")
	}
	for _, turnID := range p.CompletedTurnIDs {
		if strings.TrimSpace(turnID) == "" {
			return errors.New("completed_turn_ids must not contain empty values")
		}
	}
	for i, turn := range p.Turns {
		if err := turn.validate(); err != nil {
			return fmt.Errorf("turns[%d]: %w", i, err)
		}
	}
	if p.Continuation != nil {
		if err := p.Continuation.validate(); err != nil {
			return errors.New("continuation: " + err.Error())
		}
	}
	return nil
}
