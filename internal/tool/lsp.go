package tool

import (
	"context"
	"fmt"
	"log"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/sageil/kodacode/v1/internal/config"
	"github.com/sageil/kodacode/v1/internal/lsp"
)

var lspParams = []byte(`{
	"type": "object",
	"properties": {
		"operations": {"type": "array", "items": {"type": "object"}, "description": "Batch: array of {action, filePath, filePaths, line, character, query} objects. All run concurrently."},
		"action": {"type": "string", "enum": ["definition", "references", "inspect", "diagnostics", "symbols", "find_references"], "description": "LSP operation"},
		"filePath": {"type": "string", "description": "Source file path"},
		"filePaths": {"type": "array", "items": {"type": "string"}, "description": "Multiple file paths for batch diagnostics"},
		"line": {"type": "integer", "description": "1-indexed line number"},
		"character": {"type": "integer", "description": "0-indexed character offset"},
		"query": {"type": "string", "description": "Symbol name for symbols/find_references"}
	}
}`)

const (
	lspTimeout         = 15 * time.Second
	lspReferencesLimit = 50
	lspSymbolsLimit    = 100
	lspCacheTTL        = 30 * time.Second
)

// lspCache deduplicates repeated LSP calls for the same action+file+position
// within a short window. The LLM often calls hover or diagnostics on the same
// file/symbol multiple times in a single turn.
type lspCache struct {
	mu      sync.Mutex
	entries map[string]lspCacheEntry
}

type lspCacheEntry struct {
	result  *Result
	err     error
	expires time.Time
}

type positionQueryServer interface {
	Name() string
	Definition(ctx context.Context, filePath string, line, character int) ([]lsp.Location, error)
	References(ctx context.Context, filePath string, line, character int) ([]lsp.Location, error)
	Hover(ctx context.Context, filePath string, line, character int) (*lsp.HoverResult, error)
}

func newLSPCache() *lspCache {
	return &lspCache{entries: make(map[string]lspCacheEntry)}
}

func (c *lspCache) get(key string) (*Result, error, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.entries[key]
	if !ok || time.Now().After(e.expires) {
		if ok {
			delete(c.entries, key)
		}
		return nil, nil, false
	}
	return e.result, e.err, true
}

func (c *lspCache) set(key string, result *Result, err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	// Lazy eviction of expired entries when cache grows.
	if len(c.entries) > 100 {
		now := time.Now()
		for k, e := range c.entries {
			if now.After(e.expires) {
				delete(c.entries, k)
			}
		}
	}
	c.entries[key] = lspCacheEntry{result: result, err: err, expires: time.Now().Add(lspCacheTTL)}
}

// defaultLSPServers returns the built-in server configs used when no servers
// are configured. These match the bootstrap config and documentation:
// gopls (Go), vtsls (TS/JS), and pyright (Python).
// Servers that aren't installed fail gracefully on first use.
func defaultLSPServers() []config.LSPServerConfig {
	return []config.LSPServerConfig{
		{
			Name:       "gopls",
			Command:    "gopls",
			Env:        map[string]string{"GOFLAGS": "-mod=mod"},
			Extensions: []string{".go"},
		},
		{
			Name:       "vtsls",
			Command:    "vtsls",
			Args:       []string{"--stdio"},
			Extensions: []string{".ts", ".tsx", ".js", ".jsx"},
		},
		{
			Name:       "pyright",
			Command:    "pyright-langserver",
			Args:       []string{"--stdio"},
			Extensions: []string{".py", ".pyi"},
		},
	}
}

// ResolveLSPServers returns enabled servers, falling back to defaults if none configured.
func ResolveLSPServers(configured []config.LSPServerConfig) []config.LSPServerConfig {
	if len(configured) == 0 {
		return defaultLSPServers()
	}
	var enabled []config.LSPServerConfig
	for _, s := range configured {
		if s.IsEnabled() {
			enabled = append(enabled, s)
		}
	}
	if len(enabled) == 0 {
		return defaultLSPServers()
	}
	return enabled
}

// NewLSPTool returns a Tool that provides language server queries via
// the LSP JSON-RPC protocol. The manager handles server lifecycle
// servers start on demand when a query arrives for a matching file extension.
func NewLSPTool(mgr *lsp.Manager) *Tool {
	cache := newLSPCache()
	return &Tool{
		Name:        "lsp",
		ReadOnly:    true,
		Description: prompt("lsp"),
		Parameters:  lspParams,
		Execute: func(ctx context.Context, ectx ExecutionContext, args []byte) (*Result, error) {
			return executeLSP(ctx, ectx, args, mgr, cache)
		},
	}
}

