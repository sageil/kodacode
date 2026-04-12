package tui

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/sageil/kodacode/v1/internal/lsp"
	"github.com/sageil/kodacode/v1/internal/tui/theme"
)

// paletteSymbolsMsg delivers async symbol search results to the palette.
type paletteSymbolsMsg struct {
	symbols []lsp.SymbolResult
}

// PaletteFileResult is returned when the user selects a file.
type PaletteFileResult struct {
	Path string
}

// PaletteSymbolResult is returned when the user selects a symbol.
type PaletteSymbolResult struct {
	Name string
	File string
	Line int
}

// PaletteCommandResult is returned when the user selects a command.
type PaletteCommandResult struct {
	Command string
}

// paletteCategory identifies which data source an item belongs to.
type paletteCategory int

const (
	paletteCatFile paletteCategory = iota
	paletteCatSymbol
	paletteCatSession
	paletteCatCommand
	paletteCatHeader // non-selectable category divider
)

const paletteMaxVisible = 12

// paletteItem is a single entry in the results list.
type paletteItem struct {
	category paletteCategory
	label    string
	detail   string
	hint     string
	score    int

	// Payload — one field set per category.
	filePath    string
	symbolItem  lsp.SymbolResult
	sessionItem SessionItem
	commandName string
}

// PaletteDialog is a fuzzy finder overlay for files, sessions, and commands.
type PaletteDialog struct {
	id    string
	input textinput.Model
	keys  dialogKeys
	width int
	theme *theme.Theme

	// Data sources.
	files    []string
	symbols  []lsp.SymbolResult
	sessions []SessionItem
	commands []SlashCommand

	// LSP manager for async symbol queries (may be nil).
	lspMgr      *lsp.Manager
	appCtx      context.Context //nolint:containedctx // captured from App for tea.Cmd closures
	symbolQuery string

	// Filtered results (rebuilt on every keystroke).
	results []paletteItem
	cursor  int
	offset  int // scroll offset for visible window
}

func (d *PaletteDialog) ApplyTheme(t *theme.Theme) { d.theme = t }

// NewPaletteDialog creates a command palette pre-populated with data.
func NewPaletteDialog(
	id string,
	files []string,
	symbols []lsp.SymbolResult,
	sessions []SessionItem,
	commands []SlashCommand,
	lspMgr *lsp.Manager,
	appCtx context.Context,
	th *theme.Theme,
) PaletteDialog {
	fi := textinput.New()
	fi.Placeholder = "Search files, symbols, sessions, commands…"
	fi.CharLimit = 128
	fi.Focus()

	d := PaletteDialog{
		id:       id,
		input:    fi,
		keys:     filterDialogKeys(),
		width:    72,
		theme:    th,
		files:    files,
		symbols:  symbols,
		sessions: sessions,
		commands: commands,
		lspMgr:   lspMgr,
		appCtx:   appCtx,
	}
	d.refilter()
	return d
}

func (d *PaletteDialog) SetWidth(w int) { d.width = w }
func (d PaletteDialog) Width() int               { return d.width }

func (d PaletteDialog) Init() tea.Cmd { return nil }

func (d PaletteDialog) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case paletteSymbolsMsg:
		d.symbols = msg.symbols
		d.refilter()
		return d, nil

	case tea.KeyPressMsg:
		switch {
		case key.Matches(msg, d.keys.Up):
			d.moveCursor(-1)
			return d, nil
		case key.Matches(msg, d.keys.Down):
			d.moveCursor(1)
			return d, nil
		case key.Matches(msg, d.keys.Select):
			if item := d.selectedItem(); item != nil {
				return d, d.selectItem(item)
			}
			return d, nil
		case key.Matches(msg, d.keys.Cancel):
			return d, closeDialog(d.id, nil)
		}
	}

	// Forward to text input.
	prev := d.input.Value()
	var cmd tea.Cmd
	d.input, cmd = d.input.Update(msg)
	if d.input.Value() != prev {
		d.refilter()
		// Trigger async symbol search when query starts with @.
		if symCmd := d.fetchSymbolsIfNeeded(); symCmd != nil {
			cmd = tea.Batch(cmd, symCmd)
		}
	}
	return d, cmd
}

