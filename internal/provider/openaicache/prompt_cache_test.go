package openaicache

import (
	"errors"
	"strings"
	"testing"
)

func TestValidateRetention(t *testing.T) {
	for _, value := range []string{"", RetentionInMemory, Retention24h, " 24h "} {
		if err := ValidateRetention(value); err != nil {
			t.Fatalf("ValidateRetention(%q) error = %v", value, err)
		}
	}
	if err := ValidateRetention("forever"); !errors.Is(err, ErrRetentionInvalid) {
		t.Fatalf("ValidateRetention() error = %v, want ErrRetentionInvalid", err)
	}
}

func TestNormalizeRetention(t *testing.T) {
	if got := NormalizeRetention(" 24h "); got != Retention24h {
		t.Fatalf("NormalizeRetention() = %q, want %q", got, Retention24h)
	}
	if got := NormalizeRetention("forever"); got != "" {
		t.Fatalf("NormalizeRetention() = %q, want empty for invalid value", got)
	}
}

func TestKey(t *testing.T) {
	input := KeyInput{
		ModelID:      "gpt-5",
		AgentID:      "engineer",
		StaticPrompt: "Build carefully",
		Tools: []Tool{{
			Name:        "read",
			Description: "Read files",
			InputSchema: `{"type":"object"}`,
		}},
	}
	first := Key(input)
	second := Key(input)
	if first == "" || !strings.HasPrefix(first, "kodacode-") {
		t.Fatalf("Key() = %q, want kodacode key", first)
	}
	if first != second {
		t.Fatalf("Key() = %q then %q, want stable key", first, second)
	}
	if got := Key(KeyInput{}); got != "" {
		t.Fatalf("Key(empty) = %q, want empty", got)
	}
}
