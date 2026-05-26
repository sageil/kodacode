package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

func fetchOpenAIModelCatalogProvider(ctx context.Context, source RemoteModelCatalogProvider) (*catalogProvider, error) {
	var providers []*catalogProvider
	var errs []error

	if strings.TrimSpace(source.APIKey) != "" {
		apiProvider, err := fetchOpenAIAPIKeyModelCatalogProvider(ctx, source)
		if err != nil {
			errs = append(errs, err)
		} else if apiProvider != nil {
			providers = append(providers, apiProvider)
		}
	}
	if source.OAuth != nil {
		oauthProvider, err := fetchOpenAIOAuthModelCatalogProvider(ctx, source)
		if err != nil {
			errs = append(errs, err)
		} else if oauthProvider != nil {
			providers = append(providers, oauthProvider)
		}
	}

	switch len(providers) {
	case 0:
		return nil, errors.Join(errs...)
	case 1:
		return providers[0], errors.Join(errs...)
	default:
		merged := cloneProvider(*providers[0])
		for _, next := range providers[1:] {
			merged = mergeCatalogProviderModels(merged, *next)
		}
		return &merged, errors.Join(errs...)
	}
}

func fetchOpenAICompatibleModelCatalogProvider(ctx context.Context, source RemoteModelCatalogProvider) (*catalogProvider, error) {
	authorizer, err := newOpenAICompatibleAuthorizer(strings.TrimSpace(source.APIKey), strings.TrimSpace(source.BaseURL))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", source.ID, err)
	}
	httpClient := source.HTTPClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return fetchAuthorizedOpenAIModelCatalogProvider(ctx, source, httpClient, authorizer)
}

func fetchAuthorizedOpenAIModelCatalogProvider(
	ctx context.Context,
	source RemoteModelCatalogProvider,
	httpClient *http.Client,
	authorizer openAIRequestAuthorizer,
) (*catalogProvider, error) {
	root := modelCatalogRoot(strings.TrimSpace(source.BaseURL))
	if root == "" {
		return nil, fmt.Errorf("%s: model catalog base_url is required", source.ID)
	}

	ctx, cancel := context.WithTimeout(ctx, defaultModelCatalogTimeout)
	defer cancel()

	resp, err := doOpenAIAuthorizedRequest(ctx, httpClient, authorizer, source.ID+" models", func(ctx context.Context) (*http.Request, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(root, "/")+"/models", nil)
		if err != nil {
			return nil, fmt.Errorf("%s: create models request: %w", source.ID, err)
		}
		req.Header.Set("Accept", "application/json")
		return req, nil
	})
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("%s: read models body: %w", source.ID, err)
	}
	models, ok := parseOpenAIModelsResponse(body)
	if !ok {
		return nil, fmt.Errorf("%s: unsupported models response shape", source.ID)
	}
	return catalogProviderFromModels(source, models), nil
}

func fetchOpenAIAPIKeyModelCatalogProvider(ctx context.Context, source RemoteModelCatalogProvider) (*catalogProvider, error) {
	httpClient := source.HTTPClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	apiSource := source
	apiSource.BaseURL = openAIPlatformCatalogBaseURL(source.BaseURL)
	authorizer, err := newOpenAIAuthorizer(OpenAIConfig{
		APIKey:  strings.TrimSpace(source.APIKey),
		BaseURL: apiSource.BaseURL,
	}, httpClient)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", source.ID, err)
	}
	return fetchAuthorizedOpenAIModelCatalogProvider(ctx, apiSource, httpClient, authorizer)
}

func fetchOpenAIOAuthModelCatalogProvider(ctx context.Context, source RemoteModelCatalogProvider) (*catalogProvider, error) {
	models, err := fetchOpenAICodexModels(ctx, source)
	if err != nil {
		return nil, err
	}
	return catalogProviderFromModels(source, models), nil
}

