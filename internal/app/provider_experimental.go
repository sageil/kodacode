package app

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/sageil/kodacode/internal/provider"
)

var ErrExperimentalProviderNotEnabled = errors.New("experimental provider is not enabled")

// Experiemental Delete before Release
type ExperimentalProviderState struct {
	Enabled bool
	BaseURL string
}

// Experiemental Delete before Release
type ExperimentalProviderRegistration struct {
	ID          string
	State       func() (ExperimentalProviderState, error)
	BuildClient func() (provider.Client, error)
}

var (
	experimentalProvidersMu sync.RWMutex
	experimentalProviders   = map[string]ExperimentalProviderRegistration{}
)

// Experiemental Delete before Release
func RegisterExperimentalProvider(registration ExperimentalProviderRegistration) {
	id := strings.TrimSpace(registration.ID)
	if id == "" {
		panic("experimental provider id is required")
	}
	if registration.State == nil {
		panic(fmt.Sprintf("experimental provider %q state resolver is required", id))
	}
	if registration.BuildClient == nil {
		panic(fmt.Sprintf("experimental provider %q client builder is required", id))
	}

	experimentalProvidersMu.Lock()
	defer experimentalProvidersMu.Unlock()
	if _, exists := experimentalProviders[id]; exists {
		panic(fmt.Sprintf("experimental provider %q already registered", id))
	}
	registration.ID = id
	experimentalProviders[id] = registration
}

func experimentalProviderRegistration(providerID string) (ExperimentalProviderRegistration, bool) {
	providerID = strings.TrimSpace(providerID)
	if providerID == "" {
		return ExperimentalProviderRegistration{}, false
	}
	experimentalProvidersMu.RLock()
	defer experimentalProvidersMu.RUnlock()
	registration, ok := experimentalProviders[providerID]
	return registration, ok
}

func experimentalProviderIDs() []string {
	experimentalProvidersMu.RLock()
	defer experimentalProvidersMu.RUnlock()
	ids := make([]string, 0, len(experimentalProviders))
	for providerID := range experimentalProviders {
		ids = append(ids, providerID)
	}
	sort.Strings(ids)
	return ids
}

func experimentalProviderState(providerID string) (ExperimentalProviderState, bool, error) {
	registration, ok := experimentalProviderRegistration(providerID)
	if !ok {
		return ExperimentalProviderState{}, false, nil
	}
	state, err := registration.State()
	if err != nil {
		return ExperimentalProviderState{}, true, err
	}
	if strings.TrimSpace(state.BaseURL) == "" {
		state.BaseURL = "experimental"
	}
	return state, true, nil
}

func validateExperimentalProvider(providerID string) (bool, error) {
	state, ok, err := experimentalProviderState(providerID)
	if !ok {
		return false, nil
	}
	if err != nil {
		return true, err
	}
	if !state.Enabled {
		return true, fmt.Errorf("%w: %s", ErrExperimentalProviderNotEnabled, providerID)
	}
	return true, nil
}

func buildExperimentalProviderClient(providerID string) (provider.Client, bool, error) {
	registration, ok := experimentalProviderRegistration(providerID)
	if !ok {
		return nil, false, nil
	}
	state, err := registration.State()
	if err != nil {
		return nil, true, err
	}
	if !state.Enabled {
		return nil, true, fmt.Errorf("%w: %s", ErrExperimentalProviderNotEnabled, providerID)
	}
	client, err := registration.BuildClient()
	if err != nil {
		return nil, true, err
	}
	if client == nil {
		return nil, true, fmt.Errorf("experimental provider %q returned a nil client", providerID)
	}
	return client, true, nil
}
