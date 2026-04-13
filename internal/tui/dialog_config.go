package tui

import (
	"fmt"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/sageil/kodacode/v1/internal/config"
	"github.com/sageil/kodacode/v1/internal/tui/theme"
)

// configSource indicates where a config value originates.
type configSource int

const (
	sourceDefault configSource = iota
	sourceGlobal
	sourceProject
)

// configEntry is a single key-value row in the config viewer.
type configEntry struct {
	Key    string
	Value  string
	Source configSource
}

// configSection is a sidebar item with its content rows.
type configSection struct {
	Name    string
	Entries []configEntry
}

const (
	cfgSidebarWidth = 18
	cfgKeyWidth     = 22
	cfgBadgeWidth   = 10 // " [project]"
	cfgContentWidth = 66
	cfgDialogWidth  = cfgSidebarWidth + cfgContentWidth + 2 // separator + space
	cfgValMaxWidth  = cfgContentWidth - cfgKeyWidth - cfgBadgeWidth
)

// ConfigViewerDialog displays the effective configuration with source indicators.
type ConfigViewerDialog struct {
	id          string
	theme       *theme.Theme
	sections    []configSection
	globalPath  string
	projectPath string
	activeIdx   int
}

func (d *ConfigViewerDialog) ApplyTheme(t *theme.Theme) { d.theme = t }

func NewConfigViewerDialog(id string, th *theme.Theme, layered config.LayeredConfig, projectDir string) ConfigViewerDialog {
	d := ConfigViewerDialog{
		id:         id,
		theme:      th,
		globalPath: filepath.Join(config.ConfigDir(), "config.yaml"),
	}
	if projectDir != "" {
		d.projectPath = filepath.Join(projectDir, "kodacode.yaml")
	}
	d.sections = buildConfigSections(layered)
	return d
}

func (d ConfigViewerDialog) Init() tea.Cmd { return nil }

func (d ConfigViewerDialog) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	kp, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return d, nil
	}
	switch kp.String() {
	case "esc", "ctrl+c":
		return d, closeDialog(d.id, nil)
	case "up", "k":
		if d.activeIdx > 0 {
			d.activeIdx--
		}
	case "down", "j":
		if d.activeIdx < len(d.sections)-1 {
			d.activeIdx++
		}
	}
	return d, nil
}

