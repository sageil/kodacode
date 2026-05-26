package app

import (
	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/skill"
	"github.com/sageil/kodacode/internal/tool"
)

func (e *ToolExecutor) toolSkillCatalog(state events.SessionState) tool.SkillCatalog {
	if e.skills == nil {
		return nil
	}
	return sessionSkillCatalog{
		workspaceRoot: state.WorkspaceRoot,
		registry:      e.skills,
	}
}

type sessionSkillCatalog struct {
	workspaceRoot string
	registry      *skill.Registry
}

func (c sessionSkillCatalog) SearchSkills(query string, limit int) ([]tool.SkillMatch, error) {
	matches, err := c.registry.Search(c.workspaceRoot, query, limit)
	if err != nil {
		return nil, err
	}
	out := make([]tool.SkillMatch, 0, len(matches))
	for _, match := range matches {
		out = append(out, tool.SkillMatch{
			ID:          match.Definition.ID,
			Description: match.Definition.Description,
			Source:      string(match.Definition.Source),
			Path:        match.Definition.Path,
			Reasons:     append([]string(nil), match.Reasons...),
		})
	}
	return out, nil
}

func (c sessionSkillCatalog) LoadSkill(id string) (tool.SkillDocument, error) {
	definition, err := c.registry.Get(c.workspaceRoot, id)
	if err != nil {
		return tool.SkillDocument{}, err
	}
	return tool.SkillDocument{
		ID:          definition.ID,
		Description: definition.Description,
		Source:      string(definition.Source),
		Path:        definition.Path,
		Content:     definition.Prompt,
	}, nil
}
