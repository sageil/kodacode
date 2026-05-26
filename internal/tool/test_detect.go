package tool

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/sageil/kodacode/internal/workspace"
)

var ErrTestFrameworkNotDetected = errors.New("could not detect a supported test framework from the selected path")

type testFramework string

const (
	testFrameworkUnknown        testFramework = ""
	testFrameworkGo             testFramework = "go"
	testFrameworkNodeVitest     testFramework = "node_vitest"
	testFrameworkNodeJest       testFramework = "node_jest"
	testFrameworkNodeMocha      testFramework = "node_mocha"
	testFrameworkNodePlaywright testFramework = "node_playwright"
	testFrameworkPytest         testFramework = "pytest"
	testFrameworkCargo          testFramework = "cargo"
	testFrameworkTask           testFramework = "task"
	testFrameworkMake           testFramework = "make"
	testFrameworkMaven          testFramework = "maven"
	testFrameworkGradle         testFramework = "gradle"
	testFrameworkDotnet         testFramework = "dotnet"
	testFrameworkRspec          testFramework = "rspec"
	testFrameworkPHPUnit        testFramework = "phpunit"
	testFrameworkElixir         testFramework = "elixir"
	testFrameworkZig            testFramework = "zig"
)

type testTarget struct {
	ResolvedPath string
	StartDir     string
	IsFile       bool
	Specified    bool
}

type detectedTestFramework struct {
	Framework testFramework
	RootDir   string
	Command   string
}

func resolveTestTarget(scope *workspace.Scope, raw string) (testTarget, error) {
	if scope == nil {
		return testTarget{}, ErrWorkspaceRequired
	}
	decision, err := scope.Check(workspace.AccessRead, raw)
	if err != nil {
		return testTarget{}, err
	}
	info, err := os.Stat(decision.ResolvedPath)
	if err != nil {
		return testTarget{}, err
	}
	target := testTarget{
		ResolvedPath: decision.ResolvedPath,
		Specified:    strings.TrimSpace(raw) != "" && strings.TrimSpace(raw) != ".",
	}
	if info.IsDir() {
		target.StartDir = decision.ResolvedPath
		return target, nil
	}
	target.IsFile = true
	target.StartDir = filepath.Dir(decision.ResolvedPath)
	return target, nil
}

func detectTestFramework(workspaceRoot string, target testTarget) (detectedTestFramework, error) {
	startDir := target.StartDir
	for {
		detected, ok, err := detectFrameworkAtDir(startDir)
		if err != nil {
			return detectedTestFramework{}, err
		}
		if ok {
			detected.RootDir = startDir
			return detected, nil
		}
		if samePath(startDir, workspaceRoot) {
			break
		}
		parent := filepath.Dir(startDir)
		if samePath(parent, startDir) || !withinPath(workspaceRoot, parent) && !samePath(parent, workspaceRoot) {
			break
		}
		startDir = parent
	}
	return detectedTestFramework{}, ErrTestFrameworkNotDetected
}

func detectFrameworkAtDir(dir string) (detectedTestFramework, bool, error) {
	if taskfileHasTest(dir) {
		return detectedTestFramework{Framework: testFrameworkTask, Command: "task test"}, true, nil
	}
	if makefileHasTest(dir) {
		return detectedTestFramework{Framework: testFrameworkMake, Command: "make test"}, true, nil
	}
	if command, ok, err := detectNodeTestCommand(dir); ok || err != nil {
		if err != nil {
			return detectedTestFramework{}, false, err
		}
		return command, true, nil
	}
	switch {
	case fileExists(filepath.Join(dir, "go.mod")):
		return detectedTestFramework{Framework: testFrameworkGo, Command: "go test ./..."}, true, nil
	case fileExists(filepath.Join(dir, "pytest.ini")) || fileExists(filepath.Join(dir, "pyproject.toml")) || fileExists(filepath.Join(dir, "setup.py")):
		return detectedTestFramework{Framework: testFrameworkPytest, Command: "pytest"}, true, nil
	case fileExists(filepath.Join(dir, "Cargo.toml")):
		return detectedTestFramework{Framework: testFrameworkCargo, Command: "cargo test"}, true, nil
	case hasGradleBuild(dir):
		return detectedTestFramework{Framework: testFrameworkGradle, Command: detectGradleCommand(dir)}, true, nil
	case fileExists(filepath.Join(dir, "pom.xml")):
		return detectedTestFramework{Framework: testFrameworkMaven, Command: "mvn test"}, true, nil
	case hasDotnetProject(dir):
		return detectedTestFramework{Framework: testFrameworkDotnet, Command: "dotnet test"}, true, nil
	case hasGemfileWithRspec(dir):
		return detectedTestFramework{Framework: testFrameworkRspec, Command: "bundle exec rspec"}, true, nil
	case hasPHPUnit(dir):
		return detectedTestFramework{Framework: testFrameworkPHPUnit, Command: phpUnitCommand(dir)}, true, nil
	case fileExists(filepath.Join(dir, "mix.exs")):
		return detectedTestFramework{Framework: testFrameworkElixir, Command: "mix test"}, true, nil
	case fileExists(filepath.Join(dir, "build.zig")):
		return detectedTestFramework{Framework: testFrameworkZig, Command: "zig build test"}, true, nil
	default:
		return detectedTestFramework{}, false, nil
	}
}

