package theme

import "strings"

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

type Tones struct {
	BG         string `yaml:"bg"`
	BGAlt      string `yaml:"bg-alt"`
	Panel      string `yaml:"panel"`
	PanelAlt   string `yaml:"panel-alt"`
	Line       string `yaml:"line"`
	LineStrong string `yaml:"line-strong"`
	Soft       string `yaml:"soft"`
}

type ComponentStyle struct {
	BG          *string `yaml:"bg"`
	FG          *string `yaml:"fg"`
	Border      *string `yaml:"border"`
	BorderColor *string `yaml:"border-color"`
	Padding     []int   `yaml:"padding"`
}

type Theme struct {
	Name        string                    `yaml:"name"`
	SyntaxStyle string                    `yaml:"syntax_style"`
	Palette     Palette                   `yaml:"palette"`
	Tones       Tones                     `yaml:"tones"`
	Components  map[string]ComponentStyle `yaml:"components"`
}

const defaultThemeName = "ayu-dark"

func StaticDefault() Theme {
	return Theme{
		Name:        defaultThemeName,
		SyntaxStyle: "ayu-dark",
		Palette: Palette{
			Primary:   "#e6b450",
			Secondary: "#39bae6",
			Surface:   "#10141c",
			Overlay:   "#141821",
			Text:      "#bfbdb6",
			Subtext:   "#5a6378",
			Error:     "#d95757",
			Warning:   "#ffb454",
			Success:   "#70bf56",
			Thinking:  "#d2a6ff",
		},
		Tones: Tones{
			BG:         "#10141c",
			BGAlt:      "#0d1017",
			Panel:      "#141821",
			PanelAlt:   "#161a24",
			Line:       "#1b1f29",
			LineStrong: "#5a6378",
			Soft:       "#697184",
		},
	}
}

func isAvailableThemeName(name string) bool {
	switch strings.TrimSpace(name) {
	case "", "default":
		return false
	default:
		return true
	}
}

func (t *Theme) Resolve(component string) ComponentStyle {
	palette := StaticDefault().Palette
	if t != nil {
		palette = t.Palette
	}
	style := ComponentStyle{
		BG:     strPtr(palette.Surface),
		FG:     strPtr(palette.Text),
		Border: strPtr("rounded"),
	}
	if t == nil || t.Components == nil {
		return style
	}

	override, ok := t.Components[component]
	if !ok {
		return style
	}
	if override.BG != nil {
		style.BG = resolvedPtr(t, override.BG)
	}
	if override.FG != nil {
		style.FG = resolvedPtr(t, override.FG)
	}
	if override.Border != nil {
		style.Border = resolvedPtr(t, override.Border)
	}
	if override.BorderColor != nil {
		style.BorderColor = resolvedPtr(t, override.BorderColor)
	}
	if len(override.Padding) > 0 {
		style.Padding = append([]int(nil), override.Padding...)
	}
	return style
}

func (t *Theme) PaletteToken(name string) string {
	switch name {
	case "primary":
		return t.Palette.Primary
	case "secondary":
		return t.Palette.Secondary
	case "surface":
		return t.Palette.Surface
	case "overlay":
		return t.Palette.Overlay
	case "text":
		return t.Palette.Text
	case "subtext":
		return t.Palette.Subtext
	case "error":
		return t.Palette.Error
	case "warning":
		return t.Palette.Warning
	case "success":
		return t.Palette.Success
	case "thinking":
		return t.Palette.Thinking
	default:
		return ""
	}
}

func (t *Theme) ToneToken(name string) string {
	switch name {
	case "bg":
		return t.Tones.BG
	case "bg-alt":
		return t.Tones.BGAlt
	case "panel":
		return t.Tones.Panel
	case "panel-alt":
		return t.Tones.PanelAlt
	case "line":
		return t.Tones.Line
	case "line-strong":
		return t.Tones.LineStrong
	case "soft":
		return t.Tones.Soft
	default:
		return ""
	}
}

func resolvedPtr(t *Theme, value *string) *string {
	if value == nil {
		return nil
	}
	resolved := *value
	if paletteValue := t.PaletteToken(resolved); paletteValue != "" {
		resolved = paletteValue
	} else if toneValue := t.ToneToken(resolved); toneValue != "" {
		resolved = toneValue
	}
	return &resolved
}

func strPtr(value string) *string {
	return &value
}
