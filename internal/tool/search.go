package tool

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/sageil/kodacode/v1/internal/search"
)

var searchParams = []byte(`{
	"type": "object",
	"properties": {
		"query": {"type": "string", "description": "Search query — matches against file names and file content"},
		"path": {"type": "string", "description": "Directory to search in (defaults to working directory)"},
		"include": {"type": "string", "description": "File pattern filter (e.g. \"*.go\", \"*.{ts,tsx}\")"}
	},
	"required": ["query"]
}`)

const searchLimit = 50

func NewSearchTool(searcher *search.Searcher) *Tool {
	return &Tool{
		Name:        "search",
		ReadOnly:    true,
		Description: prompt("search"),
		Parameters:  searchParams,
		Execute: func(ctx context.Context, ectx ExecutionContext, args []byte) (*Result, error) {
			return executeSearch(ctx, ectx, args, searcher)
		},
	}
}

// searchMatch holds a single result from the search.
type searchMatch struct {
	path     string
	rank     int // 0 = exact name, 1 = name contains, 2 = content match
	lines    []contentLine
	relPath  string
}

type contentLine struct {
	num  int
	text string
}

func executeSearch(ctx context.Context, ectx ExecutionContext, args []byte, searcher *search.Searcher) (*Result, error) {
	var params struct {
		Query   string `json:"query"`
		Path    string `json:"path"`
		Include string `json:"include"`
	}
	if err := flexUnmarshal(args, &params); err != nil {
		return nil, fmt.Errorf("invalid parameters: %w", err)
	}
	if params.Query == "" {
		return nil, fmt.Errorf("query is required")
	}

	root := params.Path
	if root == "" {
		if ectx.WorkDir != "" {
			root = ectx.WorkDir
		} else {
			var err error
			root, err = os.Getwd()
			if err != nil {
				return nil, fmt.Errorf("failed to get working directory: %w", err)
			}
		}
	} else {
		root = resolvePath(root, ectx.WorkDir)
	}

	queryLower := strings.ToLower(params.Query)

	var (
		fileMatches    []searchMatch
		contentMatches []searchMatch
		mu             sync.Mutex
		wg             sync.WaitGroup
	)

	wg.Add(2)

	// 1. File name search: walk directory and match file names.
	go func() {
		defer wg.Done()
		var results []searchMatch
		fsys := os.DirFS(root)
		scanned := 0
		lastProgress := 0
		_ = fs.WalkDir(fsys, ".", func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if ctx.Err() != nil {
				return ctx.Err()
			}
			// Skip ignored directories.
			if d.IsDir() && ectx.IsIgnored(path+"/") {
				return fs.SkipDir
			}
			if d.IsDir() || ectx.IsIgnored(path) {
				return nil
			}

			scanned++
			if ectx.WriteOutput != nil && scanned-lastProgress >= 500 {
				lastProgress = scanned
				ectx.WriteOutput(fmt.Sprintf("searching… %d files scanned\n", scanned))
			}

			name := d.Name()

			// Apply include filter if set.
			if params.Include != "" {
				matched, _ := filepath.Match(params.Include, name)
				if !matched {
					return nil
				}
			}

			nameLower := strings.ToLower(name)
			absPath := filepath.Join(root, path)

			// Check for exact match (name without extension equals query).
			nameNoExt := strings.TrimSuffix(nameLower, filepath.Ext(nameLower))
			if nameNoExt == queryLower || nameLower == queryLower {
				rel, _ := filepath.Rel(root, absPath)
				results = append(results, searchMatch{
					path:    absPath,
					rank:    0,
					relPath: rel,
				})
			} else if strings.Contains(nameLower, queryLower) {
				rel, _ := filepath.Rel(root, absPath)
				results = append(results, searchMatch{
					path:    absPath,
					rank:    1,
					relPath: rel,
				})
			}
			return nil
		})
		mu.Lock()
		fileMatches = results
		mu.Unlock()
	}()

	// 2. Content search via runGrep (rg → grep → pure Go).
	go func() {
		defer wg.Done()
		fixedString := !looksLikeRegex(params.Query)
		grepMatches, err := runGrep(ctx, params.Query, params.Include, root, fixedString)
		if err != nil || len(grepMatches) == 0 {
			return
		}

		perFile := make(map[string][]contentLine)
		var fileOrder []string
		for _, m := range grepMatches {
			if _, seen := perFile[m.Path]; !seen {
				fileOrder = append(fileOrder, m.Path)
			}
			if len(perFile[m.Path]) < 3 {
				perFile[m.Path] = append(perFile[m.Path], contentLine{m.Line, m.Text})
			}
		}

		var results []searchMatch
		for _, p := range fileOrder {
			rel, _ := filepath.Rel(root, p)
			results = append(results, searchMatch{
				path:    p,
				rank:    2,
				lines:   perFile[p],
				relPath: rel,
			})
		}
		mu.Lock()
		contentMatches = results
		mu.Unlock()
	}()

	var symbolResults []search.SearchResult
	if searcher != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results, err := searcher.Search(ctx, params.Query, 10)
			if err != nil {
				return
			}
			mu.Lock()
			symbolResults = results
			mu.Unlock()
		}()
	}

	wg.Wait()

	seen := make(map[string]bool)
	var merged []searchMatch

	for _, m := range fileMatches {
		if !seen[m.path] {
			seen[m.path] = true
			merged = append(merged, m)
		}
	}
	for _, m := range contentMatches {
		if !seen[m.path] {
			seen[m.path] = true
			merged = append(merged, m)
		}
	}

	totalCount := len(merged) + len(symbolResults)
	if totalCount == 0 {
		return &Result{
			Title:  fmt.Sprintf("search: %s", params.Query),
			Output: fmt.Sprintf("No results found for %q.", params.Query),
			Metadata: map[string]any{
				"count":     0,
				"truncated": false,
			},
		}, nil
	}

	truncated := len(merged) > searchLimit
	if truncated {
		merged = merged[:searchLimit]
	}

	var buf strings.Builder
	fmt.Fprintf(&buf, "Found %d results for %q\n", totalCount, params.Query)

	if len(symbolResults) > 0 {
		buf.WriteString("\n── Symbol Matches ──\n")
		buf.WriteString(search.FormatResults(symbolResults, ectx.WorkDir))
		buf.WriteByte('\n')
	}

	var fMatches, cMatches []searchMatch
	for _, m := range merged {
		if m.rank <= 1 {
			fMatches = append(fMatches, m)
		} else {
			cMatches = append(cMatches, m)
		}
	}

	if len(fMatches) > 0 {
		buf.WriteString("\n── File Matches ──\n")
		for _, m := range fMatches {
			buf.WriteString(m.relPath)
			buf.WriteByte('\n')
		}
	}

	if len(cMatches) > 0 {
		buf.WriteString("\n── Content Matches ──\n")
		for _, m := range cMatches {
			buf.WriteString(m.relPath)
			buf.WriteByte('\n')
			for _, l := range m.lines {
				fmt.Fprintf(&buf, "  %d: %s\n", l.num, l.text)
			}
		}
	}

	result := strings.TrimRight(buf.String(), "\n")
	if truncated {
		result += fmt.Sprintf("\n\n(Results truncated. Showing %d of more than %d results.)", searchLimit, searchLimit)
	}

	return &Result{
		Title:  fmt.Sprintf("search: %s", params.Query),
		Output: result,
		Metadata: map[string]any{
			"count":     totalCount,
			"truncated": truncated,
		},
	}, nil
}

// looksLikeRegex returns true if the query contains regex special characters.
func looksLikeRegex(q string) bool {
	for _, c := range q {
		switch c {
		case '.', '*', '+', '?', '(', ')', '[', ']', '{', '}', '|', '^', '$', '\\':
			return true
		}
	}
	return false
}
