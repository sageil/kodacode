package tui

import (
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/sageil/kodacode/internal/provider"
	"github.com/sageil/kodacode/internal/tui/theme"
)

type reasoningVariantDialogResult struct {
	Model      provider.ModelRef
	Variant    string
	ApplyModel bool
	View       sessionView
}

type reasoningVariantDialogOption struct {
	Variant     string
	Label       string
	Description string
}

type reasoningVariantDialog struct {
	id          string
	frameWidth  int
	frameHeight int
	theme       *theme.Theme
	model       provider.ModelRef
	options     []reasoningVariantDialogOption
	cursor      int
	active      int
	applyModel  bool
	view        sessionView
}

func newReasoningVariantDialog(model provider.ModelRef, variants []string, current string, applyModel bool, view sessionView, th *theme.Theme) *reasoningVariantDialog {
	options := buildReasoningVariantDialogOptions(variants)
	cursor := 0
	current = strings.TrimSpace(strings.ToLower(current))
	for idx, option := range options {
		if option.Variant == current {
			cursor = idx
			break
		}
	}
	return &reasoningVariantDialog{
		id:         dialogIDReasoningVariant,
		theme:      th,
		model:      model,
		options:    options,
		cursor:     cursor,
		active:     cursor,
		applyModel: applyModel,
		view:       view,
	}
}

func buildReasoningVariantDialogOptions(variants []string) []reasoningVariantDialogOption {
	options := make([]reasoningVariantDialogOption, 0, len(variants)+1)
	label, description := reasoningVariantDialogText("")
	options = append(options, reasoningVariantDialogOption{
		Variant:     "",
		Label:       label,
		Description: description,
	})
	for _, variant := range variants {
		label, description := reasoningVariantDialogText(variant)
		if strings.TrimSpace(label) == "" {
			continue
		}
		options = append(options, reasoningVariantDialogOption{
			Variant:     variant,
			Label:       label,
			Description: description,
		})
	}
	return options
}

func reasoningVariantDialogText(variant string) (string, string) {
	switch variant {
	case "":
		return "Provider Default", "Use the provider's documented default reasoning variant"
	case provider.ReasoningVariantMinimal:
		return "Minimal", "Use minimal reasoning effort"
	case provider.ReasoningVariantNone:
		return "None", "Use the provider's none variant when supported"
	case provider.ReasoningVariantLow:
		return "Low", "Use low reasoning effort"
	case provider.ReasoningVariantMedium:
		return "Medium", "Use medium reasoning effort"
	case provider.ReasoningVariantHigh:
		return "High", "Use high reasoning effort"
	case provider.ReasoningVariantMax:
		return "Max", "Use the highest standard reasoning effort"
	case provider.ReasoningVariantXHigh:
		return "Extra High", "Use the extended reasoning effort tier"
	default:
		switch strings.TrimSpace(variant) {
		case "0":
			return "0 (Off)", "Use the provider's documented zero thinking budget"
		case "-1":
			return "-1 (Dynamic)", "Use the provider's documented dynamic thinking budget"
		default:
			if _, err := strconv.Atoi(strings.TrimSpace(variant)); err == nil {
				return variant, "Use this provider-native thinking budget"
			}
			return "", ""
		}
	}
}

func (d *reasoningVariantDialog) ID() string { return d.id }

func (d *reasoningVariantDialog) ApplyTheme(th *theme.Theme) {
	d.theme = th
}

func (d *reasoningVariantDialog) SetFrame(width, height int) {
	d.frameWidth = width
	d.frameHeight = height
}

