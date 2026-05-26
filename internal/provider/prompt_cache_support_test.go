package provider

import "testing"

func TestPromptCacheSupportForModel(t *testing.T) {
	tests := []struct {
		name       string
		ref        ModelRef
		want       PromptCacheSupport
		wantReason bool
	}{
		{
			name: "openai",
			ref:  ModelRef{ProviderID: "openai", ModelID: "gpt-5"},
			want: PromptCacheSupport{
				ProviderID:                  "openai",
				RequestHintsSupported:       true,
				CacheReadReportingSupported: true,
			},
		},
		{
			name: "openai alias",
			ref:  ModelRef{ProviderID: "openai-codex", ModelID: "gpt-5"},
			want: PromptCacheSupport{
				ProviderID:                  "openai",
				RequestHintsSupported:       true,
				CacheReadReportingSupported: true,
			},
		},
		{
			name: "anthropic",
			ref:  ModelRef{ProviderID: "anthropic", ModelID: "claude-sonnet-4-6"},
			want: PromptCacheSupport{
				ProviderID:                   "anthropic",
				RequestHintsSupported:        true,
				CacheReadReportingSupported:  true,
				CacheWriteReportingSupported: true,
			},
		},
		{
			name: "google",
			ref:  ModelRef{ProviderID: "google", ModelID: "gemini-3-pro"},
			want: PromptCacheSupport{
				ProviderID:                  "google",
				CacheReadReportingSupported: true,
			},
			wantReason: true,
		},
		{
			name:       "unsupported provider",
			ref:        ModelRef{ProviderID: "local", ModelID: "llama"},
			want:       PromptCacheSupport{ProviderID: "local"},
			wantReason: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := PromptCacheSupportForModel(tt.ref)
			reason := got.UnsupportedReason
			got.UnsupportedReason = ""
			if got != tt.want {
				t.Fatalf("PromptCacheSupportForModel() = %#v, want %#v", got, tt.want)
			}
			if tt.wantReason && reason == "" {
				t.Fatal("UnsupportedReason = empty, want reason")
			}
			if !tt.wantReason && reason != "" {
				t.Fatalf("UnsupportedReason = %q, want empty", reason)
			}
		})
	}
}
