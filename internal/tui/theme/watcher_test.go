package theme_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/sageil/kodacode/v1/internal/tui/theme"
)

func TestWatcher_ValidWrite_SendsMsg(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.yaml")
	initial := []byte("name: t\npalette:\n  primary: \"#cba6f7\"\n  surface: \"#1e1e2e\"\n  text: \"#cdd6f4\"\n")
	if err := os.WriteFile(path, initial, 0600); err != nil {
		t.Fatal(err)
	}
	received := make(chan *theme.Theme, 1)
	w, err := theme.NewWatcher(path, func(th *theme.Theme) {
		received <- th
	})
	if err != nil {
		t.Fatalf("NewWatcher: %v", err)
	}
	defer w.Close()
	updated := []byte("name: t\npalette:\n  primary: \"#89b4fa\"\n  surface: \"#1e1e2e\"\n  text: \"#cdd6f4\"\n")
	if err := os.WriteFile(path, updated, 0600); err != nil {
		t.Fatal(err)
	}
	select {
	case th := <-received:
		if th.Palette.Primary != "#89b4fa" {
			t.Errorf("NewWatcher callback: th.Palette.Primary = %s, want #89b4fa", th.Palette.Primary)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout: no theme received after file write")
	}
}

func TestWatcher_InvalidWrite_NoMsg(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.yaml")
	if err := os.WriteFile(path, []byte("not a theme\n"), 0600); err != nil {
		t.Fatal(err)
	}
	received := make(chan *theme.Theme, 1)
	w, err := theme.NewWatcher(path, func(th *theme.Theme) { received <- th })
	if err != nil {
		t.Fatalf("NewWatcher: %v", err)
	}
	defer w.Close()
	if err := os.WriteFile(path, []byte("palette:\n  primary: \"NOTACOLOR!!!\"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	select {
	case <-received:
		t.Fatal("expected no msg for invalid theme")
	case <-time.After(500 * time.Millisecond):
		// Correct. Nothing was received.
	}
}
