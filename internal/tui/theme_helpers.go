package tui

import (
	"image/color"

	"charm.land/lipgloss/v2"
	"github.com/sageil/kodacode/v1/internal/tui/theme"
)

// colorFrom returns a color.Color for the named palette token, or the
// fallback color.Color if the theme is nil or the token is unrecognised.
func colorFrom(th *theme.Theme, token string, fallback color.Color) color.Color {
	if th == nil {
		return fallback
	}
	val := th.PaletteToken(token)
	if val == "" {
		return fallback
	}
	return lipgloss.Color(val)
}

