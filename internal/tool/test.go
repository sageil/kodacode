package tool

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

var testParams = []byte(`{
	"type": "object",
	"properties": {
		"command": {"type": "string", "description": "Test command to run (e.g. 'go test ./...', 'npm test', 'pytest'). If omitted, auto-detects from project files."},
		"path": {"type": "string", "description": "Directory or file to test (defaults to working directory)"},
		"filter": {"type": "string", "description": "Test name filter/pattern (e.g. 'TestFoo' for Go, '--grep pattern' for mocha)"},
		"timeout": {"type": "number", "description": "Timeout in milliseconds (default: 120000)"}
	}
}`)

// NewTestTool returns a Tool that runs project tests and parses results.
func NewTestTool() *Tool {
	return &Tool{
		Name:        "test",
		ReadOnly:    true,
		Description: prompt("test"),
		Parameters:  testParams,
		Execute:     executeTest,
	}
}

// testFramework identifies which test framework was detected.
type testFramework int

const (
	frameworkUnknown testFramework = iota
	frameworkGo
	frameworkNPM
	frameworkPytest
	frameworkCargo
	frameworkMake
	frameworkTask
	frameworkMaven
	frameworkGradle
	frameworkDotnet
	frameworkRspec
	frameworkSwift
	frameworkPHPUnit
	frameworkElixir
	frameworkZig
)

type testTarget struct {
	dir      string
	filePath string
	isFile   bool
}

// detectFramework inspects the directory for known project files and returns
// the base command and framework type. Explicit build configs (Taskfile,
// Makefile) are checked first since they represent intentional project setup.
func detectFramework(dir string) (string, testFramework, error) {
	// Explicit build configs take precedence because they represent intentional setup.
	if taskfileHasTest(dir) {
		return "task test", frameworkTask, nil
	}
	if makefileHasTest(dir) {
		return "make test", frameworkMake, nil
	}

	// Language-specific detection.
	if fileExists(filepath.Join(dir, "go.mod")) {
		return "go test ./...", frameworkGo, nil
	}
	if fileExists(filepath.Join(dir, "package.json")) {
		return detectNodeCommand(dir) + " test", frameworkNPM, nil
	}
	if fileExists(filepath.Join(dir, "pytest.ini")) ||
		fileExists(filepath.Join(dir, "setup.py")) ||
		fileExists(filepath.Join(dir, "pyproject.toml")) {
		return "pytest", frameworkPytest, nil
	}
	if fileExists(filepath.Join(dir, "Cargo.toml")) {
		return "cargo test", frameworkCargo, nil
	}
	if hasGradleBuild(dir) {
		return detectGradleCommand(dir), frameworkGradle, nil
	}
	if fileExists(filepath.Join(dir, "pom.xml")) {
		return "mvn test", frameworkMaven, nil
	}
	if hasDotnetProject(dir) {
		return "dotnet test", frameworkDotnet, nil
	}
	if hasGemfileWithRspec(dir) {
		return "bundle exec rspec", frameworkRspec, nil
	}
	if fileExists(filepath.Join(dir, "Package.swift")) {
		return "swift test", frameworkSwift, nil
	}
	if hasPHPUnit(dir) {
		return "vendor/bin/phpunit", frameworkPHPUnit, nil
	}
	if fileExists(filepath.Join(dir, "mix.exs")) {
		return "mix test", frameworkElixir, nil
	}
	if fileExists(filepath.Join(dir, "build.zig")) {
		return "zig build test", frameworkZig, nil
	}

	return "", frameworkUnknown, fmt.Errorf("could not detect test framework")
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// detectNodeCommand returns the package manager command prefix for a Node project.
func detectNodeCommand(dir string) string {
	switch {
	case fileExists(filepath.Join(dir, "pnpm-lock.yaml")):
		return "pnpm"
	case fileExists(filepath.Join(dir, "yarn.lock")):
		return "yarn"
	case fileExists(filepath.Join(dir, "bun.lockb")) || fileExists(filepath.Join(dir, "bun.lock")):
		return "bun"
	default:
		return "npm"
	}
}

func hasGradleBuild(dir string) bool {
	return fileExists(filepath.Join(dir, "build.gradle")) ||
		fileExists(filepath.Join(dir, "build.gradle.kts"))
}

func detectGradleCommand(dir string) string {
	if fileExists(filepath.Join(dir, "gradlew")) {
		return "./gradlew test"
	}
	return "gradle test"
}

func hasDotnetProject(dir string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, e := range entries {
		ext := filepath.Ext(e.Name())
		if ext == ".csproj" || ext == ".fsproj" || ext == ".sln" {
			return true
		}
	}
	return false
}

func hasGemfileWithRspec(dir string) bool {
	data, err := os.ReadFile(filepath.Join(dir, "Gemfile"))
	if err != nil {
		return false
	}
	return strings.Contains(string(data), "rspec")
}

func hasPHPUnit(dir string) bool {
	return fileExists(filepath.Join(dir, "phpunit.xml")) ||
		fileExists(filepath.Join(dir, "phpunit.xml.dist")) ||
		fileExists(filepath.Join(dir, "vendor", "bin", "phpunit"))
}

func makefileHasTest(dir string) bool {
	for _, name := range []string{"Makefile", "makefile", "GNUmakefile"} {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			continue
		}
		for _, line := range strings.Split(string(data), "\n") {
			if strings.HasPrefix(line, "test:") || strings.HasPrefix(line, "test :") {
				return true
			}
		}
	}
	return false
}

