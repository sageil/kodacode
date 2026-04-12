package provider

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"google.golang.org/genai"
)

// syntheticThoughtSignature is the magic value that tells Cloud Code Assist
// to skip thought_signature validation. Mirrors Gemini CLI's approach:
// every first functionCall in a model turn must have a thoughtSignature.
var syntheticThoughtSignature = []byte("skip_thought_signature_validator")

// convertGoogleMessages converts a slice of provider.Message to genai Content slices.
//
// Tool name resolution: ToolResultPart only carries a ToolCallID, but Google's
// FunctionResponse requires a function name. We build a callID→name map by scanning
// all messages for ToolCallParts. If no match is found, ToolCallID is used as the
// fallback function name.
//
// When thinkingModel is true, the first functionCall part in each model turn
// is guaranteed to have a thoughtSignature (injecting a synthetic bypass if missing).
func convertGoogleMessages(msgs []Message, thinkingModel bool) ([]*genai.Content, error) {
	callIDToName := buildCallIDMap(msgs)

	contents := make([]*genai.Content, 0, len(msgs))
	for _, m := range msgs {
		role := "user"
		if m.Role == "assistant" {
			role = "model"
		}

		parts, err := convertGoogleParts(m.Parts, callIDToName)
		if err != nil {
			return nil, fmt.Errorf("convert message role=%q: %w", m.Role, err)
		}
		if len(parts) == 0 {
			continue // skip messages where all parts were filtered (e.g. reasoning-only)
		}

		// Gemini requires strict role alternation. Merge consecutive
		// same-role Content objects (can happen after compaction strips
		// orphaned tool_call/tool_result messages).
		if n := len(contents); n > 0 && contents[n-1].Role == role {
			contents[n-1].Parts = append(contents[n-1].Parts, parts...)
			continue
		}

		contents = append(contents, &genai.Content{
			Role:  role,
			Parts: parts,
		})
	}

	// Cloud Code Assist requires thoughtSignature on the first functionCall
	// in each model turn when thinking is active. Inject synthetic signatures
	// where the real one is missing (e.g. loaded from DB, or API didn't return one).
	if thinkingModel {
		ensureThoughtSignatures(contents)
	}

	return contents, nil
}

// ensureThoughtSignatures walks model turns and ensures the first functionCall
// part in each has a thoughtSignature. Injects the synthetic bypass value
// if missing. Mirrors Gemini CLI's ensureActiveLoopHasThoughtSignatures().
func ensureThoughtSignatures(contents []*genai.Content) {
	for _, c := range contents {
		if c.Role != "model" {
			continue
		}
		for _, p := range c.Parts {
			if p.FunctionCall != nil {
				if len(p.ThoughtSignature) == 0 {
					p.ThoughtSignature = syntheticThoughtSignature
				}
				break // only the first functionCall per model turn
			}
		}
	}
}

// convertGoogleParts converts a slice of MessagePart to genai Parts.
func convertGoogleParts(parts []MessagePart, callIDToName map[string]string) ([]*genai.Part, error) {
	result := make([]*genai.Part, 0, len(parts))
	for _, p := range parts {
		switch part := p.(type) {
		case TextPart:
			result = append(result, genai.NewPartFromText(part.Text))

		case ToolCallPart:
			var args map[string]any
			if part.Arguments != "" {
				if err := json.Unmarshal([]byte(part.Arguments), &args); err != nil {
					return nil, fmt.Errorf("unmarshal tool args for %q: %w", part.Name, err)
				}
			}
			p := genai.NewPartFromFunctionCall(part.Name, args)
			if len(part.ThoughtSignature) > 0 {
				p.ThoughtSignature = part.ThoughtSignature
			}
			result = append(result, p)

		case ToolResultPart:
			name, ok := callIDToName[part.ToolCallID]
			if !ok {
				name = part.ToolCallID // fallback: use call ID as function name
			}
			resp := map[string]any{"result": part.Output}
			if part.Error != nil {
				resp = map[string]any{"error": *part.Error}
			}
			result = append(result, genai.NewPartFromFunctionResponse(name, resp))

		case ReasoningPart:
			// Thinking blocks are not replayed to Google; skip silently.

		case FilePart:
			if part.URL != "" {
				b64Data := ExtractBase64Data(part.URL)
				raw, err := base64.StdEncoding.DecodeString(b64Data)
				if err == nil {
					result = append(result, genai.NewPartFromBytes(raw, part.MimeType))
				} else {
					// Fallback: include as text if base64 decode fails.
					result = append(result, genai.NewPartFromText(fmt.Sprintf("[File: %s]\n%s", part.Path, b64Data)))
				}
			}
		}
	}
	return result, nil
}

