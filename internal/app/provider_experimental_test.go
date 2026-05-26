package app

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/sageil/kodacode/internal/provider"
)

const experimentalTestProviderID = "test-experimental-provider"

var (
	experimentalTestRegisterOnce sync.Once
	experimentalTestEnabled      bool
	experimentalTestStateErr     error
	experimentalTestBuildErr     error
	experimentalTestBuildCalls   int
)

type experimentalTestClient struct{}

func (experimentalTestClient) Stream(context.Context, provider.Request) (provider.Stream, error) {
	return provider.NewSliceStream(nil), nil
}

func registerExperimentalTestProvider() {
	experimentalTestRegisterOnce.Do(func() {
		RegisterExperimentalProvider(ExperimentalProviderRegistration{
			ID: experimentalTestProviderID,
			State: func() (ExperimentalProviderState, error) {
				if experimentalTestStateErr != nil {
					return ExperimentalProviderState{}, experimentalTestStateErr
				}
				return ExperimentalProviderState{
					Enabled: experimentalTestEnabled,
					BaseURL: "https://experimental.invalid",
				}, nil
			},
			BuildClient: func() (provider.Client, error) {
				experimentalTestBuildCalls++
				if experimentalTestBuildErr != nil {
					return nil, experimentalTestBuildErr
				}
				return experimentalTestClient{}, nil
			},
		})
	})
}

func resetExperimentalTestProvider() {
	experimentalTestEnabled = false
	experimentalTestStateErr = nil
	experimentalTestBuildErr = nil
	experimentalTestBuildCalls = 0
}

func TestConfigValidateAllowsEnabledExperimentalProvider(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	registerExperimentalTestProvider()
	resetExperimentalTestProvider()
	t.Cleanup(resetExperimentalTestProvider)
	experimentalTestEnabled = true

	err := (Config{
		ModelRoute: provider.ModelRoute{
			Primary: provider.ModelRef{ProviderID: experimentalTestProviderID, ModelID: "exp-model"},
		},
	}).Validate()
	if err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestConfigValidateRejectsDisabledExperimentalProvider(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	registerExperimentalTestProvider()
	resetExperimentalTestProvider()
	t.Cleanup(resetExperimentalTestProvider)

	err := (Config{
		ModelRoute: provider.ModelRoute{
			Primary: provider.ModelRef{ProviderID: experimentalTestProviderID, ModelID: "exp-model"},
		},
	}).Validate()
	if !errors.Is(err, ErrExperimentalProviderNotEnabled) {
		t.Fatalf("Validate() error = %v, want ErrExperimentalProviderNotEnabled", err)
	}
}

func TestBuildProviderClientForIDUsesExperimentalProvider(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	registerExperimentalTestProvider()
	resetExperimentalTestProvider()
	t.Cleanup(resetExperimentalTestProvider)
	experimentalTestEnabled = true

	client, err := buildProviderClientForID(Config{}, experimentalTestProviderID)
	if err != nil {
		t.Fatalf("buildProviderClientForID() error = %v", err)
	}
	if client == nil {
		t.Fatal("buildProviderClientForID() client = nil")
	}
	if experimentalTestBuildCalls != 1 {
		t.Fatalf("build calls = %d, want 1", experimentalTestBuildCalls)
	}
}

func TestConnectedProvidersIncludesEnabledExperimentalProvider(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	registerExperimentalTestProvider()
	resetExperimentalTestProvider()
	t.Cleanup(resetExperimentalTestProvider)
	experimentalTestEnabled = true

	connected := connectedProviders(Config{})
	if len(connected) != 1 {
		t.Fatalf("connected providers = %#v", connected)
	}
	if connected[0].ProviderID != experimentalTestProviderID {
		t.Fatalf("connected[0] = %#v", connected[0])
	}
	if connected[0].BaseURL != "https://experimental.invalid" {
		t.Fatalf("connected[0].BaseURL = %q", connected[0].BaseURL)
	}
}
