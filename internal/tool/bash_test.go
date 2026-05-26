package tool

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/sageil/kodacode/internal/workspace"
)

func TestBashToolDefinitionRequiresCmd(t *testing.T) {
	definition := NewBashTool().Definition()
	var schema struct {
		Properties map[string]json.RawMessage `json:"properties"`
		Required   []string                   `json:"required"`
	}
	if err := json.Unmarshal(definition.InputSchema, &schema); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if !slices.Contains(schema.Required, "cmd") {
		t.Fatalf("required = %#v, missing cmd", schema.Required)
	}
	if len(schema.Required) != 1 {
		t.Fatalf("required = %#v, want only cmd", schema.Required)
	}
}

func TestBashToolExecutionRequestReportsMalformedShellAsInvalidArguments(t *testing.T) {
	root := t.TempDir()

	_, ok, err := NewBashTool().ExecutionRequest(root, json.RawMessage(`{"cmd":"cat > out.txt <<'TS'\nmissing terminator\n"}`))
	if err == nil {
		t.Fatal("ExecutionRequest() error = nil, want invalid arguments")
	}
	if !errors.Is(err, ErrInvalidArguments) {
		t.Fatalf("errors.Is(err, ErrInvalidArguments) = false, err = %v", err)
	}
	if ok {
		t.Fatal("ExecutionRequest() ok = true, want false")
	}
	if !strings.Contains(err.Error(), "unclosed here-document") {
		t.Fatalf("error = %q, want heredoc parse detail", err.Error())
	}
}

func TestBashToolExecutionRequestBuildsShellIntent(t *testing.T) {
	root := t.TempDir()
	subdir := filepath.Join(root, "pkg")
	if err := os.MkdirAll(subdir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	request, ok, err := NewBashTool().ExecutionRequest(root, json.RawMessage(`{"cmd":"git status","workdir":"pkg","prefix_rule":["git","status"]}`))
	if err != nil {
		t.Fatalf("ExecutionRequest() error = %v", err)
	}
	if !ok {
		t.Fatal("ExecutionRequest() ok = false, want true")
	}
	if request.Kind != "bash" {
		t.Fatalf("kind = %q", request.Kind)
	}
	if request.ShellCommand != "git status" || request.Preview != "git status" {
		t.Fatalf("request = %#v", request)
	}
	if request.LoginShell {
		t.Fatal("login shell = true, want false")
	}
	if !slices.Equal(request.PrefixRule, []string{"git", "status"}) {
		t.Fatalf("prefix rule = %#v", request.PrefixRule)
	}
	if request.Effect != ExecutionEffectRead {
		t.Fatalf("effect = %q, want %q", request.Effect, ExecutionEffectRead)
	}
}

func TestBashToolExecutionRequestClassifiesReadOnlyShellEffect(t *testing.T) {
	root := t.TempDir()

	request, ok, err := NewBashTool().ExecutionRequest(root, json.RawMessage(`{"cmd":"cd . && grep -n 'name:' tests/performance/cached-performance-test.yml && sed -n '1,40p' tests/performance/setup-test-data.js"}`))
	if err != nil {
		t.Fatalf("ExecutionRequest() error = %v", err)
	}
	if !ok {
		t.Fatal("ExecutionRequest() ok = false, want true")
	}
	if request.Effect != ExecutionEffectRead {
		t.Fatalf("effect = %q, want %q", request.Effect, ExecutionEffectRead)
	}
}

func TestBashToolExecutionRequestClassifiesCommonReadOnlyShellEffects(t *testing.T) {
	root := t.TempDir()

	for _, tc := range []struct {
		name string
		cmd  string
	}{
		{name: "awk", cmd: `awk '{print $1}' README.md`},
		{name: "jq", cmd: `jq '.scripts' package.json`},
		{name: "du", cmd: `du -sh internal`},
		{name: "tree", cmd: `tree -L 2 internal`},
		{name: "go env", cmd: `go env GOPATH`},
		{name: "go list", cmd: `go list ./...`},
		{name: "npm view", cmd: `npm view react version`},
		{name: "git ls-tree", cmd: `git ls-tree HEAD internal`},
		{name: "git config get", cmd: `git config --get user.email`},
		{name: "git config key", cmd: `git config user.email`},
		{name: "git tag list", cmd: `git tag --list 'v*'`},
		{name: "env wrapped git status", cmd: `env GIT_OPTIONAL_LOCKS=0 git status`},
		{name: "command wrapped git status", cmd: `command git status`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			request, ok, err := NewBashTool().ExecutionRequest(root, json.RawMessage(`{"cmd":`+strconv.Quote(tc.cmd)+`}`))
			if err != nil {
				t.Fatalf("ExecutionRequest() error = %v", err)
			}
			if !ok {
				t.Fatal("ExecutionRequest() ok = false, want true")
			}
			if request.Effect != ExecutionEffectRead {
				t.Fatalf("effect = %q, want %q", request.Effect, ExecutionEffectRead)
			}
		})
	}
}

