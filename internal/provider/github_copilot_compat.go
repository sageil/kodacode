package provider

import (
	"net/http"
	"strings"
)

func omitGitHubCopilotGeminiChatStreamOptions(model ModelRef) bool {
	return isGitHubCopilotGeminiModel(model)
}

func omitGitHubCopilotParallelToolCalls(model ModelRef) bool {
	return isGitHubCopilotGeminiModel(model)
}

func isGitHubCopilotGeminiModel(model ModelRef) bool {
	if CanonicalProviderID(model.ProviderID) != "github-copilot" {
		return false
	}
	return isGeminiModelID(model.ModelID)
}

func applyGitHubCopilotRequestHeaders(prompt Request, req *http.Request) {
	if CanonicalProviderID(prompt.Model.ProviderID) != "github-copilot" {
		return
	}
	req.Header.Set("x-initiator", gitHubCopilotInitiator(prompt))
	req.Header.Set("Openai-Intent", "conversation-edits")
	if gitHubCopilotHasVisionAttachments(prompt) {
		req.Header.Set("Copilot-Vision-Request", "true")
	}
}

func gitHubCopilotInitiator(prompt Request) string {
	for idx := len(prompt.Inputs) - 1; idx >= 0; idx-- {
		switch prompt.Inputs[idx].Kind {
		case InputKindAnthropicThinking:
			continue
		case InputKindUserMessage:
			return "user"
		default:
			return "agent"
		}
	}
	return "agent"
}

func gitHubCopilotHasVisionAttachments(prompt Request) bool {
	for _, input := range prompt.Inputs {
		for _, attachment := range input.Attachments {
			if strings.HasPrefix(strings.ToLower(strings.TrimSpace(attachment.MIMEType)), "image/") {
				return true
			}
		}
	}
	return false
}
