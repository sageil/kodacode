package provider

import (
	"fmt"
	"strings"
)

// ConversationToolCallIDAliases resolves between runtime tool call ids and the
// provider-safe request-local aliases used in an outbound conversation.
type ConversationToolCallIDAliases struct {
	originalToProvider map[string]string
	providerToOriginal map[string]string
}

// BuildConversationToolCallIDAliases returns the request-local alias mapping
// that normalizeConversationToolCallIDs applies to tool call/result inputs.
func BuildConversationToolCallIDAliases(req Request) ConversationToolCallIDAliases {
	_, aliases := normalizeConversationToolCallIDsWithAliases(req)
	return aliases
}

func (a ConversationToolCallIDAliases) OriginalToProvider(original string) (string, bool) {
	original = strings.TrimSpace(original)
	if original == "" || a.originalToProvider == nil {
		return "", false
	}
	mapped, ok := a.originalToProvider[original]
	return mapped, ok
}

func (a ConversationToolCallIDAliases) ProviderToOriginal(providerID string) (string, bool) {
	providerID = strings.TrimSpace(providerID)
	if providerID == "" || a.providerToOriginal == nil {
		return "", false
	}
	original, ok := a.providerToOriginal[providerID]
	return original, ok
}

// normalizeConversationToolCallIDs rewrites runtime tool call ids into
// provider-safe request-local ids for the outbound transcript only.
//
// Session history stores logical ids that are authoritative inside kodacode,
// but providers are not consistent about which id formats they accept when
// replaying prior tool calls and tool results. Some OpenAI-compatible routes
// reject ids emitted by other providers (for example `chatcmpl-tool-...`,
// `call_...`, or local runtime ids with punctuation) when the same session is
// continued after a model switch. Keep the logical ids in runtime state and
// remap only the outbound conversation so each request contains a self-
// consistent, provider-safe transcript.
func normalizeConversationToolCallIDs(req Request) Request {
	rewritten, _ := normalizeConversationToolCallIDsWithAliases(req)
	return rewritten
}

func normalizeConversationToolCallIDsWithAliases(req Request) (Request, ConversationToolCallIDAliases) {
	if len(req.Inputs) == 0 {
		return req, ConversationToolCallIDAliases{}
	}
	mapper := providerConversationToolIDMapper{
		byOriginal: make(map[string]string),
	}
	aliases := ConversationToolCallIDAliases{
		originalToProvider: make(map[string]string),
		providerToOriginal: make(map[string]string),
	}
	inputs := make([]Input, len(req.Inputs))
	changed := false
	for idx, input := range req.Inputs {
		inputs[idx] = input
		switch input.Kind {
		case InputKindToolCall, InputKindToolResult:
			original := strings.TrimSpace(input.CallID)
			mapped := mapper.mapID(original)
			if original != "" && mapped != "" {
				aliases.originalToProvider[original] = mapped
				aliases.providerToOriginal[mapped] = original
			}
			if mapped != input.CallID {
				inputs[idx].CallID = mapped
				changed = true
			}
		}
	}
	if !changed {
		return req, aliases
	}
	req.Inputs = inputs
	return req, aliases
}

type providerConversationToolIDMapper struct {
	byOriginal map[string]string
	next       int
}

func (m *providerConversationToolIDMapper) mapID(original string) string {
	original = strings.TrimSpace(original)
	if original == "" {
		return ""
	}
	if mapped, ok := m.byOriginal[original]; ok {
		return mapped
	}
	m.next++
	mapped := formatProviderConversationToolID(m.next)
	m.byOriginal[original] = mapped
	return mapped
}

func formatProviderConversationToolID(index int) string {
	const width = 8
	const alphabet = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	if index < 0 {
		index = 0
	}
	buf := [width]byte{}
	value := index
	for pos := width - 1; pos >= 0; pos-- {
		buf[pos] = alphabet[value%len(alphabet)]
		value /= len(alphabet)
	}
	if value != 0 {
		panic(fmt.Sprintf("provider conversation tool id overflow: %d", index))
	}
	return "T" + string(buf[:])
}
