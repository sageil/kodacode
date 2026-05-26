package app

import (
	"errors"
	"strings"

	"github.com/sageil/kodacode/internal/provider"
)

func (r *Runtime) utilityProviderAvailable() utilityProviderAvailableFunc {
	if r == nil {
		return nil
	}
	return func(providerID string) bool {
		return utilityProviderConfigured(r.Config, providerID)
	}
}

func utilityProviderConfigured(config Config, providerID string) bool {
	providerID = strings.TrimSpace(providerID)
	if providerID == "" {
		return false
	}
	_, err := buildProviderClientForID(config, providerID)
	return err == nil || !errors.Is(err, provider.ErrProviderNotConfigured)
}
