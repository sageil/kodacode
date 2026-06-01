package openaireasoning

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
		{
			name: "gpt 5 pro",
			ref:  Model{ProviderID: "openai", ModelID: "gpt-5-pro"},
			want: []string{"high"},
		},
		{
			name: "codex max",
			ref:  Model{ProviderID: "openai", ModelID: "gpt-5.1-codex-max"},
			want: []string{"none", "medium", "high", "xhigh"},
		},
		{
			name: "openai namespace through compatible provider",
			ref:  Model{ProviderID: "openrouter", ModelID: "openai/gpt-5.2-codex"},
			want: []string{"low", "medium", "high", "xhigh"},
		},
		{
			name: "nvidia gpt oss",
			ref:  Model{ProviderID: "nvidia", ModelID: "openai/gpt-oss-120b"},
			want: []string{"low", "medium", "high"},
		},
		{
			name: "nvidia gpt 5 unsupported",
			ref:  Model{ProviderID: "nvidia", ModelID: "gpt-5"},
			want: nil,
		},
		{
			name: "compatible provider without openai namespace unsupported",
			ref:  Model{ProviderID: "openrouter", ModelID: "gpt-5"},
			want: nil,
		},
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
		name        string
		ref         Model
		variant     string
		wantEffort  string
		wantOK      bool
		wantInvalid bool
	}{
		{
			name:       "normalizes supported variant",
			ref:        Model{ProviderID: "openai", ModelID: "gpt-5.2"},
			variant:    " HIGH ",
			wantEffort: "high",
			wantOK:     true,
		},
		{
			name:    "empty variant is unset",
			ref:     Model{ProviderID: "openai", ModelID: "gpt-5.2"},
			variant: " ",
		},
		{
			name:        "unsupported variant is invalid",
			ref:         Model{ProviderID: "openai", ModelID: "gpt-5.1"},
			variant:     "xhigh",
			wantInvalid: true,
		},
		{
			name:    "unsupported model ignores variant",
			ref:     Model{ProviderID: "openrouter", ModelID: "gpt-5.2"},
			variant: "high",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotEffort, gotOK, gotInvalid := EffortForVariant(tt.ref, tt.variant)
			if gotEffort != tt.wantEffort || gotOK != tt.wantOK || gotInvalid != tt.wantInvalid {
				t.Fatalf("EffortForVariant() = (%q, %v, %v), want (%q, %v, %v)", gotEffort, gotOK, gotInvalid, tt.wantEffort, tt.wantOK, tt.wantInvalid)
			}
		})
	}
}
