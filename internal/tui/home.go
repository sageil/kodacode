package tui

import (
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/sageil/kodacode/v1/internal/tui/theme"
)

var logoLines = []string{
	"  _  __         _          ____          _      ",
	" | |/ /___   __| | __ _   / ___|___   __| | ___ ",
	" | ' // _ \\ / _` |/ _` | | |   / _ \\ / _` |/ _ \\",
	" | . \\ (_) | (_| | (_| | | |__| (_) | (_| |  __/",
	" |_|\\_\\___/ \\__,_|\\__,_|  \\____\\___/ \\__,_|\\___|",
}

const logoWidth = 50

const inputWidth = 80

type shimTickMsg struct{}

const shimInterval = 60 * time.Millisecond

const shimWidth = 8

func shimTick() tea.Cmd {
	return tea.Tick(shimInterval, func(time.Time) tea.Msg { return shimTickMsg{} })
}

// renderLogo renders the logo with a shimmer highlight at column shimCol.
// Characters whose column index falls within [shimCol, shimCol+shimWidth)
// are rendered in secondary; all others in primary.
// Both colors come from the active theme, making the animation fully themeable.
func renderLogo(th *theme.Theme, shimCol int) string {
	primary := lipgloss.NewStyle().Bold(true).
		Foreground(colorFrom(th, "primary", lipgloss.Color("62")))
	highlight := lipgloss.NewStyle().Bold(true).
		Foreground(colorFrom(th, "secondary", lipgloss.Color("75")))

	var lines []string
	for _, line := range logoLines {
		runes := []rune(line)
		var sb strings.Builder
		for col, ch := range runes {
			s := string(ch)
			if col >= shimCol && col < shimCol+shimWidth {
				sb.WriteString(highlight.Render(s))
			} else {
				sb.WriteString(primary.Render(s))
			}
		}
		lines = append(lines, sb.String())
	}
	return strings.Join(lines, "\n")
}

type homeOpenSessionMsg struct {
	sessionID string
}

type RecentSession struct {
	ID        string
	Title     string
	UpdatedAt time.Time
}

type Home struct {
	footer         Footer
	keys           KeyMap
	model          string // currently selected model
	providerName   string // display name for current provider
	variant        string // thinking effort: "low", "high", "max"
	agentID        string // currently selected agent ID
	agentName      string // display name for current agent
	width          int
	height         int
	theme          *theme.Theme
	shimCol        int // current shimmer column position
	gitBranch      string
	projectDir     string
	recentSessions []RecentSession
	recentCursor   int // selected index in recent sessions (-1 = none)
}

func (h *Home) ApplyTheme(t *theme.Theme) {
	h.theme = t
	h.footer.ApplyTheme(t)
}

func NewHome() Home {
	f := NewFooter()
	return Home{
		footer: f,
		keys:   DefaultKeyMap(),
	}
}

// SetSize propagates dimensions.
// The footer is always constrained to logoWidth regardless of terminal width.
func (h *Home) SetSize(w, height int) {
	h.width = w
	h.height = height
	h.footer.SetBoxed(inputWidth)
}

func (h *Home) SetModel(m string)           { h.model = m }
func (h *Home) SetProviderName(name string) { h.providerName = name }

func (h *Home) SetVariant(v string) { h.variant = v }

func (h *Home) SetAgent(id, name string) {
	h.agentID = id
	h.agentName = name
}

func (h *Home) SetGitBranch(branch string) { h.gitBranch = branch }

func (h *Home) SetProjectDir(dir string) { h.projectDir = dir }

func (h *Home) SetRecentSessions(sessions []RecentSession) {
	h.recentSessions = sessions
	h.recentCursor = -1
}

// SelectedSession returns the currently highlighted recent session ID, or "".
func (h *Home) SelectedSession() string {
	if h.recentCursor >= 0 && h.recentCursor < len(h.recentSessions) {
		return h.recentSessions[h.recentCursor].ID
	}
	return ""
}

func (h Home) Init() tea.Cmd {
	return h.footer.Focus()
}

