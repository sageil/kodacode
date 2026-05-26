package lsp

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestProbeVTSLSDiagnosticsPublishing(t *testing.T) {
	if os.Getenv("RUN_VTSLS_PROBE") != "1" {
		t.Skip("set RUN_VTSLS_PROBE=1 to run against a real vtsls binary")
	}

	root, brokenPath, cleanPath := vtslsProbeWorkspace(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	server, err := Start(ctx, ServerConfig{
		Name:       "vtsls",
		Command:    "vtsls",
		Args:       []string{"--stdio"},
		Extensions: []string{".ts"},
	}, FileURI(root))
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer server.Shutdown(context.Background()) //nolint:errcheck

	brokenDiags, err := server.RefreshDiagnostics(ctx, brokenPath)
	if err != nil {
		t.Fatalf("RefreshDiagnostics(broken.ts) error = %v", err)
	}
	t.Logf("broken.ts diagnostics: %#v", brokenDiags)
	if os.Getenv("RUN_VTSLS_PROBE_REQUIRE_DIAGNOSTICS") == "1" && len(brokenDiags) == 0 {
		t.Fatal("broken.ts diagnostics = empty, want at least one diagnostic")
	}

	cleanDiags, err := server.RefreshDiagnostics(ctx, cleanPath)
	if err != nil {
		t.Fatalf("RefreshDiagnostics(clean.ts) error = %v", err)
	}
	t.Logf("clean.ts diagnostics: %#v", cleanDiags)
}

func vtslsProbeWorkspace(t *testing.T) (string, string, string) {
	t.Helper()
	if root := os.Getenv("RUN_VTSLS_PROBE_ROOT"); root != "" {
		brokenPath := os.Getenv("RUN_VTSLS_PROBE_BROKEN_FILE")
		cleanPath := os.Getenv("RUN_VTSLS_PROBE_CLEAN_FILE")
		if brokenPath == "" || cleanPath == "" {
			t.Fatal("set RUN_VTSLS_PROBE_BROKEN_FILE and RUN_VTSLS_PROBE_CLEAN_FILE when using RUN_VTSLS_PROBE_ROOT")
		}
		return root, brokenPath, cleanPath
	}

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "tsconfig.json"), []byte("{\n  \"compilerOptions\": {\"strict\": true, \"noUnusedLocals\": true, \"target\": \"ES2020\", \"module\": \"ESNext\"}\n}\n"), 0o644); err != nil {
		t.Fatalf("write tsconfig: %v", err)
	}

	cleanPath := filepath.Join(root, "clean.ts")
	if err := os.WriteFile(cleanPath, []byte("export const value = 1;\n"), 0o644); err != nil {
		t.Fatalf("write clean.ts: %v", err)
	}

	brokenPath := filepath.Join(root, "broken.ts")
	if err := os.WriteFile(brokenPath, []byte("export const unused = 1;\n"), 0o644); err != nil {
		t.Fatalf("write broken.ts: %v", err)
	}
	return root, brokenPath, cleanPath
}
