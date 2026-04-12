package tool

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/sageil/kodacode/v1/internal/lsp"
)

type fakeRenameServer struct {
	edit *lsp.WorkspaceEdit
	err  error
}

func (f fakeRenameServer) Rename(context.Context, string, int, int, string) (*lsp.WorkspaceEdit, error) {
	return f.edit, f.err
}

func TestRenameSymbolTool_AppliesWorkspaceEdits(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a.go")
	b := filepath.Join(dir, "b.go")
	if err := os.WriteFile(a, []byte("oldName()\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(b, []byte("call := oldName\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var notified []string
	tool := newRenameSymbolTool(
		func(context.Context, string, string) (renameServer, error) {
			return fakeRenameServer{edit: &lsp.WorkspaceEdit{
				Changes: map[string][]lsp.TextEdit{
					lsp.FileURI(a): {{
						Range:   lsp.Range{Start: lsp.Position{Line: 0, Character: 0}, End: lsp.Position{Line: 0, Character: 7}},
						NewText: "newName",
					}},
					lsp.FileURI(b): {{
						Range:   lsp.Range{Start: lsp.Position{Line: 0, Character: 8}, End: lsp.Position{Line: 0, Character: 15}},
						NewText: "newName",
					}},
				},
			}}, nil
		},
		func(_ context.Context, events []workspaceEditNotification) error {
			for _, event := range events {
				if event.Path != "" {
					notified = append(notified, event.Path)
				}
			}
			return nil
		},
		nil,
	)

	args := []byte(`{"filePath":"` + a + `","line":1,"character":0,"newName":"newName"}`)
	res, err := tool.Execute(t.Context(), ExecutionContext{WorkDir: dir}, args)
	if err != nil {
		t.Fatal(err)
	}
	if res.ErrorCode != "" {
		t.Fatalf("unexpected error result: %#v", res)
	}

	gotA, err := os.ReadFile(a)
	if err != nil {
		t.Fatal(err)
	}
	if string(gotA) != "newName()\n" {
		t.Fatalf("unexpected a.go content: %q", gotA)
	}
	gotB, err := os.ReadFile(b)
	if err != nil {
		t.Fatal(err)
	}
	if string(gotB) != "call := newName\n" {
		t.Fatalf("unexpected b.go content: %q", gotB)
	}
	slices.Sort(notified)
	wantNotified := []string{resolvePathAllowMissing(a), resolvePathAllowMissing(b)}
	if !slices.Equal(notified, wantNotified) {
		t.Fatalf("unexpected notify paths: %v", notified)
	}
}

func TestRenameSymbolTool_RejectsWorkspaceEditsOutsideProject(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a.go")
	if err := os.WriteFile(a, []byte("oldName()\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	tool := newRenameSymbolTool(
		func(context.Context, string, string) (renameServer, error) {
			return fakeRenameServer{edit: &lsp.WorkspaceEdit{
				Changes: map[string][]lsp.TextEdit{
					lsp.FileURI("/tmp/outside.go"): {{
						Range:   lsp.Range{Start: lsp.Position{Line: 0, Character: 0}, End: lsp.Position{Line: 0, Character: 7}},
						NewText: "newName",
					}},
				},
			}}, nil
		},
		nil,
		nil,
	)

	args := []byte(`{"filePath":"` + a + `","line":1,"character":0,"newName":"newName"}`)
	res, err := tool.Execute(t.Context(), ExecutionContext{WorkDir: dir}, args)
	if err != nil {
		t.Fatal(err)
	}
	if res == nil || res.ErrorCode != ErrCodePermission {
		t.Fatalf("expected permission error result, got %#v", res)
	}
}

func TestApplyLSPTextEdits_SortsDescending(t *testing.T) {
	content := "oldName oldName\n"
	updated, n, err := applyLSPTextEdits(content, []lsp.TextEdit{
		{
			Range:   lsp.Range{Start: lsp.Position{Line: 0, Character: 8}, End: lsp.Position{Line: 0, Character: 15}},
			NewText: "newName",
		},
		{
			Range:   lsp.Range{Start: lsp.Position{Line: 0, Character: 0}, End: lsp.Position{Line: 0, Character: 7}},
			NewText: "newName",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("edits applied = %d, want 2", n)
	}
	if updated != "newName newName\n" {
		t.Fatalf("unexpected content: %q", updated)
	}
}
