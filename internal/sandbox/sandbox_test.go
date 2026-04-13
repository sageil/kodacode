package sandbox_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sageil/kodacode/v1/internal/permission"
	"github.com/sageil/kodacode/v1/internal/provider"
	"github.com/sageil/kodacode/v1/internal/sandbox"
	"github.com/sageil/kodacode/v1/internal/tool"
)

func newTestRegistry(tools ...tool.Tool) *tool.Registry {
	r := tool.NewRegistry()
	for i := range tools {
		r.Register(&tools[i])
	}
	return r
}

var okExecutor = func(_ context.Context, _ tool.ExecutionContext, _ []byte) (*tool.Result, error) {
	return &tool.Result{Output: "ok"}, nil
}

func noAsk(_, _ string) error   { return nil }
func noPublish(_ string, _ any) {}

func bashReg() *tool.Registry {
	return newTestRegistry(tool.Tool{Name: "bash", Execute: okExecutor})
}

func readReg() *tool.Registry {
	return newTestRegistry(tool.Tool{Name: "read", Execute: okExecutor})
}

func webFetchReg() *tool.Registry {
	return newTestRegistry(tool.Tool{Name: "web_fetch", Execute: okExecutor})
}

func openReg() *tool.Registry {
	return newTestRegistry(tool.Tool{Name: "open", Execute: okExecutor})
}

func writeReg() *tool.Registry {
	return newTestRegistry(tool.Tool{Name: "write", Execute: okExecutor})
}

// --- ShellTokens -----------------------------------------------------------

func TestShellTokens(t *testing.T) {
	t.Parallel()
	tests := []struct {
		cmd  string
		want []string
	}{
		{"echo hello", []string{"echo", "hello"}},
		{`echo "hello world"`, []string{"echo", "hello world"}},
		{`echo 'single quoted'`, []string{"echo", "single quoted"}},
		{`echo hello\ world`, []string{"echo", "hello world"}},
		{"ls -la", []string{"ls", "-la"}},
		{`cat "path with spaces/file.txt"`, []string{"cat", "path with spaces/file.txt"}},
		{"", nil},
		{"  spaced  ", []string{"spaced"}},
		{`echo "nested 'quotes'"`, []string{"echo", "nested 'quotes'"}},
	}
	for _, tt := range tests {
		t.Run(tt.cmd, func(t *testing.T) {
			t.Parallel()
			got := sandbox.ShellTokens(tt.cmd)
			if len(got) != len(tt.want) {
				t.Fatalf("ShellTokens(%q) = %v, want %v", tt.cmd, got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("ShellTokens(%q)[%d] = %q, want %q", tt.cmd, i, got[i], tt.want[i])
				}
			}
		})
	}
}

// --- Path confinement ------------------------------------------------------

