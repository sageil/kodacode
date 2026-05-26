package tool

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/sageil/kodacode/internal/workspace"
)

const LocateToolName = "locate"

var (
	ErrLocatePathRequired      = errors.New("path is required")
	ErrLocatePathNotFolder     = errors.New("path must be a directory")
	ErrLocateMaxMatchesInvalid = fmt.Errorf("max_matches must be between 1 and %d", searchMaxMatchesLimit)
	errLocateMatchLimit        = errors.New("match limit reached")
)

type LocateTool struct{}

func NewLocateTool() LocateTool {
	return LocateTool{}
}

func (LocateTool) Definition() Definition {
	description := "Find or list workspace paths under a directory. Optional `query` is a substring or glob; omit it to list paths under `path`. Results are bounded and non-pageable; narrow `query` or `path` instead of repeating broad calls."
	return Definition{
		Name:                LocateToolName,
		Description:         description,
		ProviderDescription: description,
		InputSchema:         json.RawMessage(`{"type":"object","properties":{"query":{"type":["string","null"],"description":"Optional partial path or shell-style glob like \"internal/service\" or \"*.go\". Omit, use null, or use an empty string to list paths under path. If results are too broad, narrow the query instead of repeating the same locate call."},"path":{"type":"string","description":"Directory to search. Relative paths resolve from the workspace directory. Narrow this scope when the candidate set is broad."},"include_hidden":{"type":["boolean","string","null"],"description":"Use null or omit this field to accept the default false and hide entries whose names start with '.'."},"max_matches":{"type":["integer","string","null"],"description":"Use null or omit this field to accept the default limit of 200 results. Values above 200 are clamped to 200. There is no pagination or continuation token; if you hit this limit, narrow query/path, raise max_matches up to 200, or switch to search."},"limit":{"type":["integer","string","null"],"description":"Alias for max_matches. Prefer max_matches for new calls."},"max_results":{"type":["integer","string","null"],"description":"Alias for max_matches. Prefer max_matches for new calls."}},"required":["path"],"additionalProperties":false}`),
		ProviderInputSchema: json.RawMessage(`{"type":"object","properties":{"query":{"type":["string","null"],"description":"Optional partial path or glob like \"internal/service\" or \"*.go\". Omit to list paths."},"path":{"type":"string","description":"Workspace-relative directory to search."},"include_hidden":{"type":["boolean","string","null"],"description":"Whether to include dotfiles and hidden directories. Defaults to false."},"max_matches":{"type":["integer","string","null"],"description":"Maximum results. Defaults to 200 and clamps at 200."}},"required":["path"],"additionalProperties":false}`),
		ArgumentExamples:    []string{`{"query":"*.go","path":"src","include_hidden":false,"max_matches":20}`, `{"path":"src","max_matches":50}`},
		RequiresWorkspace:   true,
		ParallelSafe:        true,
	}
}

func (LocateTool) PathRequests(args json.RawMessage) ([]PathRequest, error) {
	input, err := parseLocateInput(args)
	if err != nil {
		return nil, err
	}
	return []PathRequest{{
		Access: workspace.AccessList,
		Path:   input.Path,
		Reason: "find filesystem paths",
	}}, nil
}

func (LocateTool) NormalizedInputKey(args json.RawMessage) (string, error) {
	input, err := parseLocateInput(args)
	if err != nil {
		return "", err
	}
	key, err := json.Marshal(struct {
		Query         string `json:"query"`
		Path          string `json:"path"`
		IncludeHidden bool   `json:"include_hidden"`
		MaxMatches    int    `json:"max_matches"`
	}{
		Query:         input.Query,
		Path:          input.Path,
		IncludeHidden: input.IncludeHidden,
		MaxMatches:    input.MaxMatches,
	})
	if err != nil {
		return "", err
	}
	return string(key), nil
}

