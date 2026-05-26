package agent

import (
	"path/filepath"
	"testing"
)

func TestDefaultGlobalAgentsDirFallsBackToDotConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("HOME", home)

	got, err := defaultGlobalAgentsDir()
	if err != nil {
		t.Fatalf("defaultGlobalAgentsDir() error = %v", err)
	}

	want := filepath.Join(home, ".config", "kodacode", "agents")
	if got != want {
		t.Fatalf("defaultGlobalAgentsDir() = %q, want %q", got, want)
	}
}
