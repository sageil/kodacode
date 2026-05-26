package theme

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestNamesIncludesThemesFromXDGConfigHome(t *testing.T) {
	writeUserTheme(t, "xdg-theme")

	names, err := Names()
	if err != nil {
		t.Fatalf("Names() error = %v", err)
	}
	if !slices.Contains(names, "xdg-theme") {
		t.Fatalf("Names() = %#v, missing xdg-theme", names)
	}
}

func TestLoadUsesThemesFromXDGConfigHome(t *testing.T) {
	writeUserTheme(t, "xdg-theme")

	loaded, err := Load("xdg-theme")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if loaded.Name != "xdg-theme" {
		t.Fatalf("theme name = %q, want xdg-theme", loaded.Name)
	}
	if loaded.Palette.Primary != "#112233" {
		t.Fatalf("primary = %q, want #112233", loaded.Palette.Primary)
	}
}

func TestNamesIncludeBuiltinAyuThemes(t *testing.T) {
	names, err := Names()
	if err != nil {
		t.Fatalf("Names() error = %v", err)
	}
	for _, want := range []string{
		"ayu-dark",
		"ayu-dark-unbordered",
		"ayu-mirage",
		"ayu-mirage-unbordered",
		"ayu-light",
		"ayu-light-unbordered",
	} {
		if !slices.Contains(names, want) {
			t.Fatalf("Names() = %#v, missing %q", names, want)
		}
	}
}

func TestNamesIncludeBuiltinOneDarkThemes(t *testing.T) {
	names, err := Names()
	if err != nil {
		t.Fatalf("Names() error = %v", err)
	}
	for _, want := range []string{
		"one-dark",
		"one-dark-pro",
		"one-dark-pro-flat",
		"one-dark-pro-darker",
		"one-dark-pro-mix",
		"one-dark-pro-night-flat",
	} {
		if !slices.Contains(names, want) {
			t.Fatalf("Names() = %#v, missing %q", names, want)
		}
	}
}

func TestNamesIncludeBuiltinGruvboxThemes(t *testing.T) {
	names, err := Names()
	if err != nil {
		t.Fatalf("Names() error = %v", err)
	}
	for _, want := range []string{
		"gruvbox-dark",
		"gruvbox-dark-hard",
		"gruvbox-dark-medium",
		"gruvbox-dark-soft",
		"gruvbox-light-hard",
		"gruvbox-light-medium",
		"gruvbox-light-soft",
	} {
		if !slices.Contains(names, want) {
			t.Fatalf("Names() = %#v, missing %q", names, want)
		}
	}
}

func TestNamesIncludeAdditionalBuiltinThemes(t *testing.T) {
	names, err := Names()
	if err != nil {
		t.Fatalf("Names() error = %v", err)
	}
	for _, want := range []string{
		"catppuccin-mocha",
		"coolnight",
		"dracula",
		"material-ocean",
		"nord",
		"rose-pine-moon",
		"solarized-dark",
		"tokyo-night",
	} {
		if !slices.Contains(names, want) {
			t.Fatalf("Names() = %#v, missing %q", names, want)
		}
	}
}

func TestNamesIncludeBuiltinIcebergThemes(t *testing.T) {
	names, err := Names()
	if err != nil {
		t.Fatalf("Names() error = %v", err)
	}
	for _, want := range []string{
		"iceberg",
		"iceberg-light",
	} {
		if !slices.Contains(names, want) {
			t.Fatalf("Names() = %#v, missing %q", names, want)
		}
	}
}

func TestNamesExcludeUnavailableAliases(t *testing.T) {
	names, err := Names()
	if err != nil {
		t.Fatalf("Names() error = %v", err)
	}
	for _, removed := range []string{"default"} {
		if slices.Contains(names, removed) {
			t.Fatalf("Names() = %#v, unexpectedly contains %q", names, removed)
		}
	}
}

func writeUserTheme(t *testing.T, name string) {
	t.Helper()
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)

	dir := filepath.Join(configHome, "kodacode", "themes")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	body := `name: ` + name + `
syntax_style: custom

palette:
  primary: "#112233"
  secondary: "#223344"
  surface: "#334455"
  overlay: "#445566"
  text: "#ddeeff"
  subtext: "#aabbcc"
  error: "#ff0000"
  warning: "#ffaa00"
  success: "#00ff00"
  thinking: "#aa55ff"

tones:
  bg: "#101010"
  bg-alt: "#111111"
  panel: "#222222"
  panel-alt: "#333333"
  line: "#444444"
  line-strong: "#555555"
  soft: "#666666"
`
	if err := os.WriteFile(filepath.Join(dir, name+".yaml"), []byte(body), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
}