func (d ConfigViewerDialog) View() tea.View {
	accentColor := colorFrom(d.theme, "secondary", lipgloss.Color("4"))
	dimColor := colorFrom(d.theme, "subtext", lipgloss.Color("241"))
	overlayColor := colorFrom(d.theme, "overlay", lipgloss.Color("236"))
	primaryColor := colorFrom(d.theme, "primary", lipgloss.Color("62"))
	borderColor := colorFrom(d.theme, "primary", lipgloss.Color("62"))
	textColor := colorFrom(d.theme, "text", lipgloss.Color("250"))
	globalColor := lipgloss.Color("177")
	projectColor := lipgloss.Color("114")

	badgeGlobal := lipgloss.NewStyle().Foreground(globalColor).Render("[global]")
	badgeProject := lipgloss.NewStyle().Foreground(projectColor).Render("[project]")
	badgeDefault := lipgloss.NewStyle().Foreground(dimColor).Render("[default]")

	keyStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(primaryColor).
		Width(cfgKeyWidth)
	valStyle := lipgloss.NewStyle().
		Foreground(textColor).
		Width(cfgValMaxWidth)

	// ── sidebar ──
	sidebarTitle := lipgloss.NewStyle().
		Bold(true).
		Foreground(primaryColor).
		PaddingLeft(1).PaddingBottom(1).
		Render("Config")

	var sidebarRows []string
	sidebarRows = append(sidebarRows, sidebarTitle)

	for i, s := range d.sections {
		label := s.Name
		if i == d.activeIdx {
			row := lipgloss.NewStyle().
				Bold(true).
				Foreground(primaryColor).
				Background(overlayColor).
				Width(cfgSidebarWidth - 1).
				PaddingLeft(1).
				Render(label)
			sidebarRows = append(sidebarRows, row)
		} else {
			row := lipgloss.NewStyle().
				Foreground(dimColor).
				Width(cfgSidebarWidth - 1).
				PaddingLeft(1).
				Render(label)
			sidebarRows = append(sidebarRows, row)
		}
	}

	sidebar := strings.Join(sidebarRows, "\n")

	// ── content ──
	contentTitle := lipgloss.NewStyle().
		Bold(true).
		Foreground(accentColor).
		MarginBottom(1)

	// Legend row.
	legendGlobal := lipgloss.NewStyle().Foreground(globalColor).Render("●") + " " +
		lipgloss.NewStyle().Foreground(dimColor).Render("global")
	legendProject := lipgloss.NewStyle().Foreground(projectColor).Render("●") + " " +
		lipgloss.NewStyle().Foreground(dimColor).Render("project")
	legendDefault := lipgloss.NewStyle().Foreground(dimColor).Render("● default")
	legend := legendGlobal + "  " + legendProject + "  " + legendDefault

	var contentRows []string
	if d.activeIdx < len(d.sections) {
		sec := d.sections[d.activeIdx]
		contentRows = append(contentRows, contentTitle.Render(sec.Name))
		contentRows = append(contentRows, legend)
		contentRows = append(contentRows, "")

		for _, e := range sec.Entries {
			var badge string
			switch e.Source {
			case sourceGlobal:
				badge = badgeGlobal
			case sourceProject:
				badge = badgeProject
			default:
				badge = badgeDefault
			}
			row := keyStyle.Render(e.Key) + valStyle.Render(e.Value) + badge
			contentRows = append(contentRows, row)
		}
	}

	hint := hintStyle(d.theme).Render("↑/↓ navigate · esc close")
	contentRows = append(contentRows, "", hint)

	content := strings.Join(contentRows, "\n")

	// ── compose ──
	sidebarBox := lipgloss.NewStyle().
		Width(cfgSidebarWidth).
		Render(sidebar)

	height := max(lipgloss.Height(sidebarBox), lipgloss.Height(content))
	sepLines := make([]string, height)
	for i := range sepLines {
		sepLines[i] = "│"
	}
	sep := lipgloss.NewStyle().
		Foreground(borderColor).
		Render(strings.Join(sepLines, "\n"))

	body := lipgloss.JoinHorizontal(lipgloss.Top, sidebarBox, sep, " "+content)
	box := dialogStyle(d.theme, cfgDialogWidth).Render(body)
	return tea.NewView(dropShadow(box, d.theme))
}