func TestBashToolExecutionRequestKeepsRiskyInspectionCommandsUnknown(t *testing.T) {
	root := t.TempDir()

	for _, tc := range []struct {
		name string
		cmd  string
		want ExecutionEffect
	}{
		{name: "git config set", cmd: `git config user.email test@example.com`, want: ExecutionEffectUnknown},
		{name: "git config unset", cmd: `git config --unset user.email`, want: ExecutionEffectUnknown},
		{name: "git branch delete", cmd: `git branch -D old-branch`, want: ExecutionEffectUnknown},
		{name: "git tag create", cmd: `git tag v1.0.0`, want: ExecutionEffectUnknown},
		{name: "command wrapped git tag create", cmd: `command git tag v1.0.0`, want: ExecutionEffectUnknown},
		{name: "go env write", cmd: `go env -w GOPATH=/tmp/go`, want: ExecutionEffectUnknown},
		{name: "npm install", cmd: `npm install`, want: ExecutionEffectNetwork},
		{name: "awk in-place", cmd: `awk -i inplace '{print}' README.md`, want: ExecutionEffectUnknown},
		{name: "sed long in-place", cmd: `sed --in-place 's/a/b/' README.md`, want: ExecutionEffectUnknown},
		{name: "env wrapped rm", cmd: `env FOO=1 rm ./notes.txt`, want: ExecutionEffectUnknown},
		{name: "env split string", cmd: `env -S 'rm ./notes.txt'`, want: ExecutionEffectUnknown},
	} {
		t.Run(tc.name, func(t *testing.T) {
			request, ok, err := NewBashTool().ExecutionRequest(root, json.RawMessage(`{"cmd":`+strconv.Quote(tc.cmd)+`}`))
			if err != nil {
				t.Fatalf("ExecutionRequest() error = %v", err)
			}
			if !ok {
				t.Fatal("ExecutionRequest() ok = false, want true")
			}
			if request.Effect != tc.want {
				t.Fatalf("effect = %q, want %q", request.Effect, tc.want)
			}
		})
	}
}

func TestBashToolExecutionRequestClassifiesWriteRedirectEffect(t *testing.T) {
	root := t.TempDir()

	request, ok, err := NewBashTool().ExecutionRequest(root, json.RawMessage(`{"cmd":"printf 'hello\\n' > out.txt"}`))
	if err != nil {
		t.Fatalf("ExecutionRequest() error = %v", err)
	}
	if !ok {
		t.Fatal("ExecutionRequest() ok = false, want true")
	}
	if request.Effect != ExecutionEffectWrite {
		t.Fatalf("effect = %q, want %q", request.Effect, ExecutionEffectWrite)
	}
}

func TestBashToolExecutionRequestClassifiesReadOnlyPythonHeredocEffect(t *testing.T) {
	root := t.TempDir()

	request, ok, err := NewBashTool().ExecutionRequest(root, json.RawMessage(`{"cmd":"python3 - <<'PY'\nfrom pathlib import Path\nprint(Path('README.md').read_text())\nPY"}`))
	if err != nil {
		t.Fatalf("ExecutionRequest() error = %v", err)
	}
	if !ok {
		t.Fatal("ExecutionRequest() ok = false, want true")
	}
	if request.Effect != ExecutionEffectRead {
		t.Fatalf("effect = %q, want %q", request.Effect, ExecutionEffectRead)
	}
}

