package bootstrap

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEnsureDefaults_CreatesStructure(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	EnsureDefaults()

	base := filepath.Join(tmp, "kodacode")
	for _, dir := range []string{base, filepath.Join(base, "agents"), filepath.Join(base, "themes")} {
		if info, err := os.Stat(dir); err != nil || !info.IsDir() {
			t.Errorf("expected directory %s to exist", dir)
		}
	}

	cfgPath := filepath.Join(base, "config.yaml")
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("config.yaml not created: %v", err)
	}
	if len(data) == 0 {
		t.Error("config.yaml is empty")
	}
}

func TestEnsureDefaults_Idempotent(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	EnsureDefaults()

	cfgPath := filepath.Join(tmp, "kodacode", "config.yaml")
	// Write custom content.
	custom := []byte("# my custom config\n")
	if err := os.WriteFile(cfgPath, custom, 0o644); err != nil {
		t.Fatal(err)
	}

	// Run again — should NOT overwrite.
	EnsureDefaults()

	data, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != string(custom) {
		t.Error("EnsureDefaults overwrote existing config.yaml")
	}
}