func taskfileHasTest(dir string) bool {
	for _, name := range []string{"Taskfile.yml", "Taskfile.yaml"} {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			continue
		}
		for _, line := range strings.Split(string(data), "\n") {
			trimmed := strings.TrimSpace(line)
			if trimmed == "test:" {
				return true
			}
		}
	}
	return false
}

// frameworkFromCommand guesses the framework from an explicit command string.
func frameworkFromCommand(cmd string) testFramework {
	switch {
	case strings.Contains(cmd, "cargo test"):
		return frameworkCargo
	case strings.Contains(cmd, "go test"):
		return frameworkGo
	case strings.Contains(cmd, "pytest"):
		return frameworkPytest
	case strings.Contains(cmd, "mvn test") || strings.Contains(cmd, "mvn "):
		return frameworkMaven
	case strings.Contains(cmd, "gradlew test") || strings.Contains(cmd, "gradle test"):
		return frameworkGradle
	case strings.Contains(cmd, "dotnet test"):
		return frameworkDotnet
	case strings.Contains(cmd, "rspec"):
		return frameworkRspec
	case strings.Contains(cmd, "swift test"):
		return frameworkSwift
	case strings.Contains(cmd, "phpunit"):
		return frameworkPHPUnit
	case strings.Contains(cmd, "mix test"):
		return frameworkElixir
	case strings.Contains(cmd, "zig build test"):
		return frameworkZig
	case strings.Contains(cmd, "pnpm") || strings.Contains(cmd, "yarn") ||
		strings.Contains(cmd, "bun") || strings.Contains(cmd, "npm test") ||
		strings.Contains(cmd, "npx") || strings.Contains(cmd, "vitest") ||
		strings.Contains(cmd, "jest"):
		return frameworkNPM
	case strings.Contains(cmd, "make"):
		return frameworkMake
	case strings.Contains(cmd, "task"):
		return frameworkTask
	default:
		return frameworkUnknown
	}
}

