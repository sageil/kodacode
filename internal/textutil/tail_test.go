package textutil

import "testing"

func TestAppendRuneTail(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		current string
		chunk   string
		limit   int
		want    string
	}{
		{
			name:    "no_limit_appends_all",
			current: "hello",
			chunk:   " world",
			limit:   0,
			want:    "hello world",
		},
		{
			name:    "under_limit_ascii_returns_combined",
			current: "hello",
			chunk:   " world",
			limit:   64,
			want:    "hello world",
		},
		{
			name:    "under_limit_utf8_returns_combined",
			current: "こん",
			chunk:   "にちは",
			limit:   8,
			want:    "こんにちは",
		},
		{
			name:    "over_limit_utf8_trims_by_rune",
			current: "ab界界",
			chunk:   "測定",
			limit:   4,
			want:    "界界測定",
		},
		{
			name:    "invalid_utf8_matches_rune_trimming_behavior",
			current: string([]byte{'a', 0xff}),
			chunk:   "bc",
			limit:   2,
			want:    "bc",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := AppendRuneTail(tt.current, tt.chunk, tt.limit); got != tt.want {
				t.Fatalf("AppendRuneTail(%q, %q, %d) = %q, want %q", tt.current, tt.chunk, tt.limit, got, tt.want)
			}
		})
	}
}

func TestStringAccumulator(t *testing.T) {
	t.Parallel()

	t.Run("seeds_existing_text", func(t *testing.T) {
		t.Parallel()
		var acc StringAccumulator
		if got := acc.Append("hello", " world"); got != "hello world" {
			t.Fatalf("Append() = %q, want %q", got, "hello world")
		}
		if got := acc.Append("ignored", "!"); got != "hello world!" {
			t.Fatalf("Append() second call = %q, want %q", got, "hello world!")
		}
	})

	t.Run("reset_drops_runtime_buffer", func(t *testing.T) {
		t.Parallel()
		var acc StringAccumulator
		_ = acc.Append("start", " more")
		acc.Reset()
		if got := acc.Append("again", "!"); got != "again!" {
			t.Fatalf("Append() after reset = %q, want %q", got, "again!")
		}
	})
}
