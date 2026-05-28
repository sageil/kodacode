package tui

import (
	"github.com/sageil/kodacode/internal/app"
	"github.com/sageil/kodacode/internal/provider"
	tuitheme "github.com/sageil/kodacode/internal/tui/theme"
)

type footerErrorMsg struct {
	err error
}

type primaryModelSetMsg struct {
	err error
}

type utilityModelSetMsg struct {
	state app.DialogState
	err   error
}

type reviewerModelSetMsg struct {
	state app.DialogState
	err   error
}

type modelCatalogRefreshedMsg struct {
	state    app.DialogState
	query    string
	selected provider.ModelRef
	err      error
}

type themeAppliedMsg struct {
	name  string
	theme *tuitheme.Theme
}

type layoutPersistedMsg struct {
	err error
}