// buildConfigSections constructs the sidebar sections from layered config data.
func buildConfigSections(l config.LayeredConfig) []configSection {
	m := l.Merged
	if m == nil {
		return nil
	}
	var sections []configSection

	// Session.
	session := configSection{Name: "Session"}
	session.Entries = append(session.Entries,
		cfgStr("default_agent", m.DefaultAgent, l, func(c *config.Config) string { return c.DefaultAgent }),
		cfgStr("utility_model", m.UtilityModel, l, func(c *config.Config) string { return c.UtilityModel }),
		cfgStr("reviewer_model", m.ReviewerModel, l, func(c *config.Config) string { return c.ReviewerModel }),
	)
	if m.Session.Budget > 0 {
		session.Entries = append(session.Entries,
			cfgFloat("budget", m.Session.Budget, l, func(c *config.Config) float64 { return c.Session.Budget }),
		)
	}
	if m.Session.TotalBudget > 0 {
		session.Entries = append(session.Entries,
			cfgFloat("total_budget", m.Session.TotalBudget, l, func(c *config.Config) float64 { return c.Session.TotalBudget }),
		)
	}
	session.Entries = append(session.Entries,
		cfgInt("primary_max_steps", m.Session.PrimaryMaxSteps, l, func(c *config.Config) int { return c.Session.PrimaryMaxSteps }),
		cfgInt("subagent_max_steps", m.Session.SubagentMaxSteps, l, func(c *config.Config) int { return c.Session.SubagentMaxSteps }),
		cfgInt("max_subagents", m.Session.MaxSubagents, l, func(c *config.Config) int { return c.Session.MaxSubagents }),
		cfgBoolPtr("plan_approval", m.Session.PlanApproval, l, func(c *config.Config) *bool { return c.Session.PlanApproval }),
		cfgBoolPtr("snapshot", m.Session.Snapshot, l, func(c *config.Config) *bool { return c.Session.Snapshot }),
		cfgBoolPtr("trace", m.Session.Trace, l, func(c *config.Config) *bool { return c.Session.Trace }),
	)
	sections = append(sections, session)

	// Providers. Render one section per provider.
	for _, p := range m.Providers {
		sec := configSection{Name: p.ID}
		src := providerSource(p.ID, l)
		apiKeyDisplay := "***"
		if p.APIKey == "" {
			apiKeyDisplay = "—"
		}
		sec.Entries = append(sec.Entries, configEntry{"api_key", apiKeyDisplay, src})
		if p.BaseURL != "" {
			sec.Entries = append(sec.Entries, configEntry{"base_url", p.BaseURL, src})
		}
		if p.ThinkingBudget > 0 {
			sec.Entries = append(sec.Entries, configEntry{"thinking_budget", fmt.Sprintf("%d", p.ThinkingBudget), src})
		}
		if p.ThinkingType != "" {
			sec.Entries = append(sec.Entries, configEntry{"thinking_type", p.ThinkingType, src})
		}
		if len(p.Models) > 0 {
			sec.Entries = append(sec.Entries, configEntry{"models", fmt.Sprintf("%d static", len(p.Models)), src})
		}
		sections = append(sections, sec)
	}

	// TUI.
	tui := configSection{Name: "TUI"}
	tui.Entries = append(tui.Entries,
		cfgStr("theme", m.TUI.Theme, l, func(c *config.Config) string { return c.TUI.Theme }),
		cfgInt("display_turns", m.TUI.DisplayTurns, l, func(c *config.Config) int { return c.TUI.DisplayTurns }),
		cfgInt("input_max_height", m.TUI.InputMaxHeight, l, func(c *config.Config) int { return c.TUI.InputMaxHeight }),
		cfgBool("auto_resume", m.TUI.AutoResume, l, func(c *config.Config) bool { return c.TUI.AutoResume }),
	)
	sections = append(sections, tui)

	// MCP servers.
	if len(m.MCP.Servers) > 0 {
		mcp := configSection{Name: "MCP"}
		for _, s := range m.MCP.Servers {
			src := mcpServerSource(s, l)
			name := mcpServerName(s)
			mcp.Entries = append(mcp.Entries, configEntry{name, s.Type, src})
		}
		sections = append(sections, mcp)
	}

	// LSP servers.
	if len(m.LSP.Servers) > 0 {
		lspSec := configSection{Name: "LSP"}
		for _, s := range m.LSP.Servers {
			src := lspServerSource(s, l)
			name := lspServerName(s)
			exts := strings.Join(s.Extensions, " ")
			lspSec.Entries = append(lspSec.Entries, configEntry{name, exts, src})
		}
		sections = append(sections, lspSec)
	}

	// Diagnostics.
	diag := configSection{Name: "Diagnostics"}
	diag.Entries = append(diag.Entries,
		cfgBoolPtr("enabled", m.Diagnostics.Enabled, l, func(c *config.Config) *bool { return c.Diagnostics.Enabled }),
	)
	if len(m.Diagnostics.Linters) > 0 {
		diag.Entries = append(diag.Entries, configEntry{"linters", fmt.Sprintf("%d configured", len(m.Diagnostics.Linters)), sourceDefault})
	}
	sections = append(sections, diag)

	// Paths & misc.
	paths := configSection{Name: "Paths"}
	paths.Entries = append(paths.Entries,
		configEntry{"allowed_paths", fmt.Sprintf("%d entries", len(m.AllowedPaths)), sourceDefault},
		configEntry{"ignore_patterns", fmt.Sprintf("%d patterns", len(m.IgnorePatterns)), sourceDefault},
		cfgInt("model_refresh", m.ModelRefreshInterval, l, func(c *config.Config) int { return c.ModelRefreshInterval }),
	)
	sections = append(sections, paths)

	return sections
}

// ── source resolution helpers ──

