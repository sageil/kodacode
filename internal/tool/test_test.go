package tool

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestDetectFramework_TaskfilePrecedence(t *testing.T) {
	dir := t.TempDir()
	// Both go.mod and Taskfile.yml exist — Taskfile should win.
	writeFile(t, dir, "go.mod", "module test")
	writeFile(t, dir, "Taskfile.yml", "version: '3'\ntasks:\n  test:\n    cmds:\n      - go test ./...\n")
	cmd, fw, err := detectFramework(dir)
	if err != nil {
		t.Fatal(err)
	}
	if fw != frameworkTask {
		t.Fatalf("expected frameworkTask, got %d", fw)
	}
	if cmd != "task test" {
		t.Fatalf("expected 'task test', got %q", cmd)
	}
}

func TestDetectFramework_MakefilePrecedence(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "package.json", "{}")
	writeFile(t, dir, "Makefile", "test:\n\techo ok\n")
	_, fw, err := detectFramework(dir)
	if err != nil {
		t.Fatal(err)
	}
	if fw != frameworkMake {
		t.Fatalf("expected frameworkMake, got %d", fw)
	}
}

func TestDetectFramework_Go(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "go.mod", "module test")
	cmd, fw, err := detectFramework(dir)
	if err != nil {
		t.Fatal(err)
	}
	if fw != frameworkGo {
		t.Fatalf("expected frameworkGo, got %d", fw)
	}
	if cmd != "go test ./..." {
		t.Fatalf("expected 'go test ./...', got %q", cmd)
	}
}

func TestDetectFramework_NPM(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "package.json", "{}")
	cmd, fw, err := detectFramework(dir)
	if err != nil {
		t.Fatal(err)
	}
	if fw != frameworkNPM {
		t.Fatalf("expected frameworkNPM, got %d", fw)
	}
	if cmd != "npm test" {
		t.Fatalf("expected 'npm test', got %q", cmd)
	}
}

func TestDetectFramework_Pnpm(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "package.json", "{}")
	writeFile(t, dir, "pnpm-lock.yaml", "")
	cmd, fw, err := detectFramework(dir)
	if err != nil {
		t.Fatal(err)
	}
	if fw != frameworkNPM {
		t.Fatalf("expected frameworkNPM, got %d", fw)
	}
	if cmd != "pnpm test" {
		t.Fatalf("expected 'pnpm test', got %q", cmd)
	}
}

func TestDetectFramework_Yarn(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "package.json", "{}")
	writeFile(t, dir, "yarn.lock", "")
	cmd, _, err := detectFramework(dir)
	if err != nil {
		t.Fatal(err)
	}
	if cmd != "yarn test" {
		t.Fatalf("expected 'yarn test', got %q", cmd)
	}
}

func TestDetectFramework_Bun(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "package.json", "{}")
	writeFile(t, dir, "bun.lockb", "")
	cmd, _, err := detectFramework(dir)
	if err != nil {
		t.Fatal(err)
	}
	if cmd != "bun test" {
		t.Fatalf("expected 'bun test', got %q", cmd)
	}
}

func TestDetectFramework_Pytest(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "pyproject.toml", "")
	_, fw, err := detectFramework(dir)
	if err != nil {
		t.Fatal(err)
	}
	if fw != frameworkPytest {
		t.Fatalf("expected frameworkPytest, got %d", fw)
	}
}

func TestDetectFramework_Cargo(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "Cargo.toml", "")
	_, fw, err := detectFramework(dir)
	if err != nil {
		t.Fatal(err)
	}
	if fw != frameworkCargo {
		t.Fatalf("expected frameworkCargo, got %d", fw)
	}
}

func TestDetectFramework_Maven(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "pom.xml", "<project/>")
	cmd, fw, err := detectFramework(dir)
	if err != nil {
		t.Fatal(err)
	}
	if fw != frameworkMaven {
		t.Fatalf("expected frameworkMaven, got %d", fw)
	}
	if cmd != "mvn test" {
		t.Fatalf("expected 'mvn test', got %q", cmd)
	}
}

func TestDetectFramework_GradleWrapper(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "build.gradle", "")
	writeFile(t, dir, "gradlew", "#!/bin/sh\n")
	cmd, fw, err := detectFramework(dir)
	if err != nil {
		t.Fatal(err)
	}
	if fw != frameworkGradle {
		t.Fatalf("expected frameworkGradle, got %d", fw)
	}
	if cmd != "./gradlew test" {
		t.Fatalf("expected './gradlew test', got %q", cmd)
	}
}

func TestDetectFramework_GradleNoWrapper(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "build.gradle.kts", "")
	cmd, fw, err := detectFramework(dir)
	if err != nil {
		t.Fatal(err)
	}
	if fw != frameworkGradle {
		t.Fatalf("expected frameworkGradle, got %d", fw)
	}
	if cmd != "gradle test" {
		t.Fatalf("expected 'gradle test', got %q", cmd)
	}
}

