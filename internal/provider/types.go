package provider

// MessagePart is the interface for all content types within a provider.Message.
// The unexported sentinel method prevents external implementations.
type MessagePart interface {
	providerPartType() string
}

// TextPart is plain text content.
type TextPart struct {
	Text             string
	ThoughtSignature []byte // Gemini: opaque signature attached to model response text
}

// ToolCallPart is a tool invocation requested by the model.
type ToolCallPart struct {
	ID               string
	Name             string
	Arguments        string // raw JSON
	ThoughtSignature []byte // Gemini: opaque signature required when thinking is enabled
}

// ToolResultPart is the result of a tool invocation.
type ToolResultPart struct {
	ToolCallID string
	Output     string
	Error      *string
	Metadata   map[string]any
}

// ReasoningPart is a reasoning/thinking block from the model.
// Tokens is the count for this specific block.
// Signature is the opaque value returned by the Anthropic API that must be
// echoed back verbatim when replaying the thinking block in a subsequent turn.
type ReasoningPart struct {
	Text      string
	Tokens    int
	Signature string
}

// FilePart is a file attachment. URL must be a file:// or data: URI.
type FilePart struct {
	Path     string
	MimeType string
	URL      string
}

func (TextPart) providerPartType() string       { return "text" }
func (ToolCallPart) providerPartType() string   { return "tool_call" }
func (ToolResultPart) providerPartType() string { return "tool_result" }
func (ReasoningPart) providerPartType() string  { return "reasoning" }
func (FilePart) providerPartType() string       { return "file" }
