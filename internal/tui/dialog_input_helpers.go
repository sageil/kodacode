package tui

import (
	"charm.land/bubbles/v2/textinput"
	"github.com/sageil/kodacode/internal/tui/theme"
)

func newDialogTextInput(th *theme.Theme, charLimit int) textinput.Model {
	input := textinput.New()
	input.Prompt = "❯ "
	input.CharLimit = charLimit
	applyDialogInputTheme(&input, th)
	return input
}
