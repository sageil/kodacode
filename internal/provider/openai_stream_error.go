package provider

import (
	"encoding/json"
	"strconv"
	"strings"
)

type openAIStreamErrorPayload struct {
	Message string          `json:"message"`
	Type    string          `json:"type"`
	Code    json.RawMessage `json:"code"`
}

type openAIStreamErrorFields struct {
	Message    string
	Type       string
	Code       string
	StatusCode int
}

func parseOpenAIStreamError(data []byte) error {
	if fields, ok := decodeOpenAIStreamErrorFields(data); ok {
		return providerErrorFromOpenAIStreamFields(fields)
	}
	if trimmed := strings.TrimSpace(string(data)); trimmed != "" {
		return newProviderError(trimmed, 0, retryableProviderSignals(trimmed), 0, nil)
	}
	return newProviderError("openai streaming error", 0, false, 0, nil)
}

func providerErrorFromOpenAIStreamFields(fields openAIStreamErrorFields) error {
	text := strings.TrimSpace(fields.Message)
	if text == "" {
		text = strings.TrimSpace(fields.Code)
	}
	if text == "" {
		text = strings.TrimSpace(fields.Type)
	}
	retryable := retryableProviderSignals(fields.Type, fields.Code, fields.Message)
	if fields.StatusCode > 0 && httpStatusRetryable(fields.StatusCode) {
		retryable = true
	}
	return newProviderError(text, fields.StatusCode, retryable, 0, nil)
}

func decodeOpenAIStreamErrorFields(data []byte) (openAIStreamErrorFields, bool) {
	var envelope struct {
		Error *openAIStreamErrorPayload `json:"error"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil || envelope.Error == nil {
		return openAIStreamErrorFields{}, false
	}
	return openAIStreamErrorFieldsFromPayload(envelope.Error)
}

func openAIStreamErrorFieldsFromPayload(payload *openAIStreamErrorPayload) (openAIStreamErrorFields, bool) {
	if payload == nil {
		return openAIStreamErrorFields{}, false
	}
	code, statusCode := decodeOpenAIStreamErrorCode(payload.Code)
	fields := openAIStreamErrorFields{
		Message:    strings.TrimSpace(payload.Message),
		Type:       strings.TrimSpace(payload.Type),
		Code:       code,
		StatusCode: statusCode,
	}
	if fields.Message == "" && fields.Type == "" && fields.Code == "" && fields.StatusCode == 0 {
		return openAIStreamErrorFields{}, false
	}
	return fields, true
}

func decodeOpenAIStreamErrorCode(raw json.RawMessage) (string, int) {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" {
		return "", 0
	}

	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		text = strings.TrimSpace(text)
		if text == "" {
			return "", 0
		}
		if statusCode, err := strconv.Atoi(text); err == nil {
			return text, statusCode
		}
		return text, 0
	}

	var statusCode int
	if err := json.Unmarshal(raw, &statusCode); err == nil && statusCode > 0 {
		return strconv.Itoa(statusCode), statusCode
	}

	return trimmed, 0
}
