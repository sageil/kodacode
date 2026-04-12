package tui

import (
	"fmt"
	"sort"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/sageil/kodacode/v1/internal/tui/theme"
)

type ModelItem struct {
	ProviderID     string
	ProviderName   string
	ModelID        string
	ModelName      string
	ContextSize    int
	MaxInputTokens int
	Reasoning      bool
	ToolCall       bool
	Vision         bool
	CostInput      float64
	CostOutput     float64
}

// modelRefreshRequest is a sentinel result indicating the user wants to
// refresh the model list rather than select a model.
type modelRefreshRequest struct{}

const modelListMaxVisible = 15

// ModelPickerDialog lets the user choose a provider + model combination
// with fuzzy search filtering and a scrollable, height-limited list.
type ModelPickerDialog struct {
	id       string
	title    string
	all      []ModelItem
	filtered []int
	cursor   int
	offset   int
	query    string
	keys     dialogKeys
	width    int
	theme    *theme.Theme
}

func (d *ModelPickerDialog) ApplyTheme(t *theme.Theme) {
	d.theme = t
}

// NewModelPickerDialog creates a model-picker dialog.
// selectedModelID is the full "providerID/modelID" to pre-select.
func NewModelPickerDialog(id string, items []ModelItem, selectedModelID string, th *theme.Theme) ModelPickerDialog {
	filtered := make([]int, len(items))
	for i := range items {
		filtered[i] = i
	}
	cursor := 0
	for i, item := range items {
		if item.ProviderID+"/"+item.ModelID == selectedModelID {
			cursor = i
			break
		}
	}
	offset := 0
	if cursor >= modelListMaxVisible {
		offset = cursor - modelListMaxVisible/2
	}
	// "> Provider / Model  200k  $3/$15  ✓R ✓T ✓V" + border(2) + dialog padding(6)
	width := 40 // minimum
	for _, item := range items {
		w := len(item.ProviderName) + len(" / ") + len(item.ModelName) + 30 + 2 + 6
		if w > width {
			width = w
		}
	}
	hintW := len("↑/↓ navigate • enter select • ctrl+r refresh • esc cancel") + 2 + 6
	if hintW > width {
		width = hintW
	}

	return ModelPickerDialog{
		id:       id,
		title:    "Select Model",
		all:      items,
		filtered: filtered,
		cursor:   cursor,
		offset:   offset,
		keys:     defaultDialogKeys(),
		width:    width,
		theme:    th,
	}
}

func (d *ModelPickerDialog) SetWidth(w int) { d.width = w }
func (d ModelPickerDialog) Width() int               { return d.width }

func (d ModelPickerDialog) Init() tea.Cmd { return nil }

func (d ModelPickerDialog) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	kp, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return d, nil
	}

	switch {
	case key.Matches(kp, d.keys.Up):
		if d.cursor > 0 {
			d.cursor--
			d.ensureVisible()
		}
		return d, nil
	case key.Matches(kp, d.keys.Down):
		if d.cursor < len(d.filtered)-1 {
			d.cursor++
			d.ensureVisible()
		}
		return d, nil
	case key.Matches(kp, d.keys.Select):
		if len(d.filtered) > 0 {
			return d, closeDialog(d.id, d.all[d.filtered[d.cursor]])
		}
		return d, nil
	case key.Matches(kp, d.keys.Cancel):
		return d, closeDialog(d.id, nil)
	case kp.String() == "ctrl+r":
		return d, closeDialog(d.id, modelRefreshRequest{})
	case kp.String() == "backspace":
		if len(d.query) > 0 {
			d.query = d.query[:len(d.query)-1]
			d.applyFilter()
		}
		return d, nil
	default:
		// Single printable character → append to search query.
		r := kp.String()
		if len(r) == 1 && r[0] >= ' ' && r[0] <= '~' {
			d.query += r
			d.applyFilter()
			return d, nil
		}
	}
	return d, nil
}

func (d *ModelPickerDialog) applyFilter() {
	if d.query == "" {
		d.filtered = make([]int, len(d.all))
		for i := range d.all {
			d.filtered[i] = i
		}
	} else {
		d.filtered = d.filtered[:0]
		type scored struct {
			idx   int
			score int
		}
		q := strings.ToLower(d.query)
		var matches []scored
		for i, item := range d.all {
			label := item.ProviderName + " " + item.ModelName + " " + item.ModelID
			ok, sc := fuzzyScore(d.query, label)
			if !ok {
				continue
			}
			// Only include non-substring matches if the query is very short
			// (1-2 chars) where fuzzy makes sense. For longer queries, require
			// the query to appear as a contiguous substring to avoid noisy results.
			if len(q) >= 3 && !strings.Contains(strings.ToLower(label), q) {
				continue
			}
			matches = append(matches, scored{idx: i, score: sc})
		}
		sort.Slice(matches, func(i, j int) bool {
			return matches[i].score > matches[j].score
		})
		for _, m := range matches {
			d.filtered = append(d.filtered, m.idx)
		}
	}
	d.cursor = 0
	d.offset = 0
}