func (h Home) Update(msg tea.Msg) (Home, tea.Cmd) {
	// When footer is in history search mode, let it handle all keys first.
	if _, isKey := msg.(tea.KeyPressMsg); isKey && h.footer.historySearch {
		var cmd tea.Cmd
		h.footer, cmd = h.footer.Update(msg)
		return h, cmd
	}

	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		if key.Matches(msg, h.keys.Quit) {
			return h, tea.Quit
		}
		// Ctrl+J / Ctrl+K navigate recent sessions.
		switch msg.String() {
		case "ctrl+j":
			if len(h.recentSessions) > 0 {
				h.recentCursor++
				if h.recentCursor >= len(h.recentSessions) {
					h.recentCursor = len(h.recentSessions) - 1
				}
				return h, nil
			}
		case "ctrl+k":
			if len(h.recentSessions) > 0 {
				h.recentCursor--
				if h.recentCursor < -1 {
					h.recentCursor = -1
				}
				return h, nil
			}
		case "esc":
			if h.recentCursor >= 0 {
				h.recentCursor = -1
				return h, nil
			}
		case "enter":
			// If a recent session is selected, emit a message to open it.
			if h.recentCursor >= 0 && h.recentCursor < len(h.recentSessions) {
				sid := h.recentSessions[h.recentCursor].ID
				return h, func() tea.Msg { return homeOpenSessionMsg{sessionID: sid} }
			}
		}
	case shimTickMsg:
		// Advance shimmer column. Rescheduling is handled by app.Update
		// so the chain survives route changes and open dialogs.
		h.shimCol = (h.shimCol + 1) % (logoWidth + shimWidth)
		return h, nil
	}

	var cmd tea.Cmd
	h.footer, cmd = h.footer.Update(msg)
	return h, cmd
}