func detectNodeTestCommand(dir string) (detectedTestFramework, bool, error) {
	packageJSON := filepath.Join(dir, "package.json")
	if !fileExists(packageJSON) {
		return detectedTestFramework{}, false, nil
	}
	packageManager := detectNodePackageManager(dir)
	script, err := readNodeTestScript(packageJSON)
	if err != nil {
		return detectedTestFramework{}, false, err
	}
	if strings.TrimSpace(script) != "" {
		framework := detectNodeFrameworkFromCommand(script)
		command := nodeScriptCommand(packageManager)
		return detectedTestFramework{Framework: framework, Command: command}, true, nil
	}
	for _, runner := range []struct {
		Framework testFramework
		Name      string
		Command   string
	}{
		{Framework: testFrameworkNodeVitest, Name: "vitest", Command: "./node_modules/.bin/vitest run"},
		{Framework: testFrameworkNodeJest, Name: "jest", Command: "./node_modules/.bin/jest"},
		{Framework: testFrameworkNodeMocha, Name: "mocha", Command: "./node_modules/.bin/mocha"},
		{Framework: testFrameworkNodePlaywright, Name: "playwright", Command: "./node_modules/.bin/playwright test"},
	} {
		if fileExists(filepath.Join(dir, "node_modules", ".bin", runner.Name)) {
			return detectedTestFramework{Framework: runner.Framework, Command: runner.Command}, true, nil
		}
	}
	return detectedTestFramework{}, false, nil
}

func readNodeTestScript(path string) (string, error) {
	var payload struct {
		Scripts map[string]string `json:"scripts"`
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return "", nil
	}
	return strings.TrimSpace(payload.Scripts["test"]), nil
}

func detectNodePackageManager(dir string) string {
	switch {
	case fileExists(filepath.Join(dir, "pnpm-lock.yaml")):
		return "pnpm"
	case fileExists(filepath.Join(dir, "yarn.lock")):
		return "yarn"
	case fileExists(filepath.Join(dir, "bun.lock")), fileExists(filepath.Join(dir, "bun.lockb")):
		return "bun"
	default:
		return "npm"
	}
}

func nodeScriptCommand(packageManager string) string {
	switch packageManager {
	case "pnpm":
		return "pnpm test"
	case "yarn":
		return "yarn test"
	case "bun":
		return "bun run test"
	default:
		return "npm test"
	}
}

func detectNodeFrameworkFromCommand(command string) testFramework {
	command = strings.ToLower(command)
	switch {
	case strings.Contains(command, "playwright"):
		return testFrameworkNodePlaywright
	case strings.Contains(command, "vitest"):
		return testFrameworkNodeVitest
	case strings.Contains(command, "jest"):
		return testFrameworkNodeJest
	case strings.Contains(command, "mocha"):
		return testFrameworkNodeMocha
	default:
		return testFrameworkUnknown
	}
}

func frameworkFromCommand(command string) testFramework {
	command = strings.ToLower(strings.TrimSpace(command))
	switch {
	case strings.Contains(command, "go test"):
		return testFrameworkGo
	case strings.Contains(command, "pytest"):
		return testFrameworkPytest
	case strings.Contains(command, "cargo test"):
		return testFrameworkCargo
	case strings.Contains(command, "dotnet test"):
		return testFrameworkDotnet
	case strings.Contains(command, "bundle exec rspec"), strings.Contains(command, " rspec"):
		return testFrameworkRspec
	case strings.Contains(command, "phpunit"):
		return testFrameworkPHPUnit
	case strings.Contains(command, "mix test"):
		return testFrameworkElixir
	case strings.Contains(command, "zig build test"):
		return testFrameworkZig
	case strings.Contains(command, "gradle test"), strings.Contains(command, "gradlew test"):
		return testFrameworkGradle
	case strings.Contains(command, "mvn test"):
		return testFrameworkMaven
	case strings.Contains(command, "task test"):
		return testFrameworkTask
	case strings.Contains(command, "make test"):
		return testFrameworkMake
	case strings.Contains(command, "playwright"):
		return testFrameworkNodePlaywright
	case strings.Contains(command, "vitest"):
		return testFrameworkNodeVitest
	case strings.Contains(command, "jest"):
		return testFrameworkNodeJest
	case strings.Contains(command, "mocha"):
		return testFrameworkNodeMocha
	default:
		return testFrameworkUnknown
	}
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func hasGradleBuild(dir string) bool {
	return fileExists(filepath.Join(dir, "build.gradle")) || fileExists(filepath.Join(dir, "build.gradle.kts"))
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
	for _, entry := range entries {
		switch filepath.Ext(entry.Name()) {
		case ".csproj", ".fsproj", ".sln":
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
	return strings.Contains(strings.ToLower(string(data)), "rspec")
}

func hasPHPUnit(dir string) bool {
	return fileExists(filepath.Join(dir, "phpunit.xml")) ||
		fileExists(filepath.Join(dir, "phpunit.xml.dist")) ||
		fileExists(filepath.Join(dir, "vendor", "bin", "phpunit"))
}

func phpUnitCommand(dir string) string {
	local := filepath.Join(dir, "vendor", "bin", "phpunit")
	if fileExists(local) {
		return "./vendor/bin/phpunit"
	}
	return "phpunit"
}

func taskfileHasTest(dir string) bool {
	for _, name := range []string{"Taskfile.yml", "Taskfile.yaml"} {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			continue
		}
		for _, line := range strings.Split(string(data), "\n") {
			if strings.TrimSpace(line) == "test:" {
				return true
			}
		}
	}
	return false
}

func makefileHasTest(dir string) bool {
	for _, name := range []string{"Makefile", "makefile", "GNUmakefile"} {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			continue
		}
		for _, line := range strings.Split(string(data), "\n") {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "test:") || strings.HasPrefix(trimmed, "test :") {
				return true
			}
		}
	}
	return false
}

func samePath(left, right string) bool {
	return filepath.Clean(left) == filepath.Clean(right)
}
