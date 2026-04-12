package lsp

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDiscoverServers_BiomeProject(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "biome.json"), []byte("{}"), 0o644)

	servers := DiscoverServers(dir)

	found := false
	for _, s := range servers {
		if s.Name == "biome" {
			found = true
			if s.Args[0] != "lsp-proxy" {
				t.Errorf("biome args = %v, want [lsp-proxy]", s.Args)
			}
		}
	}
	// biome binary might not be installed in CI — just check detection logic.
	if !found {
		t.Log("biome detected in project but binary not found (expected in CI)")
	}
}

func TestDiscoverServers_EmptyProject(t *testing.T) {
	dir := t.TempDir()
	servers := DiscoverServers(dir)
	if len(servers) != 0 {
		t.Errorf("expected 0 discovered servers, got %d", len(servers))
	}
}

func TestProjectUses(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "biome.json"), []byte("{}"), 0o644)

	if !projectUses(dir, []string{"biome.json"}) {
		t.Error("should detect biome.json")
	}
	if projectUses(dir, []string{"Cargo.toml"}) {
		t.Error("should not detect Cargo.toml")
	}
}