func TestConfinePath(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "hello.txt"), []byte("hi"), 0644); err != nil {
		t.Fatal(err)
	}

	t.Run("allows in-project path", func(t *testing.T) {
		t.Parallel()
		s := sandbox.New(dir, sandbox.OriginTUI, readReg(), nil)
		out, err := s.Execute(context.Background(), "s",
			provider.ToolCall{ID: "c1", Name: "read", Arguments: `{"filePath":"hello.txt"}`},
			noAsk, nil, nil, noPublish)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if out.Output != "ok" {
			t.Errorf("got %q, want %q", out.Output, "ok")
		}
	})

	t.Run("rejects escape when denied", func(t *testing.T) {
		t.Parallel()
		s := sandbox.New(dir, sandbox.OriginTUI, readReg(), nil)
		deny := func(_, _ string) error { return tool.ErrDenied }
		_, err := s.Execute(context.Background(), "s",
			provider.ToolCall{ID: "c1", Name: "read", Arguments: `{"filePath":"../../etc/passwd"}`},
			deny, nil, nil, noPublish)
		if err == nil {
			t.Fatal("expected error for escaped path when denied")
		}
	})

	t.Run("allows external when granted", func(t *testing.T) {
		t.Parallel()
		ext := filepath.Join(t.TempDir(), "external.txt")
		os.WriteFile(ext, []byte("x"), 0644) //nolint:errcheck
		s := sandbox.New(dir, sandbox.OriginTUI, readReg(), nil)
		out, err := s.Execute(context.Background(), "s",
			provider.ToolCall{ID: "c1", Name: "read", Arguments: fmt.Sprintf(`{"filePath":%q}`, ext)},
			noAsk, nil, nil, noPublish)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if out.Output != "ok" {
			t.Errorf("got %q, want %q", out.Output, "ok")
		}
	})

	t.Run("allows /dev/null", func(t *testing.T) {
		t.Parallel()
		s := sandbox.New(dir, sandbox.OriginTUI, readReg(), nil)
		out, err := s.Execute(context.Background(), "s",
			provider.ToolCall{ID: "c1", Name: "read", Arguments: `{"filePath":"/dev/null"}`},
			noAsk, nil, nil, noPublish)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if out.Output != "ok" {
			t.Errorf("got %q, want %q", out.Output, "ok")
		}
	})

	t.Run("allows configured allowed path", func(t *testing.T) {
		t.Parallel()
		extDir := t.TempDir()
		extFile := filepath.Join(extDir, "data.txt")
		os.WriteFile(extFile, []byte("d"), 0644) //nolint:errcheck
		s := sandbox.New(dir, sandbox.OriginTUI, readReg(), nil, sandbox.SandboxOptions{
			AllowedPaths: []string{extDir},
		})
		out, err := s.Execute(context.Background(), "s",
			provider.ToolCall{ID: "c1", Name: "read", Arguments: fmt.Sprintf(`{"filePath":%q}`, extFile)},
			noAsk, nil, nil, noPublish)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if out.Output != "ok" {
			t.Errorf("got %q, want %q", out.Output, "ok")
		}
	})

	t.Run("hard denies root directory without prompting", func(t *testing.T) {
		t.Parallel()
		s := sandbox.New(dir, sandbox.OriginTUI, readReg(), nil)
		out, err := s.Execute(context.Background(), "s",
			provider.ToolCall{ID: "c1", Name: "read", Arguments: `{"filePath":"/"}`},
			noAsk, nil, nil, noPublish)
		if err != nil {
			t.Fatalf("should return result not error: %v", err)
		}
		if !strings.Contains(out.Output, "access denied") {
			t.Errorf("output should contain denial message, got: %s", out.Output)
		}
	})

	t.Run("hard denies home directory without prompting", func(t *testing.T) {
		t.Parallel()
		home, _ := os.UserHomeDir()
		s := sandbox.New(dir, sandbox.OriginTUI, readReg(), nil)
		out, err := s.Execute(context.Background(), "s",
			provider.ToolCall{ID: "c1", Name: "read", Arguments: fmt.Sprintf(`{"filePath":%q}`, home)},
			noAsk, nil, nil, noPublish)
		if err != nil {
			t.Fatalf("should return result not error: %v", err)
		}
		if !strings.Contains(out.Output, "access denied") {
			t.Errorf("output should contain denial message, got: %s", out.Output)
		}
	})

	t.Run("resolves leading-slash path to project-relative when file exists", func(t *testing.T) {
		t.Parallel()
		subDir := filepath.Join(dir, "src")
		os.MkdirAll(subDir, 0755)                                              //nolint:errcheck
		os.WriteFile(filepath.Join(subDir, "server.ts"), []byte("code"), 0644) //nolint:errcheck
		s := sandbox.New(dir, sandbox.OriginTUI, readReg(), nil)
		out, err := s.Execute(context.Background(), "s",
			provider.ToolCall{ID: "c1", Name: "read", Arguments: `{"filePath":"/src/server.ts"}`},
			noAsk, nil, nil, noPublish)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if out.Output != "ok" {
			t.Errorf("got %q, want %q", out.Output, "ok")
		}
	})

	t.Run("rejects leading-slash path when file does not exist in project", func(t *testing.T) {
		t.Parallel()
		s := sandbox.New(dir, sandbox.OriginTUI, readReg(), nil)
		_, err := s.Execute(context.Background(), "s",
			provider.ToolCall{ID: "c1", Name: "read", Arguments: `{"filePath":"/etc/passwd"}`},
			nil, nil, nil, noPublish)
		if err == nil {
			t.Fatal("expected error for /etc/passwd even with strip-retry")
		}
	})

	t.Run("outside path no ask callback returns error", func(t *testing.T) {
		t.Parallel()
		s := sandbox.New(dir, sandbox.OriginTUI, readReg(), nil)
		_, err := s.Execute(context.Background(), "s",
			provider.ToolCall{ID: "c1", Name: "read", Arguments: `{"filePath":"/etc/passwd"}`},
			nil, nil, nil, noPublish)
		if err == nil {
			t.Fatal("expected error when ask is nil for outside path")
		}
	})

	t.Run("asks for new file under symlinked external directory", func(t *testing.T) {
		t.Parallel()
		externalDir := t.TempDir()
		linkPath := filepath.Join(dir, "linked-out")
		if err := os.Symlink(externalDir, linkPath); err != nil {
			t.Fatal(err)
		}
		askCalled := false
		s := sandbox.New(dir, sandbox.OriginTUI, writeReg(), nil)
		out, err := s.Execute(context.Background(), "s",
			provider.ToolCall{ID: "c1", Name: "write", Arguments: `{"filePath":"linked-out/new.txt","content":"hello"}`},
			func(_, _ string) error {
				askCalled = true
				return nil
			}, nil, nil, noPublish)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !askCalled {
			t.Fatal("expected symlinked external write to require permission")
		}
		if out.Output != "ok" {
			t.Errorf("got %q, want %q", out.Output, "ok")
		}
	})
}

