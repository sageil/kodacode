package app

import (
	"errors"
	"fmt"
	"strings"

	"github.com/sageil/kodacode/internal/provider"
)

func UserFacingModelCatalogRefreshError(err error) error {
	if err == nil {
		return nil
	}
	issues := collectModelCatalogRefreshIssues(err)
	if len(issues) == 0 {
		return userVisibleModelCatalogError("Some model lists could not be refreshed. Check the provider settings or try again.")
	}

	providers := uniqueNonBlank(issueProviders(issues))
	switch summarizeModelCatalogIssueKinds(issues) {
	case turnIssueAuth:
		return userVisibleModelCatalogError(modelCatalogAuthMessage(providers))
	case turnIssueConnection, turnIssueBusy, turnIssueRateLimited:
		return userVisibleModelCatalogError(modelCatalogTemporaryMessage(providers))
	default:
		return userVisibleModelCatalogError(modelCatalogGenericMessage(providers))
	}
}

type userVisibleModelCatalogError string

func (e userVisibleModelCatalogError) Error() string {
	return string(e)
}

type modelCatalogRefreshIssue struct {
	provider string
	kind     turnIssueKind
}

func collectModelCatalogRefreshIssues(err error) []modelCatalogRefreshIssue {
	if err == nil {
		return nil
	}
	var issues []modelCatalogRefreshIssue
	var visit func(error)
	visit = func(current error) {
		if current == nil {
			return
		}
		type unwrapMany interface{ Unwrap() []error }
		if many, ok := current.(unwrapMany); ok {
			for _, next := range many.Unwrap() {
				visit(next)
			}
			return
		}
		if next := errors.Unwrap(current); next != nil {
			providerLabel := modelCatalogProviderLabel(current)
			kind := classifyTurnIssue(current)
			if providerLabel != "" || kind != turnIssueUnknown {
				issues = append(issues, modelCatalogRefreshIssue{
					provider: providerLabel,
					kind:     kind,
				})
			}
			visit(next)
			return
		}
		issues = append(issues, modelCatalogRefreshIssue{
			provider: modelCatalogProviderLabel(current),
			kind:     classifyTurnIssue(current),
		})
	}
	visit(err)
	return normalizeModelCatalogIssues(issues)
}

func normalizeModelCatalogIssues(issues []modelCatalogRefreshIssue) []modelCatalogRefreshIssue {
	if len(issues) == 0 {
		return nil
	}
	out := make([]modelCatalogRefreshIssue, 0, len(issues))
	seen := map[string]struct{}{}
	for _, issue := range issues {
		key := issue.provider + "|" + string(issue.kind)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, issue)
	}
	return out
}

func summarizeModelCatalogIssueKinds(issues []modelCatalogRefreshIssue) turnIssueKind {
	if len(issues) == 0 {
		return turnIssueUnknown
	}
	kind := issues[0].kind
	for _, issue := range issues[1:] {
		if issue.kind != kind {
			return turnIssueProviderRequest
		}
	}
	return kind
}

func issueProviders(issues []modelCatalogRefreshIssue) []string {
	if len(issues) == 0 {
		return nil
	}
	out := make([]string, 0, len(issues))
	for _, issue := range issues {
		if strings.TrimSpace(issue.provider) != "" {
			out = append(out, issue.provider)
		}
	}
	return out
}

func uniqueNonBlank(values []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func modelCatalogAuthMessage(providers []string) string {
	if len(providers) == 1 {
		return providers[0] + " model list could not be refreshed. Check the provider access settings."
	}
	if len(providers) > 1 {
		return fmt.Sprintf("Some model lists could not be refreshed. Check the provider access settings for %s.", joinProviderLabels(providers))
	}
	return "Some model lists could not be refreshed. Check the provider access settings."
}

func modelCatalogTemporaryMessage(providers []string) string {
	if len(providers) == 1 {
		return providers[0] + " model list could not be refreshed right now. Try again in a moment."
	}
	return "Some model lists could not be refreshed right now. Try again in a moment."
}

func modelCatalogGenericMessage(providers []string) string {
	if len(providers) == 1 {
		return providers[0] + " model list could not be refreshed."
	}
	if len(providers) > 1 {
		return fmt.Sprintf("Some model lists could not be refreshed: %s.", joinProviderLabels(providers))
	}
	return "Some model lists could not be refreshed."
}

func joinProviderLabels(providers []string) string {
	switch len(providers) {
	case 0:
		return ""
	case 1:
		return providers[0]
	case 2:
		return providers[0] + " and " + providers[1]
	default:
		return strings.Join(providers[:len(providers)-1], ", ") + ", and " + providers[len(providers)-1]
	}
}

func modelCatalogProviderLabel(err error) string {
	if err == nil {
		return ""
	}
	var providerErr *provider.ProviderError
	if errors.As(err, &providerErr) && providerErr != nil {
		if label := providerLabelFromErrorMessage(providerErr.Message); label != "" {
			return label
		}
	}
	return providerLabelFromErrorMessage(err.Error())
}

func providerLabelFromErrorMessage(message string) string {
	lower := strings.ToLower(strings.TrimSpace(message))
	switch {
	case strings.HasPrefix(lower, "openai "):
		return "OpenAI"
	case strings.HasPrefix(lower, "anthropic "):
		return "Anthropic"
	case strings.HasPrefix(lower, "google "):
		return "Google"
	case strings.HasPrefix(lower, "github-copilot "):
		return "GitHub Copilot"
	case strings.HasPrefix(lower, "nvidia "):
		return "NVIDIA"
	case strings.HasPrefix(lower, "deepseek "):
		return "DeepSeek"
	case strings.HasPrefix(lower, "mistral "):
		return "Mistral"
	case strings.HasPrefix(lower, "togetherai "):
		return "Together AI"
	default:
		return ""
	}
}