// fetchSymbolsIfNeeded dispatches an async LSP workspace/symbol query
// when the user types @query and the query differs from the last fetch.
// If no LSP server is running, it starts one using the dominant file extension.
func (d *PaletteDialog) fetchSymbolsIfNeeded() tea.Cmd {
	q := d.input.Value()
	if !strings.HasPrefix(q, "@") || d.lspMgr == nil {
		return nil
	}
	symQ := strings.TrimPrefix(q, "@")
	if len(symQ) < 1 {
		return nil
	}
	if symQ == d.symbolQuery {
		return nil // already fetched this query
	}
	d.symbolQuery = symQ
	mgr := d.lspMgr
	pCtx := d.appCtx
	files := d.files
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(pCtx, 10*time.Second)
		defer cancel()

		justStarted := false
		if !mgr.HasRunningServers() {
			ext := dominantLSPExt(files, mgr.Extensions())
			if ext != "" {
				rootURI := lsp.FileURI(".")
				_, _ = mgr.ServerFor(ctx, ext, rootURI)
				justStarted = true
			}
		}

		syms, err := mgr.WorkspaceSymbolSearch(ctx, symQ)
		if err != nil {
			return paletteSymbolsMsg{}
		}
		// TypeScript LSP servers return empty during initial indexing.
		// Retry a few times with a short delay if the server just started.
		if len(syms) == 0 && justStarted {
			for range 3 {
				time.Sleep(2 * time.Second)
				syms, err = mgr.WorkspaceSymbolSearch(ctx, symQ)
				if err != nil || len(syms) > 0 {
					break
				}
			}
		}
		return paletteSymbolsMsg{symbols: syms}
	}
}

// dominantLSPExt finds the most common file extension from files that
// has a configured LSP server.
func dominantLSPExt(files []string, lspExts []string) string {
	if len(lspExts) == 0 {
		return ""
	}
	extSet := make(map[string]bool, len(lspExts))
	for _, e := range lspExts {
		extSet[strings.ToLower(e)] = true
	}
	counts := make(map[string]int)
	for _, f := range files {
		ext := strings.ToLower(filepath.Ext(f))
		if extSet[ext] {
			counts[ext]++
		}
	}
	var best string
	var bestCount int
	for ext, n := range counts {
		if n > bestCount {
			best = ext
			bestCount = n
		}
	}
	return best
}

// selectItem returns the close command for the selected item.
func (d PaletteDialog) selectItem(item *paletteItem) tea.Cmd {
	switch item.category {
	case paletteCatFile:
		return closeDialog(d.id, PaletteFileResult{Path: item.filePath})
	case paletteCatSymbol:
		return closeDialog(d.id, PaletteSymbolResult{
			Name: item.symbolItem.Name,
			File: item.symbolItem.File,
			Line: item.symbolItem.Line,
		})
	case paletteCatSession:
		return closeDialog(d.id, SessionDialogResult{Session: item.sessionItem})
	case paletteCatCommand:
		return closeDialog(d.id, PaletteCommandResult{Command: item.commandName})
	default:
		return nil
	}
}

// selectedItem returns the currently selected item, or nil.
func (d PaletteDialog) selectedItem() *paletteItem {
	if d.cursor < 0 || d.cursor >= len(d.results) {
		return nil
	}
	item := &d.results[d.cursor]
	if item.category == paletteCatHeader {
		return nil
	}
	return item
}

// moveCursor moves the selection by delta, skipping header items.
func (d *PaletteDialog) moveCursor(delta int) {
	if len(d.results) == 0 {
		return
	}
	next := d.cursor + delta
	for next >= 0 && next < len(d.results) {
		if d.results[next].category != paletteCatHeader {
			d.cursor = next
			d.ensureVisible()
			return
		}
		next += delta
	}
}

