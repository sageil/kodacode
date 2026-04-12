package tool

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/sageil/kodacode/v1/internal/config"
)

// LSPDiagnoser returns formatted diagnostics for a file.
type LSPDiagnoser interface {
	FileDiagnostics(ctx context.Context, filePath string) string
}

type linter struct {
	command   string
	args      []string
	exts      []string
	dirTarget func(filePath, projectDir string) string
}

var (
	resolvedLinters []linter
	linterOnce      sync.Once
)

// ResolveLinters builds the linter list from config, falling back to
// auto-detection when no linters are configured.
func ResolveLinters(projectDir string, cfg []config.LinterConfig) {
	linterOnce.Do(func() {
		if len(cfg) > 0 {
			resolvedLinters = lintersFromConfig(cfg)
		} else {
			resolvedLinters = detectLinters(projectDir)
		}
	})
}

func lintersFromConfig(cfg []config.LinterConfig) []linter {
	var linters []linter
	for _, c := range cfg {
		parts := strings.Fields(c.Command)
		if len(parts) == 0 {
			continue
		}
		l := linter{
			command: parts[0],
			args:    parts[1:],
			exts:    c.Extensions,
		}
		if c.DirMode {
			l.dirTarget = parentDirTarget
		}
		linters = append(linters, l)
	}
	return linters
}

func detectLinters(dir string) []linter {
	var linters []linter

	if fileExists(filepath.Join(dir, "biome.json")) || fileExists(filepath.Join(dir, "biome.jsonc")) {
		if _, err := exec.LookPath("biome"); err == nil {
			linters = append(linters, linter{
				command: "biome",
				args:    []string{"lint", "--no-errors-on-unmatched"},
				exts:    []string{".ts", ".tsx", ".js", ".jsx", ".json", ".css"},
			})
		}
	}

	if len(linters) == 0 && hasESLintConfig(dir) {
		if _, err := exec.LookPath("eslint"); err == nil {
			linters = append(linters, linter{
				command: "eslint",
				args:    []string{"--no-warn-ignored"},
				exts:    []string{".ts", ".tsx", ".js", ".jsx"},
			})
		}
	}

	if fileExists(filepath.Join(dir, "tsconfig.json")) {
		if _, err := exec.LookPath("tsc"); err == nil {
			linters = append(linters, linter{
				command:   "tsc",
				args:      []string{"--noEmit", "--pretty", "false"},
				exts:      []string{".ts", ".tsx"},
				dirTarget: parentDirTarget,
			})
		}
	}

	if fileExists(filepath.Join(dir, ".golangci.yml")) || fileExists(filepath.Join(dir, ".golangci.yaml")) {
		if _, err := exec.LookPath("golangci-lint"); err == nil {
			linters = append(linters, linter{
				command:   "golangci-lint",
				args:      []string{"run"},
				exts:      []string{".go"},
				dirTarget: goPackageTarget,
			})
		}
	} else if fileExists(filepath.Join(dir, "go.mod")) {
		linters = append(linters, linter{
			command:   "go",
			args:      []string{"vet"},
			exts:      []string{".go"},
			dirTarget: goPackageTarget,
		})
	}

	if fileExists(filepath.Join(dir, "ruff.toml")) || hasRuffInPyproject(dir) {
		if _, err := exec.LookPath("ruff"); err == nil {
			linters = append(linters, linter{
				command: "ruff",
				args:    []string{"check"},
				exts:    []string{".py", ".pyi"},
			})
		}
	} else if fileExists(filepath.Join(dir, ".flake8")) {
		if _, err := exec.LookPath("flake8"); err == nil {
			linters = append(linters, linter{
				command: "flake8",
				exts:    []string{".py"},
			})
		}
	}

	if fileExists(filepath.Join(dir, "Cargo.toml")) {
		if _, err := exec.LookPath("cargo"); err == nil {
			linters = append(linters, linter{
				command:   "cargo",
				args:      []string{"clippy", "--message-format=short"},
				exts:      []string{".rs"},
				dirTarget: parentDirTarget,
			})
		}
	}

	return linters
}