// --- Tool name normalization -----------------------------------------------

func TestToolNameNormalization(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	reg := newTestRegistry(tool.Tool{Name: "bash", Execute: okExecutor})
	s := sandbox.New(dir, sandbox.OriginTUI, reg, nil)

	t.Run("lowercase match", func(t *testing.T) {
		t.Parallel()
		out, err := s.Execute(context.Background(), "s",
			provider.ToolCall{ID: "c1", Name: "Bash", Arguments: `{"command":"echo hi"}`},
			noAsk, nil, nil, noPublish)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if out.Output != "ok" {
			t.Errorf("got %q, want %q", out.Output, "ok")
		}
	})

	t.Run("unknown tool returns structured error", func(t *testing.T) {
		t.Parallel()
		out, err := s.Execute(context.Background(), "s",
			provider.ToolCall{ID: "c1", Name: "nonexistent", Arguments: `{}`},
			noAsk, nil, nil, noPublish)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if out == nil || out.Output == "" {
			t.Error("expected structured error message")
		}
	})
}

// --- KnownTool -------------------------------------------------------------

func TestKnownTool(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	reg := newTestRegistry(
		tool.Tool{Name: "bash"},
		tool.Tool{Name: "web-search-prime_search"},
	)
	s := sandbox.New(dir, sandbox.OriginTUI, reg, nil)

	tests := []struct {
		name string
		want bool
	}{
		{"bash", true},
		{"Bash", true},
		{"ls", false},
		{"web-search-prime_search", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := s.KnownTool(tt.name); got != tt.want {
				t.Errorf("KnownTool(%q) = %v, want %v", tt.name, got, tt.want)
			}
		})
	}
}

// --- IsReadOnly ------------------------------------------------------------

