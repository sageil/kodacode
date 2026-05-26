package app

import (
	"testing"

	"github.com/sageil/kodacode/internal/provider"
)

func TestNativeContinuationContractForRequestUsesTransportFamily(t *testing.T) {
	tests := []struct {
		name    string
		request provider.Request
		want    string
	}{
		{
			name:    "anthropic native loop",
			request: provider.Request{Model: provider.ModelRef{ProviderID: "anthropic", ModelID: "claude-sonnet-4-6"}},
			want:    "anthropic_tool_loop",
		},
		{
			name:    "google native loop",
			request: provider.Request{Model: provider.ModelRef{ProviderID: "google", ModelID: "gemini-3-pro"}},
			want:    "google_tool_loop",
		},
		{
			name:    "openai native loop",
			request: provider.Request{Model: provider.ModelRef{ProviderID: "openai", ModelID: "gpt-5"}},
			want:    "openai_tool_loop",
		},
		{
			name:    "github copilot uses openai loop",
			request: provider.Request{Model: provider.ModelRef{ProviderID: "github-copilot", ModelID: "claude-sonnet-4"}},
			want:    "openai_tool_loop",
		},
		{
			name:    "nvidia uses openai loop",
			request: provider.Request{Model: provider.ModelRef{ProviderID: "nvidia", ModelID: "openai/gpt-oss-20b"}},
			want:    "openai_tool_loop",
		},
		{
			name:    "compatible provider uses openai loop",
			request: provider.Request{Model: provider.ModelRef{ProviderID: "openrouter", ModelID: "openai/gpt-4.1"}},
			want:    "openai_tool_loop",
		},
		{
			name: "deepseek reasoning uses reasoning replay loop",
			request: provider.Request{
				Model:           provider.ModelRef{ProviderID: "openrouter", ModelID: "deepseek/deepseek-v4-pro"},
				ThinkingEnabled: true,
			},
			want: "openai_reasoning_tool_loop",
		},
		{
			name: "qwen reasoning uses reasoning replay loop",
			request: provider.Request{
				Model:           provider.ModelRef{ProviderID: "openrouter", ModelID: "qwen/qwen3.5-32b"},
				ThinkingEnabled: true,
			},
			want: "openai_reasoning_tool_loop",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := nativeContinuationContractForRequest(tc.request); got != tc.want {
				t.Fatalf("nativeContinuationContractForRequest() = %q, want %q", got, tc.want)
			}
		})
	}
}
