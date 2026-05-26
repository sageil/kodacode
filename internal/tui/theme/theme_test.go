package theme

import (
	"errors"
	"testing"
)

func TestLoadBuiltinTheme(t *testing.T) {
	loaded, err := Load("rose-pine-moon")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if loaded.Name != "rose-pine-moon" {
		t.Fatalf("theme = %#v", loaded)
	}
	if loaded.Palette.Primary == "" || loaded.Palette.Text == "" {
		t.Fatalf("theme palette = %#v", loaded.Palette)
	}
}

func TestLoadBuiltinAyuVariants(t *testing.T) {
	bordered, err := Load("ayu-dark")
	if err != nil {
		t.Fatalf("Load(ayu-dark) error = %v", err)
	}
	unbordered, err := Load("ayu-dark-unbordered")
	if err != nil {
		t.Fatalf("Load(ayu-dark-unbordered) error = %v", err)
	}
	if bordered.Name != "ayu-dark" || unbordered.Name != "ayu-dark-unbordered" {
		t.Fatalf("loaded themes = %q, %q", bordered.Name, unbordered.Name)
	}
	if bordered.Tones.Line == unbordered.Tones.Line {
		t.Fatalf("ayu dark variants should differ on line tone, got %q", bordered.Tones.Line)
	}
	if bordered.Palette.Primary != "#e6b450" {
		t.Fatalf("ayu-dark primary = %q, want #e6b450", bordered.Palette.Primary)
	}
	if unbordered.Tones.BG != "#0d1017" {
		t.Fatalf("ayu-dark-unbordered bg = %q, want #0d1017", unbordered.Tones.BG)
	}
}

func TestLoadBuiltinOneDarkVariants(t *testing.T) {
	loaded, err := Load("one-dark-pro")
	if err != nil {
		t.Fatalf("Load(one-dark-pro) error = %v", err)
	}
	night, err := Load("one-dark-pro-night-flat")
	if err != nil {
		t.Fatalf("Load(one-dark-pro-night-flat) error = %v", err)
	}
	if loaded.Palette.Primary != "#61afef" {
		t.Fatalf("one-dark-pro primary = %q, want #61afef", loaded.Palette.Primary)
	}
	if loaded.Tones.BG != "#282c34" {
		t.Fatalf("one-dark-pro bg = %q, want #282c34", loaded.Tones.BG)
	}
	if night.Tones.BG != "#16191d" {
		t.Fatalf("one-dark-pro-night-flat bg = %q, want #16191d", night.Tones.BG)
	}
	if loaded.Tones.BG == night.Tones.BG {
		t.Fatalf("one dark variants should differ on background tone, got %q", loaded.Tones.BG)
	}
}

func TestLoadBuiltinGruvboxVariants(t *testing.T) {
	dark, err := Load("gruvbox-dark-medium")
	if err != nil {
		t.Fatalf("Load(gruvbox-dark-medium) error = %v", err)
	}
	classic, err := Load("gruvbox-dark")
	if err != nil {
		t.Fatalf("Load(gruvbox-dark) error = %v", err)
	}
	light, err := Load("gruvbox-light-soft")
	if err != nil {
		t.Fatalf("Load(gruvbox-light-soft) error = %v", err)
	}
	if dark.Palette.Primary != "#689d6a" {
		t.Fatalf("gruvbox-dark-medium primary = %q, want #689d6a", dark.Palette.Primary)
	}
	if dark.Tones.BG != "#282828" {
		t.Fatalf("gruvbox-dark-medium bg = %q, want #282828", dark.Tones.BG)
	}
	if dark.Tones.BGAlt != "#1d2021" {
		t.Fatalf("gruvbox-dark-medium bg-alt = %q, want #1d2021", dark.Tones.BGAlt)
	}
	if dark.Tones.Panel != "#32302f" {
		t.Fatalf("gruvbox-dark-medium panel = %q, want #32302f", dark.Tones.Panel)
	}
	if light.Tones.BG != "#f2e5bc" {
		t.Fatalf("gruvbox-light-soft bg = %q, want #f2e5bc", light.Tones.BG)
	}
	if light.Tones.Panel != "#fbf1c7" {
		t.Fatalf("gruvbox-light-soft panel = %q, want #fbf1c7", light.Tones.Panel)
	}
	if light.Tones.PanelAlt != "#f9f5d7" {
		t.Fatalf("gruvbox-light-soft panel-alt = %q, want #f9f5d7", light.Tones.PanelAlt)
	}
	if dark.Tones.BG == light.Tones.BG {
		t.Fatalf("gruvbox dark/light variants should differ on background tone, got %q", dark.Tones.BG)
	}
	if dark.Tones.BG == dark.Tones.Panel {
		t.Fatalf("gruvbox-dark-medium bg and panel should differ, got %q", dark.Tones.BG)
	}
	if light.Tones.BG == light.Tones.Panel {
		t.Fatalf("gruvbox-light-soft bg and panel should differ, got %q", light.Tones.BG)
	}
	if classic.Palette.Primary != "#d79921" {
		t.Fatalf("gruvbox-dark primary = %q, want #d79921", classic.Palette.Primary)
	}
	if classic.Tones.PanelAlt != "#504945" {
		t.Fatalf("gruvbox-dark panel-alt = %q, want #504945", classic.Tones.PanelAlt)
	}
}

