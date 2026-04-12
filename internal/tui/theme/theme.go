// Package theme provides palette loading, detection, and live-reloading of TUI color themes.
package theme

// Palette holds named color tokens.
type Palette struct {
	Primary   string `yaml:"primary"`
	Secondary string `yaml:"secondary"`
	Surface   string `yaml:"surface"`
	Overlay   string `yaml:"overlay"`
	Text      string `yaml:"text"`
	Subtext   string `yaml:"subtext"`
	Error     string `yaml:"error"`
	Warning   string `yaml:"warning"`
	Success   string `yaml:"success"`
	Thinking  string `yaml:"thinking"`
}

// ComponentStyle holds per-component style values.
// Pointer fields distinguish absence from zero value.
type ComponentStyle struct {
	BG          *string `yaml:"bg"`
	FG          *string `yaml:"fg"`
	Border      *string `yaml:"border"`
	BorderColor *string `yaml:"border-color"`
	Padding     []int   `yaml:"padding"`
	SelectedBG  *string `yaml:"selected-bg"`
	SelectedFG  *string `yaml:"selected-fg"`
	TitleFG     *string `yaml:"title-fg"`
}

// Theme is the top-level theme definition.
type Theme struct {
	Name        string                    `yaml:"name"`
	SyntaxStyle string                    `yaml:"syntax_style"`
	Palette     Palette                   `yaml:"palette"`
	Components  map[string]ComponentStyle `yaml:"components"`
}

// Themeable is implemented by every TUI component that responds to theme changes.
type Themeable interface {
	ApplyTheme(t *Theme)
}

// StaticDefault returns the compiled-in dark-mode fallback theme.
func StaticDefault() Theme {
	return Theme{
		Name: "default",
		Palette: Palette{
			Primary:   "#cba6f7",
			Secondary: "#89b4fa",
			Surface:   "#1e1e2e",
			Overlay:   "#313244",
			Text:      "#cdd6f4",
			Subtext:   "#a6adc8",
			Error:     "#f38ba8",
			Warning:   "#fab387",
			Success:   "#a6e3a1",
			Thinking:  "#b4befe",
		},
	}
}

// paletteRef resolves a string that may be a palette token name to its hex/ANSI value.
func (th *Theme) paletteRef(val string) string {
	switch val {
	case "primary":
		return th.Palette.Primary
	case "secondary":
		return th.Palette.Secondary
	case "surface":
		return th.Palette.Surface
	case "overlay":
		return th.Palette.Overlay
	case "text":
		return th.Palette.Text
	case "subtext":
		return th.Palette.Subtext
	case "error":
		return th.Palette.Error
	case "warning":
		return th.Palette.Warning
	case "success":
		return th.Palette.Success
	case "thinking":
		return th.Palette.Thinking
	default:
		return val
	}
}

// PaletteToken resolves a named palette token to its hex/ANSI value.
// Returns "" for unrecognised tokens (unlike the private paletteRef which
// returns the input string unchanged for unknown tokens).
func (th *Theme) PaletteToken(token string) string {
	switch token {
	case "primary":
		return th.Palette.Primary
	case "secondary":
		return th.Palette.Secondary
	case "surface":
		return th.Palette.Surface
	case "overlay":
		return th.Palette.Overlay
	case "text":
		return th.Palette.Text
	case "subtext":
		return th.Palette.Subtext
	case "error":
		return th.Palette.Error
	case "warning":
		return th.Palette.Warning
	case "success":
		return th.Palette.Success
	case "thinking":
		return th.Palette.Thinking
	default:
		return ""
	}
}

// IsLight returns true if the theme's surface color appears light.
// Used as a fallback for syntax style selection when no explicit
// syntax_style is configured.
func (th *Theme) IsLight() bool {
	s := th.Palette.Surface
	if len(s) != 7 || s[0] != '#' {
		return false
	}
	r := hexVal(s[1])<<4 | hexVal(s[2])
	g := hexVal(s[3])<<4 | hexVal(s[4])
	b := hexVal(s[5])<<4 | hexVal(s[6])
	// Perceived luminance (ITU-R BT.601).
	lum := 0.299*float64(r) + 0.587*float64(g) + 0.114*float64(b)
	return lum > 128
}

func hexVal(c byte) int {
	switch {
	case c >= '0' && c <= '9':
		return int(c - '0')
	case c >= 'a' && c <= 'f':
		return int(c-'a') + 10
	case c >= 'A' && c <= 'F':
		return int(c-'A') + 10
	default:
		return 0
	}
}

func strPtr(s string) *string { return &s }

// ResolveSparse returns the ComponentStyle for the named component with only
// the fields that are explicitly set in the YAML — no palette-derived defaults.
// Use this for opt-in styling where absence means "render without that style".
func (th *Theme) ResolveSparse(component string) ComponentStyle {
	if th == nil || th.Components == nil {
		return ComponentStyle{}
	}
	override, ok := th.Components[component]
	if !ok {
		return ComponentStyle{}
	}
	cs := ComponentStyle{}
	if override.BG != nil {
		cs.BG = resolvePtr(th, override.BG)
	}
	if override.FG != nil {
		cs.FG = resolvePtr(th, override.FG)
	}
	if override.Border != nil {
		cs.Border = resolvePtr(th, override.Border)
	}
	if override.BorderColor != nil {
		cs.BorderColor = resolvePtr(th, override.BorderColor)
	}
	if len(override.Padding) > 0 {
		cs.Padding = override.Padding
	}
	return cs
}

func resolvePtr(th *Theme, p *string) *string {
	if p == nil {
		return nil
	}
	resolved := th.paletteRef(*p)
	return &resolved
}

// Resolve returns a fully-populated ComponentStyle for the named component.
// Palette-derived defaults are applied first; component overrides follow.
func (th *Theme) Resolve(component string) ComponentStyle {
	// Palette-derived defaults.
	cs := ComponentStyle{
		BG:     strPtr(th.Palette.Surface),
		FG:     strPtr(th.Palette.Text),
		Border: strPtr("rounded"),
	}

	override, ok := th.Components[component]
	if !ok {
		return cs
	}

	if override.BG != nil {
		cs.BG = resolvePtr(th, override.BG)
	}
	if override.FG != nil {
		cs.FG = resolvePtr(th, override.FG)
	}
	if override.Border != nil {
		cs.Border = resolvePtr(th, override.Border)
	}
	if override.BorderColor != nil {
		cs.BorderColor = resolvePtr(th, override.BorderColor)
	}
	if len(override.Padding) > 0 {
		cs.Padding = override.Padding
	}
	if override.SelectedBG != nil {
		cs.SelectedBG = resolvePtr(th, override.SelectedBG)
	}
	if override.SelectedFG != nil {
		cs.SelectedFG = resolvePtr(th, override.SelectedFG)
	}
	if override.TitleFG != nil {
		cs.TitleFG = resolvePtr(th, override.TitleFG)
	}
	return cs
}
