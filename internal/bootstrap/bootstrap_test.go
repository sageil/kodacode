package bootstrap

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sageil/kodacode/internal/agent"
	"github.com/sageil/kodacode/internal/tui/theme"
)

func TestEnsureDefaultsCreatesStructureAndDataArtifacts(t *testing.T) {
	configHome := filepath.Join(t.TempDir(), "config")
	dataHome := filepath.Join(t.TempDir(), "data")
	t.Setenv("XDG_CONFIG_HOME", configHome)
	t.Setenv("XDG_DATA_HOME", dataHome)

	if err := EnsureDefaults(); err != nil {
		t.Fatalf("EnsureDefaults() error = %v", err)
	}

	base := filepath.Join(configHome, "kodacode")
	for _, dir := range []string{
		base,
		filepath.Join(base, "agents"),
		filepath.Join(base, "prompts"),
		filepath.Join(base, "themes"),
	} {
		info, err := os.Stat(dir)
		if err != nil {
			t.Fatalf("Stat(%q) error = %v", dir, err)
		}
		if !info.IsDir() {
			t.Fatalf("%q is not a directory", dir)
		}
	}

	assertNonEmptyFile(t, filepath.Join(base, "config.yaml"))
	assertFileExists(t, filepath.Join(base, "auth.yaml"))
	assertFileContains(t, filepath.Join(base, "config.yaml"), "theme: ayu-dark")
	assertFileContains(t, filepath.Join(base, "config.yaml"), "yaml-language-server: $schema=https://raw.githubusercontent.com/sageil/kodacode/main/schema/config.schema.json")

	assertCopiedFiles(t, agent.BuiltinAgentFS(), ".", filepath.Join(base, "agents"))
	assertCopiedFiles(t, theme.BuiltinThemeFS(), "themes", filepath.Join(base, "themes"))

	for _, name := range []string{"default.txt", "openai.txt", "anthropic.txt", "gemini.txt", "deepseek.txt"} {
		assertNonEmptyFile(t, filepath.Join(base, "prompts", name))
	}

	assertFileExists(t, filepath.Join(dataHome, "kodacode", observabilityLogName))
	assertFileExists(t, filepath.Join(dataHome, "kodacode", "kodacode.db"))
}

func TestEnsureDefaultsDoesNotOverwriteExistingFiles(t *testing.T) {
	configHome := filepath.Join(t.TempDir(), "config")
	dataHome := filepath.Join(t.TempDir(), "data")
	t.Setenv("XDG_CONFIG_HOME", configHome)
	t.Setenv("XDG_DATA_HOME", dataHome)

	if err := EnsureDefaults(); err != nil {
		t.Fatalf("EnsureDefaults() error = %v", err)
	}

	base := filepath.Join(configHome, "kodacode")
	configPath := filepath.Join(base, "config.yaml")
	authPath := filepath.Join(base, "auth.yaml")
	agentPath := filepath.Join(base, "agents", "builder.md")

	customConfig := []byte("version: 1\nlogging:\n  debug: true\n")
	customAuth := []byte("openai:\n  type: api\n  access: \"test\"\n")
	customAgent := []byte("custom agent\n")

	if err := os.WriteFile(configPath, customConfig, 0o600); err != nil {
		t.Fatalf("WriteFile(config) error = %v", err)
	}
	if err := os.WriteFile(authPath, customAuth, 0o600); err != nil {
		t.Fatalf("WriteFile(auth) error = %v", err)
	}
	if err := os.WriteFile(agentPath, customAgent, 0o644); err != nil {
		t.Fatalf("WriteFile(agent) error = %v", err)
	}

	if err := EnsureDefaults(); err != nil {
		t.Fatalf("EnsureDefaults() second run error = %v", err)
	}

	assertFileContent(t, configPath, string(customConfig))
	assertFileContent(t, authPath, string(customAuth))
	assertFileContent(t, agentPath, string(customAgent))
	assertFileExists(t, filepath.Join(dataHome, "kodacode", debugLogName))
}

func TestEnsureDefaultsRespectsCustomDataPaths(t *testing.T) {
	configHome := filepath.Join(t.TempDir(), "config")
	dataHome := filepath.Join(t.TempDir(), "data")
	customLogs := filepath.Join(t.TempDir(), "logs")
	customDB := filepath.Join(t.TempDir(), "state", "sessions.db")
	t.Setenv("XDG_CONFIG_HOME", configHome)
	t.Setenv("XDG_DATA_HOME", dataHome)

	base := filepath.Join(configHome, "kodacode")
	if err := os.MkdirAll(base, 0o700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	config := "version: 1\nlogging:\n  dir: " + customLogs + "\nsessions:\n  db_path: " + customDB + "\n"
	if err := os.WriteFile(filepath.Join(base, "config.yaml"), []byte(config), 0o600); err != nil {
		t.Fatalf("WriteFile(config) error = %v", err)
	}

	if err := EnsureDefaults(); err != nil {
		t.Fatalf("EnsureDefaults() error = %v", err)
	}

	assertFileExists(t, filepath.Join(customLogs, observabilityLogName))
	assertFileExists(t, customDB)
}

const (
	observabilityLogName = "ops.log"
	debugLogName         = "debug.log"
)

func assertCopiedFiles(t *testing.T, fsys fs.FS, root, dest string) {
	t.Helper()

	entries, err := fs.ReadDir(fsys, root)
	if err != nil {
		t.Fatalf("ReadDir(%q) error = %v", root, err)
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		assertNonEmptyFile(t, filepath.Join(dest, entry.Name()))
	}
}

func assertNonEmptyFile(t *testing.T, path string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", path, err)
	}
	if len(data) == 0 {
		t.Fatalf("%q is empty", path)
	}
}

func assertFileExists(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("Stat(%q) error = %v", path, err)
	}
}

func assertFileContent(t *testing.T, path, want string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", path, err)
	}
	if got := string(data); got != want {
		t.Fatalf("content mismatch for %q:\n got: %q\nwant: %q", path, got, want)
	}
}

func assertFileContains(t *testing.T, path, want string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", path, err)
	}
	if !strings.Contains(string(data), want) {
		t.Fatalf("%q does not contain %q", path, want)
	}
}