func TestIsReadOnly(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	reg := newTestRegistry(
		tool.Tool{Name: "read", ReadOnly: true},
		tool.Tool{Name: "write"},
	)
	s := sandbox.New(dir, sandbox.OriginTUI, reg, nil)

	if !s.IsReadOnly("read") {
		t.Error("IsReadOnly(read) = false, want true")
	}
	if s.IsReadOnly("write") {
		t.Error("IsReadOnly(write) = true, want false")
	}
}

// --- MCP tool dispatch -----------------------------------------------------

func TestMCPToolDispatch(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	called := false
	reg := newTestRegistry(tool.Tool{
		Name: "web-search-prime_search",
		Execute: func(_ context.Context, _ tool.ExecutionContext, _ []byte) (*tool.Result, error) {
			called = true
			return &tool.Result{Output: "results"}, nil
		},
	})
	s := sandbox.New(dir, sandbox.OriginTUI, reg, nil)
	out, err := s.Execute(context.Background(), "s",
		provider.ToolCall{ID: "c1", Name: "web-search-prime_search", Arguments: `{"query":"foo"}`},
		noAsk, nil, nil, noPublish)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Error("executor was not called")
	}
	if out.Output != "results" {
		t.Errorf("got %q, want %q", out.Output, "results")
	}
}

// --- Bash outside-path checks (table-driven) -------------------------------

func TestBashOutsidePath(t *testing.T) {
	t.Parallel()
	rawDir := t.TempDir()
	dir, _ := filepath.EvalSymlinks(rawDir)

	tests := []struct {
		name    string
		cmd     string
		wantAsk bool
		wantErr bool
	}{
		// Should trigger permission
		{"tilde path", "cat ~/.bashrc", true, false},
		{"absolute path outside", "cat /etc/passwd", true, false},
		{"$HOME env var", "cat $HOME/.bashrc", true, false},
		{"${HOME} env var", "cat ${HOME}/.bashrc", true, false},
		{"dot-dot escape", "cat ../../etc/passwd", true, false},
		{"tilde alone", "cd ~", false, true},

		// Should NOT trigger permission
		{"in-project command", "ls -la", false, false},
		{"dev null redirect", "echo hi > /dev/null", false, false},
		{"dev null read", "cat /dev/null", false, false},
		{"sed regex", "sed '/^export/d' file.txt", false, false},
		{"awk regex", "awk '/^[0-9]+/{ print }' file.txt", false, false},
		{"go wildcard", "go test ./...", false, false},
		{"empty command", "", false, false},
		{"non-path env var", "echo $file", false, false},
		{"heredoc with paths", "cat <<EOF\n/etc/passwd\nEOF", false, false},

		// echo/printf content should not trigger.
		{"echo unquoted path", "echo /usr/local/bin", false, false},
		{"echo quoted path", `echo "export PATH=/usr/local/bin:$PATH"`, false, false},
		{"echo multiple paths", "echo /etc/hosts /usr/bin/env", false, false},
		{"printf path", `printf '%s\n' /etc/hosts`, false, false},
		{"echo pipe to other cmd", "echo /usr/local/bin | grep bin", false, false},
		{"echo redirect in-project", "echo /usr/local/bin > output.txt", false, false},

		// echo redirect outside should trigger.
		{"echo redirect outside", "echo hello > /tmp/outside.txt", true, false},
		{"echo append outside", "echo hello >> /tmp/outside.txt", true, false},

		// pipeline: second segment is not echo, should check its args
		{"pipe to cat outside", "echo foo | cat /etc/passwd", true, false},

		// semicolon resets context
		{"semicolon resets cmd", "echo /foo ; cat /etc/passwd", true, false},
		{"and-and resets cmd", "echo /foo && cat /etc/passwd", true, false},

		// in-project absolute path
		{"absolute in-project path", fmt.Sprintf("cat %s/file.txt", dir), false, false},

		// Quoted patterns containing slashes should not trigger.
		{"grep regex with slashes", fmt.Sprintf(`cd %s && grep -r "// @ts-ignore\|// @ts-nocheck" src --include="*.ts"`, dir), false, false},
		{"grep double slash pattern", `grep -r "//TODO" src`, false, false},
		{"sed substitution", `sed 's/foo/bar/g' file.txt`, false, false},
		{"single-quoted existing path triggers", `grep '/usr/local/bin' file.txt`, true, false},
		{"single-quoted nonexistent path skips", `grep '/nonexistent/path' file.txt`, false, false},

		// Quoted external file access must trigger.
		{"cat single-quoted external", `cat '/etc/passwd'`, true, false},
		{"cat double-quoted external", `cat "/etc/passwd"`, true, false},

		// Quoted redirect targets should still trigger.
		{"quoted redirect outside", `echo hello > "/tmp/outside.txt"`, true, false},
		{"python dash c", `python3 -c 'open("/etc/passwd").read()'`, true, false},
		{"xargs indirect path", `printf '/etc/passwd\n' | xargs cat`, true, false},
		{"flag value path", `tool --config=/etc/app.conf`, true, false},
		{"env assignment path", `CONFIG=/etc/app.conf tool`, true, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			askCalled := false
			s := sandbox.New(dir, sandbox.OriginTUI, bashReg(), nil)
			askFn := func(_, _ string) error {
				askCalled = true
				return nil
			}
			_, err := s.Execute(context.Background(), "s",
				provider.ToolCall{ID: "c1", Name: "bash", Arguments: fmt.Sprintf(`{"command":%q}`, tt.cmd)},
				askFn, nil, nil, noPublish)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if askCalled != tt.wantAsk {
				t.Errorf("ask called = %v, want %v for cmd %q", askCalled, tt.wantAsk, tt.cmd)
			}
		})
	}
}

