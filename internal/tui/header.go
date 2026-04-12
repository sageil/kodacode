package tui

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/sageil/kodacode/v1/internal/tui/theme"
)

const headerHeight = 2 // header line + separator

// Header renders the session top bar as a single background band:
type Header struct {
	agentID      string
	agentName    string
	modelID      string
	modelInfo    string // e.g. "200k  $3/$15"
	providerName string // display name for current provider
	variant      string // thinking effort: "low", "high", "max"
	activeModel  string // utility model currently in use (empty when using primary)
	title        string
	width        int
	theme        *theme.Theme
}

func (h *Header) ApplyTheme(t *theme.Theme) {
	h.theme = t
}

func NewHeader() Header { return Header{} }

func (h *Header) SetSize(w int) { h.width = w }

func (h *Header) SetAgent(id, name string) {
	h.agentID = id
	h.agentName = name
}

func (h *Header) SetModel(id string)          { h.modelID = id }
func (h *Header) SetProviderName(name string) { h.providerName = name }

func (h *Header) SetVariant(v string) { h.variant = v }

func (h *Header) SetModelInfo(info string) { h.modelInfo = info }

func (h *Header) SetSessionID(_ string) {}

func (h *Header) SetActiveModel(model string) { h.activeModel = model }
func (h *Header) SetTitle(title string)       { h.title = title }

// stripProviderPrefix removes the "provider/" prefix from a model ID.
func stripProviderPrefix(modelID string) string {
	if _, after, ok := strings.Cut(modelID, "/"); ok {
		return after
	}
	return modelID
}

func (h Header) View() string {
	primary := lipgloss.NewStyle().Bold(true).Foreground(colorFrom(h.theme, "primary", lipgloss.Color("62")))
	dim := lipgloss.NewStyle().Foreground(colorFrom(h.theme, "subtext", lipgloss.Color("241")))
	faint := lipgloss.NewStyle().Foreground(colorFrom(h.theme, "subtext", lipgloss.Color("241"))).Faint(true)
	overlayColor := colorFrom(h.theme, "overlay", lipgloss.Color("236"))

	barWidth := h.width
	if barWidth < 1 {
		barWidth = 80
	}

	// Left: agent name (bold primary) + model badge + metadata
	var leftParts []string
	if h.agentID != "" {
		display := h.agentName
		if display == "" {
			display = formatAgentName(h.agentID)
		}
		leftParts = append(leftParts, primary.Render(display))
	}
	if h.modelID != "" {
		// Model + provider + variant as a pill badge with background.
		badgeContent := stripProviderPrefix(h.modelID)
		if h.providerName != "" {
			badgeContent += " (" + h.providerName + ")"
		}
		if h.variant != "" {
			badgeContent += " · " + h.variant
		}
		badgeTextColor := colorFrom(h.theme, "text", lipgloss.Color("7"))
		badge := lipgloss.NewStyle().
			Foreground(badgeTextColor).
			Background(overlayColor).
			PaddingLeft(1).
			PaddingRight(1).
			Render(badgeContent)
		leftParts = append(leftParts, badge)

		// Model info (context, price) as dim text after the badge.
		if h.modelInfo != "" {
			leftParts = append(leftParts, dim.Render(h.modelInfo))
		}
	}
	leftStr := strings.Join(leftParts, "  ")

	// Center: active utility model (shown during mid-loop downgrade)
	var center string
	if h.activeModel != "" {
		center = dim.Render("active: " + h.activeModel)
	}

	// Right: session title
	var right string
	if h.title == "" {
		right = faint.Render("Untitled")
	} else {
		right = dim.Render(truncate(h.title, 40))
	}

	leftWidth := lipgloss.Width(leftStr) + 2
	rightWidth := lipgloss.Width(right) + 2
	centerWidth := lipgloss.Width(center)
	used := leftWidth + rightWidth + centerWidth
	gap := max(barWidth-used, 2)
	var bar string
	if centerWidth > 0 {
		leftGap := gap / 2
		rightGap := gap - leftGap
		bar = " " + leftStr + strings.Repeat(" ", leftGap) + center + strings.Repeat(" ", rightGap) + right + " "
	} else {
		bar = " " + leftStr + strings.Repeat(" ", gap) + right + " "
	}
	separator := dim.Render(strings.Repeat("─", barWidth))

	return bar + "\n" + separator
}
