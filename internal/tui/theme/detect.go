package theme

import "github.com/sageil/kodacode/v1/internal/tui/termpalette"

// FromPalette derives a Theme from a detected terminal palette.
// If the palette is empty (detection failed), returns StaticDefault().
func FromPalette(p termpalette.Palette) Theme {
	if p.Fg == "" && p.Bg == "" {
		return StaticDefault()
	}
	surface := p.Bg
	text := p.Fg
	if surface == "" {
		surface = "#1e1e2e"
	}
	if text == "" {
		text = "#cdd6f4"
	}
	// Map ANSI 16-color indices to semantic palette tokens.
	// color[4] = blue → primary, color[5] = magenta → secondary,
	// color[1] = red → error, color[3] = yellow → warning, color[2] = green → success
	pick := func(idx int, fallback string) string {
		if idx < len(p.Colors) && p.Colors[idx] != "" {
			return p.Colors[idx]
		}
		return fallback
	}
	return Theme{
		Name: "terminal",
		Palette: Palette{
			Primary:   pick(4, "#89b4fa"),
			Secondary: pick(5, "#cba6f7"),
			Surface:   surface,
			Overlay:   pick(0, "#313244"),
			Text:      text,
			Subtext:   pick(7, "#a6adc8"),
			Error:     pick(1, "#f38ba8"),
			Warning:   pick(3, "#fab387"),
			Success:   pick(2, "#a6e3a1"),
		},
	}
}