// --- Bash delete checks ----------------------------------------------------

func TestBashDelete(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	tests := []struct {
		name   string
		perm   permission.Config
		cmd    string
		wantOk bool
	}{
		{"rm asks by default", nil, "rm file.txt", true},
		{"rm allowed", permission.Config{"bash_rm": &permission.Rule{Action: permission.ActionAllow}}, "rm file.txt", true},
		{"rm denied", permission.Config{"bash_rm": &permission.Rule{Action: permission.ActionDeny}}, "rm file.txt", false},
		{"rmdir asks", permission.Config{"bash_rm": &permission.Rule{Action: permission.ActionAsk}}, "rmdir dir", true},
		{"no rm no ask", nil, "ls -la", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			s := sandbox.New(dir, sandbox.OriginTUI, bashReg(), tt.perm)
			_, err := s.Execute(context.Background(), "s",
				provider.ToolCall{ID: "c1", Name: "bash", Arguments: fmt.Sprintf(`{"command":%q}`, tt.cmd)},
				noAsk, nil, nil, noPublish)
			if tt.wantOk && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !tt.wantOk && err == nil {
				t.Fatal("expected error, got nil")
			}
		})
	}
}

func TestBashGitMutationsRequireGitTool(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	t.Run("mutating git via bash is denied", func(t *testing.T) {
		t.Parallel()
		s := sandbox.New(dir, sandbox.OriginTUI, bashReg(), nil)
		_, err := s.Execute(context.Background(), "s",
			provider.ToolCall{ID: "c1", Name: "bash", Arguments: `{"command":"git commit -m test"}`},
			noAsk, nil, nil, noPublish)
		if err == nil {
			t.Fatal("expected mutating git command to be denied")
		}
		if !strings.Contains(err.Error(), "use the git tool") {
			t.Fatalf("error = %v, want git tool guidance", err)
		}
	})

	t.Run("read-only git via bash is allowed", func(t *testing.T) {
		t.Parallel()
		s := sandbox.New(dir, sandbox.OriginTUI, bashReg(), nil)
		out, err := s.Execute(context.Background(), "s",
			provider.ToolCall{ID: "c1", Name: "bash", Arguments: `{"command":"git status"}`},
			noAsk, nil, nil, noPublish)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if out.Output != "ok" {
			t.Fatalf("output = %q, want ok", out.Output)
		}
	})
}