// applyVerbose injects verbose flags into the command for known frameworks.
// Only applies when the command does not already contain a verbose flag.
func applyVerbose(cmd string, fw testFramework) string {
	switch fw {
	case frameworkGo:
		if !strings.Contains(cmd, " -v") {
			return strings.Replace(cmd, "go test", "go test -v", 1)
		}
	case frameworkPytest:
		if !strings.Contains(cmd, " -v") {
			return cmd + " -v"
		}
	case frameworkCargo:
		if !strings.Contains(cmd, "--nocapture") {
			return cmd + " -- --nocapture"
		}
	case frameworkDotnet:
		if !strings.Contains(cmd, "--verbosity") {
			return cmd + " --verbosity normal"
		}
	case frameworkRspec:
		if !strings.Contains(cmd, "--format") {
			return cmd + " --format documentation"
		}
	case frameworkElixir:
		if !strings.Contains(cmd, "--trace") {
			return cmd + " --trace"
		}
	case frameworkZig:
		if !strings.Contains(cmd, "--verbose") {
			return cmd + " --verbose"
		}
	case frameworkNPM:
		if !strings.Contains(cmd, "--verbose") {
			return cmd + " -- --verbose"
		}
	case frameworkMaven:
		// Maven Surefire: show individual test names and output.
		if !strings.Contains(cmd, "-Dsurefire") {
			return cmd + " -Dsurefire.useFile=false"
		}
	case frameworkGradle:
		if !strings.Contains(cmd, "--info") {
			return cmd + " --info"
		}
	case frameworkPHPUnit:
		if !strings.Contains(cmd, "--verbose") && !strings.Contains(cmd, " -v") {
			return cmd + " --verbose"
		}
	case frameworkSwift:
		if !strings.Contains(cmd, "--verbose") && !strings.Contains(cmd, " -v") {
			return cmd + " --verbose"
		}
	}
	return cmd
}

// applyFilter appends a filter to the command based on the framework.
// The filter value is shell-quoted to prevent injection via metacharacters.
func applyFilter(cmd, filter string, fw testFramework) string {
	if filter == "" {
		return cmd
	}
	safe := shellQuote(filter)
	switch fw {
	case frameworkGo:
		return cmd + " -run " + safe
	case frameworkPytest:
		return cmd + " -k " + safe
	case frameworkCargo:
		return cmd + " " + safe
	case frameworkMaven:
		return cmd + " -Dtest=" + safe
	case frameworkGradle:
		return cmd + " --tests " + safe
	case frameworkDotnet:
		return cmd + " --filter " + safe
	case frameworkRspec:
		return cmd + " --example " + safe
	case frameworkPHPUnit:
		return cmd + " --filter " + safe
	case frameworkElixir:
		return cmd + " --only " + safe
	default:
		return cmd + " " + safe
	}
}

// shellQuote wraps s in single quotes, escaping any embedded single quotes.
// This prevents shell metacharacter injection when the filter is interpolated
// into a command string passed to `sh -c`.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}

func resolveTestTarget(baseDir, rawPath string) (testTarget, error) {
	target := testTarget{dir: baseDir}
	if rawPath == "" {
		return target, nil
	}
	path := resolvePath(rawPath, baseDir)
	info, err := os.Stat(path)
	if err != nil {
		return testTarget{}, fmt.Errorf("test path %q: %w", path, err)
	}
	if info.IsDir() {
		target.dir = path
		return target, nil
	}
	target.dir = filepath.Dir(path)
	target.filePath = path
	target.isFile = true
	return target, nil
}

func applyTestTarget(command string, fw testFramework, target testTarget, explicitCommand bool) string {
	if explicitCommand || !target.isFile || target.filePath == "" {
		return command
	}
	relTarget := filepath.Base(target.filePath)
	switch fw {
	case frameworkPytest, frameworkRspec, frameworkPHPUnit:
		return command + " " + shellQuote(relTarget)
	case frameworkGo:
		if strings.HasPrefix(command, "go test") {
			return "go test ."
		}
	}
	return command
}