func hasESLintConfig(dir string) bool {
	patterns := []string{
		".eslintrc", ".eslintrc.js", ".eslintrc.cjs", ".eslintrc.json", ".eslintrc.yml", ".eslintrc.yaml",
		"eslint.config.js", "eslint.config.mjs", "eslint.config.cjs", "eslint.config.ts",
	}
	for _, p := range patterns {
		if fileExists(filepath.Join(dir, p)) {
			return true
		}
	}
	return false
}

func hasRuffInPyproject(dir string) bool {
	data, err := os.ReadFile(filepath.Join(dir, "pyproject.toml"))
	if err != nil {
		return false
	}
	return strings.Contains(string(data), "[tool.ruff")
}

func linterForFile(ext string) *linter {
	for i := range resolvedLinters {
		if slices.Contains(resolvedLinters[i].exts, ext) {
			return &resolvedLinters[i]
		}
	}
	return nil
}

func runLinter(ctx context.Context, dir string, l *linter, target string) string {
	args := make([]string, len(l.args), len(l.args)+1)
	copy(args, l.args)
	if target != "" {
		args = append(args, target)
	}

	cmd := exec.CommandContext(ctx, l.command, args...)
	cmd.Dir = dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	_ = cmd.Run()

	if output := stdout.String(); output != "" {
		return strings.TrimSpace(output)
	}
	return strings.TrimSpace(stderr.String())
}

func goPackageTarget(filePath, projectDir string) string {
	rel, err := filepath.Rel(projectDir, filepath.Dir(filePath))
	if err != nil {
		return "./..."
	}
	return "./" + rel + "/..."
}

func parentDirTarget(filePath, projectDir string) string {
	rel, err := filepath.Rel(projectDir, filepath.Dir(filePath))
	if err != nil {
		return "."
	}
	return rel
}

// RunDiagnostics runs configured linters and optional LSP diagnostics against
// the given files. Dir-mode linters are deduplicated by directory. Returns
// empty string when no issues are found.
func RunDiagnostics(ctx context.Context, projectDir string, files []string, mgr LSPDiagnoser) string {
	type linterGroup struct {
		l       *linter
		targets []string
	}

	groups := make(map[string]*linterGroup)
	seenTarget := make(map[string]bool)

	for _, f := range files {
		ext := filepath.Ext(f)
		if ext == "" {
			continue
		}
		l := linterForFile(ext)
		if l == nil {
			continue
		}
		key := l.command + " " + strings.Join(l.args, " ")
		entry, ok := groups[key]
		if !ok {
			entry = &linterGroup{l: l}
			groups[key] = entry
		}
		if l.dirTarget != nil {
			target := l.dirTarget(f, projectDir)
			dedupKey := key + ":" + target
			if !seenTarget[dedupKey] {
				seenTarget[dedupKey] = true
				entry.targets = append(entry.targets, target)
			}
		} else {
			entry.targets = append(entry.targets, f)
		}
	}

	if len(groups) == 0 && mgr == nil {
		return ""
	}

	lintCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	var (
		sb strings.Builder
		mu sync.Mutex
		wg sync.WaitGroup
	)

	appendOutput := func(output string) {
		mu.Lock()
		if sb.Len() > 0 {
			sb.WriteString("\n")
		}
		sb.WriteString(output)
		mu.Unlock()
	}

	for _, entry := range groups {
		for _, target := range entry.targets {
			wg.Add(1)
			go func(l *linter, t string) {
				defer wg.Done()
				defer func() {
					if r := recover(); r != nil {
						log.Printf("linter panic (%s): %v", l.command, r)
					}
				}()
				if output := runLinter(lintCtx, projectDir, l, t); output != "" {
					appendOutput(fmt.Sprintf("[%s]\n%s", l.command, output))
				}
			}(entry.l, target)
		}
	}

	if mgr != nil {
		for _, f := range files {
			wg.Add(1)
			go func(fp string) {
				defer wg.Done()
				defer func() {
					if r := recover(); r != nil {
						log.Printf("lsp diagnostics panic (%s): %v", fp, r)
					}
				}()
				if diags := mgr.FileDiagnostics(lintCtx, fp); diags != "" {
					appendOutput(fmt.Sprintf("[lsp: %s]\n%s", filepath.Base(fp), diags))
				}
			}(f)
		}
	}

	wg.Wait()
	return sb.String()
}
