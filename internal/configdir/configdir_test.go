package configdir

import (
	"path/filepath"
	"testing"
)

func TestResolveRootPrefersXDGConfigHome(t *testing.T) {
	got := resolveRoot(
		func(key string) string {
			if key == "XDG_CONFIG_HOME" {
				return "/tmp/xdg"
			}
			return ""
		},
		func() (string, error) {
			return "/Users/example", nil
		},
		func() string {
			return "/tmp"
		},
	)

	want := filepath.Join("/tmp/xdg", "kodacode")
	if got != want {
		t.Fatalf("resolveRoot() = %q, want %q", got, want)
	}
}

func TestResolveRootFallsBackToDotConfigUnderHome(t *testing.T) {
	got := resolveRoot(
		func(string) string { return "" },
		func() (string, error) {
			return "/Users/example", nil
		},
		func() string {
			return "/tmp"
		},
	)

	want := filepath.Join("/Users/example", ".config", "kodacode")
	if got != want {
		t.Fatalf("resolveRoot() = %q, want %q", got, want)
	}
}
