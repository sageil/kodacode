package tool

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/sageil/kodacode/v1/internal/lsp"
)

type LSPDiagnosticsAdapter struct {
	mgr     *lsp.Manager
	rootURI string
}

func NewLSPDiagnosticsAdapter(mgr *lsp.Manager, projectDir string) *LSPDiagnosticsAdapter {
	return &LSPDiagnosticsAdapter{
		mgr:     mgr,
		rootURI: lsp.FileURI(projectDir),
	}
}

// FileDiagnostics returns cached LSP diagnostics for the file. If none are
// cached, it opens the file and waits briefly for the server to respond.
// TypeScript servers often need 10-30s for project analysis on first open,
// so this method prioritizes returning quickly over completeness — the model
// can always call `lsp diagnostics` explicitly for a thorough check.
func (a *LSPDiagnosticsAdapter) FileDiagnostics(ctx context.Context, filePath string) string {
	ext := filepath.Ext(filePath)
	if ext == "" {
		return ""
	}

	srv, err := a.mgr.ServerFor(ctx, ext, a.rootURI)
	if err != nil {
		return ""
	}

	if err := srv.NotifyChanged(ctx, filePath); err != nil {
		return ""
	}

	diags := srv.WaitDiagnostics(ctx, filePath)
	if len(diags) == 0 {
		return ""
	}
	text, _, _ := formatGroupedDiagnostics(filePath, filePath, diags)
	return text
}

type diagGroup struct {
	severity int
	message  string
	lines    []int
}

func formatGroupedDiagnostics(filePath, displayPath string, diags []lsp.Diagnostic) (string, int, int) {
	lineCount := countFileLines(filePath)

	groups := make(map[string]*diagGroup)
	var order []string
	errors, warnings := 0, 0

	for _, d := range diags {
		line := d.Range.Start.Line + 1
		if lineCount > 0 && line > lineCount {
			continue
		}
		switch d.Severity {
		case 1:
			errors++
		case 2:
			warnings++
		}
		key := fmt.Sprintf("%d:%s", d.Severity, d.Message)
		if g, ok := groups[key]; ok {
			g.lines = append(g.lines, line)
		} else {
			groups[key] = &diagGroup{
				severity: d.Severity,
				message:  d.Message,
				lines:    []int{line},
			}
			order = append(order, key)
		}
	}

	// Sort: errors first, then warnings, then by first occurrence.
	sort.SliceStable(order, func(i, j int) bool {
		return groups[order[i]].severity < groups[order[j]].severity
	})

	total := errors + warnings
	for _, d := range diags {
		if d.Severity > 2 {
			total++
		}
	}

	var sb strings.Builder

	// Summary line.
	var parts []string
	if errors > 0 {
		parts = append(parts, fmt.Sprintf("%d errors", errors))
	}
	if warnings > 0 {
		parts = append(parts, fmt.Sprintf("%d warnings", warnings))
	}
	infoHint := total - errors - warnings
	if infoHint > 0 {
		parts = append(parts, fmt.Sprintf("%d info/hints", infoHint))
	}
	fmt.Fprintf(&sb, "%s (%d unique) in %s\n", strings.Join(parts, ", "), len(groups), displayPath)

	// Grouped lines.
	for _, key := range order {
		g := groups[key]
		sev := lsp.SeverityString(g.severity)
		lineStrs := make([]string, len(g.lines))
		for i, l := range g.lines {
			lineStrs[i] = fmt.Sprintf("%d", l)
		}
		linesRef := "line " + lineStrs[0]
		if len(g.lines) > 1 {
			linesRef = "lines " + strings.Join(lineStrs, ", ")
		}
		if len(g.lines) > 1 {
			fmt.Fprintf(&sb, "  %s: %s (×%d) %s\n", sev, g.message, len(g.lines), linesRef)
		} else {
			fmt.Fprintf(&sb, "  %s: %s %s\n", sev, g.message, linesRef)
		}
	}

	return strings.TrimSpace(sb.String()), errors, warnings
}

func countFileLines(filePath string) int {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return 0
	}
	if len(data) == 0 {
		return 0
	}
	n := bytes.Count(data, []byte{'\n'})
	if data[len(data)-1] != '\n' {
		n++
	}
	return n
}