func (LocateTool) Execute(_ context.Context, ectx ExecutionContext, args json.RawMessage) (Result, error) {
	input, err := parseLocateInput(args)
	if err != nil {
		return Result{}, err
	}

	decision, err := ectx.ResolvePath(workspace.AccessList, input.Path)
	if err != nil {
		return Result{}, err
	}
	info, err := os.Stat(decision.ResolvedPath)
	if err != nil {
		return Result{}, err
	}
	if !info.IsDir() {
		return Result{}, ErrLocatePathNotFolder
	}

	result, err := locateMatches(decision.ResolvedPath, searchWorkspaceRoot(ectx), input)
	if err != nil {
		return Result{}, err
	}
	observed := observeLocateDirResources(result.ObservedDirs)
	if len(result.Paths) == 0 {
		lines := make([]string, 0, 2)
		if strings.TrimSpace(input.Notice) != "" {
			lines = append(lines, "notice: "+strings.TrimSpace(input.Notice))
		}
		lines = append(lines, "no paths found")
		return Result{Output: strings.Join(lines, "\n"), ObservedResources: observed}, nil
	}

	lines := make([]string, 0, len(result.Paths)+2)
	if strings.TrimSpace(input.Notice) != "" {
		lines = append(lines, "notice: "+strings.TrimSpace(input.Notice))
	}
	lines = append(lines, result.Paths...)
	if result.Truncated {
		lines = append(lines, locateTruncationNotice(input.MaxMatches))
	}
	return Result{Output: strings.Join(lines, "\n"), ObservedResources: observed}, nil
}

type locateInput struct {
	Query         string `json:"query"`
	Path          string `json:"path"`
	IncludeHidden bool   `json:"include_hidden"`
	MaxMatches    int    `json:"max_matches"`
	Notice        string `json:"-"`
}

func parseLocateInput(args json.RawMessage) (_ locateInput, err error) {
	defer func() {
		err = normalizeToolInputError(LocateToolName, err)
	}()
	var raw struct {
		Query         json.RawMessage `json:"query"`
		Path          string          `json:"path"`
		IncludeHidden json.RawMessage `json:"include_hidden"`
		MaxMatches    json.RawMessage `json:"max_matches"`
		Limit         json.RawMessage `json:"limit"`
		MaxResults    json.RawMessage `json:"max_results"`
	}
	if err := DecodeArgs(LocateToolName, args, &raw); err != nil {
		return locateInput{}, err
	}
	if strings.TrimSpace(raw.Path) == "" {
		return locateInput{}, ErrLocatePathRequired
	}
	query, err := decodeOptionalStringArg(LocateToolName, raw.Query, "query")
	if err != nil {
		return locateInput{}, err
	}
	query = strings.TrimSpace(query)
	if err := validateLocateQuery(query); err != nil {
		return locateInput{}, err
	}
	maxMatches := searchDefaultMaxMatches
	notice := ""
	includeHidden, hasIncludeHidden, err := decodeOptionalBoolArg(LocateToolName, raw.IncludeHidden, "include_hidden")
	if err != nil {
		return locateInput{}, err
	}
	if !hasIncludeHidden {
		includeHidden = false
	}
	maxMatchesRaw := raw.MaxMatches
	maxMatchesField := "max_matches"
	if !hasNonNullRawJSON(maxMatchesRaw) {
		if hasNonNullRawJSON(raw.Limit) {
			maxMatchesRaw = raw.Limit
			maxMatchesField = "limit"
		} else if hasNonNullRawJSON(raw.MaxResults) {
			maxMatchesRaw = raw.MaxResults
			maxMatchesField = "max_results"
		}
	}
	if value, ok, err := decodeOptionalIntArg(LocateToolName, maxMatchesRaw, maxMatchesField); err != nil {
		return locateInput{}, err
	} else if ok {
		maxMatches = value
	}
	if maxMatches <= 0 {
		return locateInput{}, ErrLocateMaxMatchesInvalid
	}
	if maxMatches > searchMaxMatchesLimit {
		maxMatches = searchMaxMatchesLimit
		notice = fmt.Sprintf("max_matches clamped to %d", searchMaxMatchesLimit)
	}
	return locateInput{
		Query:         query,
		Path:          strings.TrimSpace(raw.Path),
		IncludeHidden: includeHidden,
		MaxMatches:    maxMatches,
		Notice:        notice,
	}, nil
}

type locateResult struct {
	Paths        []string
	ObservedDirs []string
	Truncated    bool
}