func lspCacheKey(action, filePath string, line, character int) string {
	switch action {
	case "diagnostics":
		return action + ":" + filePath
	default:
		return fmt.Sprintf("%s:%s:%d:%d", action, filePath, line, character)
	}
}

// lspOp is a single LSP operation, used both for top-level params and batch entries.
type lspOp struct {
	Action    string   `json:"action"`
	FilePath  string   `json:"filePath"`
	FilePaths []string `json:"filePaths"`
	Line      int      `json:"line"`
	Character int      `json:"character"`
	Query     string   `json:"query"`
}

func executeLSP(ctx context.Context, ectx ExecutionContext, args []byte, mgr *lsp.Manager, cache *lspCache) (*Result, error) {
	args = normalizeFilePathField(args)
	var params struct {
		lspOp
		Operations []lspOp `json:"operations"`
	}
	if err := flexUnmarshal(args, &params); err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(ctx, lspTimeout)
	defer cancel()

	rootURI := lsp.FileURI(ectx.WorkDir)

	// Build the list of operations either from the batch array or the single top-level action.
	ops := params.Operations
	if len(ops) == 0 {
		if params.Action == "" {
			return nil, fmt.Errorf("action or operations is required")
		}
		ops = []lspOp{params.lspOp}
	}

	// Single operation run directly, no batch overhead.
	if len(ops) == 1 {
		return executeSingleLSP(ctx, ectx.WorkDir, mgr, cache, rootURI, ops[0])
	}

	// Batch fan out all operations concurrently.
	type indexedResult struct {
		idx    int
		title  string
		output string
	}
	results := make([]indexedResult, len(ops))
	var wg sync.WaitGroup

	for i, op := range ops {
		wg.Add(1)
		go func(idx int, op lspOp) {
			defer wg.Done()
			r, err := executeSingleLSP(ctx, ectx.WorkDir, mgr, cache, rootURI, op)
			if err != nil {
				results[idx] = indexedResult{idx: idx, title: op.Action, output: fmt.Sprintf("error: %v", err)}
				return
			}
			results[idx] = indexedResult{idx: idx, title: r.Title, output: r.Output}
		}(i, op)
	}
	wg.Wait()

	var sb strings.Builder
	for i, r := range results {
		if i > 0 {
			sb.WriteString("\n\n")
		}
		fmt.Fprintf(&sb, "## %s\n%s", r.title, r.output)
	}

	return &Result{
		Title:  fmt.Sprintf("lsp: %d operations", len(ops)),
		Output: sb.String(),
		Metadata: map[string]any{
			"batch": true,
			"count": len(ops),
		},
	}, nil
}

// executeSingleLSP runs one LSP operation with caching.
func executeSingleLSP(ctx context.Context, workDir string, mgr *lsp.Manager, cache *lspCache, rootURI string, op lspOp) (*Result, error) {
	switch op.Action {
	case "symbols":
		if op.Query == "" {
			return nil, fmt.Errorf("query is required for symbols action")
		}
		key := "symbols:" + op.Query
		if r, err, ok := cache.get(key); ok {
			return r, err
		}
		r, err := lspWorkspaceSymbols(ctx, mgr, rootURI, op.Query)
		cache.set(key, r, err)
		return r, err

	case "definition", "references", "inspect":
		if op.FilePath == "" {
			return nil, fmt.Errorf("filePath is required for %s", op.Action)
		}
		if op.Line < 1 {
			return nil, fmt.Errorf("line is required and must be >= 1 for %s", op.Action)
		}
		filePath := resolvePath(op.FilePath, workDir)
		displayPath := filePath
		ext := filepath.Ext(filePath)
		srv, err := mgr.ServerFor(ctx, ext, rootURI)
		if err != nil {
			return nil, err
		}
		line := op.Line - 1
		key := lspCacheKey(op.Action, filePath, line, op.Character)
		if r, err, ok := cache.get(key); ok {
			return r, err
		}
		r, err := executePositionLSPAction(ctx, op.Action, srv, filePath, displayPath, line, op.Character)
		cache.set(key, r, err)
		return r, err

	case "diagnostics":
		files := op.FilePaths
		if op.FilePath != "" {
			files = append(files, op.FilePath)
		}
		if len(files) == 0 {
			return nil, fmt.Errorf("filePath or filePaths is required for diagnostics")
		}
		// Deduplicate file list.
		seen := make(map[string]struct{}, len(files))
		unique := files[:0:0]
		for _, f := range files {
			resolved := resolvePath(f, workDir)
			if _, dup := seen[resolved]; !dup {
				seen[resolved] = struct{}{}
				unique = append(unique, f)
			}
		}
		if len(unique) == 1 {
			key := lspCacheKey("diagnostics", resolvePath(unique[0], workDir), 0, 0)
			if r, err, ok := cache.get(key); ok {
				return r, err
			}
			r, err := batchDiagnostics(ctx, mgr, rootURI, workDir, unique)
			cache.set(key, r, err)
			return r, err
		}
		return batchDiagnostics(ctx, mgr, rootURI, workDir, unique)

	case "find_references":
		if op.Query == "" {
			return nil, fmt.Errorf("query is required for find_references")
		}
		key := "find_references:" + op.Query
		if r, err, ok := cache.get(key); ok {
			return r, err
		}
		r, err := lspFindReferences(ctx, mgr, rootURI, workDir, op.Query)
		cache.set(key, r, err)
		return r, err
	}

	return nil, fmt.Errorf("unknown action: %s", op.Action)
}