func TestConfiguredPermissionEnforcement(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	t.Run("read env file asks under defaults", func(t *testing.T) {
		t.Parallel()
		envFile := filepath.Join(dir, ".env")
		if err := os.WriteFile(envFile, []byte("SECRET=1"), 0644); err != nil {
			t.Fatal(err)
		}

		askCalled := false
		s := sandbox.New(dir, sandbox.OriginTUI, readReg(), permission.Defaults())
		out, err := s.Execute(context.Background(), "s",
			provider.ToolCall{ID: "c1", Name: "read", Arguments: `{"filePath":".env"}`},
			func(_, _ string) error {
				askCalled = true
				return nil
			}, nil, nil, noPublish)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !askCalled {
			t.Fatal("expected read .env to require permission")
		}
		if out.Output != "ok" {
			t.Errorf("got %q, want %q", out.Output, "ok")
		}
	})

	t.Run("external_directory deny blocks escaped read", func(t *testing.T) {
		t.Parallel()
		extFile := filepath.Join(t.TempDir(), "outside.txt")
		if err := os.WriteFile(extFile, []byte("x"), 0644); err != nil {
			t.Fatal(err)
		}

		askCalled := false
		s := sandbox.New(dir, sandbox.OriginTUI, readReg(), permission.Config{
			"external_directory": &permission.Rule{Action: permission.ActionDeny},
		})
		_, err := s.Execute(context.Background(), "s",
			provider.ToolCall{ID: "c1", Name: "read", Arguments: fmt.Sprintf(`{"filePath":%q}`, extFile)},
			func(_, _ string) error {
				askCalled = true
				return nil
			}, nil, nil, noPublish)
		if err == nil {
			t.Fatal("expected denied external read")
		}
		if askCalled {
			t.Fatal("did not expect prompt for denied external read")
		}
	})

	t.Run("web_fetch ask rule is enforced", func(t *testing.T) {
		t.Parallel()
		askCalled := false
		s := sandbox.New(dir, sandbox.OriginTUI, webFetchReg(), permission.Config{
			"web_fetch": &permission.Rule{Action: permission.ActionAsk},
		})
		out, err := s.Execute(context.Background(), "s",
			provider.ToolCall{ID: "c1", Name: "web_fetch", Arguments: `{"url":"https://example.com"}`},
			func(_, _ string) error {
				askCalled = true
				return nil
			}, nil, nil, noPublish)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !askCalled {
			t.Fatal("expected web_fetch ask rule to prompt")
		}
		if out.Output != "ok" {
			t.Errorf("got %q, want %q", out.Output, "ok")
		}
	})

	t.Run("open is path-confined", func(t *testing.T) {
		t.Parallel()
		extFile := filepath.Join(t.TempDir(), "outside.txt")
		if err := os.WriteFile(extFile, []byte("x"), 0644); err != nil {
			t.Fatal(err)
		}

		s := sandbox.New(dir, sandbox.OriginTUI, openReg(), permission.Config{
			"external_directory": &permission.Rule{Action: permission.ActionDeny},
		})
		_, err := s.Execute(context.Background(), "s",
			provider.ToolCall{ID: "c1", Name: "open", Arguments: fmt.Sprintf(`{"filePath":%q}`, extFile)},
			noAsk, nil, nil, noPublish)
		if err == nil {
			t.Fatal("expected open on external path to be denied")
		}
	})
}

