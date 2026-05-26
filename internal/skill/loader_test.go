package skill

import (
	"path/filepath"
	"reflect"
	"testing"
)

func TestDefaultGlobalSkillsDirUsesXDGConfigHome(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "/tmp/kodacode-xdg")
	t.Setenv("HOME", "/tmp/kodacode-home")

	dir, err := defaultGlobalSkillsDir()
	if err != nil {
		t.Fatalf("defaultGlobalSkillsDir() error = %v", err)
	}
	if dir != filepath.Join("/tmp/kodacode-xdg", "kodacode", "skills") {
		t.Fatalf("dir = %q", dir)
	}
}

func TestDefaultGlobalSkillsDirFallsBackToDotConfig(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("HOME", "/tmp/kodacode-home")

	dir, err := defaultGlobalSkillsDir()
	if err != nil {
		t.Fatalf("defaultGlobalSkillsDir() error = %v", err)
	}
	if dir != filepath.Join("/tmp/kodacode-home", ".config", "kodacode", "skills") {
		t.Fatalf("dir = %q", dir)
	}
}

func TestProjectSkillsDirsUsesWorkspaceKodacodeSkills(t *testing.T) {
	root := t.TempDir()
	got := projectSkillsDirs(root)
	want := []string{
		filepath.Join(root, ".kodacode", "skills"),
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("projectSkillsDirs() = %#v, want %#v", got, want)
	}
}