func locateMatches(rootPath, workspaceRoot string, input locateInput) (locateResult, error) {
	matches := make([]string, 0, input.MaxMatches)
	observedDirs := make([]string, 0, 16)
	seenDirs := make(map[string]struct{}, 16)
	matcher := newLocateMatcher(input.Query)
	truncated := false

	var walk func(string) error
	walk = func(dir string) error {
		entries, observed, err := ReadDirWithObservedResource(dir)
		if err != nil {
			return err
		}
		if observed != nil {
			cleaned := filepath.Clean(observed.Path)
			if _, ok := seenDirs[cleaned]; !ok {
				seenDirs[cleaned] = struct{}{}
				observedDirs = append(observedDirs, cleaned)
			}
		}
		for _, entry := range entries {
			name := entry.Name()
			isDir := entry.IsDir()
			if isDir {
				switch name {
				case ".git", "node_modules", "vendor":
					continue
				}
			}
			if !input.IncludeHidden && strings.HasPrefix(name, ".") {
				continue
			}

			current := filepath.Join(dir, name)
			rel, err := filepath.Rel(rootPath, current)
			if err != nil {
				return err
			}
			rel = filepath.ToSlash(rel)
			if matcher.Match(rel) {
				label := locateDisplayPath(workspaceRoot, current)
				if isDir {
					label += "/"
				}
				matches = append(matches, label)
				if len(matches) >= input.MaxMatches {
					return errLocateMatchLimit
				}
			}
			if isDir {
				if err := walk(current); err != nil {
					return err
				}
			}
		}
		return nil
	}

	err := walk(rootPath)
	if err != nil && !errors.Is(err, errLocateMatchLimit) {
		return locateResult{}, err
	}
	if errors.Is(err, errLocateMatchLimit) {
		truncated = true
	}
	return locateResult{
		Paths:        matches,
		ObservedDirs: observedDirs,
		Truncated:    truncated,
	}, nil
}

func locateTruncationNotice(maxMatches int) string {
	if maxMatches < searchMaxMatchesLimit {
		return fmt.Sprintf("notice: showing first %d matches. Narrow query/path or raise max_matches up to %d.", maxMatches, searchMaxMatchesLimit)
	}
	return fmt.Sprintf("notice: showing first %d matches. Narrow query/path.", maxMatches)
}

type locateMatcher struct {
	query        string
	queryFolded  string
	globPattern  bool
	pathSpecific bool
}

func newLocateMatcher(query string) locateMatcher {
	query = strings.TrimSpace(query)
	return locateMatcher{
		query:        query,
		queryFolded:  strings.ToLower(query),
		globPattern:  locateHasGlob(query),
		pathSpecific: strings.Contains(query, "/"),
	}
}

func validateLocateQuery(query string) error {
	if !locateHasGlob(query) {
		return nil
	}
	sample := "placeholder"
	if strings.Contains(query, "/") {
		sample = "placeholder/path"
	}
	if _, err := path.Match(query, sample); err != nil {
		return fmt.Errorf("query contains an invalid glob pattern: %w", err)
	}
	return nil
}

func locateHasGlob(query string) bool {
	return strings.ContainsAny(query, "*?[")
}

func (m locateMatcher) Match(relativePath string) bool {
	if m.globPattern {
		return m.matchGlob(relativePath)
	}
	return strings.Contains(strings.ToLower(relativePath), m.queryFolded)
}

func (m locateMatcher) matchGlob(relativePath string) bool {
	if ok, err := path.Match(m.query, relativePath); err == nil && ok {
		return true
	}
	if m.pathSpecific {
		return false
	}
	ok, err := path.Match(m.query, path.Base(relativePath))
	return err == nil && ok
}

func locateDisplayPath(workspaceRoot, absolutePath string) string {
	if workspaceRoot == "" {
		return filepath.ToSlash(absolutePath)
	}
	rel, err := filepath.Rel(workspaceRoot, absolutePath)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return filepath.ToSlash(absolutePath)
	}
	return filepath.ToSlash(rel)
}

func observeLocateDirResources(paths []string) []ObservedResource {
	if len(paths) == 0 {
		return nil
	}
	out := make([]ObservedResource, 0, len(paths))
	for _, path := range paths {
		if observed, ok := ObserveDirEntriesResource(path); ok {
			out = append(out, observed)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