func TestDetectFramework_Dotnet(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "MyApp.csproj", "<Project/>")
	cmd, fw, err := detectFramework(dir)
	if err != nil {
		t.Fatal(err)
	}
	if fw != frameworkDotnet {
		t.Fatalf("expected frameworkDotnet, got %d", fw)
	}
	if cmd != "dotnet test" {
		t.Fatalf("expected 'dotnet test', got %q", cmd)
	}
}

func TestDetectFramework_Rspec(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "Gemfile", "gem 'rspec'\n")
	cmd, fw, err := detectFramework(dir)
	if err != nil {
		t.Fatal(err)
	}
	if fw != frameworkRspec {
		t.Fatalf("expected frameworkRspec, got %d", fw)
	}
	if cmd != "bundle exec rspec" {
		t.Fatalf("expected 'bundle exec rspec', got %q", cmd)
	}
}

func TestDetectFramework_Swift(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "Package.swift", "")
	cmd, fw, err := detectFramework(dir)
	if err != nil {
		t.Fatal(err)
	}
	if fw != frameworkSwift {
		t.Fatalf("expected frameworkSwift, got %d", fw)
	}
	if cmd != "swift test" {
		t.Fatalf("expected 'swift test', got %q", cmd)
	}
}

func TestDetectFramework_PHPUnit(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "phpunit.xml", "<phpunit/>")
	cmd, fw, err := detectFramework(dir)
	if err != nil {
		t.Fatal(err)
	}
	if fw != frameworkPHPUnit {
		t.Fatalf("expected frameworkPHPUnit, got %d", fw)
	}
	if cmd != "vendor/bin/phpunit" {
		t.Fatalf("expected 'vendor/bin/phpunit', got %q", cmd)
	}
}

func TestDetectFramework_Elixir(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "mix.exs", "")
	cmd, fw, err := detectFramework(dir)
	if err != nil {
		t.Fatal(err)
	}
	if fw != frameworkElixir {
		t.Fatalf("expected frameworkElixir, got %d", fw)
	}
	if cmd != "mix test" {
		t.Fatalf("expected 'mix test', got %q", cmd)
	}
}

func TestDetectFramework_Zig(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "build.zig", "")
	cmd, fw, err := detectFramework(dir)
	if err != nil {
		t.Fatal(err)
	}
	if fw != frameworkZig {
		t.Fatalf("expected frameworkZig, got %d", fw)
	}
	if cmd != "zig build test" {
		t.Fatalf("expected 'zig build test', got %q", cmd)
	}
}

func TestResolveTestTarget_FileUsesParentDirectory(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "pkg", "main_test.go")
	if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filePath, []byte("package pkg\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	target, err := resolveTestTarget(dir, filePath)
	if err != nil {
		t.Fatal(err)
	}
	if !target.isFile {
		t.Fatal("target.isFile = false, want true")
	}
	if target.dir != filepath.Dir(filePath) {
		t.Fatalf("target.dir = %q, want %q", target.dir, filepath.Dir(filePath))
	}
}

func TestApplyTestTarget_PytestFileAddsRelativeFileArgument(t *testing.T) {
	target := testTarget{dir: "/repo/tests", filePath: "/repo/tests/test_app.py", isFile: true}
	got := applyTestTarget("pytest", frameworkPytest, target, false)
	if got != "pytest 'test_app.py'" {
		t.Fatalf("applyTestTarget() = %q, want %q", got, "pytest 'test_app.py'")
	}
}

func TestApplyTestTarget_GoFileUsesPackageDirectory(t *testing.T) {
	target := testTarget{dir: "/repo/pkg", filePath: "/repo/pkg/foo_test.go", isFile: true}
	got := applyTestTarget("go test ./...", frameworkGo, target, false)
	if got != "go test ." {
		t.Fatalf("applyTestTarget() = %q, want %q", got, "go test .")
	}
}

func TestDetectFramework_Make(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "Makefile", "test:\n\techo ok\n")
	_, fw, err := detectFramework(dir)
	if err != nil {
		t.Fatal(err)
	}
	if fw != frameworkMake {
		t.Fatalf("expected frameworkMake, got %d", fw)
	}
}

