package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

const SkillToolName = "skill"

type SkillTool struct{}

func NewSkillTool() SkillTool {
	return SkillTool{}
}

func (SkillTool) Definition() Definition {
	return Definition{
		Name:                SkillToolName,
		Description:         "Load a skill's instructions by id. For large skills, the initial call may return a section table of contents so you can load only the relevant section next.",
		ProviderDescription: "Load a skill by id. Large skills may return a section table of contents first.",
		InputSchema:         json.RawMessage(`{"type":"object","properties":{"id":{"type":"string","description":"Skill identifier to load."},"section":{"type":["string","null"],"description":"Optional section id to load from the skill. Use null or omit this field to load the top-level skill."}},"required":["id"],"additionalProperties":false}`),
		ArgumentExamples:    []string{`{"id":"openai-docs","section":null}`},
	}
}

func (SkillTool) Execute(_ context.Context, ectx ExecutionContext, args json.RawMessage) (Result, error) {
	catalog, err := ectx.Skills()
	if err != nil {
		return Result{}, err
	}
	input, err := parseSkillToolInput(args)
	if err != nil {
		return Result{}, err
	}
	document, err := catalog.LoadSkill(input.ID)
	if err != nil {
		return Result{}, err
	}

	sections := parseSkillSections(document.Content)
	document.Sections = sectionsToOutput(sections)
	if input.Section != "" {
		section, ok := findSkillSection(sections, input.Section)
		if !ok {
			return Result{}, fmt.Errorf("skill %q does not have section %q", input.ID, input.Section)
		}
		return Result{Output: skillSectionOutput(document, section)}, nil
	}
	if shouldReturnSkillTOC(document.Content, sections) {
		return Result{Output: skillTOCOutput(document)}, nil
	}
	return Result{Output: skillDocumentOutput(document)}, nil
}

type skillToolInput struct {
	ID      string
	Section string
}

func parseSkillToolInput(args json.RawMessage) (_ skillToolInput, err error) {
	defer func() {
		err = normalizeToolInputError(SkillToolName, err)
	}()
	var raw struct {
		ID      string  `json:"id"`
		Section *string `json:"section"`
	}
	if err := DecodeArgs(SkillToolName, args, &raw); err != nil {
		return skillToolInput{}, err
	}
	id := strings.TrimSpace(raw.ID)
	if id == "" {
		return skillToolInput{}, fmt.Errorf("id is required")
	}
	return skillToolInput{
		ID:      id,
		Section: strings.TrimSpace(stringValue(raw.Section)),
	}, nil
}

func skillDocumentOutput(document SkillDocument) string {
	payload, err := json.Marshal(struct {
		Mode  string        `json:"mode"`
		Skill SkillDocument `json:"skill"`
	}{
		Mode:  "full",
		Skill: document,
	})
	if err != nil {
		return `{"mode":"full","skill":{}}`
	}
	return string(payload)
}

func skillTOCOutput(document SkillDocument) string {
	document.Content = ""
	payload, err := json.Marshal(struct {
		Mode  string        `json:"mode"`
		Skill SkillDocument `json:"skill"`
	}{
		Mode:  "toc",
		Skill: document,
	})
	if err != nil {
		return `{"mode":"toc","skill":{}}`
	}
	return string(payload)
}

func skillSectionOutput(document SkillDocument, section parsedSkillSection) string {
	document.Content = ""
	document.Sections = nil
	payload, err := json.Marshal(struct {
		Mode    string        `json:"mode"`
		Skill   SkillDocument `json:"skill"`
		Section SkillSection  `json:"section"`
		Content string        `json:"content"`
	}{
		Mode:  "section",
		Skill: document,
		Section: SkillSection{
			ID:    section.ID,
			Title: section.Title,
		},
		Content: section.Content,
	})
	if err != nil {
		return `{"mode":"section","skill":{},"section":{},"content":""}`
	}
	return string(payload)
}