func fetchOpenAICodexModels(ctx context.Context, source RemoteModelCatalogProvider) ([]CatalogModel, error) {
	clientVersion := openAICodexClientVersion(source.OAuth)
	if clientVersion == "" {
		return nil, errors.New("openai codex client version is required")
	}

	httpClient := source.HTTPClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	authorizer, err := newOpenAIAuthorizer(OpenAIConfig{
		OAuth:   source.OAuth,
		BaseURL: openAICodexCatalogBaseURL(source.BaseURL),
	}, httpClient)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", source.ID, err)
	}

	ctx, cancel := context.WithTimeout(ctx, defaultModelCatalogTimeout)
	defer cancel()

	modelsURL := strings.TrimRight(openAICodexCatalogBaseURL(source.BaseURL), "/") + "/models"
	resp, err := doOpenAIAuthorizedRequest(ctx, httpClient, authorizer, source.ID+" codex models", func(ctx context.Context) (*http.Request, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, modelsURL, nil)
		if err != nil {
			return nil, fmt.Errorf("%s: create codex models request: %w", source.ID, err)
		}
		query := req.URL.Query()
		query.Set("client_version", clientVersion)
		req.URL.RawQuery = query.Encode()
		req.Header.Set("Accept", "application/json")
		return req, nil
	})
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("%s: read codex models body: %w", source.ID, err)
	}
	models := parseOpenAICodexModelsResponse(body)
	if len(models) == 0 {
		return nil, errors.New("openai codex models: empty response")
	}
	return models, nil
}

func parseOpenAICodexModelsResponse(body []byte) []CatalogModel {
	var payload struct {
		Models []struct {
			Slug                     string `json:"slug"`
			DisplayName              string `json:"display_name"`
			SupportedInAPI           bool   `json:"supported_in_api"`
			Visibility               string `json:"visibility"`
			DefaultReasoningLevel    string `json:"default_reasoning_level"`
			SupportedReasoningLevels []struct {
				Effort string `json:"effort"`
			} `json:"supported_reasoning_levels"`
		} `json:"models"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil
	}

	models := make([]CatalogModel, 0, len(payload.Models))
	for _, entry := range payload.Models {
		modelID := strings.TrimSpace(entry.Slug)
		if modelID == "" || !entry.SupportedInAPI {
			continue
		}
		visibility := strings.ToLower(strings.TrimSpace(entry.Visibility))
		if visibility != "" && visibility != "list" {
			continue
		}
		models = append(models, CatalogModel{
			ID:        modelID,
			Name:      firstNonBlank(strings.TrimSpace(entry.DisplayName), modelID),
			Reasoning: len(entry.SupportedReasoningLevels) > 0 || strings.TrimSpace(entry.DefaultReasoningLevel) != "",
		})
	}
	return models
}

func openAIPlatformCatalogBaseURL(baseURL string) string {
	root := modelCatalogRoot(strings.TrimSpace(baseURL))
	if root == "" || strings.Contains(root, "chatgpt.com/backend-api/codex") {
		return DefaultOpenAIBaseURL()
	}
	return root
}

func openAICodexCatalogBaseURL(baseURL string) string {
	root := modelCatalogRoot(strings.TrimSpace(baseURL))
	if root == "" {
		root = DefaultOpenAIOAuthBaseURL()
	}
	root = strings.TrimRight(root, "/")
	root = strings.TrimSuffix(root, "/responses")
	if !strings.HasSuffix(root, "/codex") {
		root = "https://chatgpt.com/backend-api/codex"
	}
	return root
}

func openAICodexClientVersion(oauth *OpenAIOAuthConfig) string {
	if version := strings.TrimSpace(os.Getenv("KODACODE_OPENAI_CLIENT_VERSION")); version != "" {
		return version
	}
	if oauth != nil {
		if version := strings.TrimSpace(oauth.Entry.ClientVersion); version != "" {
			return version
		}
		if oauth.Store != nil {
			if current := oauth.Store.Get("openai"); current != nil {
				if version := strings.TrimSpace(current.ClientVersion); version != "" {
					return version
				}
			}
		}
	}

	versionFile := filepath.Join(defaultCodexHome(), "version.json")
	data, err := os.ReadFile(versionFile)
	if err == nil {
		var payload struct {
			LatestVersion string `json:"latest_version"`
		}
		if json.Unmarshal(data, &payload) == nil {
			if version := strings.TrimSpace(payload.LatestVersion); version != "" {
				return version
			}
		}
	}
	return ""
}

func defaultCodexHome() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(os.TempDir(), ".codex")
	}
	return filepath.Join(home, ".codex")
}

func mergeCatalogProviderModels(primary, secondary catalogProvider) catalogProvider {
	merged := cloneProvider(primary)
	merged.ID = firstNonBlank(merged.ID, secondary.ID)
	merged.Name = firstNonBlank(merged.Name, secondary.Name, merged.ID)
	if merged.Models == nil {
		merged.Models = make(map[string]CatalogModel, len(secondary.Models))
	}
	for modelID, model := range secondary.Models {
		if existing, ok := merged.Models[modelID]; ok {
			merged.Models[modelID] = mergeCatalogModels(existing, model)
			continue
		}
		merged.Models[modelID] = model
	}
	return merged
}
