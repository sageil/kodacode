package skills

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/sageil/kodacode/v1/internal/config"
)

func writeSkillFile(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, name, "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestResolveAccess_ModelAllowAllFalseRestrictsSkills(t *testing.T) {
	cfg := &config.Config{
		Skills: config.GlobalSkillsConfig{
			Models: map[string]config.ModelSkillsConfig{
				"openai/gpt-4o": {AllowAll: false},
			},
		},
	}
	policy := ResolveAccess(cfg, "openai", "gpt-4o", config.AgentConfig{})
	if !policy.AllowAllSet || policy.AllowAll {
		t.Fatalf("policy = %+v, want explicit allow_all=false", policy)
	}

	idx := &Index{
		skills: []Skill{{Name: "go-review"}, {Name: "go-testing"}},
		byName: map[string]Skill{
			"go-review":  {Name: "go-review"},
			"go-testing": {Name: "go-testing"},
		},
	}
	if got := idx.Filter(policy); len(got) != 0 {
		t.Fatalf("len(Filter) = %d, want 0 when allow_all=false and no allowlist", len(got))
	}
}

func TestFilter_DenyOnlyPolicyStillAllowsOtherSkills(t *testing.T) {
	idx := &Index{
		skills: []Skill{{Name: "go-review"}, {Name: "secret-skill"}},
		byName: map[string]Skill{
			"go-review":    {Name: "go-review"},
			"secret-skill": {Name: "secret-skill"},
		},
	}
	got := idx.Filter(AccessPolicy{Deny: []string{"secret-skill"}})
	if len(got) != 1 || got[0].Name != "go-review" {
		t.Fatalf("Filter() = %+v, want only go-review", got)
	}
}

func TestLoadIndex_CachesAndInvalidatesOnSkillChange(t *testing.T) {
	dir := t.TempDir()
	writeSkillFile(t, dir, "go-review", "# Go Review\n")

	first := LoadIndex([]string{dir})
	second := LoadIndex([]string{dir})
	if first != second {
		t.Fatal("LoadIndex should reuse cached index when inputs are unchanged")
	}

	writeSkillFile(t, dir, "go-review", "# Go Review Updated\n")
	third := LoadIndex([]string{dir})
	if third == second {
		t.Fatal("LoadIndex should invalidate cached index after skill file changes")
	}
}
