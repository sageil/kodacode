package provider

import (
	"fmt"
	"strings"
)

func rejectCustomTools(providerName string, tools []Tool) error {
	for _, tool := range tools {
		if tool.KindOrDefault() == ToolKindCustom {
			return fmt.Errorf("%s provider does not support custom tool %q", strings.TrimSpace(providerName), tool.Name)
		}
	}
	return nil
}

func SupportsCustomTools(model ModelRef) bool {
	switch CanonicalProviderID(model.ProviderID) {
	case "openai":
		return true
	case "github-copilot":
		major, ok := gitHubCopilotGPTMajor(model.ModelID)
		return ok && major >= 5
	default:
		return false
	}
}
