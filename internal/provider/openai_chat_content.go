package provider

import (
	"encoding/json"
	"fmt"
	"strings"
)

type openAIChatParsedContent struct {
	Text      string
	Reasoning string
}

func openAIChatMessageContent(raw json.RawMessage) (openAIChatParsedContent, error) {
	text := strings.TrimSpace(string(raw))
	if text == "" || text == "null" {
		return openAIChatParsedContent{}, nil
	}
	switch text[0] {
	case '"':
		var value string
		if err := json.Unmarshal(raw, &value); err != nil {
			return openAIChatParsedContent{}, err
		}
		return openAIChatParsedContent{Text: value}, nil
	case '[':
		var parts []struct {
			Type     string          `json:"type"`
			Text     string          `json:"text"`
			Content  string          `json:"content"`
			Thinking json.RawMessage `json:"thinking"`
		}
		if err := json.Unmarshal(raw, &parts); err != nil {
			return openAIChatParsedContent{}, err
		}
		var textBuilder strings.Builder
		var reasoningBuilder strings.Builder
		for _, part := range parts {
			switch strings.TrimSpace(part.Type) {
			case "thinking":
				reasoning, err := openAIChatThinkingText(part.Thinking)
				if err != nil {
					return openAIChatParsedContent{}, err
				}
				reasoningBuilder.WriteString(reasoning)
			case "text", "output_text":
				if part.Text != "" {
					textBuilder.WriteString(part.Text)
				} else if part.Content != "" {
					textBuilder.WriteString(part.Content)
				}
			default:
				if part.Text != "" {
					textBuilder.WriteString(part.Text)
				} else if part.Content != "" {
					textBuilder.WriteString(part.Content)
				}
			}
		}
		return openAIChatParsedContent{
			Text:      textBuilder.String(),
			Reasoning: reasoningBuilder.String(),
		}, nil
	case '{':
		var part struct {
			Type     string          `json:"type"`
			Text     string          `json:"text"`
			Content  string          `json:"content"`
			Thinking json.RawMessage `json:"thinking"`
		}
		if err := json.Unmarshal(raw, &part); err != nil {
			return openAIChatParsedContent{}, err
		}
		if strings.TrimSpace(part.Type) == "thinking" {
			reasoning, err := openAIChatThinkingText(part.Thinking)
			if err != nil {
				return openAIChatParsedContent{}, err
			}
			return openAIChatParsedContent{Reasoning: reasoning}, nil
		}
		if part.Text != "" {
			return openAIChatParsedContent{Text: part.Text}, nil
		}
		return openAIChatParsedContent{Text: part.Content}, nil
	default:
		return openAIChatParsedContent{}, fmt.Errorf("unsupported chat message content payload: %s", text)
	}
}

func openAIChatThinkingText(raw json.RawMessage) (string, error) {
	text := strings.TrimSpace(string(raw))
	if text == "" || text == "null" {
		return "", nil
	}
	switch text[0] {
	case '"':
		var value string
		if err := json.Unmarshal(raw, &value); err != nil {
			return "", err
		}
		return value, nil
	case '[':
		var parts []struct {
			Text    string `json:"text"`
			Content string `json:"content"`
		}
		if err := json.Unmarshal(raw, &parts); err != nil {
			return "", err
		}
		var builder strings.Builder
		for _, part := range parts {
			if part.Text != "" {
				builder.WriteString(part.Text)
			} else if part.Content != "" {
				builder.WriteString(part.Content)
			}
		}
		return builder.String(), nil
	case '{':
		var part struct {
			Text    string `json:"text"`
			Content string `json:"content"`
		}
		if err := json.Unmarshal(raw, &part); err != nil {
			return "", err
		}
		if part.Text != "" {
			return part.Text, nil
		}
		return part.Content, nil
	default:
		return "", fmt.Errorf("unsupported chat thinking payload: %s", text)
	}
}
