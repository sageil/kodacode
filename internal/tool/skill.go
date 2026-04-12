package tool

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/sageil/kodacode/v1/internal/skills"
)

var skillParams = []byte(`{
	"type": "object",
	"properties": {
		"name": {
			"type": "string",
			"description": "The skill name to load (e.g. 'go-testing', 'code-review')"
		},
		"section": {
			"type": "string",
			"description": "Optional section ID to load from the skill (use the top-level skill call to inspect available sections first)"
		}
	},
	"required": ["name"]
}`)

var searchSkillsParams = []byte(`{
	"type": "object",
	"properties": {
		"query": {
			"type": "string",
			"description": "What kind of skill you need (topic, task, or phrase)"
		},
		"limit": {
			"type": "integer",
			"description": "Maximum number of matches to return (default 5)"
		}
	},
	"required": ["query"]
}`)

// NewSkillTool returns a tool that loads skill content by name.
// The model calls this instead of guessing file paths.
func NewSkillTool() *Tool {
	return &Tool{
		Name:        "skill",
		ReadOnly:    true,
		Description: "Load a skill's instructions by name. For large skills, the initial call may return sections first so you can load only the relevant section.",
		Parameters:  skillParams,
		Execute:     executeSkill,
	}
}

func NewSearchSkillsTool() *Tool {
	return &Tool{
		Name:        "search_skills",
		ReadOnly:    true,
		Description: "Search available skills by topic, task, description, trigger phrases, and section titles. Use this before loading a skill when you are not sure which one is relevant.",
		Parameters:  searchSkillsParams,
		Execute:     executeSearchSkills,
	}
}

func executeSkill(ctx context.Context, ectx ExecutionContext, args []byte) (*Result, error) {
	var params struct {
		Name    string `json:"name"`
		Section string `json:"section"`
	}
	if err := flexUnmarshal(args, &params); err != nil {
		return nil, fmt.Errorf("skill: invalid arguments: %w", err)
	}
	if params.Name == "" {
		return ErrorResult(ErrCodeInvalidArgs, "skill: name is required", false), nil
	}

	// Sanitize: prevent path traversal.
	name := filepath.Base(params.Name)
	idx := skills.LoadIndex(ectx.SkillDirs)
	skill, content, err := skills.LoadSkill(idx, name, params.Section, ectx.SkillPolicy)
	if err != nil {
		var available []string
		for _, item := range idx.Filter(ectx.SkillPolicy) {
			available = append(available, item.Name)
		}
		sort.Strings(available)
		return ErrorResult(ErrCodeNotFound, fmt.Sprintf("Skill %q not found. Available skills: %s", name, strings.Join(available, ", ")), true), nil
	}

	if params.Section == "" && shouldReturnSkillTOC(content, skill.Sections) {
		return &Result{
			Title:  fmt.Sprintf("skill: %s", name),
			Output: formatSkillTOC(skill),
		}, nil
	}

	return &Result{
		Title:  fmt.Sprintf("skill: %s", name),
		Output: content,
	}, nil
}

func executeSearchSkills(ctx context.Context, ectx ExecutionContext, args []byte) (*Result, error) {
	var params struct {
		Query string `json:"query"`
		Limit int    `json:"limit"`
	}
	if err := flexUnmarshal(args, &params); err != nil {
		return nil, fmt.Errorf("search_skills: invalid arguments: %w", err)
	}
	if strings.TrimSpace(params.Query) == "" {
		return ErrorResult(ErrCodeInvalidArgs, "search_skills: query is required", false), nil
	}

	idx := skills.LoadIndex(ectx.SkillDirs)
	matches := idx.Search(params.Query, ectx.SkillPolicy, params.Limit)
	if len(matches) == 0 {
		return ErrorResult(ErrCodeNotFound, fmt.Sprintf("No skills matched %q.", params.Query), true), nil
	}
	return &Result{
		Title:  fmt.Sprintf("search_skills: %s", params.Query),
		Output: formatSkillMatches(matches),
	}, nil
}

func shouldReturnSkillTOC(content string, sections []skills.Section) bool {
	if len(sections) == 0 {
		return false
	}
	if len(content) > 1600 {
		return true
	}
	return strings.Count(content, "\n") > 60
}

func formatSkillTOC(skill skills.Skill) string {
	var sb strings.Builder
	sb.WriteString("Skill: ")
	sb.WriteString(skill.Name)
	if skill.Description != "" {
		sb.WriteString("\nDescription: ")
		sb.WriteString(skill.Description)
	}
	sb.WriteString("\nSections:\n")
	for _, section := range skill.Sections {
		sb.WriteString("- ")
		sb.WriteString(section.ID)
		sb.WriteString(": ")
		sb.WriteString(section.Title)
		sb.WriteByte('\n')
	}
	if len(skill.Suggests.Before) > 0 {
		sb.WriteString("Suggested before: ")
		sb.WriteString(strings.Join(skill.Suggests.Before, ", "))
		sb.WriteByte('\n')
	}
	if len(skill.Suggests.After) > 0 {
		sb.WriteString("Suggested after: ")
		sb.WriteString(strings.Join(skill.Suggests.After, ", "))
		sb.WriteByte('\n')
	}
	sb.WriteString("Load a section with skill({\"name\":\"")
	sb.WriteString(skill.Name)
	sb.WriteString("\",\"section\":\"SECTION_ID\"}).")
	return strings.TrimSpace(sb.String())
}

func formatSkillMatches(matches []skills.Match) string {
	var sb strings.Builder
	sb.WriteString("Matching skills:\n")
	for _, match := range matches {
		sb.WriteString("- ")
		sb.WriteString(match.Skill.Name)
		if match.Skill.Description != "" {
			sb.WriteString(": ")
			sb.WriteString(match.Skill.Description)
		}
		if len(match.Reasons) > 0 {
			sb.WriteString(" [")
			sb.WriteString(strings.Join(match.Reasons, ", "))
			sb.WriteString("]")
		}
		if len(match.Skill.Sections) > 0 {
			sb.WriteString("\n  sections: ")
			names := make([]string, 0, len(match.Skill.Sections))
			for _, section := range match.Skill.Sections {
				names = append(names, section.ID)
			}
			sb.WriteString(strings.Join(names, ", "))
		}
		if len(match.Skill.Suggests.Before) > 0 {
			sb.WriteString("\n  load before: ")
			sb.WriteString(strings.Join(match.Skill.Suggests.Before, ", "))
		}
		if len(match.Skill.Suggests.After) > 0 {
			sb.WriteString("\n  consider after: ")
			sb.WriteString(strings.Join(match.Skill.Suggests.After, ", "))
		}
		sb.WriteByte('\n')
	}
	sb.WriteString("Load one with skill({\"name\":\"SKILL_NAME\"})")
	return strings.TrimSpace(sb.String())
}
