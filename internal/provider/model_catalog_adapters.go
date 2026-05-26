package provider

import (
	"context"
	"fmt"
	"net/http"
	"slices"
	"strings"

	anthropicsdk "github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"google.golang.org/genai"
)

func catalogModelFromCopilot(model GitHubCopilotModel) CatalogModel {
	return CatalogModel{
		ID:                 model.ID,
		Name:               firstNonBlank(model.Name, model.ID),
		Family:             model.Family,
		ContextSize:        model.ContextSize,
		MaxInputTokens:     model.MaxInputTokens,
		MaxOutputTokens:    model.MaxOutputTokens,
		Reasoning:          model.Reasoning,
		ToolCalls:          model.ToolCalls,
		ToolCallsKnown:     true,
		Vision:             model.Vision,
		VisionKnown:        true,
		SupportedEndpoints: cloneStrings(model.SupportedEndpoints),
	}
}

func fetchAnthropicModelCatalogProvider(ctx context.Context, source RemoteModelCatalogProvider) (*catalogProvider, error) {
	apiKey := strings.TrimSpace(source.APIKey)
	if apiKey == "" {
		return nil, fmt.Errorf("%s: %w", source.ID, ErrAnthropicAPIKeyRequired)
	}
	options := []option.RequestOption{option.WithAPIKey(apiKey)}
	if baseURL := strings.TrimSpace(source.BaseURL); baseURL != "" {
		options = append(options, option.WithBaseURL(baseURL))
	}
	if source.HTTPClient != nil {
		options = append(options, option.WithHTTPClient(source.HTTPClient))
	}

	ctx, cancel := context.WithTimeout(ctx, defaultModelCatalogTimeout)
	defer cancel()

	client := anthropicsdk.NewClient(options...)
	pager := client.Models.ListAutoPaging(ctx, anthropicsdk.ModelListParams{})
	models := make([]CatalogModel, 0, 16)
	for pager.Next() {
		models = append(models, catalogModelFromAnthropic(pager.Current()))
	}
	if err := pager.Err(); err != nil {
		return nil, fmt.Errorf("%s: fetch models: %w", source.ID, err)
	}
	return catalogProviderFromModels(source, models), nil
}

func fetchGoogleModelCatalogProvider(ctx context.Context, source RemoteModelCatalogProvider) (*catalogProvider, error) {
	apiKey := strings.TrimSpace(source.APIKey)
	if apiKey == "" {
		return nil, fmt.Errorf("%s: %w", source.ID, ErrGoogleAPIKeyRequired)
	}
	httpClient := source.HTTPClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}

	ctx, cancel := context.WithTimeout(ctx, defaultModelCatalogTimeout)
	defer cancel()

	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:     apiKey,
		Backend:    genai.BackendGeminiAPI,
		HTTPClient: httpClient,
		HTTPOptions: genai.HTTPOptions{
			BaseURL: firstNonBlank(strings.TrimSpace(source.BaseURL), defaultGoogleBaseURL),
		},
	})
	if err != nil {
		return nil, fmt.Errorf("%s: create client: %w", source.ID, err)
	}

	models := make([]CatalogModel, 0, 16)
	for model, err := range client.Models.All(ctx) {
		if err != nil {
			return nil, fmt.Errorf("%s: fetch models: %w", source.ID, err)
		}
		if model == nil {
			continue
		}
		catalogModel, ok := catalogModelFromGoogle(*model)
		if !ok {
			continue
		}
		models = append(models, catalogModel)
	}
	return catalogProviderFromModels(source, models), nil
}

func fetchGitHubCopilotModelCatalogProvider(ctx context.Context, source RemoteModelCatalogProvider) (*catalogProvider, error) {
	models, err := FetchGitHubCopilotModels(ctx, GitHubCopilotConfig{
		Token:      strings.TrimSpace(source.APIKey),
		BaseURL:    strings.TrimSpace(source.BaseURL),
		OAuth:      source.GitHubCopilotOAuth,
		HTTPClient: source.HTTPClient,
	})
	if err != nil {
		return nil, fmt.Errorf("%s: %w", source.ID, err)
	}
	out := make([]CatalogModel, 0, len(models))
	for _, model := range models {
		out = append(out, catalogModelFromCopilot(model))
	}
	return catalogProviderFromModels(source, out), nil
}

func catalogProviderFromModels(source RemoteModelCatalogProvider, models []CatalogModel) *catalogProvider {
	providerID := strings.TrimSpace(source.ID)
	if providerID == "" {
		return nil
	}
	providerEntry := &catalogProvider{
		ID:     providerID,
		Name:   firstNonBlank(source.Name, providerID),
		Models: make(map[string]CatalogModel, len(models)),
	}
	for _, model := range models {
		modelID := strings.TrimSpace(model.ID)
		if modelID == "" {
			continue
		}
		model.ID = modelID
		model.Name = firstNonBlank(model.Name, modelID)
		providerEntry.Models[modelID] = model
	}
	return providerEntry
}

func catalogModelFromAnthropic(model anthropicsdk.ModelInfo) CatalogModel {
	return CatalogModel{
		ID:              strings.TrimSpace(model.ID),
		Name:            firstNonBlank(strings.TrimSpace(model.DisplayName), strings.TrimSpace(model.ID)),
		ContextSize:     int(model.MaxInputTokens),
		MaxInputTokens:  int(model.MaxInputTokens),
		MaxOutputTokens: int(model.MaxTokens),
		Reasoning: model.Capabilities.Thinking.Supported ||
			model.Capabilities.Effort.Supported,
		Vision:      model.Capabilities.ImageInput.Supported,
		VisionKnown: true,
	}
}

func catalogModelFromGoogle(model genai.Model) (CatalogModel, bool) {
	modelID := strings.TrimSpace(strings.TrimPrefix(model.Name, "models/"))
	if modelID == "" {
		modelID = strings.TrimSpace(model.DisplayName)
	}
	if modelID == "" {
		return CatalogModel{}, false
	}
	if len(model.SupportedActions) > 0 && !slices.Contains(model.SupportedActions, "generateContent") {
		return CatalogModel{}, false
	}
	if !isGeminiChatModel(modelID) {
		return CatalogModel{}, false
	}
	return CatalogModel{
		ID:              modelID,
		Name:            firstNonBlank(strings.TrimSpace(model.DisplayName), modelID),
		ContextSize:     int(model.InputTokenLimit),
		MaxInputTokens:  int(model.InputTokenLimit),
		MaxOutputTokens: int(model.OutputTokenLimit),
		Reasoning:       model.Thinking,
	}, true
}