func cfgStr(key, val string, l config.LayeredConfig, get func(*config.Config) string) configEntry {
	if val == "" {
		val = "—"
	}
	src := sourceDefault
	if l.Project != nil && get(l.Project) != get(l.Default) && get(l.Project) == get(l.Merged) {
		src = sourceProject
	} else if l.Global != nil && get(l.Global) != get(l.Default) {
		src = sourceGlobal
	}
	return configEntry{key, val, src}
}

func cfgInt(key string, val int, l config.LayeredConfig, get func(*config.Config) int) configEntry {
	src := sourceDefault
	if l.Project != nil && get(l.Project) != get(l.Default) && get(l.Project) == get(l.Merged) {
		src = sourceProject
	} else if l.Global != nil && get(l.Global) != get(l.Default) {
		src = sourceGlobal
	}
	return configEntry{key, fmt.Sprintf("%d", val), src}
}

func cfgFloat(key string, val float64, l config.LayeredConfig, get func(*config.Config) float64) configEntry {
	src := sourceDefault
	if l.Project != nil && get(l.Project) != get(l.Default) && get(l.Project) == get(l.Merged) {
		src = sourceProject
	} else if l.Global != nil && get(l.Global) != get(l.Default) {
		src = sourceGlobal
	}
	return configEntry{key, fmt.Sprintf("%.2f", val), src}
}

func cfgBool(key string, val bool, l config.LayeredConfig, get func(*config.Config) bool) configEntry {
	src := sourceDefault
	if l.Project != nil && get(l.Project) != get(l.Default) && get(l.Project) == get(l.Merged) {
		src = sourceProject
	} else if l.Global != nil && get(l.Global) != get(l.Default) {
		src = sourceGlobal
	}
	return configEntry{key, fmt.Sprintf("%v", val), src}
}

func cfgBoolPtr(key string, val *bool, l config.LayeredConfig, get func(*config.Config) *bool) configEntry {
	display := "—"
	if val != nil {
		display = fmt.Sprintf("%v", *val)
	}
	dp := get(l.Default)
	src := sourceDefault
	if l.Project != nil {
		pp := get(l.Project)
		mp := get(l.Merged)
		if !boolPtrEq(pp, dp) && boolPtrEq(pp, mp) {
			src = sourceProject
		}
	}
	if src == sourceDefault && l.Global != nil {
		gp := get(l.Global)
		if !boolPtrEq(gp, dp) {
			src = sourceGlobal
		}
	}
	return configEntry{key, display, src}
}

func boolPtrEq(a, b *bool) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return *a == *b
}

func providerSource(id string, l config.LayeredConfig) configSource {
	if l.Project != nil {
		for _, p := range l.Project.Providers {
			if p.ID == id {
				return sourceProject
			}
		}
	}
	if l.Global != nil {
		for _, p := range l.Global.Providers {
			if p.ID == id {
				return sourceGlobal
			}
		}
	}
	return sourceDefault
}

func mcpServerSource(s config.MCPServerConfig, l config.LayeredConfig) configSource {
	name := mcpServerName(s)
	if l.Project != nil {
		for _, ms := range l.Project.MCP.Servers {
			if mcpServerName(ms) == name {
				return sourceProject
			}
		}
	}
	if l.Global != nil {
		for _, ms := range l.Global.MCP.Servers {
			if mcpServerName(ms) == name {
				return sourceGlobal
			}
		}
	}
	return sourceDefault
}

func lspServerSource(s config.LSPServerConfig, l config.LayeredConfig) configSource {
	name := lspServerName(s)
	if l.Project != nil {
		for _, ls := range l.Project.LSP.Servers {
			if lspServerName(ls) == name {
				return sourceProject
			}
		}
	}
	if l.Global != nil {
		for _, ls := range l.Global.LSP.Servers {
			if lspServerName(ls) == name {
				return sourceGlobal
			}
		}
	}
	return sourceDefault
}

func mcpServerName(s config.MCPServerConfig) string {
	if s.Name != "" {
		return s.Name
	}
	return s.Command
}

func lspServerName(s config.LSPServerConfig) string {
	if s.Name != "" {
		return s.Name
	}
	return s.Command
}
