package tool

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sageil/kodacode/v1/internal/skills"
)

func TestSkillTool_ProjectOverridesGlobal(t *testing.T) {
	projectSkillsDir := t.TempDir()
	globalSkillsDir := t.TempDir()

	writeSkillFixture(t, filepath.Join(globalSkillsDir, "reviewer"), "---\nname: reviewer\ndescription: Global reviewer\n---\n# Reviewer\nGlobal content\n")
	writeSkillFixture(t, filepath.Join(projectSkillsDir, "reviewer"), "---\nname: reviewer\ndescription: Project reviewer\n---\n# Reviewer\nProject content\n")

	result, err := NewSkillTool().Execute(context.Background(), ExecutionContext{
		SkillDirs: []string{projectSkillsDir, globalSkillsDir},
	}, []byte(`{"name":"reviewer"}`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result.Output, "Project content") {
		t.Fatalf("skill output = %q, want project override", result.Output)
	}
	if strings.Contains(result.Output, "Global content") {
		t.Fatalf("skill output should not use global version: %q", result.Output)
	}
}

func TestSkillTool_LargeSkillReturnsTOCAndSectionLoads(t *testing.T) {
	skillsDir := t.TempDir()
	writeSkillFixture(t, filepath.Join(skillsDir, "go-migrations"), largeSkillContent())

	tool := NewSkillTool()
	ectx := ExecutionContext{SkillDirs: []string{skillsDir}}

	result, err := tool.Execute(context.Background(), ectx, []byte(`{"name":"go-migrations"}`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result.Output, "Sections:") {
		t.Fatalf("skill output should return TOC for large skill: %q", result.Output)
	}
	if !strings.Contains(result.Output, "usage") {
		t.Fatalf("skill TOC missing section id: %q", result.Output)
	}

	result, err = tool.Execute(context.Background(), ectx, []byte(`{"name":"go-migrations","section":"usage"}`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result.Output, "# Usage") {
		t.Fatalf("section load missing section content: %q", result.Output)
	}
	if strings.Contains(result.Output, "# Reference") {
		t.Fatalf("section load should not include later sections: %q", result.Output)
	}
}

func TestSearchSkillsAndSkillTool_RespectPolicy(t *testing.T) {
	skillsDir := t.TempDir()
	writeSkillFixture(t, filepath.Join(skillsDir, "go-review"), "---\nname: go-review\ndescription: Review Go changes\ntriggers:\n  - review go code\n---\n# Go Review\n")
	writeSkillFixture(t, filepath.Join(skillsDir, "secret-skill"), "---\nname: secret-skill\ndescription: Secret workflow\ntriggers:\n  - secret workflow\n---\n# Secret\n")

	ectx := ExecutionContext{
		SkillDirs: []string{skillsDir},
		SkillPolicy: skills.AccessPolicy{
			Deny: []string{"secret-skill"},
		},
	}

	result, err := NewSearchSkillsTool().Execute(context.Background(), ectx, []byte(`{"query":"secret workflow"}`))
	if err != nil {
		t.Fatal(err)
	}
	if result.ErrorCode != ErrCodeNotFound {
		t.Fatalf("search error_code = %q, want %q", result.ErrorCode, ErrCodeNotFound)
	}

	result, err = NewSkillTool().Execute(context.Background(), ectx, []byte(`{"name":"secret-skill"}`))
	if err != nil {
		t.Fatal(err)
	}
	if result.ErrorCode != ErrCodeNotFound {
		t.Fatalf("skill error_code = %q, want %q", result.ErrorCode, ErrCodeNotFound)
	}
}

func TestSearchSkillsTool_IncludesSuggestedHints(t *testing.T) {
	skillsDir := t.TempDir()
	writeSkillFixture(t, filepath.Join(skillsDir, "go-review"), `---
name: go-review
description: Review Go changes
triggers:
  - review go code
suggests:
  before: [go-testing]
  after: [polish]
---
# Go Review
`)

	result, err := NewSearchSkillsTool().Execute(context.Background(), ExecutionContext{
		SkillDirs: []string{skillsDir},
	}, []byte(`{"query":"review go"}`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result.Output, "load before: go-testing") {
		t.Fatalf("search output missing before suggestion: %q", result.Output)
	}
	if !strings.Contains(result.Output, "consider after: polish") {
		t.Fatalf("search output missing after suggestion: %q", result.Output)
	}
}

func writeSkillFixture(t *testing.T, dir, content string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func largeSkillContent() string {
	var sb strings.Builder
	sb.WriteString("---\nname: go-migrations\ndescription: Go migration guidance\n---\n")
	sb.WriteString("# Overview\n")
	sb.WriteString(strings.Repeat("overview line\n", 80))
	sb.WriteString("# Usage\n")
	sb.WriteString("Use this skill when changing migration runners.\n")
	sb.WriteString(strings.Repeat("usage line\n", 40))
	sb.WriteString("# Reference\n")
	sb.WriteString(strings.Repeat("reference line\n", 20))
	return sb.String()
}