func executeTest(ctx context.Context, ectx ExecutionContext, args []byte) (*Result, error) {
	var params struct {
		Command string  `json:"command"`
		Path    string  `json:"path"`
		Filter  string  `json:"filter"`
		Timeout float64 `json:"timeout"`
	}
	if err := flexUnmarshal(args, &params); err != nil {
		return nil, err
	}

	// Resolve working directory.
	dir := ectx.WorkDir
	if dir == "" {
		var err error
		dir, err = os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("getwd: %w", err)
		}
	}
	target, err := resolveTestTarget(dir, params.Path)
	if err != nil {
		return nil, err
	}
	dir = target.dir

	// Determine test command and framework.
	var command string
	var fw testFramework
	explicitCommand := params.Command != ""
	if params.Command != "" {
		command = params.Command
		fw = frameworkFromCommand(command)
	} else {
		command, fw, err = detectFramework(dir)
		if err != nil {
			return nil, err
		}
	}

	command = applyTestTarget(command, fw, target, explicitCommand)
	command = applyVerbose(command, fw)
	command = applyFilter(command, params.Filter, fw)

	timeout := DefaultTestTimeout
	if params.Timeout > 0 {
		timeout = time.Duration(params.Timeout) * time.Millisecond
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	sh, shArgs := shellExecArgs(command)
	cmd := exec.CommandContext(ctx, sh, shArgs...)
	cmd.Dir = dir
	setProcAttr(cmd)

	// Combined stdout+stderr capture.
	var buf bytes.Buffer
	var mu sync.Mutex

	pr, pw := io.Pipe()
	cmd.Stdout = pw
	cmd.Stderr = pw

	done := make(chan struct{})
	go func() {
		defer close(done)
		tmp := make([]byte, 4096)
		for {
			n, err := pr.Read(tmp)
			if n > 0 {
				chunk := string(tmp[:n])
				mu.Lock()
				buf.Write(tmp[:n])
				if ectx.WriteOutput != nil {
					ectx.WriteOutput(chunk)
				}
				mu.Unlock()
			}
			if err != nil {
				return
			}
		}
	}()

	if err := cmd.Start(); err != nil {
		_ = pw.Close()
		<-done
		return nil, fmt.Errorf("start: %w", err)
	}

	cmdDone := make(chan error, 1)
	go func() {
		cmdDone <- cmd.Wait()
		_ = pw.Close()
	}()

	var timedOut bool
	var code int

	abort := ectx.Abort
	if abort == nil {
		abort = make(chan struct{})
	}

	select {
	case err := <-cmdDone:
		code = exitCode(err)
	case <-ctx.Done():
		timedOut = true
		_ = killTree(cmd.Process.Pid)
		_ = pw.Close()
		<-cmdDone
		code = -1
	case <-abort:
		_ = killTree(cmd.Process.Pid)
		_ = pw.Close()
		<-cmdDone
		code = -1
	}

	<-done

	mu.Lock()
	output := buf.String()
	mu.Unlock()

	if timedOut {
		output += fmt.Sprintf("\n\ntest timed out after %d ms", int(params.Timeout))
	}

	summary := parseSummary(output, fw)

	if code == 0 {
		return &Result{
			Title:  command,
			Output: summary,
			Metadata: map[string]any{
				"exit_code": code,
				"command":   command,
			},
		}, nil
	}

	output += "\n\n" + summary
	tr := TruncateWithBudget(output, "tail", ectx.ContextUsage)

	return &Result{
		Title:  command,
		Output: tr.Content,
		Metadata: map[string]any{
			"exit_code": code,
			"command":   command,
		},
	}, nil
}

var (
	goPassRe = regexp.MustCompile(`(?m)^--- PASS:`)
	goFailRe = regexp.MustCompile(`(?m)^--- FAIL:`)
	goSkipRe = regexp.MustCompile(`(?m)^--- SKIP:`)

	// Generic patterns for other frameworks.
	genericPassRe = regexp.MustCompile(`(?im)(PASS|passed|✓|✔)`)
	genericFailRe = regexp.MustCompile(`(?im)(FAIL|failed|✗|✘|ERROR)`)
	genericSkipRe = regexp.MustCompile(`(?im)(SKIP|skipped|pending)`)
)

func parseSummary(output string, fw testFramework) string {
	var passed, failed, skipped int

	switch fw {
	case frameworkGo:
		passed = len(goPassRe.FindAllString(output, -1))
		failed = len(goFailRe.FindAllString(output, -1))
		skipped = len(goSkipRe.FindAllString(output, -1))
	default:
		passed = len(genericPassRe.FindAllString(output, -1))
		failed = len(genericFailRe.FindAllString(output, -1))
		skipped = len(genericSkipRe.FindAllString(output, -1))
	}

	return fmt.Sprintf("✓ %d passed  ✗ %d failed  ○ %d skipped", passed, failed, skipped)
}
