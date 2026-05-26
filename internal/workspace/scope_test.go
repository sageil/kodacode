package workspace

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestScopeAuthorizeAllowsPathsInsideRoot(t *testing.T) {
	root := t.TempDir()
	scope, err := New(root)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	got, err := scope.Authorize(AccessRead, "src/app.go")
	if err != nil {
		t.Fatalf("Authorize() error = %v", err)
	}
	if !got.WithinRoot || got.Granted || got.ResolvedPath != filepath.Join(scope.Root(), "src", "app.go") {
		t.Fatalf("decision = %#v", got)
	}
}

func TestScopeAuthorizeRequiresApprovalForExternalPath(t *testing.T) {
	root := t.TempDir()
	external := filepath.Join(t.TempDir(), "outside.txt")

	scope, err := New(root)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	got, err := scope.Authorize(AccessRead, external)
	if !errors.Is(err, ErrPermissionRequired) {
		t.Fatalf("Authorize() error = %v, want ErrPermissionRequired", err)
	}
	if got.WithinRoot || got.Granted || !got.RequiresApproval() {
		t.Fatalf("decision = %#v", got)
	}
}

func TestScopeAuthorizeExpandsHomePathBeforePermissionCheck(t *testing.T) {
	root := t.TempDir()
	home := t.TempDir()
	t.Setenv("HOME", home)

	scope, err := New(root)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	got, err := scope.Authorize(AccessList, "~/Documents")
	if !errors.Is(err, ErrPermissionRequired) {
		t.Fatalf("Authorize() error = %v, want ErrPermissionRequired", err)
	}
	want := resolvePathAllowMissing(filepath.Join(home, "Documents"))
	if got.ResolvedPath != want {
		t.Fatalf("resolved path = %q, want %q", got.ResolvedPath, want)
	}
	if got.WithinRoot || got.Granted || !got.RequiresApproval() {
		t.Fatalf("decision = %#v", got)
	}
}

func TestScopeGrantAllowsExternalPathAfterApproval(t *testing.T) {
	root := t.TempDir()
	externalDir := t.TempDir()
	externalFile := filepath.Join(externalDir, "notes.txt")

	scope, err := New(root)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if _, err := scope.Grant(externalDir, true); err != nil {
		t.Fatalf("Grant() error = %v", err)
	}

	got, err := scope.Authorize(AccessList, externalFile)
	if err != nil {
		t.Fatalf("Authorize() error = %v", err)
	}
	if got.WithinRoot || !got.Granted {
		t.Fatalf("decision = %#v", got)
	}
}

func TestScopeGrantExactPathDoesNotAllowSibling(t *testing.T) {
	root := t.TempDir()
	externalDir := t.TempDir()
	allowed := filepath.Join(externalDir, "allowed.txt")
	sibling := filepath.Join(externalDir, "sibling.txt")

	scope, err := New(root)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if _, err := scope.Grant(allowed, false); err != nil {
		t.Fatalf("Grant() error = %v", err)
	}

	if _, err := scope.Authorize(AccessRead, allowed); err != nil {
		t.Fatalf("Authorize(allowed) error = %v", err)
	}
	if _, err := scope.Authorize(AccessRead, sibling); !errors.Is(err, ErrPermissionRequired) {
		t.Fatalf("Authorize(sibling) error = %v, want ErrPermissionRequired", err)
	}
}

func TestScopeAuthorizeRejectsSymlinkEscapeWithoutApproval(t *testing.T) {
	root := t.TempDir()
	externalDir := t.TempDir()

	if err := os.Symlink(externalDir, filepath.Join(root, "linked")); err != nil {
		t.Fatalf("Symlink() error = %v", err)
	}

	scope, err := New(root)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	_, err = scope.Authorize(AccessRead, filepath.Join("linked", "secret.txt"))
	if !errors.Is(err, ErrPermissionRequired) {
		t.Fatalf("Authorize() error = %v, want ErrPermissionRequired", err)
	}
}

func TestScopeNewRequiresExistingDirectory(t *testing.T) {
	if _, err := New(filepath.Join(t.TempDir(), "missing")); !errors.Is(err, ErrRootNotDirectory) {
		t.Fatalf("New() error = %v, want ErrRootNotDirectory", err)
	}
}

func TestScopeNewCanonicalizesExistingPathCaseOnCaseInsensitiveFilesystem(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("path-case normalization regression is specific to macOS case-insensitive filesystems")
	}

	parent := t.TempDir()
	actual := filepath.Join(parent, "Kairo")
	if err := os.Mkdir(actual, 0o755); err != nil {
		t.Fatalf("Mkdir(%s) error = %v", actual, err)
	}
	lower := filepath.Join(parent, "kairo")
	if _, err := os.Stat(lower); err != nil {
		t.Skipf("filesystem does not resolve case-insensitive path aliases: %v", err)
	}

	scope, err := New(lower)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	want := resolvePathAllowMissing(actual)
	if got := scope.Root(); got != want {
		t.Fatalf("root = %q, want %q", got, want)
	}
}
