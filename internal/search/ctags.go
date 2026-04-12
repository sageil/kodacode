package search

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

type ctagsEntry struct {
	Type      string `json:"_type"`
	Name      string `json:"name"`
	Path      string `json:"path"`
	Language  string `json:"language"`
	Kind      string `json:"kind"`
	Line      int    `json:"line"`
	Scope     string `json:"scope"`
	ScopeKind string `json:"scopeKind"`
	Signature string `json:"signature"`
}

func ResolveCtagsBinary(override string) (string, error) {
	if override != "" {
		return exec.LookPath(override)
	}
	for _, name := range []string{"ctags", "universal-ctags"} {
		if path, err := exec.LookPath(name); err == nil {
			out, err := exec.Command(path, "--version").Output()
			if err == nil && strings.Contains(string(out), "Universal Ctags") {
				return path, nil
			}
		}
	}
	return "", fmt.Errorf("universal-ctags not found in PATH")
}

func ExtractSymbols(ctx context.Context, binary string, files []string) ([]Symbol, error) {
	if len(files) == 0 {
		return nil, nil
	}

	args := []string{
		"--output-format=json",
		"--fields=+Sn",
		"--extras=-{anonymous}",
		"--kinds-all=*",
		"-L", "-",
	}
	cmd := exec.CommandContext(ctx, binary, args...)
	cmd.Stdin = strings.NewReader(strings.Join(files, "\n"))

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("ctags stdout pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("ctags start: %w", err)
	}

	var symbols []Symbol
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 256*1024), 1024*1024)

	for scanner.Scan() {
		var entry ctagsEntry
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
			continue
		}
		if entry.Type != "tag" {
			continue
		}
		symbols = append(symbols, Symbol{
			FilePath:  entry.Path,
			Name:      entry.Name,
			Kind:      normalizeKind(entry.Kind),
			Language:  DetectLanguage(entry.Path, entry.Language),
			Signature: entry.Signature,
			Line:      entry.Line,
			Parent:    entry.Scope,
			Tokens:    SplitTokens(entry.Name),
		})
	}

	if err := scanner.Err(); err != nil {
		_ = cmd.Wait()
		if len(symbols) > 0 {
			return symbols, nil
		}
		return nil, fmt.Errorf("ctags scanner: %w", err)
	}

	if err := cmd.Wait(); err != nil {
		if len(symbols) > 0 {
			return symbols, nil
		}
		return nil, fmt.Errorf("ctags: %w", err)
	}
	return symbols, nil
}

func normalizeKind(kind string) string {
	switch strings.ToLower(kind) {
	case "func", "function", "method", "subroutine":
		return "function"
	case "class", "struct", "type", "typedef":
		return "type"
	case "interface", "protocol":
		return "interface"
	case "const", "constant", "enumerator":
		return "const"
	case "var", "variable", "field", "member", "property":
		return "variable"
	case "package", "module", "namespace":
		return "package"
	case "import":
		return "import"
	default:
		return kind
	}
}
