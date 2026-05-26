package provider

import "testing"

func BenchmarkSystemPromptUserOverrideMiss(b *testing.B) {
	b.Setenv("XDG_CONFIG_HOME", b.TempDir())

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if got := SystemPrompt("github-copilot", "gpt-5"); got == "" {
			b.Fatal("SystemPrompt() returned empty string")
		}
	}
}
