package tool

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

type stubSkillCatalog struct {
	matches  []SkillMatch
	document SkillDocument
	onSearch func(query string, limit int)
}

func (s stubSkillCatalog) SearchSkills(query string, limit int) ([]SkillMatch, error) {
	if s.onSearch != nil {
		s.onSearch(query, limit)
	}
	return s.matches, nil
}

func (s stubSkillCatalog) LoadSkill(id string) (SkillDocument, error) {
	return s.document, nil
}

func TestSearchSkillsToolReturnsStructuredMatches(t *testing.T) {
	result, err := NewSearchSkillsTool().Execute(context.Background(), ExecutionContext{
		SkillCatalog: stubSkillCatalog{
			matches: []SkillMatch{{
				ID:          "migration",
				Description: "Mongo migration workflow",
				Source:      "project",
			}},
		},
	}, json.RawMessage(`{"query":"mongo migration","limit":5}`))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !containsAll(result.Output, `"matches":[`, `"migration"`, `"project"`) {
		t.Fatalf("output = %q", result.Output)
	}
}

func TestSearchSkillsToolAcceptsStringLimit(t *testing.T) {
	capturedLimit := 0
	_, err := NewSearchSkillsTool().Execute(context.Background(), ExecutionContext{
		SkillCatalog: stubSkillCatalog{
			onSearch: func(_ string, limit int) {
				capturedLimit = limit
			},
		},
	}, json.RawMessage(`{"query":"mongo migration","limit":"7"}`))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if capturedLimit != 7 {
		t.Fatalf("captured limit = %d, want 7", capturedLimit)
	}
}

func TestSkillToolReturnsTOCForLargeSectionedSkill(t *testing.T) {
	content := strings.Join([]string{
		"# Overview",
		"Use this skill when changing migrations.",
		"",
		"## Checklist",
		"- inspect indexes",
		"- verify rollout",
		"",
		"## Validation",
		strings.Repeat("detailed guidance\n", 120),
	}, "\n")

	result, err := NewSkillTool().Execute(context.Background(), ExecutionContext{
		SkillCatalog: stubSkillCatalog{
			document: SkillDocument{
				ID:      "migration",
				Source:  "project",
				Content: content,
			},
		},
	}, json.RawMessage(`{"id":"migration","section":null}`))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !containsAll(result.Output, `"mode":"toc"`, `"sections":[`, `"overview"`, `"checklist"`) {
		t.Fatalf("output = %q", result.Output)
	}
}

func TestSkillToolReturnsRequestedSection(t *testing.T) {
	content := strings.Join([]string{
		"# Overview",
		"Use this skill when changing migrations.",
		"",
		"## Checklist",
		"- inspect indexes",
		"- verify rollout",
	}, "\n")

	result, err := NewSkillTool().Execute(context.Background(), ExecutionContext{
		SkillCatalog: stubSkillCatalog{
			document: SkillDocument{
				ID:      "migration",
				Source:  "project",
				Content: content,
			},
		},
	}, json.RawMessage(`{"id":"migration","section":"checklist"}`))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !containsAll(result.Output, `"mode":"section"`, `"checklist"`, `- inspect indexes`) {
		t.Fatalf("output = %q", result.Output)
	}
}

func TestSkillToolRequiresSkillCatalog(t *testing.T) {
	_, err := NewSkillTool().Execute(context.Background(), ExecutionContext{}, json.RawMessage(`{"id":"migration","section":null}`))
	if !errors.Is(err, ErrSkillCatalogRequired) {
		t.Fatalf("Execute() error = %v, want ErrSkillCatalogRequired", err)
	}
}