func TestLoadBuiltinIcebergVariants(t *testing.T) {
	dark, err := Load("iceberg")
	if err != nil {
		t.Fatalf("Load(iceberg) error = %v", err)
	}
	light, err := Load("iceberg-light")
	if err != nil {
		t.Fatalf("Load(iceberg-light) error = %v", err)
	}
	if dark.Palette.Primary != "#84a0c6" {
		t.Fatalf("iceberg primary = %q, want #84a0c6", dark.Palette.Primary)
	}
	if dark.Tones.BG != "#161821" {
		t.Fatalf("iceberg bg = %q, want #161821", dark.Tones.BG)
	}
	if light.Palette.Primary != "#2d539e" {
		t.Fatalf("iceberg-light primary = %q, want #2d539e", light.Palette.Primary)
	}
	if light.Tones.BG != "#e8e9ec" {
		t.Fatalf("iceberg-light bg = %q, want #e8e9ec", light.Tones.BG)
	}
	if dark.Tones.BG == light.Tones.BG {
		t.Fatalf("iceberg dark/light variants should differ on background tone, got %q", dark.Tones.BG)
	}
}

func TestLoadBuiltinAdditionalThemes(t *testing.T) {
	for _, tt := range []struct {
		name    string
		primary string
		panel   string
	}{
		{name: "coolnight", primary: "#47ff9c", panel: "#0d2137"},
		{name: "dracula", primary: "#bd93f9", panel: "#44475a"},
		{name: "material-ocean", primary: "#81a1c1", panel: "#1e2132"},
		{name: "nord", primary: "#81a1c1", panel: "#3b4252"},
		{name: "one-dark", primary: "#61afef", panel: "#21252b"},
		{name: "solarized-dark", primary: "#268bd2", panel: "#073642"},
	} {
		loaded, err := Load(tt.name)
		if err != nil {
			t.Fatalf("Load(%s) error = %v", tt.name, err)
		}
		if loaded.Palette.Primary != tt.primary {
			t.Fatalf("%s primary = %q, want %q", tt.name, loaded.Palette.Primary, tt.primary)
		}
		if loaded.Tones.Panel != tt.panel {
			t.Fatalf("%s panel = %q, want %q", tt.name, loaded.Tones.Panel, tt.panel)
		}
		if loaded.Tones.BG == loaded.Tones.PanelAlt {
			t.Fatalf("%s bg and panel-alt should differ, got %q", tt.name, loaded.Tones.BG)
		}
	}
}

func TestLoadBytesRejectsInvalidColor(t *testing.T) {
	_, err := LoadBytes([]byte(`
name: broken
palette:
  primary: "not-a-color"
  secondary: "#ffffff"
  surface: "#000000"
  overlay: "#111111"
  text: "#eeeeee"
  subtext: "#aaaaaa"
  error: "#ff0000"
  warning: "#ffff00"
  success: "#00ff00"
  thinking: "#0000ff"
`))
	if err == nil {
		t.Fatal("LoadBytes() error = nil, want validation error")
	}
}

func TestLoadMissingThemeReturnsNotFound(t *testing.T) {
	_, err := Load("not-a-real-theme")
	if !errors.Is(err, ErrThemeNotFound) {
		t.Fatalf("Load() error = %v, want ErrThemeNotFound", err)
	}
}

func TestLoadBlankThemeReturnsStaticDefault(t *testing.T) {
	loaded, err := Load("")
	if err != nil {
		t.Fatalf("Load(\"\") error = %v", err)
	}
	if loaded.Name != StaticDefault().Name {
		t.Fatalf("theme name = %q, want %q", loaded.Name, StaticDefault().Name)
	}
}

func TestLoadUnavailableThemeNamesReturnNotFound(t *testing.T) {
	for _, name := range []string{"default", "missing-theme"} {
		if _, err := Load(name); !errors.Is(err, ErrThemeNotFound) {
			t.Fatalf("Load(%q) error = %v, want ErrThemeNotFound", name, err)
		}
	}
}