func (d *PaletteDialog) ensureVisible() {
	if d.cursor < d.offset {
		d.offset = d.cursor
	}
	if d.cursor >= d.offset+paletteMaxVisible {
		d.offset = d.cursor - paletteMaxVisible + 1
	}
}

// refilter rebuilds d.results from the current query.
func (d *PaletteDialog) refilter() {
	q := d.input.Value()

	// Detect prefix shortcuts.
	isCmd := strings.HasPrefix(q, "/")
	isSess := strings.HasPrefix(q, "#")
	isSym := strings.HasPrefix(q, "@")
	matchQ := q
	if isSess {
		matchQ = strings.TrimPrefix(q, "#")
	} else if isSym {
		matchQ = strings.TrimPrefix(q, "@")
	}

	// Score each data source.
	var fileItems, symbolItems, sessionItems, cmdItems []paletteItem

	if !isCmd && !isSess && !isSym {
		for _, f := range d.files {
			ok, score := fuzzyScore(matchQ, f)
			if !ok {
				continue
			}
			name := filepath.Base(f)
			dir := filepath.Dir(f)
			fileItems = append(fileItems, paletteItem{
				category: paletteCatFile,
				label:    name,
				detail:   dir,
				hint:     "attach",
				score:    score,
				filePath: f,
			})
		}
	}

	symQ := matchQ
	if isSym {
		symQ = matchQ // prefix already stripped
	}
	for _, sym := range d.symbols {
		ok, score := fuzzyScore(symQ, sym.Name)
		if !ok {
			continue
		}
		if isSym {
			score += 1000
		}
		symbolItems = append(symbolItems, paletteItem{
			category:   paletteCatSymbol,
			label:      sym.Name,
			detail:     fmt.Sprintf("%s  %s:%d", sym.Kind, filepath.Base(sym.File), sym.Line),
			hint:       "attach",
			score:      score,
			symbolItem: sym,
		})
	}

	sessQ := matchQ
	if isSess {
		sessQ = matchQ // prefix already stripped
	}
	for _, s := range d.sessions {
		title := s.Title
		if title == "" {
			title = s.ID
		}
		ok, score := fuzzyScore(sessQ, title)
		if !ok {
			continue
		}
		if isSess {
			score += 1000
		}
		sessionItems = append(sessionItems, paletteItem{
			category:    paletteCatSession,
			label:       title,
			hint:        "resume",
			score:       score,
			sessionItem: s,
		})
	}

	for _, c := range d.commands {
		// Match against both command name and description.
		ok1, s1 := fuzzyScore(matchQ, c.Name)
		ok2, s2 := fuzzyScore(matchQ, c.Desc)
		if !ok1 && !ok2 {
			continue
		}
		score := max(s1, s2)
		if isCmd {
			score += 1000
		}
		cmdItems = append(cmdItems, paletteItem{
			category:    paletteCatCommand,
			label:       c.Name,
			detail:      c.Desc,
			hint:        "run",
			score:       score,
			commandName: c.Name,
		})
	}

	sortByScore(fileItems)
	sortByScore(symbolItems)
	sortByScore(sessionItems)
	sortByScore(cmdItems)

	type catGroup struct {
		name  string
		items []paletteItem
	}
	var groups []catGroup
	if isCmd {
		groups = []catGroup{
			{"COMMANDS", cmdItems},
			{"FILES", fileItems},
			{"SYMBOLS", symbolItems},
			{"SESSIONS", sessionItems},
		}
	} else if isSym {
		groups = []catGroup{
			{"SYMBOLS", symbolItems},
			{"FILES", fileItems},
			{"SESSIONS", sessionItems},
			{"COMMANDS", cmdItems},
		}
	} else if isSess {
		groups = []catGroup{
			{"SESSIONS", sessionItems},
			{"FILES", fileItems},
			{"SYMBOLS", symbolItems},
			{"COMMANDS", cmdItems},
		}
	} else {
		groups = []catGroup{
			{"FILES", fileItems},
			{"SYMBOLS", symbolItems},
			{"SESSIONS", sessionItems},
			{"COMMANDS", cmdItems},
		}
	}

	d.results = d.results[:0]
	maxPerCat := 8
	if q != "" {
		maxPerCat = 12
	}
	for _, g := range groups {
		if len(g.items) == 0 {
			continue
		}
		limited := g.items
		if len(limited) > maxPerCat {
			limited = limited[:maxPerCat]
		}
		d.results = append(d.results, paletteItem{
			category: paletteCatHeader,
			label:    fmt.Sprintf("%s (%d)", g.name, len(g.items)),
		})
		d.results = append(d.results, limited...)
	}

	// Position cursor on first selectable item.
	d.cursor = 0
	d.offset = 0
	for i, item := range d.results {
		if item.category != paletteCatHeader {
			d.cursor = i
			break
		}
	}
}

