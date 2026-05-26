package tool

import (
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
)

const searchMaxMatchesLimit = 200
const searchDefaultMaxMatches = searchMaxMatchesLimit

var (
	ErrSearchQueryRequired     = errors.New("query is required")
	ErrSearchPathRequired      = errors.New("path is required")
	ErrSearchModeInvalid       = errors.New("mode must be lexical or hybrid")
	ErrSearchMaxMatchesInvalid = fmt.Errorf("max_matches must be between 1 and %d", searchMaxMatchesLimit)
)

type searchInput struct {
	Query         string `json:"query"`
	Path          string `json:"path"`
	Mode          string `json:"mode"`
	Glob          string `json:"glob"`
	Regex         bool   `json:"regex"`
	CaseSensitive bool   `json:"case_sensitive"`
	MaxMatches    int    `json:"max_matches"`
	Notice        string `json:"-"`
}

func parseSearchInput(args json.RawMessage) (_ searchInput, err error) {
	defer func() {
		err = normalizeToolInputError(SearchToolName, err)
	}()
	var raw struct {
		Query         string          `json:"query"`
		Path          string          `json:"path"`
		Mode          *string         `json:"mode"`
		Glob          *string         `json:"glob"`
		Regex         json.RawMessage `json:"regex"`
		CaseSensitive json.RawMessage `json:"case_sensitive"`
		MaxMatches    json.RawMessage `json:"max_matches"`
		Limit         json.RawMessage `json:"limit"`
		MaxResults    json.RawMessage `json:"max_results"`
	}
	if err := DecodeArgs(SearchToolName, args, &raw); err != nil {
		return searchInput{}, err
	}
	if strings.TrimSpace(raw.Query) == "" {
		return searchInput{}, ErrSearchQueryRequired
	}
	if strings.TrimSpace(raw.Path) == "" {
		return searchInput{}, ErrSearchPathRequired
	}
	if err := validateSearchPath(strings.TrimSpace(raw.Path)); err != nil {
		return searchInput{}, err
	}
	mode := ""
	if raw.Mode != nil {
		mode = strings.TrimSpace(*raw.Mode)
	}
	switch mode {
	case "", "lexical", "hybrid":
	default:
		return searchInput{}, ErrSearchModeInvalid
	}
	regex, hasRegex, err := decodeOptionalBoolArg(SearchToolName, raw.Regex, "regex")
	if err != nil {
		return searchInput{}, err
	}
	if !hasRegex {
		regex = false
	}
	caseSensitive, hasCaseSensitive, err := decodeOptionalBoolArg(SearchToolName, raw.CaseSensitive, "case_sensitive")
	if err != nil {
		return searchInput{}, err
	}
	if !hasCaseSensitive {
		caseSensitive = false
	}
	maxMatches := searchDefaultMaxMatches
	notice := ""
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
	if value, ok, err := decodeOptionalIntArg(SearchToolName, maxMatchesRaw, maxMatchesField); err != nil {
		return searchInput{}, err
	} else if ok {
		maxMatches = value
	}
	if maxMatches <= 0 {
		return searchInput{}, ErrSearchMaxMatchesInvalid
	}
	if maxMatches > searchMaxMatchesLimit {
		maxMatches = searchMaxMatchesLimit
		notice = fmt.Sprintf("max_matches clamped to %d", searchMaxMatchesLimit)
	}
	return searchInput{
		Query:         raw.Query,
		Path:          raw.Path,
		Mode:          mode,
		Glob:          strings.TrimSpace(stringValue(raw.Glob)),
		Regex:         regex,
		CaseSensitive: caseSensitive,
		MaxMatches:    maxMatches,
		Notice:        notice,
	}, nil
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func joinSearchNotice(values ...string) string {
	parts := make([]string, 0, len(values))
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			parts = append(parts, trimmed)
		}
	}
	return strings.Join(parts, "; ")
}

func validateSearchPath(path string) error {
	if !isFilesystemRootPath(path) {
		return nil
	}
	return InvalidArguments(SearchToolName, fmt.Errorf("path must not be the filesystem root like %q; use \".\" or a workspace-relative path for project-wide search", filepath.Clean(path)))
}

func isFilesystemRootPath(path string) bool {
	path = strings.TrimSpace(path)
	if path == "" || !filepath.IsAbs(path) {
		return false
	}
	cleaned := filepath.Clean(path)
	if cleaned == "" {
		return false
	}
	return filepath.Dir(cleaned) == cleaned
}