func (h Home) View() string {
	logo := renderLogo(h.theme, h.shimCol)
	dim := lipgloss.NewStyle().Foreground(colorFrom(h.theme, "subtext", lipgloss.Color("241")))
	accent := lipgloss.NewStyle().Foreground(colorFrom(h.theme, "secondary", lipgloss.Color("4")))
	primary := lipgloss.NewStyle().Bold(true).Foreground(colorFrom(h.theme, "primary", lipgloss.Color("62")))
	greenColor := colorFrom(h.theme, "success", lipgloss.Color("76"))
	greenStyle := lipgloss.NewStyle().Foreground(greenColor)
	// Badges: agent + model · variant
	var badgeLine string
	if h.agentID != "" {
		display := h.agentName
		if display == "" {
			display = formatAgentName(h.agentID)
		}
		agentBadge := primary.Render(display)
		badgeLine = agentBadge
	}
	if h.model != "" {
		modelContent := stripProviderPrefix(h.model)
		if h.providerName != "" {
			modelContent += " (" + h.providerName + ")"
		}
		if h.variant != "" {
			modelContent += " · " + h.variant
		}
		modelBadge := accent.Render(modelContent)
		if badgeLine != "" {
			badgeLine += "  "
		}
		badgeLine += modelBadge
	}

	// Dashboard: project path + git branch
	var dashParts []string
	if h.projectDir != "" {
		dashParts = append(dashParts, dim.Render(h.projectDir))
	}
	if h.gitBranch != "" {
		dashParts = append(dashParts, greenStyle.Render("⎇")+" "+dim.Render(h.gitBranch))
	}
	dashLine := strings.Join(dashParts, "    ")

	var content strings.Builder
	content.WriteString(logo)

	// No model is configured, so show the welcome/setup message.
	if h.model == "" {
		warnColor := colorFrom(h.theme, "warning", lipgloss.Color("214"))
		warnStyle := lipgloss.NewStyle().Foreground(warnColor)
		content.WriteString("\n\n")
		content.WriteString(primary.Render("Welcome to KodaCode!"))
		content.WriteString("\n\n")
		content.WriteString(dim.Render("Get started by connecting a provider:"))
		content.WriteString("\n")
		content.WriteString("  " + accent.Render("/connect") + dim.Render("    Add an AI provider"))
		content.WriteString("\n")
		content.WriteString("  " + accent.Render("/help") + dim.Render("       View all commands"))
		content.WriteString("\n\n")
		content.WriteString(warnStyle.Render("⚠ No model configured."))

		// Calculate vertical layout for welcome view.
		contentStr := content.String()
		contentLines := strings.Count(contentStr, "\n") + 1
		footerH := h.footer.Height()
		unit := contentLines + 1 + footerH + 1
		topPad := max((h.height-unit)/2, 0)
		logoOffset := max((h.width-inputWidth)/2, 0)
		leftPad := strings.Repeat(" ", logoOffset)

		var sb strings.Builder
		for range topPad {
			sb.WriteString("\n")
		}
		sb.WriteString(center(contentStr, h.width, 0))
		sb.WriteString("\n")
		for i, line := range strings.Split(h.footer.View(), "\n") {
			if i > 0 {
				sb.WriteString("\n")
			}
			sb.WriteString(leftPad + line)
		}
		sb.WriteString("\n\n" + leftPad + dim.Render("  ") +
			accent.Render("/connect") + dim.Render("  ") +
			accent.Render("/help"))
		return sb.String()
	}

	if badgeLine != "" {
		content.WriteString("\n\n")
		content.WriteString(badgeLine)
	}
	if dashLine != "" {
		content.WriteString("\n")
		content.WriteString(dashLine)
	}

	// Recent sessions.
	selStyle := lipgloss.NewStyle().Bold(true).Foreground(colorFrom(h.theme, "primary", lipgloss.Color("62")))
	var recentBlock string
	if len(h.recentSessions) > 0 {
		var rsb strings.Builder
		rsb.WriteString(dim.Render("RECENT SESSIONS") + "  " + dim.Render("Ctrl+J/K navigate · Enter open") + "\n")
		maxSessions := 5
		if len(h.recentSessions) < maxSessions {
			maxSessions = len(h.recentSessions)
		}
		for i, s := range h.recentSessions[:maxSessions] {
			title := s.Title
			if title == "" {
				title = "Untitled"
			}
			title = truncate(title, 40)
			ago := formatTimeAgo(s.UpdatedAt)
			// Right-align the time within inputWidth.
			prefix := "  "
			titleStyle := accent
			if i == h.recentCursor {
				prefix = "> "
				titleStyle = selStyle
			}
			gap := max(inputWidth-4-len(title)-len(ago), 2)
			rsb.WriteString(prefix + titleStyle.Render(title) + strings.Repeat(" ", gap) + dim.Render(ago) + "\n")
		}
		recentBlock = rsb.String()
	}

	// Shortcuts.
	shortcutStyle := dim
	keyStyle := accent
	shortcuts := shortcutStyle.Render("  ") +
		keyStyle.Render("Tab") + shortcutStyle.Render(" agents  ") +
		keyStyle.Render("/models") + shortcutStyle.Render("  ") +
		keyStyle.Render("/sessions") + shortcutStyle.Render("  ") +
		keyStyle.Render("/help")

	// Calculate vertical layout.
	contentStr := content.String()
	contentLines := strings.Count(contentStr, "\n") + 1
	footerH := h.footer.Height()
	recentLines := 0
	if recentBlock != "" {
		recentLines = strings.Count(recentBlock, "\n") + 2 // +2 for gaps
	}
	shortcutLines := 1

	unit := contentLines + 1 + footerH + recentLines + shortcutLines
	topPad := max((h.height-unit)/2, 0)

	logoOffset := max((h.width-inputWidth)/2, 0)
	leftPad := strings.Repeat(" ", logoOffset)

	var sb strings.Builder
	for range topPad {
		sb.WriteString("\n")
	}
	sb.WriteString(center(contentStr, h.width, 0))
	sb.WriteString("\n")

	// Footer (input box).
	for i, line := range strings.Split(h.footer.View(), "\n") {
		if i > 0 {
			sb.WriteString("\n")
		}
		sb.WriteString(leftPad + line)
	}

	// Recent sessions below footer.
	if recentBlock != "" {
		sb.WriteString("\n\n")
		for line := range strings.SplitSeq(strings.TrimRight(recentBlock, "\n"), "\n") {
			sb.WriteString(leftPad + line + "\n")
		}
	}

	// Shortcuts.
	sb.WriteString("\n" + leftPad + shortcuts)

	_ = primary // used via badgeLine

	return sb.String()
}

func formatTimeAgo(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		m := int(d.Minutes())
		if m == 1 {
			return "1 min ago"
		}
		return fmt.Sprintf("%d mins ago", m)
	case d < 24*time.Hour:
		h := int(d.Hours())
		if h == 1 {
			return "1 hour ago"
		}
		return fmt.Sprintf("%d hours ago", h)
	case d < 48*time.Hour:
		return "yesterday"
	default:
		days := int(d.Hours() / 24)
		return fmt.Sprintf("%d days ago", days)
	}
}