// fuzzyScore returns whether pattern matches text as a subsequence and a quality score.
// Contiguous substring matches get a large bonus so "free" ranks "(free)" above
// scattered subsequence matches like "Flash Preview".
func fuzzyScore(pattern, text string) (bool, int) {
	if pattern == "" {
		return true, 0
	}
	lp := strings.ToLower(pattern)
	lt := strings.ToLower(text)
	rp := []rune(lp)
	rt := []rune(lt)
	origRunes := []rune(text)

	pi := 0
	score := 0
	lastMatch := -1

	for ti := 0; ti < len(rt) && pi < len(rp); ti++ {
		if rt[ti] == rp[pi] {
			// Consecutive match bonus.
			if ti == lastMatch+1 {
				score += 8
			}
			// Word boundary bonus (after /, ., -, _, space, or start).
			if ti == 0 || rt[ti-1] == '/' || rt[ti-1] == '.' || rt[ti-1] == '-' || rt[ti-1] == '_' || rt[ti-1] == ' ' || rt[ti-1] == '(' {
				score += 6
			}
			// CamelCase boundary bonus.
			if ti > 0 && unicode.IsUpper(origRunes[ti]) && unicode.IsLower(origRunes[ti-1]) {
				score += 4
			}
			// Prefix match bonus.
			if ti == pi {
				score += 3
			}
			score += 2
			lastMatch = ti
			pi++
		}
	}

	if pi < len(rp) {
		return false, 0
	}

	// Large bonus for contiguous substring match — ensures "free" in "(free)"
	// ranks far above scattered subsequence matches.
	if strings.Contains(lt, lp) {
		score += 100
	}

	return true, score
}

func sortByScore(items []paletteItem) {
	if len(items) <= 1 {
		return
	}
	// Simple insertion sort — lists are small after filtering.
	for i := 1; i < len(items); i++ {
		for j := i; j > 0 && items[j].score > items[j-1].score; j-- {
			items[j], items[j-1] = items[j-1], items[j]
		}
	}
}

