package tool

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
)

type grepMatch struct {
	Path string
	Line int
	Text string
}

var (
	grepBinary     string
	grepBinaryOnce sync.Once
)

func resolveGrepBinary() string {
	grepBinaryOnce.Do(func() {
		if _, err := exec.LookPath("rg"); err == nil {
			grepBinary = "rg"
		} else if _, err := exec.LookPath("grep"); err == nil {
			grepBinary = "grep"
		} else {
			grepBinary = ""
		}
	})
	return grepBinary
}

// runGrep searches for pattern in root, returning matches. It uses rg if
// available, falls back to grep, then to a pure Go walker.
func runGrep(ctx context.Context, pattern, include, root string, fixedString bool) ([]grepMatch, error) {
	bin := resolveGrepBinary()
	switch bin {
	case "rg":
		return runExternalGrep(ctx, bin, rgArgs(pattern, include, root, fixedString), "|")
	case "grep":
		return runExternalGrep(ctx, bin, gnuGrepArgs(pattern, include, root, fixedString), ":")
	default:
		return goGrep(ctx, pattern, include, root, fixedString)
	}
}

func rgArgs(pattern, include, root string, fixedString bool) []string {
	args := []string{"-nH", "--hidden", "--no-messages", "--field-match-separator=|"}
	if fixedString {
		args = append(args, "--fixed-strings")
	}
	args = append(args, "--regexp", pattern)
	if include != "" {
		args = append(args, "--glob", include)
	}
	return append(args, root)
}

func gnuGrepArgs(pattern, include, root string, fixedString bool) []string {
	args := []string{"-rnH"}
	if fixedString {
		args = append(args, "-F")
	}
	if include != "" {
		args = append(args, "--include="+include)
	}
	return append(args, "-e", pattern, root)
}

func runExternalGrep(ctx context.Context, bin string, args []string, sep string) ([]grepMatch, error) {
	cmd := exec.CommandContext(ctx, bin, args...)
	output, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			code := exitErr.ExitCode()
			if code == 1 {
				return nil, nil
			}
			if code == 2 && len(output) > 0 {
				// partial errors — proceed with what we have
			} else {
				return nil, fmt.Errorf("%s failed (exit %d): %s", bin, code, string(exitErr.Stderr))
			}
		} else {
			return nil, fmt.Errorf("failed to run %s: %w", bin, err)
		}
	}
	return parseGrepOutput(output, sep), nil
}

func parseGrepOutput(output []byte, sep string) []grepMatch {
	var matches []grepMatch
	scanner := bufio.NewScanner(strings.NewReader(string(output)))
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
	for scanner.Scan() {
		parts := strings.SplitN(scanner.Text(), sep, 3)
		if len(parts) < 3 {
			continue
		}
		num, err := strconv.Atoi(parts[1])
		if err != nil {
			continue
		}
		text := parts[2]
		if len(text) > 2000 {
			text = text[:2000]
		}
		matches = append(matches, grepMatch{Path: parts[0], Line: num, Text: text})
	}
	return matches
}

// goGrep is a pure Go fallback when neither rg nor grep is installed.
// Walks the directory tree, skipping hidden dirs and matching file patterns.
func goGrep(ctx context.Context, pattern, include, root string, fixedString bool) ([]grepMatch, error) {
	var re *regexp.Regexp
	if fixedString {
		re = regexp.MustCompile(regexp.QuoteMeta(pattern))
	} else {
		var err error
		re, err = regexp.Compile(pattern)
		if err != nil {
			return nil, fmt.Errorf("invalid regex: %w", err)
		}
	}

	var includeGlob string
	if include != "" {
		includeGlob = include
	}

	var matches []grepMatch
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		name := d.Name()
		if d.IsDir() {
			if name == ".git" || name == "node_modules" || name == "vendor" || name == ".cache" {
				return filepath.SkipDir
			}
			return nil
		}
		if includeGlob != "" {
			if matched, _ := filepath.Match(includeGlob, name); !matched {
				return nil
			}
		}
		info, err := d.Info()
		if err != nil || info.Size() > 1024*1024 {
			return nil
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		for i, line := range strings.Split(string(data), "\n") {
			if re.MatchString(line) {
				text := line
				if len(text) > 2000 {
					text = text[:2000]
				}
				matches = append(matches, grepMatch{Path: path, Line: i + 1, Text: text})
			}
		}
		return nil
	})
	return matches, err
}