func TestBashToolExecutionRequestKeepsMutatingPythonHeredocUnknown(t *testing.T) {
	root := t.TempDir()

	request, ok, err := NewBashTool().ExecutionRequest(root, json.RawMessage(`{"cmd":"python3 - <<'PY'\nfrom pathlib import Path\nPath('README.md').write_text('changed')\nPY"}`))
	if err != nil {
		t.Fatalf("ExecutionRequest() error = %v", err)
	}
	if !ok {
		t.Fatal("ExecutionRequest() ok = false, want true")
	}
	if request.Effect != ExecutionEffectUnknown {
		t.Fatalf("effect = %q, want %q", request.Effect, ExecutionEffectUnknown)
	}
}

func TestBashToolExecutionRequestPreservesExplicitLoginShellWrapper(t *testing.T) {
	root := t.TempDir()

	request, ok, err := NewBashTool().ExecutionRequest(root, json.RawMessage(`{"cmd":"bash -lc 'npm test'"}`))
	if err != nil {
		t.Fatalf("ExecutionRequest() error = %v", err)
	}
	if !ok {
		t.Fatal("ExecutionRequest() ok = false, want true")
	}
	if request.ShellCommand != "npm test" {
		t.Fatalf("shell command = %q, want %q", request.ShellCommand, "npm test")
	}
	if !request.LoginShell {
		t.Fatal("login shell = false, want true")
	}
}

func TestBashToolExecutionRequestDoesNotTreatLiteralNullAsMissingValue(t *testing.T) {
	root := t.TempDir()

	_, ok, err := NewBashTool().ExecutionRequest(root, json.RawMessage(`{"cmd":"pwd","workdir":"null","shell":"null"}`))
	if err == nil {
		t.Fatal("ExecutionRequest() error = nil, want nonexistent literal null workdir to fail")
	}
	if ok {
		t.Fatal("ExecutionRequest() ok = true, want false on nonexistent literal null workdir")
	}
}

func TestBashToolExecutionRequestTreatsStringNullScalarOverridesAsMissing(t *testing.T) {
	root := t.TempDir()

	request, ok, err := NewBashTool().ExecutionRequest(root, json.RawMessage(`{"cmd":"git status","login":"null","tty":"null","yield_time_ms":"null","max_output_tokens":"null"}`))
	if err != nil {
		t.Fatalf("ExecutionRequest() error = %v", err)
	}
	if !ok {
		t.Fatal("ExecutionRequest() ok = false, want true")
	}
	if request.LoginShell {
		t.Fatal("login shell = true, want false")
	}
	if request.TTY {
		t.Fatal("tty = true, want false")
	}
	if request.YieldTimeMS != 0 {
		t.Fatalf("YieldTimeMS = %d, want 0", request.YieldTimeMS)
	}
	if request.MaxOutputTokens != 0 {
		t.Fatalf("MaxOutputTokens = %d, want 0", request.MaxOutputTokens)
	}
}

func TestBashToolExecutionRequestAcceptsStringBoolAndIntOverrides(t *testing.T) {
	root := t.TempDir()

	request, ok, err := NewBashTool().ExecutionRequest(root, json.RawMessage(`{"cmd":"git status","login":"TRUE","tty":"false","yield_time_ms":"250","max_output_tokens":"8000"}`))
	if err != nil {
		t.Fatalf("ExecutionRequest() error = %v", err)
	}
	if !ok {
		t.Fatal("ExecutionRequest() ok = false, want true")
	}
	if !request.LoginShell {
		t.Fatal("login shell = false, want true")
	}
	if request.TTY {
		t.Fatal("tty = true, want false")
	}
	if request.YieldTimeMS != 250 {
		t.Fatalf("YieldTimeMS = %d, want 250", request.YieldTimeMS)
	}
	if request.MaxOutputTokens != 8000 {
		t.Fatalf("MaxOutputTokens = %d, want 8000", request.MaxOutputTokens)
	}
}