func executePositionLSPAction(ctx context.Context, action string, srv positionQueryServer, filePath, displayPath string, line, character int) (*Result, error) {
	switch action {
	case "definition":
		return lspDefinition(ctx, srv, filePath, displayPath, line, character)
	case "references":
		return lspReferences(ctx, srv, filePath, displayPath, line, character)
	case "inspect":
		return lspInspect(ctx, srv, filePath, displayPath, line, character)
	default:
		return nil, fmt.Errorf("unknown action: %s", action)
	}
}

func lspDefinition(ctx context.Context, srv positionQueryServer, filePath, displayPath string, line, character int) (*Result, error) {
	var (
		firstLocations []lsp.Location
		firstErr       error
	)
	for i, candidate := range bestEffortPositionCandidates(filePath, line+1, character) {
		locations, err := srv.Definition(ctx, filePath, line, candidate)
		if i == 0 {
			firstLocations = locations
			firstErr = err
		}
		if err == nil && len(locations) > 0 {
			return formatDefinitionResult(srv, displayPath, line, character, locations), nil
		}
	}
	if firstErr != nil {
		return nil, firstErr
	}
	return formatDefinitionResult(srv, displayPath, line, character, firstLocations), nil
}

func formatDefinitionResult(srv positionQueryServer, displayPath string, line, character int, locations []lsp.Location) *Result {
	if len(locations) == 0 {
		return &Result{
			Title:  fmt.Sprintf("lsp definition: %s:%d:%d", displayPath, line+1, character),
			Output: "No definition found.",
			Metadata: map[string]any{
				"action": "definition",
				"server": srv.Name(),
			},
		}
	}

	var sb strings.Builder
	for _, loc := range locations {
		path := lsp.URIToPath(loc.URI)
		fmt.Fprintf(&sb, "%s:%d:%d\n", path, loc.Range.Start.Line+1, loc.Range.Start.Character+1)
	}

	return &Result{
		Title:  fmt.Sprintf("lsp definition: %s:%d:%d", displayPath, line+1, character),
		Output: strings.TrimSpace(sb.String()),
		Metadata: map[string]any{
			"action": "definition",
			"server": srv.Name(),
		},
	}
}

func lspReferences(ctx context.Context, srv positionQueryServer, filePath, displayPath string, line, character int) (*Result, error) {
	var (
		firstLocations []lsp.Location
		firstErr       error
	)
	for i, candidate := range bestEffortPositionCandidates(filePath, line+1, character) {
		locations, err := srv.References(ctx, filePath, line, candidate)
		if i == 0 {
			firstLocations = locations
			firstErr = err
		}
		if err == nil && len(locations) > 0 {
			return formatReferencesResult(srv, displayPath, line, character, locations), nil
		}
	}
	if firstErr != nil {
		return nil, firstErr
	}
	return formatReferencesResult(srv, displayPath, line, character, firstLocations), nil
}