func (d *ModelPickerDialog) ensureVisible() {
	if d.cursor < d.offset {
		d.offset = d.cursor
	}
	if d.cursor >= d.offset+modelListMaxVisible {
		d.offset = d.cursor - modelListMaxVisible + 1
	}
}

func (d ModelPickerDialog) View() tea.View {
	sel := selectedItemStyle(d.theme)
	norm := itemStyle(d.theme)
	dim := lipgloss.NewStyle().Foreground(colorFrom(d.theme, "subtext", lipgloss.Color("241")))
	green := lipgloss.NewStyle().Foreground(colorFrom(d.theme, "success", lipgloss.Color("2")))
	accent := lipgloss.NewStyle().Foreground(colorFrom(d.theme, "secondary", lipgloss.Color("4")))
	title := titleStyle(d.theme).Render(d.title)

	// Search input line.
	searchPrompt := dim.Render("Search: ")
	searchText := d.query
	if searchText == "" {
		searchText = dim.Render("type to filter...")
	}
	searchLine := searchPrompt + searchText

	const (
		colProvider = 12
		colModel    = 24
		colCtx      = 5
		colPrice    = 8
		colCaps     = 5
	)

	header := dim.Render(fmt.Sprintf("  %-*s %-*s %*s %*s %s",
		colProvider, "Provider",
		colModel, "Model",
		colCtx, "Ctx",
		colPrice, "$/M",
		"Caps"))

	var rows []string
	end := min(d.offset+modelListMaxVisible, len(d.filtered))
	for vi := d.offset; vi < end; vi++ {
		item := d.all[d.filtered[vi]]

		provName := truncate(item.ProviderName, colProvider)
		modelName := truncate(item.ModelName, colModel)
		ctx := formatContextSize(item.ContextSize)
		price := formatPrice(item.CostInput, item.CostOutput)

		// Capability dots.
		r := dim.Render("·")
		if item.Reasoning {
			r = green.Render("R")
		}
		t := dim.Render("·")
		if item.ToolCall {
			t = green.Render("T")
		}
		v := dim.Render("·")
		if item.Vision {
			v = green.Render("V")
		}
		caps := r + " " + t + " " + v

		provCol := accent.Render(fmt.Sprintf("%-*s", colProvider, provName))
		ctxCol := dim.Render(fmt.Sprintf("%*s", colCtx, ctx))
		priceCol := dim.Render(fmt.Sprintf("%*s", colPrice, price))

		if vi == d.cursor {
			modelCol := sel.Render(fmt.Sprintf("%-*s", colModel, modelName))
			rows = append(rows, sel.Render("> ")+provCol+" "+modelCol+" "+ctxCol+" "+priceCol+" "+caps)
		} else {
			modelCol := norm.Render(fmt.Sprintf("%-*s", colModel, modelName))
			rows = append(rows, norm.Render("  ")+provCol+" "+modelCol+" "+ctxCol+" "+priceCol+" "+caps)
		}
	}

	// Scroll indicators.
	if d.offset > 0 {
		rows = append([]string{dim.Render("  ↑ more")}, rows...)
	}
	if end < len(d.filtered) {
		rows = append(rows, dim.Render("  ↓ more"))
	}

	if len(d.filtered) == 0 {
		rows = append(rows, dim.Render("  no matches"))
	}

	count := dim.Render(strings.Repeat(" ", 2) + formatCount(len(d.filtered), len(d.all)))
	hint := hintStyle(d.theme).Render("↑/↓ navigate • enter select • ctrl+r refresh • esc cancel")

	body := title + "\n" + searchLine + "\n\n" + header + "\n" + strings.Join(rows, "\n") + "\n" + count + "\n\n" + hint
	box := dialogStyle(d.theme, d.width).Render(body)
	return tea.NewView(box)
}

func formatCount(filtered, total int) string {
	if filtered == total {
		return ""
	}
	return fmt.Sprintf("  %d/%d", filtered, total)
}

func formatContextSize(ctx int) string {
	if ctx <= 0 {
		return ""
	}
	if ctx >= 1000000 {
		return fmt.Sprintf("%.0fM", float64(ctx)/1000000)
	}
	return fmt.Sprintf("%dk", ctx/1000)
}

func formatPrice(input, output float64) string {
	if input == 0 && output == 0 {
		return "plan"
	}
	return fmt.Sprintf("$%s/$%s", formatCost(input), formatCost(output))
}

func formatCost(v float64) string {
	switch {
	case v == 0:
		return "0"
	case v >= 10:
		return fmt.Sprintf("%.0f", v)
	case v >= 1:
		return fmt.Sprintf("%.1f", v)
	default:
		return fmt.Sprintf("%.2f", v)
	}
}
