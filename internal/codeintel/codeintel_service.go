package codeintel

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/sageil/kodacode/internal/lsp"
	"github.com/sageil/kodacode/internal/tool"
	"github.com/sageil/kodacode/internal/workspaceedit"
)

const (
	codeIntelDiagnosticsTimeout           = 15 * time.Second
	codeIntelCodeActionDiagnosticsTimeout = 3 * time.Second
)

type WorkspaceLSPStatus struct {
	ActiveServers []string
}

type CodeIntelService struct {
	config   Config
	mu       sync.Mutex
	managers map[string]*lsp.Manager
}

func NewCodeIntelService(config Config) *CodeIntelService {
	return &CodeIntelService{
		config:   config,
		managers: make(map[string]*lsp.Manager),
	}
}

func (s *CodeIntelService) Navigator(primaryRoot string, additionalRoots []string) tool.CodeIntel {
	roots := orderedCodeIntelRoots(primaryRoot, additionalRoots)
	if len(roots) == 0 {
		return nil
	}
	return &workspaceCodeIntel{
		service: s,
		roots:   roots,
	}
}

func (s *CodeIntelService) SyncMutation(ctx context.Context, primaryRoot string, additionalRoots []string, plan workspaceedit.SyncPlan) {
	roots := orderedCodeIntelRoots(primaryRoot, additionalRoots)
	if len(roots) == 0 {
		return
	}
	for _, path := range plan.Changed {
		if manager := s.managerForPath(roots, path); manager != nil {
			_ = manager.NotifyChanged(ctx, path)
		}
	}
	for _, rename := range plan.Renamed {
		oldManager := s.managerForPath(roots, rename.OldPath)
		newManager := s.managerForPath(roots, rename.NewPath)
		switch {
		case oldManager == nil && newManager == nil:
			continue
		case oldManager != nil && oldManager == newManager:
			_ = oldManager.NotifyRenamed(ctx, rename.OldPath, rename.NewPath)
		default:
			if oldManager != nil {
				_ = oldManager.NotifyDeleted(rename.OldPath)
			}
			if newManager != nil {
				_ = newManager.NotifyChanged(ctx, rename.NewPath)
			}
		}
	}
	for _, path := range plan.Deleted {
		if manager := s.managerForPath(roots, path); manager != nil {
			_ = manager.NotifyDeleted(path)
		}
	}
}

func (s *CodeIntelService) Close(ctx context.Context) {
	s.mu.Lock()
	managers := make([]*lsp.Manager, 0, len(s.managers))
	for _, manager := range s.managers {
		managers = append(managers, manager)
	}
	s.managers = make(map[string]*lsp.Manager)
	s.mu.Unlock()

	for _, manager := range managers {
		manager.Shutdown(ctx)
	}
}

func (s *CodeIntelService) managerForPath(roots []string, path string) *lsp.Manager {
	root := bestCodeIntelRoot(roots, path)
	if root == "" {
		return nil
	}
	return s.managerForRoot(root)
}