// buildCallIDMap scans all messages and returns a map of ToolCallPart.ID → ToolCallPart.Name.
// Used to resolve function names for ToolResultPart conversions.
func buildCallIDMap(msgs []Message) map[string]string {
	m := make(map[string]string)
	for _, msg := range msgs {
		for _, p := range msg.Parts {
			if tc, ok := p.(ToolCallPart); ok {
				m[tc.ID] = tc.Name
			}
		}
	}
	return m
}

// buildGoogleConfig builds a GenerateContentConfig from ChatOptions.
func buildGoogleConfig(opts ChatOptions, skipToolChoice bool, thinkingModel bool) (*genai.GenerateContentConfig, error) {
	cfg := &genai.GenerateContentConfig{}

	// System instruction: concatenate all system parts into one Content block.
	if len(opts.SystemParts) > 0 {
		var sb strings.Builder
		for i, part := range opts.SystemParts {
			if i > 0 && part != "" {
				sb.WriteString("\n\n")
			}
			sb.WriteString(part)
		}
		if text := sb.String(); text != "" {
			cfg.SystemInstruction = &genai.Content{
				Parts: []*genai.Part{genai.NewPartFromText(text)},
			}
		}
	}

	if opts.Temperature != nil {
		t := float32(*opts.Temperature)
		cfg.Temperature = &t
	}

	if opts.MaxTokens > 0 {
		cfg.MaxOutputTokens = int32(opts.MaxTokens)
	}

	if len(opts.Tools) > 0 {
		decls := make([]*genai.FunctionDeclaration, 0, len(opts.Tools))
		for _, t := range opts.Tools {
			var schema any
			if len(t.Parameters) > 0 {
				if err := json.Unmarshal(t.Parameters, &schema); err != nil {
					return nil, fmt.Errorf("unmarshal tool schema for %q: %w", t.Name, err)
				}
			}
			decls = append(decls, &genai.FunctionDeclaration{
				Name:                 t.Name,
				Description:          t.Description,
				ParametersJsonSchema: schema,
			})
		}
		cfg.Tools = []*genai.Tool{{FunctionDeclarations: decls}}
		if !skipToolChoice {
			cfg.ToolConfig = &genai.ToolConfig{
				FunctionCallingConfig: &genai.FunctionCallingConfig{
					Mode: genai.FunctionCallingConfigModeAny,
				},
			}
		}
	}

	if opts.ReasoningBudget != nil || opts.ReasoningEffort != "" {
		think := &genai.ThinkingConfig{
			IncludeThoughts: true,
		}
		if opts.ReasoningEffort != "" {
			think.ThinkingLevel = genai.ThinkingLevel(strings.ToUpper(opts.ReasoningEffort))
			log.Printf("[variant-google] setting thinking_level=%s", strings.ToUpper(opts.ReasoningEffort))
		} else {
			budget := int32(*opts.ReasoningBudget)
			think.ThinkingBudget = &budget
			log.Printf("[variant-google] setting thinking_budget=%d", budget)
		}
		cfg.ThinkingConfig = think
	} else if thinkingModel {
		cfg.ThinkingConfig = &genai.ThinkingConfig{
			IncludeThoughts: true,
		}
		log.Printf("[variant-google] adaptive thinking (no budget/level)")
	}

	return cfg, nil
}
