// Package message defines the typed content model for conversation message parts.
// Each part has a Type string (stored in the DB) and a JSON payload that maps
// to one of the concrete Content types: TextContent, ToolCallContent,
// ToolResultContent, ReasoningContent, or FileContent.
package message

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
)

// TextContent holds plain text from a message part.
type TextContent struct {
	Text             string `json:"text"`
	ThoughtSignature []byte `json:"thought_signature,omitempty"` // Gemini: opaque signature for thinking mode
}

// ToolCallContent represents a request to invoke a tool.
type ToolCallContent struct {
	ID               string `json:"id"`
	Name             string `json:"name"`
	Arguments        string `json:"arguments"`
	ThoughtSignature []byte `json:"thought_signature,omitempty"` // Gemini: opaque signature for thinking mode
}

// ToolResultContent holds the output of a tool invocation.
type ToolResultContent struct {
	ToolCallID string         `json:"tool_call_id"`
	Output     string         `json:"output"`
	Error      *string        `json:"error,omitempty"`
	ErrorCode  string         `json:"error_code,omitempty"`
	Metadata   map[string]any `json:"metadata,omitempty"`
}

// ReasoningContent holds model reasoning/thinking output.
// Signature is the opaque value provided by the Anthropic API that must be
// echoed back verbatim when replaying the thinking block in a subsequent turn.
type ReasoningContent struct {
	Text      string `json:"text"`
	Tokens    int    `json:"tokens"`
	Signature string `json:"signature,omitempty"`
}

// FileContent references a file attached to a message part.
type FileContent struct {
	Path       string `json:"path"`
	MimeType   string `json:"mime_type"`
	URL        string `json:"url,omitempty"`
	StorageKey string `json:"storage_key,omitempty"`
	Size       int64  `json:"size,omitempty"`
}

// Content is the sealed interface implemented by all part content types.
type Content interface {
	contentType() string
}

func (TextContent) contentType() string       { return "text" }
func (ToolCallContent) contentType() string   { return "tool_call" }
func (ToolResultContent) contentType() string { return "tool_result" }
func (ReasoningContent) contentType() string  { return "reasoning" }
func (FileContent) contentType() string       { return "file" }

// MarshalContent serialises a Content value to a JSON string.
func MarshalContent(c Content) (string, error) {
	b, err := json.Marshal(c)
	if err != nil {
		return "", fmt.Errorf("marshal content: %w", err)
	}
	return string(b), nil
}

// UnmarshalContent deserialises raw JSON into the Content type identified by
// partType. It returns an error for unknown part types.
func UnmarshalContent(partType, raw string) (Content, error) {
	switch partType {
	case "text":
		var c TextContent
		if err := json.Unmarshal([]byte(raw), &c); err != nil {
			return nil, fmt.Errorf("unmarshal %s content: %w", partType, err)
		}
		return c, nil
	case "tool_call":
		var c ToolCallContent
		if err := json.Unmarshal([]byte(raw), &c); err != nil {
			return nil, fmt.Errorf("unmarshal %s content: %w", partType, err)
		}
		return c, nil
	case "tool_result":
		var c ToolResultContent
		if err := json.Unmarshal([]byte(raw), &c); err != nil {
			return nil, fmt.Errorf("unmarshal %s content: %w", partType, err)
		}
		return c, nil
	case "reasoning":
		var c ReasoningContent
		if err := json.Unmarshal([]byte(raw), &c); err != nil {
			return nil, fmt.Errorf("unmarshal %s content: %w", partType, err)
		}
		return c, nil
	case "file":
		var c FileContent
		if err := json.Unmarshal([]byte(raw), &c); err != nil {
			return nil, fmt.Errorf("unmarshal %s content: %w", partType, err)
		}
		return c, nil
	default:
		return nil, fmt.Errorf("unknown part type %q", partType)
	}
}

// AttachmentSummary returns a compact text marker for a stored attachment.
func AttachmentSummary(c FileContent) string {
	name := strings.TrimSpace(filepath.Base(c.Path))
	if name == "." || name == "/" || name == "" {
		name = "attachment"
	}

	var meta []string
	if c.MimeType != "" {
		meta = append(meta, c.MimeType)
	}
	if c.Size > 0 {
		meta = append(meta, fmt.Sprintf("%d bytes", c.Size))
	}
	if len(meta) == 0 {
		return fmt.Sprintf("[Attachment: %s stored locally]", name)
	}
	return fmt.Sprintf("[Attachment: %s (%s) stored locally]", name, strings.Join(meta, ", "))
}
