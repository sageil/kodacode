package tool

import "errors"

var ErrSkillCatalogRequired = errors.New("skill catalog is required")

type SkillMatch struct {
	ID          string   `json:"id"`
	Description string   `json:"description,omitempty"`
	Source      string   `json:"source,omitempty"`
	Path        string   `json:"path,omitempty"`
	Reasons     []string `json:"reasons,omitempty"`
}

type SkillSection struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

type SkillDocument struct {
	ID          string         `json:"id"`
	Description string         `json:"description,omitempty"`
	Source      string         `json:"source,omitempty"`
	Path        string         `json:"path,omitempty"`
	Content     string         `json:"content,omitempty"`
	Sections    []SkillSection `json:"sections,omitempty"`
}

type SkillCatalog interface {
	SearchSkills(query string, limit int) ([]SkillMatch, error)
	LoadSkill(id string) (SkillDocument, error)
}