func TestBashOutsidePathHonorsExternalDirectoryPermission(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	t.Run("allow skips prompt", func(t *testing.T) {
		t.Parallel()
		askCalled := false
		s := sandbox.New(dir, sandbox.OriginTUI, bashReg(), permission.Config{
			"external_directory": &permission.Rule{Action: permission.ActionAllow},
		})
		_, err := s.Execute(context.Background(), "s",
			provider.ToolCall{ID: "c1", Name: "bash", Arguments: `{"command":"cat /etc/passwd"}`},
			func(_, _ string) error {
				askCalled = true
				return nil
			}, nil, nil, noPublish)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if askCalled {
			t.Fatal("did not expect prompt when external_directory is allow")
		}
	})

	t.Run("deny blocks without prompt", func(t *testing.T) {
		t.Parallel()
		askCalled := false
		s := sandbox.New(dir, sandbox.OriginTUI, bashReg(), permission.Config{
			"external_directory": &permission.Rule{Action: permission.ActionDeny},
		})
		_, err := s.Execute(context.Background(), "s",
			provider.ToolCall{ID: "c1", Name: "bash", Arguments: `{"command":"cat /etc/passwd"}`},
			func(_, _ string) error {
				askCalled = true
				return nil
			}, nil, nil, noPublish)
		if err == nil {
			t.Fatal("expected denied bash external path")
		}
		if askCalled {
			t.Fatal("did not expect prompt when external_directory is deny")
		}
	})

	t.Run("path-specific allow works for bash", func(t *testing.T) {
		t.Parallel()
		allowedFile := filepath.Join(t.TempDir(), "allowed.txt")
		if err := os.WriteFile(allowedFile, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		askCalled := false
		s := sandbox.New(dir, sandbox.OriginTUI, bashReg(), permission.Config{
			"external_directory": &permission.Rule{
				Patterns: []permission.Pattern{
					{Glob: "*", Action: permission.ActionDeny},
					{Glob: filepath.Base(allowedFile), Action: permission.ActionAllow},
				},
			},
		})
		_, err := s.Execute(context.Background(), "s",
			provider.ToolCall{ID: "c1", Name: "bash", Arguments: fmt.Sprintf(`{"command":"cat %s"}`, allowedFile)},
			func(_, _ string) error {
				askCalled = true
				return nil
			}, nil, nil, noPublish)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if askCalled {
			t.Fatal("did not expect prompt for explicitly allowed external bash path")
		}
	})

	t.Run("external workdir requires permission", func(t *testing.T) {
		t.Parallel()
		externalDir := t.TempDir()
		askCalled := false
		s := sandbox.New(dir, sandbox.OriginTUI, bashReg(), nil)
		_, err := s.Execute(context.Background(), "s",
			provider.ToolCall{ID: "c1", Name: "bash", Arguments: fmt.Sprintf(`{"command":"pwd","workdir":%q}`, externalDir)},
			func(_, _ string) error {
				askCalled = true
				return nil
			}, nil, nil, noPublish)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !askCalled {
			t.Fatal("expected external bash workdir to require permission")
		}
	})

	t.Run("dynamic shell dispatch requires permission", func(t *testing.T) {
		t.Parallel()
		askCalled := false
		s := sandbox.New(dir, sandbox.OriginTUI, bashReg(), nil)
		_, err := s.Execute(context.Background(), "s",
			provider.ToolCall{ID: "c1", Name: "bash", Arguments: `{"command":"bash -c 'cat /etc/passwd'"}`},
			func(_, _ string) error {
				askCalled = true
				return nil
			}, nil, nil, noPublish)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !askCalled {
			t.Fatal("expected bash -c command to require permission review")
		}
	})

	t.Run("root workdir is denied", func(t *testing.T) {
		t.Parallel()
		askCalled := false
		s := sandbox.New(dir, sandbox.OriginTUI, bashReg(), nil)
		_, err := s.Execute(context.Background(), "s",
			provider.ToolCall{ID: "c1", Name: "bash", Arguments: `{"command":"pwd","workdir":"/"}`},
			func(_, _ string) error {
				askCalled = true
				return nil
			}, nil, nil, noPublish)
		if err == nil {
			t.Fatal("expected root workdir to be denied")
		}
		if askCalled {
			t.Fatal("did not expect prompt for denied root workdir")
		}
	})
}

// --- Context helpers -------------------------------------------------------

func TestContextRoundTrip(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	reg := newTestRegistry(tool.Tool{Name: "bash"})
	s := sandbox.New(dir, sandbox.OriginTUI, reg, nil)

	ctx := sandbox.WithContext(context.Background(), s)
	got := sandbox.FromContext(ctx)
	if got != s {
		t.Errorf("FromContext returned %p, want %p", got, s)
	}
}

func TestFromContextNil(t *testing.T) {
	t.Parallel()
	got := sandbox.FromContext(context.Background())
	if got != nil {
		t.Errorf("FromContext on empty context = %p, want nil", got)
	}
}

// --- Middleware ------------------------------------------------------------

func TestNewMiddleware(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	reg := newTestRegistry(tool.Tool{Name: "bash"})
	s := sandbox.New(dir, sandbox.OriginTUI, reg, nil)
	mw := sandbox.NewMiddleware(s)
	if mw == nil {
		t.Fatal("NewMiddleware returned nil")
	}
}

// --- SandboxOptions --------------------------------------------------------

func TestNewWithOptions(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	reg := newTestRegistry(tool.Tool{Name: "bash"})
	s := sandbox.New(dir, sandbox.OriginTUI, reg, nil, sandbox.SandboxOptions{
		AllowedPaths:   []string{"/tmp"},
		IgnorePatterns: []string{"*.log"},
		SkillDirs:      []string{"/skills"},
	})
	if s == nil {
		t.Fatal("New with options returned nil")
	}
}

// --- Edge cases for confinePathArg / resolvePathArg ------------------------

func TestPathArgEdgeCases(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	t.Run("missing field passes through", func(t *testing.T) {
		t.Parallel()
		reg := newTestRegistry(tool.Tool{Name: "glob", Execute: okExecutor})
		s := sandbox.New(dir, sandbox.OriginTUI, reg, nil)
		out, err := s.Execute(context.Background(), "s",
			provider.ToolCall{ID: "c1", Name: "glob", Arguments: `{"pattern":"*.go"}`},
			noAsk, nil, nil, noPublish)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if out.Output != "ok" {
			t.Errorf("got %q, want %q", out.Output, "ok")
		}
	})

	t.Run("non-string path field passes through", func(t *testing.T) {
		t.Parallel()
		reg := newTestRegistry(tool.Tool{Name: "read", Execute: okExecutor})
		s := sandbox.New(dir, sandbox.OriginTUI, reg, nil)
		out, err := s.Execute(context.Background(), "s",
			provider.ToolCall{ID: "c1", Name: "read", Arguments: `{"filePath":123}`},
			noAsk, nil, nil, noPublish)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if out.Output != "ok" {
			t.Errorf("got %q, want %q", out.Output, "ok")
		}
	})

	t.Run("invalid JSON passes through", func(t *testing.T) {
		t.Parallel()
		reg := newTestRegistry(tool.Tool{Name: "read", Execute: okExecutor})
		s := sandbox.New(dir, sandbox.OriginTUI, reg, nil)
		out, err := s.Execute(context.Background(), "s",
			provider.ToolCall{ID: "c1", Name: "read", Arguments: `not json`},
			noAsk, nil, nil, noPublish)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if out.Output != "ok" {
			t.Errorf("got %q, want %q", out.Output, "ok")
		}
	})
}

// --- Bash with invalid JSON ------------------------------------------------

func TestBashInvalidJSON(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	s := sandbox.New(dir, sandbox.OriginTUI, bashReg(), nil)
	out, err := s.Execute(context.Background(), "s",
		provider.ToolCall{ID: "c1", Name: "bash", Arguments: `not json`},
		noAsk, nil, nil, noPublish)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Output != "ok" {
		t.Errorf("got %q, want %q", out.Output, "ok")
	}
}
