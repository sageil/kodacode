package app

import (
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/sageil/kodacode/internal/provider"
)

func TestUserFacingModelCatalogRefreshErrorAuth(t *testing.T) {
	err := errors.Join(
		fmtProviderCatalogError("openai", http.StatusUnauthorized, "Unauthorized"),
		fmtProviderCatalogError("togetherai", http.StatusUnauthorized, "Unauthorized"),
	)

	got := UserFacingModelCatalogRefreshError(err)
	if got == nil {
		t.Fatal("UserFacingModelCatalogRefreshError() = nil")
	}
	want := "Some model lists could not be refreshed. Check the provider access settings for OpenAI and Together AI."
	if got.Error() != want {
		t.Fatalf("error = %q, want %q", got.Error(), want)
	}
}

func TestUserFacingModelCatalogRefreshErrorTemporary(t *testing.T) {
	err := fmtProviderCatalogError("mistral", http.StatusTooManyRequests, "rate limit exceeded")
	got := UserFacingModelCatalogRefreshError(err)
	if got == nil {
		t.Fatal("UserFacingModelCatalogRefreshError() = nil")
	}
	want := "Mistral model list could not be refreshed right now. Try again in a moment."
	if got.Error() != want {
		t.Fatalf("error = %q, want %q", got.Error(), want)
	}
}

func fmtProviderCatalogError(providerID string, statusCode int, message string) error {
	return fmt.Errorf("%s models: %w", providerID, &provider.ProviderError{
		Message:    providerID + " models: " + message,
		StatusCode: statusCode,
	})
}