func TestBashToolExecutionRequestLeavesSafeLocalCommandsWithoutNetworkTargets(t *testing.T) {
	root := t.TempDir()

	request, ok, err := NewBashTool().ExecutionRequest(root, json.RawMessage(`{"cmd":"git status --short"}`))
	if err != nil {
		t.Fatalf("ExecutionRequest() error = %v", err)
	}
	if !ok {
		t.Fatal("ExecutionRequest() ok = false, want true")
	}
	if len(request.NetworkTargets) != 0 {
		t.Fatalf("network targets = %#v", request.NetworkTargets)
	}
}

func TestBashToolExecutionRequestLeavesAmbiguousCommandsWithoutPreflightNetworkTargets(t *testing.T) {
	root := t.TempDir()

	request, ok, err := NewBashTool().ExecutionRequest(root, json.RawMessage(`{"cmd":"./script.sh --check"}`))
	if err != nil {
		t.Fatalf("ExecutionRequest() error = %v", err)
	}
	if !ok {
		t.Fatal("ExecutionRequest() ok = false, want true")
	}
	if len(request.NetworkTargets) != 0 {
		t.Fatalf("network targets = %#v", request.NetworkTargets)
	}
}

func TestBashToolExecutionRequestClassifiesPackageRegistryCommands(t *testing.T) {
	root := t.TempDir()

	tests := []struct {
		name    string
		command string
	}{
		{name: "npm install", command: "npm install react"},
		{name: "npm ci", command: "npm ci"},
		{name: "pnpm add", command: "pnpm add react"},
		{name: "yarn install", command: "yarn install"},
		{name: "bun add", command: "bun add react"},
		{name: "go get", command: "go get github.com/example/tool"},
		{name: "go mod tidy", command: "go mod tidy"},
		{name: "cargo add", command: "cargo add serde"},
		{name: "cargo install", command: "cargo install cargo-watch"},
		{name: "composer install", command: "composer install"},
		{name: "composer require", command: "composer require laravel/framework"},
		{name: "poetry install", command: "poetry install"},
		{name: "poetry add", command: "poetry add requests"},
		{name: "pip install", command: "pip install requests"},
		{name: "uv sync", command: "uv sync"},
		{name: "uv pip", command: "uv pip install requests"},
		{name: "zig fetch", command: "zig fetch zls"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request, ok, err := NewBashTool().ExecutionRequest(root, json.RawMessage(`{"cmd":"`+tt.command+`"}`))
			if err != nil {
				t.Fatalf("ExecutionRequest() error = %v", err)
			}
			if !ok {
				t.Fatal("ExecutionRequest() ok = false, want true")
			}
			if len(request.NetworkTargets) != 1 || request.NetworkTargets[0] != "package registries" {
				t.Fatalf("network targets = %#v", request.NetworkTargets)
			}
		})
	}
}

func TestCommandScopedNetworkTargetUsesPrefixRuleWhenPresent(t *testing.T) {
	if got := CommandScopedNetworkTarget("go test ./...", []string{"go", "test"}); got != "command: go test" {
		t.Fatalf("command scoped target = %q", got)
	}
}

