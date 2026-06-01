package provider

import (
	"bytes"
	"encoding/json"
	"strings"
)

func parseOpenAIResponseTerminal(data []byte) (UsageReport, bool, FinishReason, string, error) {
	type usageEnvelope struct {
		InputTokens        int `json:"input_tokens"`
		InputTokensDetails struct {
			CachedTokens int `json:"cached_tokens"`
		} `json:"input_tokens_details"`
		OutputTokens        int `json:"output_tokens"`
		OutputTokensDetails struct {
			ReasoningTokens int `json:"reasoning_tokens"`
		} `json:"output_tokens_details"`
		TotalTokens int `json:"total_tokens"`
	}
	type responseCompleted struct {
		ID                string `json:"id"`
		Model             string `json:"model"`
		Status            string `json:"status"`
		IncompleteDetails *struct {
			Reason string `json:"reason"`
		} `json:"incomplete_details"`
		Usage      *usageEnvelope  `json:"usage"`
		OutputText string          `json:"output_text"`
		Output     json.RawMessage `json:"output"`
		Response   *struct {
			ID                string `json:"id"`
			Model             string `json:"model"`
			Status            string `json:"status"`
			IncompleteDetails *struct {
				Reason string `json:"reason"`
			} `json:"incomplete_details"`
			Usage      *usageEnvelope  `json:"usage"`
			OutputText string          `json:"output_text"`
			Output     json.RawMessage `json:"output"`
		} `json:"response"`
	}
	var payload responseCompleted
	if err := json.Unmarshal(data, &payload); err != nil {
		return UsageReport{}, false, FinishReasonUnknown, "", err
	}
	id := strings.TrimSpace(payload.ID)
	model := strings.TrimSpace(payload.Model)
	usage := payload.Usage
	outputText, err := openAIResponseTerminalOutputText(payload.OutputText, payload.Output)
	if err != nil {
		return UsageReport{}, false, FinishReasonUnknown, "", err
	}
	status := strings.TrimSpace(payload.Status)
	incompleteReason := ""
	if payload.IncompleteDetails != nil {
		incompleteReason = strings.TrimSpace(payload.IncompleteDetails.Reason)
	}
	if payload.Response != nil {
		if id == "" {
			id = strings.TrimSpace(payload.Response.ID)
		}
		if model == "" {
			model = strings.TrimSpace(payload.Response.Model)
		}
		if status == "" {
			status = strings.TrimSpace(payload.Response.Status)
		}
		if incompleteReason == "" && payload.Response.IncompleteDetails != nil {
			incompleteReason = strings.TrimSpace(payload.Response.IncompleteDetails.Reason)
		}
		if usage == nil {
			usage = payload.Response.Usage
		}
		if outputText == "" {
			outputText, err = openAIResponseTerminalOutputText(payload.Response.OutputText, payload.Response.Output)
			if err != nil {
				return UsageReport{}, false, FinishReasonUnknown, "", err
			}
		}
	}
	finishReason := openAIResponsesFinishReason(status, incompleteReason)
	if usage == nil {
		return UsageReport{}, false, finishReason, outputText, nil
	}
	return UsageReport{
		RequestID:            id,
		Model:                model,
		InputTokens:          max(usage.InputTokens, 0),
		CacheReadInputTokens: max(usage.InputTokensDetails.CachedTokens, 0),
		OutputTokens:         max(usage.OutputTokens, 0),
		ReasoningTokens:      max(usage.OutputTokensDetails.ReasoningTokens, 0),
		TotalTokens:          max(usage.TotalTokens, 0),
	}, true, finishReason, outputText, nil
}

func openAIResponseTerminalOutputText(outputText string, output json.RawMessage) (string, error) {
	if outputText != "" {
		return outputText, nil
	}
	output = bytes.TrimSpace(output)
	if len(output) == 0 || bytes.Equal(output, []byte("null")) {
		return "", nil
	}
	var items []json.RawMessage
	if err := json.Unmarshal(output, &items); err != nil {
		return "", err
	}
	var builder strings.Builder
	for _, item := range items {
		text, err := openAIResponseOutputItemText(item)
		if err != nil {
			return "", err
		}
		builder.WriteString(text)
	}
	return builder.String(), nil
}

func openAIResponseOutputItemText(raw json.RawMessage) (string, error) {
	var item struct {
		Type    string          `json:"type"`
		Content json.RawMessage `json:"content"`
	}
	if err := json.Unmarshal(raw, &item); err != nil {
		return "", err
	}
	if strings.TrimSpace(item.Type) != "message" {
		return "", nil
	}
	return openAIResponseMessageContentText(item.Content)
}

func openAIResponseMessageContentText(raw json.RawMessage) (string, error) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return "", nil
	}
	if raw[0] == '"' {
		var value string
		if err := json.Unmarshal(raw, &value); err != nil {
			return "", err
		}
		return value, nil
	}
	if raw[0] == '{' {
		return openAIResponseContentPartText(raw)
	}
	var parts []json.RawMessage
	if err := json.Unmarshal(raw, &parts); err != nil {
		return "", err
	}
	var builder strings.Builder
	for _, part := range parts {
		text, err := openAIResponseContentPartText(part)
		if err != nil {
			return "", err
		}
		builder.WriteString(text)
	}
	return builder.String(), nil
}

func openAIResponseContentPartText(raw json.RawMessage) (string, error) {
	var part struct {
		Type    string `json:"type"`
		Text    string `json:"text"`
		Content string `json:"content"`
	}
	if err := json.Unmarshal(raw, &part); err != nil {
		return "", err
	}
	switch strings.TrimSpace(part.Type) {
	case "", "output_text", "text":
		if part.Text != "" {
			return part.Text, nil
		}
		return part.Content, nil
	default:
		return "", nil
	}
}

func openAIResponsesFinishReason(status, incompleteReason string) FinishReason {
	status = strings.TrimSpace(strings.ToLower(status))
	incompleteReason = strings.TrimSpace(strings.ToLower(incompleteReason))
	switch incompleteReason {
	case "max_output_tokens", "max_tokens", "length":
		return FinishReasonLength
	case "content_filter", "safety":
		return FinishReasonContentFilter
	}
	switch status {
	case "", "completed":
		return FinishReasonStop
	case "incomplete":
		return FinishReasonUnknown
	case "failed":
		return FinishReasonError
	default:
		return FinishReasonUnknown
	}
}
