package theme_test

import (
	"testing"

	"github.com/sageil/kodacode/v1/internal/tui/termpalette"
	"github.com/sageil/kodacode/v1/internal/tui/theme"
	"gopkg.in/yaml.v3"
)

func TestResolve_PaletteDefaults(t *testing.T) {
	th := theme.Theme{
		Palette: theme.Palette{
			Surface: "#1e1e2e",
			Text:    "#cdd6f4",
		},
		Components: nil,
	}
	cs := th.Resolve("header")
	if cs.BG == nil || *cs.BG != "#1e1e2e" {
		t.Errorf("Resolve(%q).BG = %v, want %q", "header", cs.BG, "#1e1e2e")
	}
}

func TestResolve_ComponentOverride(t *testing.T) {
	override := "#181825"
	th := theme.Theme{
		Palette: theme.Palette{Surface: "#1e1e2e", Text: "#cdd6f4"},
		Components: map[string]theme.ComponentStyle{
			"assistant": {BG: &override},
		},
	}
	cs := th.Resolve("assistant")
	if cs.BG == nil || *cs.BG != "#181825" {
		t.Errorf("Resolve(%q).BG = %v, want %q", "assistant", cs.BG, "#181825")
	}
	// Non-overridden fields must retain palette defaults.
	if cs.FG == nil || *cs.FG != "#cdd6f4" {
		t.Errorf("Resolve(%q).FG = %v, want %q (palette default)", "assistant", cs.FG, "#cdd6f4")
	}
	if cs.Border == nil || *cs.Border != "rounded" {
		t.Errorf("Resolve(%q).Border = %v, want %q (default)", "assistant", cs.Border, "rounded")
	}
}

func TestResolve_PaletteRef(t *testing.T) {
	ref := "surface"
	th := theme.Theme{
		Palette: theme.Palette{Surface: "#1e1e2e"},
		Components: map[string]theme.ComponentStyle{
			"prompt": {BG: &ref},
		},
	}
	cs := th.Resolve("prompt")
	if cs.BG == nil || *cs.BG != "#1e1e2e" {
		t.Errorf("Resolve(%q).BG = %v, want %q", "prompt", cs.BG, "#1e1e2e")
	}
}

func TestLoader_MissingFile_FallsBackToDefault(t *testing.T) {
	l := theme.NewLoader(theme.LoaderConfig{Path: "/nonexistent/path.yaml"})
	th, err := l.Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if th == nil {
		t.Fatal("expected non-nil theme")
	}
	if th.Name != "default" {
		t.Errorf("Load().Name = %q, want %q", th.Name, "default")
	}
}

func TestLoader_InvalidColor_ReturnsError(t *testing.T) {
	data := []byte(`
palette:
  primary: "notacolor!!!!"
`)
	th, err := theme.LoadBytes(data)
	_ = th
	if err == nil {
		t.Fatal("expected error for invalid color")
	}
}

func TestLoader_ValidFile_ParsesCorrectly(t *testing.T) {
	data := []byte(`
name: test
palette:
  primary: "#cba6f7"
  surface: "#1e1e2e"
  text: "#cdd6f4"
`)
	th, err := theme.LoadBytes(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if th.Name != "test" {
		t.Errorf("LoadBytes().Name = %q, want %q", th.Name, "test")
	}
	if th.Palette.Primary != "#cba6f7" {
		t.Errorf("LoadBytes().Palette.Primary = %q, want %q", th.Palette.Primary, "#cba6f7")
	}
	if th.Palette.Surface != "#1e1e2e" {
		t.Errorf("LoadBytes().Palette.Surface = %q, want %q", th.Palette.Surface, "#1e1e2e")
	}
	if th.Palette.Text != "#cdd6f4" {
		t.Errorf("LoadBytes().Palette.Text = %q, want %q", th.Palette.Text, "#cdd6f4")
	}
}

func TestLoader_InvalidComponentColor_ReturnsError(t *testing.T) {
	data := []byte(`
name: t
palette:
  primary: "#cba6f7"
components:
  header:
    bg: "notacolor!!!!"
`)
	_, err := theme.LoadBytes(data)
	if err == nil {
		t.Fatal("expected error for invalid component color")
	}
}

func TestLoader_ValidComponentColor_NoError(t *testing.T) {
	data := []byte(`
name: t
palette:
  primary: "#cba6f7"
components:
  header:
    bg: "#ff0000"
    fg: "text"
    border-color: "3"
`)
	_, err := theme.LoadBytes(data)
	if err != nil {
		t.Fatalf("unexpected error for valid component colors: %v", err)
	}
}

func TestFromPalette_Dark(t *testing.T) {
	p := termpalette.Palette{
		IsDark: true,
		Fg:     "#cdd6f4",
		Bg:     "#1e1e2e",
		Colors: [16]string{
			"#1e1e2e", "#f38ba8", "#a6e3a1", "#fab387",
			"#89b4fa", "#cba6f7", "#89dceb", "#cdd6f4",
			"#585b70", "#f38ba8", "#a6e3a1", "#fab387",
			"#89b4fa", "#cba6f7", "#89dceb", "#cdd6f4",
		},
	}
	th := theme.FromPalette(p)
	if th.Palette.Surface != "#1e1e2e" {
		t.Errorf("FromPalette().Palette.Surface = %s, want %s", th.Palette.Surface, "#1e1e2e")
	}
	if th.Palette.Primary != "#89b4fa" {
		t.Errorf("FromPalette().Palette.Primary = %s, want %s (color 4)", th.Palette.Primary, "#89b4fa")
	}
}

func TestFromPalette_FallbackOnEmpty(t *testing.T) {
	// Empty palette (detection failed) should return static default.
	th := theme.FromPalette(termpalette.Palette{})
	if th.Palette.Surface == "" {
		t.Error("expected non-empty surface from fallback")
	}
}

func TestStaticDefault_PassesValidation(t *testing.T) {
	d := theme.StaticDefault()
	data, err := yaml.Marshal(d)
	if err != nil {
		t.Fatalf("yaml.Marshal failed: %v", err)
	}
	_, err = theme.LoadBytes(data)
	if err != nil {
		t.Fatalf("static default failed validation: %v", err)
	}
}