func TestBashToolExecutionRequestClassifiesResolvedPackageScriptServerIntent(t *testing.T) {
	root := t.TempDir()
	client := filepath.Join(root, "client")
	if err := os.MkdirAll(client, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(client, "package.json"), []byte(`{"scripts":{"dev":"vite --host 0.0.0.0 --port 5173"}}`), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	request, ok, err := NewBashTool().ExecutionRequest(root, json.RawMessage(`{"cmd":"npm run dev","workdir":"client"}`))
	if err != nil {
		t.Fatalf("ExecutionRequest() error = %v", err)
	}
	if !ok {
		t.Fatal("ExecutionRequest() ok = false, want true")
	}
	if request.Intent != ExecutionIntentServer {
		t.Fatalf("intent = %q, want %q", request.Intent, ExecutionIntentServer)
	}
	if request.IntentCommand != "vite --host 0.0.0.0 --port 5173" {
		t.Fatalf("intent command = %q", request.IntentCommand)
	}
	if len(request.ReadyPatterns) == 0 {
		t.Fatal("ready patterns = empty, want server readiness hints")
	}
}

func TestBashToolExecutionRequestClassifiesResolvedViteBuildScriptAsOneShot(t *testing.T) {
	root := t.TempDir()
	client := filepath.Join(root, "client")
	if err := os.MkdirAll(client, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(client, "package.json"), []byte(`{"scripts":{"build":"vite build","dev":"vite --host 0.0.0.0 --port 5173"}}`), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	request, ok, err := NewBashTool().ExecutionRequest(root, json.RawMessage(`{"cmd":"cd client && npm run build","workdir":"."}`))
	if err != nil {
		t.Fatalf("ExecutionRequest() error = %v", err)
	}
	if !ok {
		t.Fatal("ExecutionRequest() ok = false, want true")
	}
	if request.Intent != ExecutionIntentOneShot {
		t.Fatalf("intent = %q, want %q", request.Intent, ExecutionIntentOneShot)
	}
	if request.IntentCommand != "vite build" {
		t.Fatalf("intent command = %q", request.IntentCommand)
	}
	if len(request.ReadyPatterns) != 0 {
		t.Fatalf("ready patterns = %#v, want empty", request.ReadyPatterns)
	}
}

func TestBashToolExecutionRequestClassifiesCdWrappedOneShotCommand(t *testing.T) {
	root := t.TempDir()
	client := filepath.Join(root, "client")
	if err := os.MkdirAll(client, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	request, ok, err := NewBashTool().ExecutionRequest(root, json.RawMessage(`{"cmd":"cd client && ls -la .","workdir":"."}`))
	if err != nil {
		t.Fatalf("ExecutionRequest() error = %v", err)
	}
	if !ok {
		t.Fatal("ExecutionRequest() ok = false, want true")
	}
	if request.Intent != ExecutionIntentOneShot {
		t.Fatalf("intent = %q, want %q", request.Intent, ExecutionIntentOneShot)
	}
	if request.IntentCommand != "ls -la ." {
		t.Fatalf("intent command = %q", request.IntentCommand)
	}
	if request.OpaquePathReason != "" {
		t.Fatalf("opaque path reason = %q, want empty", request.OpaquePathReason)
	}
}

func TestBashToolExecutionRequestClassifiesInstallThenPackageScriptServerIntent(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "package.json"), []byte(`{"scripts":{"dev":"ts-node-dev src/server.ts"}}`), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	request, ok, err := NewBashTool().ExecutionRequest(root, json.RawMessage(`{"cmd":"npm install --no-audit --no-fund && npm run dev"}`))
	if err != nil {
		t.Fatalf("ExecutionRequest() error = %v", err)
	}
	if !ok {
		t.Fatal("ExecutionRequest() ok = false, want true")
	}
	if request.Intent != ExecutionIntentServer {
		t.Fatalf("intent = %q, want %q", request.Intent, ExecutionIntentServer)
	}
	if request.IntentCommand != "ts-node-dev src/server.ts" {
		t.Fatalf("intent command = %q", request.IntentCommand)
	}
	if len(request.ReadyPatterns) == 0 {
		t.Fatal("ready patterns = empty, want server readiness hints")
	}
	if request.Effect != ExecutionEffectProcess {
		t.Fatalf("effect = %q, want %q", request.Effect, ExecutionEffectProcess)
	}
	if len(request.NetworkTargets) != 1 || request.NetworkTargets[0] != "package registries" {
		t.Fatalf("network targets = %#v", request.NetworkTargets)
	}
}

func TestBashToolExecutionRequestClassifiesDirectPersistentLauncherIntents(t *testing.T) {
	root := t.TempDir()

	tests := []struct {
		name         string
		command      string
		wantIntent   ExecutionIntent
		wantPatterns bool
	}{
		{
			name:         "go air watcher",
			command:      "air",
			wantIntent:   ExecutionIntentWatcher,
			wantPatterns: false,
		},
		{
			name:         "rust cargo watch watcher",
			command:      "cargo watch -x run",
			wantIntent:   ExecutionIntentWatcher,
			wantPatterns: false,
		},
		{
			name:         "rust trunk serve server",
			command:      "trunk serve --port 8080",
			wantIntent:   ExecutionIntentServer,
			wantPatterns: true,
		},
		{
			name:         "csharp dotnet watch watcher",
			command:      "dotnet watch run",
			wantIntent:   ExecutionIntentWatcher,
			wantPatterns: false,
		},
		{
			name:         "php built in server",
			command:      "php -S 127.0.0.1:8000 -t public",
			wantIntent:   ExecutionIntentServer,
			wantPatterns: true,
		},
		{
			name:         "php artisan serve",
			command:      "php artisan serve --host=127.0.0.1 --port=8000",
			wantIntent:   ExecutionIntentServer,
			wantPatterns: true,
		},
		{
			name:         "php symfony serve",
			command:      "symfony serve --port=8000",
			wantIntent:   ExecutionIntentServer,
			wantPatterns: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request, ok, err := NewBashTool().ExecutionRequest(root, json.RawMessage(`{"cmd":"`+tt.command+`"}`))
			if err != nil {
				t.Fatalf("ExecutionRequest() error = %v", err)
			}
			if !ok {
				t.Fatal("ExecutionRequest() ok = false, want true")
			}
			if request.Intent != tt.wantIntent {
				t.Fatalf("intent = %q, want %q", request.Intent, tt.wantIntent)
			}
			if request.IntentCommand != tt.command {
				t.Fatalf("intent command = %q, want %q", request.IntentCommand, tt.command)
			}
			if tt.wantPatterns && len(request.ReadyPatterns) == 0 {
				t.Fatalf("ready patterns = %#v, want non-empty", request.ReadyPatterns)
			}
			if !tt.wantPatterns && len(request.ReadyPatterns) != 0 {
				t.Fatalf("ready patterns = %#v, want empty", request.ReadyPatterns)
			}
		})
	}
}

func TestBashToolExecutionRequestKeepsGenericRuntimeCommandsAsOneShot(t *testing.T) {
	root := t.TempDir()

	tests := []struct {
		name    string
		command string
	}{
		{name: "go run", command: "go run ."},
		{name: "cargo run", command: "cargo run"},
		{name: "dotnet run", command: "dotnet run"},
		{name: "vite build", command: "vite build"},
		{name: "vite build with mode", command: "vite --mode production build"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request, ok, err := NewBashTool().ExecutionRequest(root, json.RawMessage(`{"cmd":"`+tt.command+`"}`))
			if err != nil {
				t.Fatalf("ExecutionRequest() error = %v", err)
			}
			if !ok {
				t.Fatal("ExecutionRequest() ok = false, want true")
			}
			if request.Intent != ExecutionIntentOneShot {
				t.Fatalf("intent = %q, want %q", request.Intent, ExecutionIntentOneShot)
			}
		})
	}
}

