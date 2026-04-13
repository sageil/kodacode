package tui

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/sageil/kodacode/v1/internal/tui/theme"
)

type ReplayItem struct {
	TurnIndex int
	Summary   string
	Files     []string
	CreatedAt string
}

type ReplayDialog struct {
	id     string
	items  []ReplayItem
	cursor int
	keys   dialogKeys
	width  int
	theme  *theme.Theme
}

func (d *ReplayDialog) ApplyTheme(t *theme.Theme) { d.theme = t }

func NewReplayDialog(id string, items []ReplayItem, th *theme.Theme) ReplayDialog {
	items = prepareReplayItems(items)
	return ReplayDialog{
		id:    id,
		items: items,
		keys:  defaultDialogKeys(),
		width: 64,
		theme: th,
	}
}

func (d ReplayDialog) Init() tea.Cmd { return nil }

func (d ReplayDialog) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	kp, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return d, nil
	}
	switch {
	case key.Matches(kp, d.keys.Up):
		if d.cursor > 0 {
			d.cursor--
		}
	case key.Matches(kp, d.keys.Down):
		if d.cursor < len(d.items)-1 {
			d.cursor++
		}
	case key.Matches(kp, d.keys.Select):
		if len(d.items) > 0 {
			return d, closeDialog(d.id, d.items[d.cursor])
		}
	case key.Matches(kp, d.keys.Cancel):
		return d, closeDialog(d.id, nil)
	}
	return d, nil
}

func (d ReplayDialog) View() tea.View {
	title := titleStyle(d.theme).Render("Session Snapshots")
	hint := hintStyle(d.theme).Render("↑/↓ navigate · enter restore · esc close")
	accent := lipgloss.NewStyle().Foreground(colorFrom(d.theme, "secondary", lipgloss.Color("141")))
	dim := lipgloss.NewStyle().Foreground(colorFrom(d.theme, "subtext", lipgloss.Color("241")))
	currentTag := lipgloss.NewStyle().Foreground(colorFrom(d.theme, "success", lipgloss.Color("71")))

	var sb strings.Builder
	sb.WriteString(title + "\n\n")

	if len(d.items) == 0 {
		sb.WriteString(dim.Render("  No session snapshots available.") + "\n")
	}

	for i, item := range d.items {
		dot := "○"
		dotStyle := dim
		if i == 0 {
			dot = "●"
			dotStyle = currentTag
		} else if len(item.Files) > 0 {
			dot = "●"
			dotStyle = accent
		}

		turnLabel := fmt.Sprintf("Turn %d", item.TurnIndex)
		fileLabel := fmt.Sprintf("%d files", len(item.Files))
		timeLabel := replayRelativeTime(item.CreatedAt)

		summary := item.Summary
		if summary == "" {
			summary = fmt.Sprintf("step %d", item.TurnIndex)
		}

		if i == d.cursor {
			sel := selectedItemStyle(d.theme)
			line := dotStyle.Render(dot) + " " + sel.Render(turnLabel)
			if i == 0 {
				line += " " + currentTag.Render("current")
			}
			sb.WriteString(line + "\n")
			sb.WriteString("  " + dim.Render(summary) + "\n")
			sb.WriteString("  " + dim.Render(fileLabel+" · "+timeLabel) + "\n")
			if len(item.Files) > 0 && len(item.Files) <= 8 {
				for _, f := range item.Files {
					sb.WriteString("    " + dim.Render(f) + "\n")
				}
			} else if len(item.Files) > 8 {
				for _, f := range item.Files[:6] {
					sb.WriteString("    " + dim.Render(f) + "\n")
				}
				sb.WriteString("    " + dim.Render(fmt.Sprintf("… and %d more", len(item.Files)-6)) + "\n")
			}
		} else {
			norm := itemStyle(d.theme)
			line := dotStyle.Render(dot) + " " + norm.Render(turnLabel)
			line += "  " + dim.Render(summary)
			line += "  " + dim.Render(fileLabel+" · "+timeLabel)
			sb.WriteString(line + "\n")
		}
	}

	sb.WriteString("\n" + hint)
	body := sb.String()

	box := dialogStyle(d.theme, d.width).Render(body)
	return tea.NewView(dropShadow(box, d.theme))
}

func replayRelativeTime(rfc3339 string) string {
	t, err := time.Parse(time.RFC3339, rfc3339)
	if err != nil {
		return ""
	}
	return relativeTime(t)
}

// prepareReplayItems filters, deduplicates, and sorts replay items for display.
func prepareReplayItems(items []ReplayItem) []ReplayItem {
	// Deduplicate by turn index. Keep the entry with the most files.
	best := make(map[int]ReplayItem, len(items))
	for _, item := range items {
		if existing, ok := best[item.TurnIndex]; !ok || len(item.Files) > len(existing.Files) {
			best[item.TurnIndex] = item
		}
	}

	out := make([]ReplayItem, 0, len(best))
	for _, item := range best {
		out = append(out, item)
	}

	// Sort by turn index descending (most recent first).
	sort.Slice(out, func(i, j int) bool {
		return out[i].TurnIndex > out[j].TurnIndex
	})

	return out
}
