package app

import (
	"context"
	"slices"
	"strings"

	"github.com/sageil/kodacode/internal/prompt"
)

type AvailableSkill struct {
	ID          string
	Description string
	WhenToUse   string
	Source      string
}

func (r *Runtime) ListSkills(_ context.Context, workspaceRoot string) ([]AvailableSkill, error) {
	if r == nil || r.Skills == nil {
		return nil, nil
	}
	catalog, err := r.Skills.Catalog(workspaceRoot)
	if err != nil {
		return nil, err
	}
	if len(catalog) == 0 {
		return nil, nil
	}
	skills := make([]AvailableSkill, 0, len(catalog))
	for _, definition := range catalog {
		id := strings.TrimSpace(definition.ID)
		if id == "" {
			continue
		}
		if !definition.EffectiveUserInvocable() {
			continue
		}
		skills = append(skills, AvailableSkill{
			ID:          id,
			Description: strings.TrimSpace(definition.Description),
			WhenToUse:   strings.TrimSpace(definition.WhenToUse),
			Source:      string(definition.Source),
		})
	}
	slices.SortFunc(skills, func(a, b AvailableSkill) int {
		if rank := availableSkillSourceRank(a.Source) - availableSkillSourceRank(b.Source); rank != 0 {
			return rank
		}
		if a.ID != b.ID {
			if a.ID < b.ID {
				return -1
			}
			return 1
		}
		if a.Description < b.Description {
			return -1
		}
		if a.Description > b.Description {
			return 1
		}
		return 0
	})
	return skills, nil
}

func availableSkillSourceRank(source string) int {
	switch prompt.Source(strings.TrimSpace(source)) {
	case prompt.SourceProject:
		return 0
	case prompt.SourceGlobal:
		return 1
	default:
		return 2
	}
}
