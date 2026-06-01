package mistralreasoning

import (
	"reflect"
	"testing"
)

func TestSupportedVariants(t *testing.T) {
	tests := []struct {
		name string
		ref  Model
		want []string
	}{
		{name: "small latest", ref: Model{ProviderID: "mistral", ModelID: "mistral-small-latest"}, want: []string{"none", "high"}},
		{name: "small 4", ref: Model{ProviderID: "mistral", ModelID: " mistral-small-4 "}, want: []string{"none", "high"}},
		{name: "medium 2604", ref: Model{ProviderID: "mistral", ModelID: "mistral-medium-2604"}, want: []string{"none", "high"}},
		{name: "unsupported model", ref: Model{ProviderID: "mistral", ModelID: "mistral-large-latest"}, want: nil},
		{name: "non mistral provider", ref: Model{ProviderID: "openrouter", ModelID: "mistral-small-latest"}, want: nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := SupportedVariants(tt.ref); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("SupportedVariants() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestEffortForVariant(t *testing.T) {
	tests := []struct {
		name            string
		ref             Model
		variant         string
		wantEffort      string
		wantOK          bool
		wantUnsupported bool
	}{
		{name: "empty", ref: Model{ProviderID: "mistral", ModelID: "mistral-small-latest"}, variant: ""},
		{name: "normalizes high", ref: Model{ProviderID: "mistral", ModelID: "mistral-small-latest"}, variant: " HIGH ", wantEffort: "high", wantOK: true},
		{name: "supports none", ref: Model{ProviderID: "mistral", ModelID: "mistral-small-latest"}, variant: "none", wantEffort: "none", wantOK: true},
		{name: "unsupported variant", ref: Model{ProviderID: "mistral", ModelID: "mistral-small-latest"}, variant: "medium", wantUnsupported: true},
		{name: "unsupported model", ref: Model{ProviderID: "mistral", ModelID: "mistral-large-latest"}, variant: "high"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotEffort, gotOK, gotUnsupported := EffortForVariant(tt.ref, tt.variant)
			if gotEffort != tt.wantEffort || gotOK != tt.wantOK || gotUnsupported != tt.wantUnsupported {
				t.Fatalf("EffortForVariant() = (%q, %v, %v), want (%q, %v, %v)", gotEffort, gotOK, gotUnsupported, tt.wantEffort, tt.wantOK, tt.wantUnsupported)
			}
		})
	}
}

func TestSupportsNativeReasoning(t *testing.T) {
	tests := []struct {
		name string
		ref  Model
		want bool
	}{
		{name: "magistral small", ref: Model{ProviderID: "mistral", ModelID: "magistral-small-latest"}, want: true},
		{name: "magistral medium", ref: Model{ProviderID: "mistral", ModelID: " magistral-medium "}, want: true},
		{name: "non native mistral", ref: Model{ProviderID: "mistral", ModelID: "mistral-small-latest"}},
		{name: "non mistral provider", ref: Model{ProviderID: "openrouter", ModelID: "magistral-small-latest"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := SupportsNativeReasoning(tt.ref); got != tt.want {
				t.Fatalf("SupportsNativeReasoning() = %v, want %v", got, tt.want)
			}
		})
	}
}