func (d PaletteDialog) View() tea.View {
	title := titleStyle(d.theme).Render("Command Palette")
	hint := hintStyle(d.theme).Render("↑/↓ navigate · ↵ select · esc close · / cmd · @ sym · # session")

	headerStyle := lipgloss.NewStyle().
		Foreground(colorFrom(d.theme, "subtext", lipgloss.Color("241"))).
		Bold(true)
	sel := selectedItemStyle(d.theme)
	norm := itemStyle(d.theme)
	dim := hintStyle(d.theme)
	accent := lipgloss.NewStyle().
		Foreground(colorFrom(d.theme, "secondary", lipgloss.Color("141")))
	sessionDot := lipgloss.NewStyle().
		Foreground(colorFrom(d.theme, "success", lipgloss.Color("71")))

	contentWidth := d.width - 8

	// Visible window.
	end := min(d.offset+paletteMaxVisible, len(d.results))
	visible := d.results[d.offset:end]

	var rows []string
	for i, item := range visible {
		idx := d.offset + i

		if item.category == paletteCatHeader {
			rows = append(rows, headerStyle.Render("  "+item.label))
			continue
		}

		isSelected := idx == d.cursor

		var line string
		switch item.category {
		case paletteCatFile:
			ext := fileExtBadge(item.label)
			name := truncate(item.label, contentWidth/2)
			dir := truncate(item.detail, contentWidth-len(name)-len(ext)-8)
			if isSelected {
				line = sel.Render(" "+ext) + " " + sel.Render(name) + "  " + dim.Render(dir+"/")
				if item.hint != "" {
					line += " " + accent.Render(item.hint)
				}
			} else {
				line = dim.Render(" "+ext) + " " + norm.Render(name) + "  " + dim.Render(dir+"/")
			}

		case paletteCatSymbol:
			name := truncate(item.label, contentWidth/2)
			detail := truncate(item.detail, contentWidth-len(name)-6)
			if isSelected {
				line = accent.Render(" λ") + " " + sel.Render(name) + "  " + dim.Render(detail)
			} else {
				line = dim.Render(" λ") + " " + accent.Render(name) + "  " + dim.Render(detail)
			}

		case paletteCatSession:
			label := truncate(item.label, contentWidth-12)
			timeStr := ""
			if !item.sessionItem.CreatedAt.IsZero() {
				timeStr = relativeTime(item.sessionItem.CreatedAt)
			}
			if isSelected {
				line = sessionDot.Render(" ◉") + " " + sel.Render(label)
				if timeStr != "" {
					line += "  " + dim.Render(timeStr)
				}
				if item.hint != "" {
					line += " " + accent.Render(item.hint)
				}
			} else {
				line = dim.Render(" ○") + " " + norm.Render(label)
				if timeStr != "" {
					line += "  " + dim.Render(timeStr)
				}
			}

		case paletteCatCommand:
			cmd := item.label
			desc := truncate(item.detail, contentWidth-len(cmd)-6)
			if isSelected {
				line = accent.Render(" /") + " " + sel.Render(cmd) + "  " + dim.Render(desc)
			} else {
				line = dim.Render(" /") + " " + accent.Render(cmd) + "  " + dim.Render(desc)
			}
		}

		rows = append(rows, line)
	}

	// Scroll indicators.
	if d.offset > 0 {
		rows = append([]string{dim.Render("  ↑ more")}, rows...)
	}
	if end < len(d.results) {
		rows = append(rows, dim.Render("  ↓ more"))
	}

	var body string
	if len(d.results) == 0 {
		empty := dim.Render("  No results")
		body = title + "\n\n" + d.input.View() + "\n\n" + empty + "\n\n" + hint
	} else {
		body = title + "\n\n" + d.input.View() + "\n\n" +
			strings.Join(rows, "\n") + "\n\n" + hint
	}

	box := dialogStyle(d.theme, d.width).Render(body)
	return tea.NewView(dropShadow(box, d.theme))
}

func fileExtBadge(filename string) string {
	ext := strings.ToLower(filepath.Ext(filename))
	switch ext {
	case ".ts", ".tsx":
		return "ts"
	case ".js", ".jsx", ".mjs", ".cjs":
		return "js"
	case ".go":
		return "go"
	case ".py", ".pyi":
		return "py"
	case ".rs":
		return "rs"
	case ".rb", ".rake":
		return "rb"
	case ".java":
		return "jv"
	case ".kt", ".kts":
		return "kt"
	case ".swift":
		return "sw"
	case ".c", ".h":
		return " c"
	case ".cpp", ".cc", ".cxx", ".hpp":
		return "c+"
	case ".cs":
		return "c#"
	case ".lua":
		return "lu"
	case ".zig":
		return "zg"
	case ".php":
		return "ph"
	case ".yml", ".yaml":
		return "ym"
	case ".json":
		return "js"
	case ".md":
		return "md"
	case ".css", ".scss", ".less":
		return "cs"
	case ".html", ".htm":
		return "ht"
	case ".sh", ".bash", ".zsh":
		return "sh"
	case ".sql":
		return "sq"
	case ".toml":
		return "tm"
	case ".env":
		return "ev"
	case ".lock":
		return "lk"
	default:
		if ext != "" {
			ext = ext[1:] // strip the dot
			if len(ext) > 2 {
				ext = ext[:2]
			}
			return ext
		}
		return "  "
	}
}
