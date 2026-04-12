package tool

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

var grepParams = []byte(`{
	"type": "object",
	"properties": {
		"pattern": {"type": "string", "description": "The regex pattern to search for"},
		"path": {"type": "string", "description": "The directory to search in"},
		"include": {"type": "string", "description": "File pattern filter (e.g. \"*.js\")"}
	},
	"required": ["pattern"]
}`)

const grepLimit = 100

func NewGrepTool() *Tool {
	return &Tool{
		Name:        "grep",
		ReadOnly:    true,
		Description: prompt("grep"),
		Parameters:  grepParams,
		Execute:     executeGrep,
	}
}

func executeGrep(ctx context.Context, ectx ExecutionContext, args []byte) (*Result, error) {
	var params struct {
		Pattern string `json:"pattern"`
		Path    string `json:"path"`
		Include string `json:"include"`
	}
	if err := flexUnmarshal(args, &params); err != nil {
		return nil, fmt.Errorf("invalid parameters: %w", err)
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

	if ectx.WriteOutput != nil {
		ectx.WriteOutput(fmt.Sprintf("searching %s…\n", filepath.Base(root)))
	}

	matches, err := runGrep(ctx, params.Pattern, params.Include, root, false)
	if err != nil {
		return nil, err
	}

	if len(matches) == 0 {
		return &Result{
			Title:  fmt.Sprintf("grep: %s", params.Pattern),
			Output: "No matches found.",
			Metadata: map[string]any{
				"count":     0,
				"truncated": false,
			},
		}, nil
	}

	mtimes := map[string]int64{}
	for _, m := range matches {
		if _, ok := mtimes[m.Path]; ok {
			continue
		}
		info, err := os.Stat(m.Path)
		if err != nil {
			mtimes[m.Path] = 0
			continue
		}
		mtimes[m.Path] = info.ModTime().UnixNano()
	}

	sort.SliceStable(matches, func(i, j int) bool {
		if matches[i].Path != matches[j].Path {
			return mtimes[matches[i].Path] > mtimes[matches[j].Path]
		}
		return matches[i].Line < matches[j].Line
	})

	truncated := len(matches) > grepLimit
	if truncated {
		matches = matches[:grepLimit]
	}

	var buf strings.Builder
	var current string
	for _, m := range matches {
		display := m.Path
		if rel, err := filepath.Rel(root, m.Path); err == nil {
			display = rel
		}
		if m.Path != current {
			if current != "" {
				buf.WriteByte('\n')
			}
			buf.WriteString(display)
			buf.WriteString(":\n")
			current = m.Path
		}
		fmt.Fprintf(&buf, "  Line %d: %s\n", m.Line, m.Text)
	}

	result := strings.TrimRight(buf.String(), "\n")
	if truncated {
		result += fmt.Sprintf("\n\n(Results truncated. Showing %d of more than %d matches.)", grepLimit, grepLimit)
	}

	return &Result{
		Title:  fmt.Sprintf("grep: %s", params.Pattern),
		Output: result,
		Metadata: map[string]any{
			"count":     len(matches),
			"truncated": truncated,
		},
	}, nil
}