func TestBashToolExecutionRequestClassifiesViteBuildWatchAsWatcher(t *testing.T) {
	root := t.TempDir()

	request, ok, err := NewBashTool().ExecutionRequest(root, json.RawMessage(`{"cmd":"vite build --watch"}`))
	if err != nil {
		t.Fatalf("ExecutionRequest() error = %v", err)
	}
	if !ok {
		t.Fatal("ExecutionRequest() ok = false, want true")
	}
	if request.Intent != ExecutionIntentWatcher {
		t.Fatalf("intent = %q, want %q", request.Intent, ExecutionIntentWatcher)
	}
	if len(request.ReadyPatterns) != 0 {
		t.Fatalf("ready patterns = %#v, want empty", request.ReadyPatterns)
	}
}

func TestBashToolExecutionRequestLeavesCdWrappedServerLaunchNonOpaque(t *testing.T) {
	root := t.TempDir()
	client := filepath.Join(root, "client")
	if err := os.MkdirAll(client, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(client, "package.json"), []byte(`{"scripts":{"dev":"vite --host 0.0.0.0 --port 5173"}}`), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	request, ok, err := NewBashTool().ExecutionRequest(root, json.RawMessage(`{"cmd":"cd client && npm run dev","workdir":"."}`))
	if err != nil {
		t.Fatalf("ExecutionRequest() error = %v", err)
	}
	if !ok {
		t.Fatal("ExecutionRequest() ok = false, want true")
	}
	if request.Intent != ExecutionIntentServer {
		t.Fatalf("intent = %q, want %q", request.Intent, ExecutionIntentServer)
	}
	if request.OpaquePathReason != "" {
		t.Fatalf("opaque path reason = %q, want empty", request.OpaquePathReason)
	}
}

func TestBashToolExecutionRequestClassifiesCdWrappedInstallThenServerLaunch(t *testing.T) {
	root := t.TempDir()
	client := filepath.Join(root, "client")
	if err := os.MkdirAll(client, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(client, "package.json"), []byte(`{"scripts":{"dev":"vite --host 0.0.0.0 --port 5173"}}`), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	request, ok, err := NewBashTool().ExecutionRequest(root, json.RawMessage(`{"cmd":"cd client && npm install --no-audit --no-fund && npm run dev","workdir":"."}`))
	if err != nil {
		t.Fatalf("ExecutionRequest() error = %v", err)
	}
	if !ok {
		t.Fatal("ExecutionRequest() ok = false, want true")
	}
	if request.Intent != ExecutionIntentServer {
		t.Fatalf("intent = %q, want %q", request.Intent, ExecutionIntentServer)
	}
	if request.IntentCommand != "vite --host 0.0.0.0 --port 5173" {
		t.Fatalf("intent command = %q", request.IntentCommand)
	}
	if request.OpaquePathReason != "" {
		t.Fatalf("opaque path reason = %q, want empty", request.OpaquePathReason)
	}
}

func TestBashToolExecutionRequestKeepsDynamicCdOpaque(t *testing.T) {
	root := t.TempDir()

	request, ok, err := NewBashTool().ExecutionRequest(root, json.RawMessage(`{"cmd":"cd \"$DIR\" && ls -la .","workdir":"."}`))
	if err != nil {
		t.Fatalf("ExecutionRequest() error = %v", err)
	}
	if !ok {
		t.Fatal("ExecutionRequest() ok = false, want true")
	}
	if request.OpaquePathReason == "" {
		t.Fatal("opaque path reason = empty, want dynamic cd to remain opaque")
	}
}

func TestBashToolExecutionRequestKeepsConditionalCdOpaque(t *testing.T) {
	root := t.TempDir()
	client := filepath.Join(root, "client")
	if err := os.MkdirAll(client, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	request, ok, err := NewBashTool().ExecutionRequest(root, json.RawMessage(`{"cmd":"cd client || ls -la .","workdir":"."}`))
	if err != nil {
		t.Fatalf("ExecutionRequest() error = %v", err)
	}
	if !ok {
		t.Fatal("ExecutionRequest() ok = false, want true")
	}
	if request.OpaquePathReason == "" {
		t.Fatal("opaque path reason = empty, want conditional cd to remain opaque")
	}
}

func TestBashToolExecutionRequestRecordsCdTargetPathAccess(t *testing.T) {
	root := t.TempDir()
	client := filepath.Join(root, "client")
	if err := os.MkdirAll(client, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	request, ok, err := NewBashTool().ExecutionRequest(root, json.RawMessage(`{"cmd":"cd client && pwd","workdir":"."}`))
	if err != nil {
		t.Fatalf("ExecutionRequest() error = %v", err)
	}
	if !ok {
		t.Fatal("ExecutionRequest() ok = false, want true")
	}
	scope, err := workspace.New(root)
	if err != nil {
		t.Fatalf("workspace.New() error = %v", err)
	}
	decision, err := scope.Check(workspace.AccessWorkdir, "client")
	if err != nil {
		t.Fatalf("scope.Check() error = %v", err)
	}
	found := false
	for _, pathRequest := range request.PathRequests {
		if pathRequest.Access == workspace.AccessWorkdir && pathRequest.Path == decision.ResolvedPath {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("path requests = %#v, want workdir access for %q", request.PathRequests, decision.ResolvedPath)
	}
}