func formatReferencesResult(srv positionQueryServer, displayPath string, line, character int, locations []lsp.Location) *Result {
	if len(locations) == 0 {
		return &Result{
			Title:  fmt.Sprintf("lsp references: %s:%d:%d", displayPath, line+1, character),
			Output: "No references found.",
			Metadata: map[string]any{
				"action":    "references",
				"count":     0,
				"truncated": false,
				"server":    srv.Name(),
			},
		}
	}

	truncated := len(locations) > lspReferencesLimit
	if truncated {
		locations = locations[:lspReferencesLimit]
	}

	var sb strings.Builder
	for _, loc := range locations {
		path := lsp.URIToPath(loc.URI)
		fmt.Fprintf(&sb, "%s:%d:%d\n", path, loc.Range.Start.Line+1, loc.Range.Start.Character+1)
	}

	result := strings.TrimSpace(sb.String())
	if truncated {
		result += fmt.Sprintf("\n\n(Results truncated. Showing %d of more than %d references.)", lspReferencesLimit, lspReferencesLimit)
	}

	return &Result{
		Title:  fmt.Sprintf("lsp references: %s:%d:%d", displayPath, line+1, character),
		Output: result,
		Metadata: map[string]any{
			"action":    "references",
			"count":     len(locations),
			"truncated": truncated,
			"server":    srv.Name(),
		},
	}
}

func lspInspect(ctx context.Context, srv positionQueryServer, filePath, displayPath string, line, character int) (*Result, error) {
	var (
		firstHover *lsp.HoverResult
		firstErr   error
	)
	for i, candidate := range bestEffortPositionCandidates(filePath, line+1, character) {
		hover, err := srv.Hover(ctx, filePath, line, candidate)
		if i == 0 {
			firstHover = hover
			firstErr = err
		}
		if err == nil && hover != nil && hover.ExtractHoverText() != "" {
			return formatInspectResult(srv, displayPath, line, character, hover), nil
		}
	}
	if firstErr != nil {
		return nil, firstErr
	}
	return formatInspectResult(srv, displayPath, line, character, firstHover), nil
}

func formatInspectResult(srv positionQueryServer, displayPath string, line, character int, hover *lsp.HoverResult) *Result {
	text := ""
	if hover != nil {
		text = hover.ExtractHoverText()
	}
	if text == "" {
		text = "No type information available."
	}

	return &Result{
		Title:  fmt.Sprintf("lsp inspect: %s:%d:%d", displayPath, line+1, character),
		Output: text,
		Metadata: map[string]any{
			"action": "inspect",
			"server": srv.Name(),
		},
	}
}

func lspDiagnostics(ctx context.Context, srv *lsp.Server, filePath, displayPath string) (*Result, error) {
	if err := srv.EnsureOpen(ctx, filePath); err != nil {
		return nil, err
	}

	diags := srv.WaitDiagnostics(ctx, filePath)
	if len(diags) == 0 {
		return &Result{
			Title:  fmt.Sprintf("lsp diagnostics: %s", displayPath),
			Output: "No diagnostics found. Note: if this is the first query for this file, the language server may still be loading — retry after a few seconds.",
			Metadata: map[string]any{
				"action": "diagnostics",
				"server": srv.Name(),
			},
		}, nil
	}

	text, errors, warnings := formatGroupedDiagnostics(filePath, displayPath, diags)

	return &Result{
		Title:  fmt.Sprintf("lsp diagnostics: %s", displayPath),
		Output: text,
		Metadata: map[string]any{
			"action":   "diagnostics",
			"count":    len(diags),
			"errors":   errors,
			"warnings": warnings,
			"server":   srv.Name(),
		},
	}, nil
}

