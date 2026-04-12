package tui

import (
	"fmt"
	"sort"
	"strings"

	"charm.land/lipgloss/v2"
)

type SlashCommand struct {
	Name     string
	Desc     string
	Category string // groups commands in help dialog; empty = unlisted
	HelpText string // display name in help (e.g. "/pin <text>"); defaults to Name
}

func (f *Footer) SetSlashCommands(cmds []SlashCommand) {
	f.slashCommands = cmds
}

const maxSlashVisible = 10

func (f Footer) SlashCompletionHeight() int {
	if !f.slashActive || len(f.slashFiltered) == 0 {
		return 0
	}
	n := min(len(f.slashFiltered), maxSlashVisible)
	start := 0
	if f.slashCursor >= n {
		start = f.slashCursor - n + 1
	}
	end := start + n
	h := n
	if start > 0 {
		h++ // "↑ more"
	}
	if end < len(f.slashFiltered) {
		h++ // "↓ N more"
	}
	return h
}

func (f *Footer) filterSlashCommands() {
	text := f.input.Value()
	if !strings.HasPrefix(text, "/") {
		f.slashActive = false
		return
	}
	query := strings.ToLower(strings.TrimPrefix(text, "/"))
	if strings.Contains(query, " ") {
		f.slashActive = false
		return
	}
	f.slashActive = true
	// Collect prefix matches first, then fuzzy-only matches.
	// This ensures "/h" selects "/help" before "/theme".
	var prefix, fuzzyOnly []SlashCommand
	for _, cmd := range f.slashCommands {
		name := strings.TrimPrefix(cmd.Name, "/")
		if query == "" || strings.HasPrefix(name, query) {
			prefix = append(prefix, cmd)
		} else if ok, _ := fuzzyScore(query, name); ok {
			fuzzyOnly = append(fuzzyOnly, cmd)
		}
	}
	// Sort prefix matches by name length so shorter (more specific) matches rank first.
	sort.Slice(prefix, func(i, j int) bool {
		return len(prefix[i].Name) < len(prefix[j].Name)
	})
	f.slashFiltered = append(f.slashFiltered[:0], prefix...)
	f.slashFiltered = append(f.slashFiltered, fuzzyOnly...)
	if f.slashCursor >= len(f.slashFiltered) {
		f.slashCursor = max(0, len(f.slashFiltered)-1)
	}
}

func (f Footer) renderSlashCompletion() string {
	if !f.slashActive || len(f.slashFiltered) == 0 {
		return ""
	}

	dim := lipgloss.NewStyle().Foreground(colorFrom(f.theme, "subtext", lipgloss.Color("241")))
	selectedStyle := lipgloss.NewStyle().
		Foreground(colorFrom(f.theme, "text", lipgloss.Color("7"))).
		Background(colorFrom(f.theme, "overlay", lipgloss.Color("237"))).
		Bold(true)
	nameStyle := lipgloss.NewStyle().Foreground(colorFrom(f.theme, "primary", lipgloss.Color("62")))

	maxVisible := min(len(f.slashFiltered), maxSlashVisible)

	// Scroll window to keep cursor visible.
	start := 0
	if f.slashCursor >= maxVisible {
		start = f.slashCursor - maxVisible + 1
	}
	end := start + maxVisible

	w := f.width
	if f.boxed {
		w = f.boxWidth
	}
	w = max(w, 20)

	var sb strings.Builder

	if start > 0 {
		sb.WriteString(dim.Render("  ↑ more") + "\n")
	}

	for i := start; i < end; i++ {
		cmd := f.slashFiltered[i]
		label := fmt.Sprintf("  %-14s %s", cmd.Name, cmd.Desc)
		if len(label) > w-2 {
			label = label[:w-2]
		}
		if i == f.slashCursor {
			for len(label) < w-2 {
				label += " "
			}
			sb.WriteString(selectedStyle.Render(label))
		} else {
			name := fmt.Sprintf("  %-14s", cmd.Name)
			sb.WriteString(nameStyle.Render(name) + dim.Render(" "+cmd.Desc))
		}
		sb.WriteString("\n")
	}

	if end < len(f.slashFiltered) {
		sb.WriteString(dim.Render(fmt.Sprintf("  ↓ %d more", len(f.slashFiltered)-end)) + "\n")
	}

	return sb.String()
}
