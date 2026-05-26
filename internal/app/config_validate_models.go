package app

import (
	"fmt"
	"strings"

	"github.com/sageil/kodacode/internal/provider"
)

func (c Config) validateCompatibleProviderIDs() error {
	if err := validateCompatibleProviderID(strings.TrimSpace(c.OpenAICompatible.ProviderID)); err != nil {
		return err
	}
	for providerID := range c.CompatibleProviders {
		if err := validateCompatibleProviderID(providerID); err != nil {
			return err
		}
	}
	return nil
}

func (c Config) validateModelRoute(route provider.ModelRoute) error {
	for _, model := range route.Candidates() {
		if err := c.validateModelProvider(model); err != nil {
			return err
		}
	}
	return nil
}

func (c Config) validateModelRouteReference(route provider.ModelRoute) error {
	for _, model := range route.Candidates() {
		if err := c.validateModelProviderReference(model); err != nil {
			return err
		}
	}
	return nil
}

func (c Config) validateModelProvider(model provider.ModelRef) error {
	if err := c.validateModelProviderReference(model); err != nil {
		return err
	}
	switch model.ProviderID {
	case "anthropic":
		if strings.TrimSpace(c.Anthropic.APIKey) == "" {
			return ErrAnthropicAPIKeyRequired
		}
	case "google":
		if strings.TrimSpace(c.Google.APIKey) == "" {
			return ErrGoogleAPIKeyRequired
		}
	case "nvidia":
		if strings.TrimSpace(c.NVIDIA.APIKey) == "" {
			return ErrNVIDIAAPIKeyRequired
		}
	case "deepseek":
		if strings.TrimSpace(c.DeepSeek.APIKey) == "" {
			return ErrDeepSeekAPIKeyRequired
		}
	case "openai", openAICodexProviderID, "github-copilot":
		return nil
	default:
		if handled, err := validateExperimentalProvider(model.ProviderID); handled {
			return err
		}
		compatible, ok := c.configuredCompatibleProvider(model.ProviderID)
		if !ok {
			return fmt.Errorf("%w: %s", ErrUnsupportedModelProvider, model.ProviderID)
		}
		baseURL := compatibleProviderBaseURL(model.ProviderID, compatible.BaseURL)
		if strings.TrimSpace(compatible.APIKey) == "" && !compatibleProviderAllowsEmptyAPIKey(baseURL) {
			return ErrOpenAICompatibleAPIKeyRequired
		}
	}
	return nil
}

func (c Config) validateModelProviderReference(model provider.ModelRef) error {
	switch model.ProviderID {
	case "openai":
		return nil
	case openAICodexProviderID:
		return nil
	case "anthropic":
		return nil
	case "google":
		return nil
	case "nvidia":
		return nil
	case "github-copilot":
		return nil
	case "deepseek":
		return nil
	default:
		if handled, err := validateExperimentalProvider(model.ProviderID); handled {
			return err
		}
		compatible, ok := c.configuredCompatibleProvider(model.ProviderID)
		if !ok {
			return fmt.Errorf("%w: %s", ErrUnsupportedModelProvider, model.ProviderID)
		}
		if strings.TrimSpace(compatibleProviderBaseURL(model.ProviderID, compatible.BaseURL)) == "" {
			return ErrOpenAICompatibleBaseURLRequired
		}
		return nil
	}
}

func (c Config) validateModelOverrides() error {
	if len(c.ModelOverrides) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(c.ModelOverrides))
	for index, override := range c.ModelOverrides {
		if err := override.Ref.Validate(); err != nil {
			return fmt.Errorf("model_overrides[%d].ref: %w", index, err)
		}
		if err := c.validateModelProviderReference(override.Ref); err != nil {
			return fmt.Errorf("model_overrides[%d].ref: %w", index, err)
		}
		key := strings.TrimSpace(override.Ref.String())
		if _, ok := seen[key]; ok {
			return fmt.Errorf("model_overrides[%d].ref duplicates %q", index, key)
		}
		seen[key] = struct{}{}
		if override.ContextSize != nil && *override.ContextSize <= 0 {
			return fmt.Errorf("model_overrides[%d].context_size must be positive", index)
		}
		if override.MaxInputTokens != nil && *override.MaxInputTokens <= 0 {
			return fmt.Errorf("model_overrides[%d].max_input_tokens must be positive", index)
		}
		if override.MaxOutputTokens != nil && *override.MaxOutputTokens <= 0 {
			return fmt.Errorf("model_overrides[%d].max_output_tokens must be positive", index)
		}
		if override.DefaultOutputTokens != nil && *override.DefaultOutputTokens <= 0 {
			return fmt.Errorf("model_overrides[%d].default_output_tokens must be positive", index)
		}
		if override.CostInput != nil && *override.CostInput < 0 {
			return fmt.Errorf("model_overrides[%d].cost_input must be non-negative", index)
		}
		if override.CostOutput != nil && *override.CostOutput < 0 {
			return fmt.Errorf("model_overrides[%d].cost_output must be non-negative", index)
		}
	}
	return nil
}

func validateCompatibleProviderID(providerID string) error {
	switch strings.TrimSpace(providerID) {
	case "", "openai":
		if strings.TrimSpace(providerID) == "openai" {
			return fmt.Errorf("%w: %s", ErrOpenAICompatibleReservedProviderID, providerID)
		}
		return nil
	case "anthropic", "google", "nvidia", "github-copilot", "deepseek":
		return fmt.Errorf("%w: %s", ErrOpenAICompatibleReservedProviderID, providerID)
	default:
		return nil
	}
}

func (c Config) configuredCompatibleProvider(providerID string) (OpenAICompatibleProviderConfig, bool) {
	providerID = strings.TrimSpace(providerID)
	if providerID == "" {
		return OpenAICompatibleProviderConfig{}, false
	}
	if compatible, ok := c.CompatibleProviders[providerID]; ok {
		compatible.ProviderID = providerID
		return compatible, true
	}
	if providerID == strings.TrimSpace(c.OpenAICompatible.ProviderID) {
		return OpenAICompatibleProviderConfig{
			ProviderID: providerID,
			APIKey:     strings.TrimSpace(c.OpenAICompatible.APIKey),
			BaseURL:    strings.TrimSpace(c.OpenAICompatible.BaseURL),
		}, true
	}
	return OpenAICompatibleProviderConfig{}, false
}