func (d *reasoningVariantDialog) Update(msg tea.Msg) (dialogModel, tea.Cmd) {
	key, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return d, nil
	}
	switch key.String() {
	case "esc":
		return d, closeDialog(d.id, nil)
	case "up":
		if d.cursor > 0 {
			d.cursor--
		}
	case "down":
		if d.cursor < len(d.options)-1 {
			d.cursor++
		}
	case "enter":
		if len(d.options) == 0 || d.cursor < 0 || d.cursor >= len(d.options) {
			return d, nil
		}
		return d, closeDialog(d.id, reasoningVariantDialogResult{
			Model:      d.model,
			Variant:    d.options[d.cursor].Variant,
			ApplyModel: d.applyModel,
			View:       d.view,
		})
	}
	return d, nil
}

func (d *reasoningVariantDialog) Draw(surface dialogSurface, area dialogRenderArea) *tea.Cursor {
	d.SetFrame(area.width, area.height)
	width := desiredDialogWidth(d.frameWidth, 44, 72)
	content := renderStandaloneDialogContent(d.theme, max(width-dialogFrameInset*2, 1), dialogStandaloneFrame{
		Title: "Reasoning Variant",
		Body:  d.bodyView(),
		Hint:  "↑/↓ select • enter apply • esc cancel",
	})
	return drawDialogFrameOnSurface(surface, area, d.theme, width, content, nil)
}

func reasoningVariantEffortBar(th *theme.Theme, variant string) string {
	const maxBars = 5
	var filled int
	switch variant {
	case provider.ReasoningVariantMinimal:
		filled = 1
	case provider.ReasoningVariantLow:
		filled = 2
	case provider.ReasoningVariantMedium:
		filled = 3
	case provider.ReasoningVariantHigh:
		filled = 4
	case provider.ReasoningVariantXHigh, provider.ReasoningVariantMax:
		filled = 5
	}
	filledStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(colorFor(th, "primary", "#7aa2f7")))
	emptyStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(colorFor(th, "soft", "#565f89")))
	bar := filledStyle.Render(strings.Repeat("█", filled)) + emptyStyle.Render(strings.Repeat("░", maxBars-filled))
	if variant == provider.ReasoningVariantMax {
		bar += filledStyle.Render("+")
	}
	return bar
}

func (d *reasoningVariantDialog) bodyView() string {
	dot := lipgloss.NewStyle().
		Foreground(lipgloss.Color(colorFor(d.theme, "soft", "#565f89"))).
		Render(" · ")
	modelStr := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color(colorFor(d.theme, "text", "#ecf0ff"))).
		Render(d.model.ModelID)
	lines := []string{
		lipgloss.JoinHorizontal(lipgloss.Left,
			dialogSectionStyle(d.theme).Render(strings.ToUpper(d.model.ProviderID)),
			dot,
			modelStr,
		),
		"",
	}

	selected := dialogSelectedItemStyle(d.theme)
	normal := dialogItemStyle(d.theme)
	subtle := dialogHintStyle(d.theme)
	cursorGlyph := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(colorFor(d.theme, "primary", "#7aa2f7"))).Render("▸")
	activeGlyph := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#9ece6a")).Render("✓")

	maxLabelWidth := 0
	for _, opt := range d.options {
		if len(opt.Label) > maxLabelWidth {
			maxLabelWidth = len(opt.Label)
		}
	}

	var cursorDesc string
	for idx, option := range d.options {
		var gutter string
		switch idx {
		case d.cursor:
			gutter = cursorGlyph
		case d.active:
			gutter = activeGlyph
		default:
			gutter = " "
		}

		paddedLabel := option.Label + strings.Repeat(" ", max(maxLabelWidth-len(option.Label), 0))
		bar := reasoningVariantEffortBar(d.theme, option.Variant)

		var row string
		if idx == d.cursor {
			row = gutter + " " + selected.Render(" "+paddedLabel+" ") + "  " + bar
			cursorDesc = strings.TrimSpace(option.Description)
		} else {
			row = gutter + "  " + normal.Render(paddedLabel) + "   " + bar
		}
		lines = append(lines, row)
	}

	if cursorDesc != "" {
		lines = append(lines, subtle.Render("   "+cursorDesc))
	}
	return strings.Join(lines, "\n")
}
