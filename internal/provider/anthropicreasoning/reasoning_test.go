package anthropicreasoning

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
			name: "effort model",
			ref:  Model{ProviderID: "anthropic", ModelID: "claude-opus-4-5"},
			want: []string{"low", "medium", "high", "xhigh", "max"},
		},
		{
			name: "thinking output model",
			ref:  Model{ProviderID: "anthropic", ModelID: "claude-sonnet-4-6"},
			want: []string{"low", "medium", "high", "xhigh", "max"},
		},
		{
			name: "unsupported model",
			ref:  Model{ProviderID: "anthropic", ModelID: "claude-sonnet-4-5"},
			want: nil,
		},
		{
			name: "non anthropic provider",
			ref:  Model{ProviderID: "github-copilot", ModelID: "claude-sonnet-4-6"},
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

func TestSupportsThinkingOutput(t *testing.T) {
	tests := []struct {
		name string
		ref  Model
		want bool
	}{
		{
			name: "mythos",
			ref:  Model{ProviderID: "anthropic", ModelID: " claude-mythos-preview-20260101 "},
			want: true,
		},
		{
			name: "opus 4 5 supports effort only",
			ref:  Model{ProviderID: "anthropic", ModelID: "claude-opus-4-5"},
		},
		{
			name: "opus 4 6",
			ref:  Model{ProviderID: "anthropic", ModelID: "claude-opus-4-6"},
			want: true,
		},
		{
			name: "non anthropic provider",
			ref:  Model{ProviderID: "github-copilot", ModelID: "claude-opus-4-6"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := SupportsThinkingOutput(tt.ref); got != tt.want {
				t.Fatalf("SupportsThinkingOutput() = %v, want %v", got, tt.want)
			}
		})
	}
}
