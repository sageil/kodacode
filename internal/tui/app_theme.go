package tui

import (
	"os"
	"path/filepath"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/sageil/kodacode/v1/internal/config"
	"github.com/sageil/kodacode/v1/internal/tui/termpalette"
	"github.com/sageil/kodacode/v1/internal/tui/theme"
)

// applyThemeByName loads the named theme (or re-detects from terminal for "default")
// and returns a Cmd that emits ThemeChangedMsg. It also persists the selection to config
// and to the database (via the settings API).
func (a App) applyThemeByName(name string) tea.Cmd {
	api := a.api
	ctx := a.ctx
	return func() tea.Msg {
		var th *theme.Theme
		if name == "default" {
			p, err := termpalette.Detect(os.Stdin, os.Stdout, 200*time.Millisecond)
			if err != nil {
				d := theme.StaticDefault()
				th = &d
			} else {
				derived := theme.FromPalette(p)
				th = &derived
			}
			// Clear persisted theme so next startup re-detects.
			if cfg, cerr := config.Load(""); cerr == nil {
				cfg.TUI.Theme = ""
				_ = config.Save(cfg)
			}
			_ = api.SetSetting(ctx, "tui.theme", "")
		} else {
			themePath := filepath.Join(config.ThemesDir(), name+".yaml")
			loaded, err := theme.NewLoader(theme.LoaderConfig{Path: themePath}).Load()
			if err != nil {
				// silently ignore — theme file not found or invalid
				return nil
			}
			th = loaded
			if cfg, cerr := config.Load(""); cerr == nil {
				cfg.TUI.Theme = name
				_ = config.Save(cfg)
			}
			_ = api.SetSetting(ctx, "tui.theme", name)
		}
		return ThemeChangedMsg{Theme: th, Name: name}
	}
}