func batchDiagnostics(ctx context.Context, mgr *lsp.Manager, rootURI, workDir string, files []string) (*Result, error) {
	type fileResult struct {
		display string
		output  string
	}

	results := make([]fileResult, len(files))
	var wg sync.WaitGroup

	for i, raw := range files {
		filePath := resolvePath(raw, workDir)
		display := filePath
		ext := filepath.Ext(filePath)

		wg.Add(1)
		go func(idx int, fp, dp, ext string) {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					log.Printf("batch diagnostics panic (%s): %v", fp, r)
				}
			}()
			var parts []string

			srv, err := mgr.ServerFor(ctx, ext, rootURI)
			if err == nil {
				// Notify the server of any file changes before requesting diagnostics.
				_ = srv.NotifyChanged(ctx, fp)
				r, err := lspDiagnostics(ctx, srv, fp, dp)
				if err == nil && r != nil && !strings.HasPrefix(r.Output, "No diagnostics found") {
					parts = append(parts, r.Output)
				}
			}

			l := linterForFile(ext)
			if l != nil {
				if output := runLinter(ctx, workDir, l, fp); output != "" {
					parts = append(parts, output)
				}
			}

			if len(parts) > 0 {
				results[idx] = fileResult{display: dp, output: strings.Join(parts, "\n")}
			}
		}(i, filePath, display, ext)
	}

	wg.Wait()

	var sb strings.Builder
	totalIssues := 0
	for _, r := range results {
		if r.output == "" {
			continue
		}
		totalIssues++
		if sb.Len() > 0 {
			sb.WriteString("\n")
		}
		sb.WriteString(r.output)
	}

	if totalIssues == 0 {
		return &Result{
			Title:  fmt.Sprintf("diagnostics: %d files", len(files)),
			Output: "No diagnostics found.",
			Metadata: map[string]any{
				"action": "diagnostics",
				"files":  len(files),
			},
		}, nil
	}

	return &Result{
		Title:  fmt.Sprintf("diagnostics: %d files, %d with issues", len(files), totalIssues),
		Output: strings.TrimSpace(sb.String()),
		Metadata: map[string]any{
			"action":            "diagnostics",
			"files":             len(files),
			"files_with_issues": totalIssues,
		},
	}, nil
}

func lspWorkspaceSymbols(ctx context.Context, mgr *lsp.Manager, rootURI, query string) (*Result, error) {
	mgr.EnsureWorkspaceServers(ctx, rootURI)
	symbols, err := mgr.WorkspaceSymbol(ctx, query)
	if err != nil {
		return nil, err
	}
	if len(symbols) == 0 {
		return &Result{
			Title:  fmt.Sprintf("lsp symbols: %s", query),
			Output: fmt.Sprintf("No symbols found matching %q.", query),
			Metadata: map[string]any{
				"action":    "symbols",
				"count":     0,
				"truncated": false,
			},
		}, nil
	}

	truncated := len(symbols) > lspSymbolsLimit
	if truncated {
		symbols = symbols[:lspSymbolsLimit]
	}

	var sb strings.Builder
	for _, sym := range symbols {
		path := lsp.URIToPath(sym.Location.URI)
		kind := lsp.SymbolKindName(sym.Kind)
		line := sym.Location.Range.Start.Line + 1
		fmt.Fprintf(&sb, "%s  %s  %s:%d\n", sym.Name, kind, path, line)
	}

	result := strings.TrimSpace(sb.String())
	if truncated {
		result += fmt.Sprintf("\n\n(Results truncated. Showing %d of more than %d symbols.)", lspSymbolsLimit, lspSymbolsLimit)
	}

	return &Result{
		Title:  fmt.Sprintf("lsp symbols: %s", query),
		Output: result,
		Metadata: map[string]any{
			"action":    "symbols",
			"count":     len(symbols),
			"truncated": truncated,
		},
	}, nil
}

func lspFindReferences(ctx context.Context, mgr *lsp.Manager, rootURI, workDir, query string) (*Result, error) {
	mgr.EnsureWorkspaceServers(ctx, rootURI)
	symbols, err := mgr.WorkspaceSymbol(ctx, query)
	if err != nil || len(symbols) == 0 {
		return &Result{
			Title:  fmt.Sprintf("find_references: %s", query),
			Output: fmt.Sprintf("No symbol found matching %q. Try using grep to search for it.", query),
			Metadata: map[string]any{
				"action": "find_references",
				"count":  0,
			},
		}, nil
	}

	sym := symbols[0]
	filePath := lsp.URIToPath(sym.Location.URI)
	ext := filepath.Ext(filePath)
	srv, err := mgr.ServerFor(ctx, ext, rootURI)
	if err != nil {
		return nil, err
	}

	line := sym.Location.Range.Start.Line
	char := sym.Location.Range.Start.Character
	displayPath := filePath

	result, err := lspReferences(ctx, srv, filePath, displayPath, line, char)
	if err != nil {
		return nil, err
	}

	result.Title = fmt.Sprintf("find_references: %s (%s:%d)", query, displayPath, line+1)
	if result.Metadata == nil {
		result.Metadata = make(map[string]any)
	}
	result.Metadata["action"] = "find_references"
	result.Metadata["symbol"] = sym.Name
	result.Metadata["kind"] = lsp.SymbolKindName(sym.Kind)

	return result, nil
}
