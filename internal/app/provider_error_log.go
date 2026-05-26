package app

import (
	"errors"

	"github.com/sageil/kodacode/internal/provider"
)

func providerErrorLogFields(err error) []any {
	var providerErr *provider.ProviderError
	if !errors.As(err, &providerErr) || providerErr == nil {
		return nil
	}
	fields := make([]any, 0, 6)
	if providerErr.StatusCode > 0 {
		fields = append(fields, "provider_status", providerErr.StatusCode)
	}
	fields = append(fields, "provider_retryable", providerErr.Retryable)
	if providerErr.RetryAfter > 0 {
		fields = append(fields, "provider_retry_after_ms", providerErr.RetryAfter.Milliseconds())
	}
	if providerErr.AuthDebug != nil {
		auth := providerErr.AuthDebug
		if auth.ProviderID != "" {
			fields = append(fields, "auth_provider", auth.ProviderID)
		}
		if auth.FailurePhase != "" {
			fields = append(fields, "auth_failure_phase", auth.FailurePhase)
		}
		if auth.AccessHash != "" {
			fields = append(fields, "auth_access_hash", auth.AccessHash)
		}
		if auth.AccessExpiresAt > 0 {
			fields = append(fields, "auth_access_expires_at", auth.AccessExpiresAt)
			fields = append(fields, "auth_access_expires_in_ms", auth.AccessExpiresInMs)
		}
		if auth.TokenUpdateAt > 0 {
			fields = append(fields, "auth_token_update_at", auth.TokenUpdateAt)
			fields = append(fields, "auth_token_update_age_ms", auth.TokenUpdateAgeMs)
		}
		if auth.TokenUpdateSource != "" {
			fields = append(fields, "auth_token_update_source", auth.TokenUpdateSource)
			fields = append(fields, "auth_token_update_forced", auth.TokenUpdateForced)
			fields = append(fields, "auth_token_update_changed", auth.TokenUpdateChanged)
		}
	}
	return fields
}