func TestDetectFramework_None(t *testing.T) {
	dir := t.TempDir()
	_, _, err := detectFramework(dir)
	if err == nil {
		t.Fatal("expected error for empty directory")
	}
	if err.Error() != "could not detect test framework" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestApplyFilter(t *testing.T) {
	tests := []struct {
		cmd    string
		filter string
		fw     testFramework
		want   string
	}{
		{"go test ./...", "TestFoo", frameworkGo, "go test ./... -run 'TestFoo'"},
		{"pytest", "test_bar", frameworkPytest, "pytest -k 'test_bar'"},
		{"cargo test", "my_test", frameworkCargo, "cargo test 'my_test'"},
		{"npm test", "", frameworkNPM, "npm test"},
		{"make test", "foo", frameworkMake, "make test 'foo'"},
		{"mvn test", "MyTest", frameworkMaven, "mvn test -Dtest='MyTest'"},
		{"./gradlew test", "MyTest", frameworkGradle, "./gradlew test --tests 'MyTest'"},
		{"dotnet test", "MyTest", frameworkDotnet, "dotnet test --filter 'MyTest'"},
		{"bundle exec rspec", "my_spec", frameworkRspec, "bundle exec rspec --example 'my_spec'"},
		{"vendor/bin/phpunit", "MyTest", frameworkPHPUnit, "vendor/bin/phpunit --filter 'MyTest'"},
		{"mix test", "my_test", frameworkElixir, "mix test --only 'my_test'"},
	}
	for _, tt := range tests {
		got := applyFilter(tt.cmd, tt.filter, tt.fw)
		if got != tt.want {
			t.Errorf("applyFilter(%q, %q, %d) = %q, want %q", tt.cmd, tt.filter, tt.fw, got, tt.want)
		}
	}
}

func TestParseSummary_Go(t *testing.T) {
	output := `=== RUN   TestA
--- PASS: TestA (0.00s)
=== RUN   TestB
--- FAIL: TestB (0.01s)
=== RUN   TestC
--- SKIP: TestC (0.00s)
=== RUN   TestD
--- PASS: TestD (0.00s)
FAIL
`
	summary := parseSummary(output, frameworkGo)
	expected := "✓ 2 passed  ✗ 1 failed  ○ 1 skipped"
	if summary != expected {
		t.Fatalf("expected %q, got %q", expected, summary)
	}
}

func TestFrameworkFromCommand(t *testing.T) {
	tests := []struct {
		cmd  string
		want testFramework
	}{
		{"go test ./...", frameworkGo},
		{"npm test", frameworkNPM},
		{"pnpm test", frameworkNPM},
		{"yarn test", frameworkNPM},
		{"bun test", frameworkNPM},
		{"npx vitest run", frameworkNPM},
		{"vitest run", frameworkNPM},
		{"jest --verbose", frameworkNPM},
		{"pytest -v", frameworkPytest},
		{"cargo test", frameworkCargo},
		{"mvn test", frameworkMaven},
		{"./gradlew test", frameworkGradle},
		{"gradle test", frameworkGradle},
		{"dotnet test", frameworkDotnet},
		{"bundle exec rspec", frameworkRspec},
		{"swift test", frameworkSwift},
		{"vendor/bin/phpunit", frameworkPHPUnit},
		{"mix test", frameworkElixir},
		{"zig build test", frameworkZig},
		{"make test", frameworkMake},
		{"task test", frameworkTask},
		{"ruby test.rb", frameworkUnknown},
	}
	for _, tt := range tests {
		got := frameworkFromCommand(tt.cmd)
		if got != tt.want {
			t.Errorf("frameworkFromCommand(%q) = %d, want %d", tt.cmd, got, tt.want)
		}
	}
}

func TestApplyVerbose(t *testing.T) {
	tests := []struct {
		cmd  string
		fw   testFramework
		want string
	}{
		{"go test ./...", frameworkGo, "go test -v ./..."},
		{"go test -v ./...", frameworkGo, "go test -v ./..."},
		{"pytest", frameworkPytest, "pytest -v"},
		{"pytest -v", frameworkPytest, "pytest -v"},
		{"cargo test", frameworkCargo, "cargo test -- --nocapture"},
		{"cargo test -- --nocapture", frameworkCargo, "cargo test -- --nocapture"},
		{"dotnet test", frameworkDotnet, "dotnet test --verbosity normal"},
		{"bundle exec rspec", frameworkRspec, "bundle exec rspec --format documentation"},
		{"mix test", frameworkElixir, "mix test --trace"},
		{"zig build test", frameworkZig, "zig build test --verbose"},
		{"npm test", frameworkNPM, "npm test -- --verbose"},
		{"npm test -- --verbose", frameworkNPM, "npm test -- --verbose"},
		{"mvn test", frameworkMaven, "mvn test -Dsurefire.useFile=false"},
		{"./gradlew test", frameworkGradle, "./gradlew test --info"},
		{"vendor/bin/phpunit", frameworkPHPUnit, "vendor/bin/phpunit --verbose"},
		{"swift test", frameworkSwift, "swift test --verbose"},
		{"make test", frameworkMake, "make test"},
		{"task test", frameworkTask, "task test"},
	}
	for _, tt := range tests {
		got := applyVerbose(tt.cmd, tt.fw)
		if got != tt.want {
			t.Errorf("applyVerbose(%q, %d) = %q, want %q", tt.cmd, tt.fw, got, tt.want)
		}
	}
}

func TestExecuteTest_EchoCommand(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "go.mod", "module test")

	result, err := executeTest(context.Background(), ExecutionContext{
		WorkDir: dir,
	}, []byte(`{"command": "echo '--- PASS: TestHello (0.00s)'"}`))
	if err != nil {
		t.Fatal(err)
	}
	if result.Title != "echo '--- PASS: TestHello (0.00s)'" {
		t.Fatalf("unexpected title: %q", result.Title)
	}
	if result.Metadata["exit_code"] != 0 {
		t.Fatalf("expected exit code 0, got %v", result.Metadata["exit_code"])
	}
}

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}