func (s *CodeIntelService) managerForRoot(root string) *lsp.Manager {
	root = filepath.Clean(strings.TrimSpace(root))
	if root == "" {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if manager := s.managers[root]; manager != nil {
		return manager
	}
	manager := lsp.NewManager(root, ResolveServers(s.config, root))
	s.managers[root] = manager
	return manager
}

func (s *CodeIntelService) WorkspaceServerStatus(primaryRoot string, additionalRoots []string) *WorkspaceLSPStatus {
	if s == nil {
		return nil
	}
	roots := orderedCodeIntelRoots(primaryRoot, additionalRoots)
	if len(roots) == 0 {
		return nil
	}

	active := make(map[string]struct{})
	for _, root := range roots {
		s.mu.Lock()
		manager := s.managers[root]
		s.mu.Unlock()
		if manager == nil {
			continue
		}
		for _, name := range manager.RunningServerNames() {
			name = strings.TrimSpace(name)
			if name == "" {
				continue
			}
			active[name] = struct{}{}
		}
	}

	activeNames := sortedNames(active)
	if len(activeNames) == 0 {
		return nil
	}
	return &WorkspaceLSPStatus{
		ActiveServers: activeNames,
	}
}

func sortedNames(values map[string]struct{}) []string {
	if len(values) == 0 {
		return nil
	}
	names := make([]string, 0, len(values))
	for name := range values {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

type workspaceCodeIntel struct {
	service *CodeIntelService
	roots   []string
}

type diagnosticsServer interface {
	RefreshDiagnostics(ctx context.Context, filePath string) ([]lsp.Diagnostic, error)
}

func (w *workspaceCodeIntel) Definition(ctx context.Context, filePath string, line, character int) ([]tool.CodeIntelLocation, error) {
	manager := w.service.managerForPath(w.roots, filePath)
	if manager == nil {
		return nil, codeIntelNoticeError(codeIntelUnavailableNotice(fmt.Errorf("no code-intelligence workspace root matched %q", filePath)))
	}
	server, err := manager.ServerForPath(ctx, filePath)
	if err != nil {
		return nil, codeIntelUnavailableError(err)
	}
	locations, err := server.Definition(ctx, filePath, line, character)
	if err != nil {
		return nil, err
	}
	out := make([]tool.CodeIntelLocation, 0, len(locations))
	for _, location := range locations {
		out = append(out, tool.CodeIntelLocation{
			Path:      lsp.URIToPath(location.URI),
			Line:      location.Range.Start.Line + 1,
			Character: location.Range.Start.Character,
			EndLine:   location.Range.End.Line + 1,
			EndChar:   location.Range.End.Character,
		})
	}
	return out, nil
}

func (w *workspaceCodeIntel) Diagnostics(ctx context.Context, filePaths []string) ([]tool.CodeIntelFileDiagnostics, error) {
	grouped := groupCodeIntelPathsByRoot(w.roots, filePaths)
	return collectWorkspaceDiagnostics(ctx, grouped, codeIntelDiagnosticsTimeout, func(ctx context.Context, root, path string) (diagnosticsServer, error) {
		manager := w.service.managerForRoot(root)
		return manager.ServerForPath(ctx, path)
	})
}

func (s *CodeIntelService) DiagnosticsForFiles(ctx context.Context, primaryRoot string, additionalRoots []string, filePaths []string, timeout time.Duration) ([]tool.CodeIntelFileDiagnostics, error) {
	if s == nil || len(filePaths) == 0 {
		return nil, nil
	}
	roots := orderedCodeIntelRoots(primaryRoot, additionalRoots)
	if len(roots) == 0 {
		return nil, nil
	}
	grouped := groupCodeIntelPathsByRoot(roots, filePaths)
	return collectWorkspaceDiagnostics(ctx, grouped, timeout, func(ctx context.Context, root, path string) (diagnosticsServer, error) {
		manager := s.managerForRoot(root)
		return manager.ServerForPath(ctx, path)
	})
}

func collectWorkspaceDiagnostics(ctx context.Context, grouped map[string][]string, timeout time.Duration, serverForPath func(context.Context, string, string) (diagnosticsServer, error)) ([]tool.CodeIntelFileDiagnostics, error) {
	capacity := 0
	for _, paths := range grouped {
		capacity += len(paths)
	}
	out := make([]tool.CodeIntelFileDiagnostics, 0, capacity)
	for root, paths := range grouped {
		for _, path := range paths {
			fileDiagnostics := tool.CodeIntelFileDiagnostics{Path: path}
			server, err := serverForPath(ctx, root, path)
			if err != nil {
				if ctx.Err() != nil {
					return nil, ctx.Err()
				}
				fileDiagnostics.Error = normalizeDiagnosticsRefreshError(err, timeout)
				out = append(out, fileDiagnostics)
				continue
			}
			diagnostics, err := refreshServerDiagnostics(ctx, server, path, timeout)
			if err != nil {
				if ctx.Err() != nil {
					return nil, ctx.Err()
				}
				fileDiagnostics.Error = normalizeDiagnosticsRefreshError(err, timeout)
				out = append(out, fileDiagnostics)
				continue
			}
			for _, diagnostic := range diagnostics {
				fileDiagnostics.Diagnostics = append(fileDiagnostics.Diagnostics, tool.CodeIntelDiagnostic{
					Line:      diagnostic.Range.Start.Line + 1,
					Character: diagnostic.Range.Start.Character,
					Severity:  severityLabel(diagnostic.Severity),
					Message:   strings.TrimSpace(diagnostic.Message),
					Source:    strings.TrimSpace(diagnostic.Source),
				})
			}
			out = append(out, fileDiagnostics)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, nil
}

func refreshServerDiagnostics(ctx context.Context, server diagnosticsServer, filePath string, timeout time.Duration) ([]lsp.Diagnostic, error) {
	waitCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return server.RefreshDiagnostics(waitCtx, filePath)
}

func normalizeDiagnosticsRefreshError(err error, timeout time.Duration) string {
	switch {
	case err == nil:
		return ""
	case errors.Is(err, context.DeadlineExceeded):
		return fmt.Sprintf("timed out waiting for diagnostics after %s", timeout)
	default:
		return strings.TrimSpace(err.Error())
	}
}

func (w *workspaceCodeIntel) Symbols(ctx context.Context, query string) (tool.CodeIntelSymbolsResult, error) {
	seen := map[string]bool{}
	out := make([]tool.CodeIntelSymbol, 0, 16)
	errs := make([]error, 0, len(w.roots))
	for _, root := range w.roots {
		manager := w.service.managerForRoot(root)
		symbols, err := manager.WorkspaceSymbol(ctx, query)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		for _, symbol := range symbols {
			path := lsp.URIToPath(symbol.Location.URI)
			key := fmt.Sprintf("%s\x00%s\x00%d", symbol.Name, path, symbol.Location.Range.Start.Line+1)
			if seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, tool.CodeIntelSymbol{
				Name: symbol.Name,
				Kind: lsp.SymbolKindName(symbol.Kind),
				Path: path,
				Line: symbol.Location.Range.Start.Line + 1,
			})
		}
	}
	if len(out) == 0 && len(errs) > 0 {
		return tool.CodeIntelSymbolsResult{Notice: codeIntelUnavailableNotice(errors.Join(errs...))}, nil
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Name != out[j].Name {
			return out[i].Name < out[j].Name
		}
		if out[i].Path != out[j].Path {
			return out[i].Path < out[j].Path
		}
		return out[i].Line < out[j].Line
	})
	return tool.CodeIntelSymbolsResult{Symbols: out}, nil
}

func orderedCodeIntelRoots(primaryRoot string, additionalRoots []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, 1+len(additionalRoots))
	appendRoot := func(root string) {
		root = filepath.Clean(strings.TrimSpace(root))
		if root == "" || seen[root] {
			return
		}
		seen[root] = true
		out = append(out, root)
	}
	appendRoot(primaryRoot)
	for _, root := range additionalRoots {
		appendRoot(root)
	}
	return out
}

func bestCodeIntelRoot(roots []string, path string) string {
	path = filepath.Clean(path)
	best := ""
	for _, root := range roots {
		rel, err := filepath.Rel(root, path)
		if err != nil {
			continue
		}
		if rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))) {
			if len(root) > len(best) {
				best = root
			}
		}
	}
	if best != "" {
		return best
	}
	return ""
}

func groupCodeIntelPathsByRoot(roots []string, paths []string) map[string][]string {
	grouped := make(map[string][]string)
	for _, path := range paths {
		root := bestCodeIntelRoot(roots, path)
		if root == "" {
			continue
		}
		grouped[root] = append(grouped[root], path)
	}
	return grouped
}

func severityLabel(severity int) string {
	switch severity {
	case 1:
		return "error"
	case 2:
		return "warning"
	case 3:
		return "info"
	case 4:
		return "hint"
	default:
		return "unknown"
	}
}
