package tui

import (
	"encoding/json"
	"strings"

	"github.com/sageil/kodacode/internal/events"
)

type searchSkillsToolOutput struct {
	Matches []struct {
		ID          string   `json:"id"`
		Description string   `json:"description"`
		Source      string   `json:"source"`
		Reasons     []string `json:"reasons"`
	} `json:"matches"`
}

type skillToolOutput struct {
	Mode  string `json:"mode"`
	Skill struct {
		ID          string `json:"id"`
		Description string `json:"description"`
		Source      string `json:"source"`
		Content     string `json:"content"`
		Sections    []struct {
			ID    string `json:"id"`
			Title string `json:"title"`
		} `json:"sections"`
	} `json:"skill"`
	Section struct {
		ID    string `json:"id"`
		Title string `json:"title"`
	} `json:"section"`
	Content string `json:"content"`
}

func renderSearchSkillsToolDetailMarkdown(call *events.ToolCallState, rawOutput string) (string, bool) {
	if call == nil {
		return "", false
	}
	input, ok := parseSearchSkillsToolViewInput(call.Input)
	if !ok {
		return "", false
	}
	var output searchSkillsToolOutput
	if strings.TrimSpace(rawOutput) == "" || json.Unmarshal([]byte(rawOutput), &output) != nil {
		return "", false
	}

	lines := []string{"Query: `" + input.Query + "`"}
	if input.Limit > 0 {
		lines = append(lines, "Limit: "+intToString(input.Limit))
	}
	lines = append(lines, "", "## Matches")
	if len(output.Matches) == 0 {
		lines = append(lines, "_No skills matched._")
		return strings.Join(lines, "\n"), true
	}
	for _, match := range output.Matches {
		label := "- `" + strings.TrimSpace(match.ID) + "`"
		if desc := strings.TrimSpace(match.Description); desc != "" {
			label += ": " + desc
		}
		lines = append(lines, label)
		meta := make([]string, 0, 3)
		if source := strings.TrimSpace(match.Source); source != "" {
			meta = append(meta, "source: "+source)
		}
		if len(match.Reasons) > 0 {
			meta = append(meta, "reasons: "+strings.Join(compactNonBlank(match.Reasons), "; "))
		}
		if len(meta) > 0 {
			lines = append(lines, "  "+strings.Join(meta, " • "))
		}
	}
	return strings.Join(lines, "\n"), true
}

func renderSkillToolDetailMarkdown(call *events.ToolCallState, rawOutput string) (string, bool) {
	if call == nil {
		return "", false
	}
	input, ok := parseSkillToolViewInput(call.Input)
	if !ok {
		return "", false
	}
	var output skillToolOutput
	if strings.TrimSpace(rawOutput) == "" || json.Unmarshal([]byte(rawOutput), &output) != nil {
		return "", false
	}

	skillID := strings.TrimSpace(output.Skill.ID)
	if skillID == "" {
		skillID = input.ID
	}
	lines := []string{"Skill: `" + skillID + "`"}
	if section := strings.TrimSpace(input.Section); section != "" && strings.TrimSpace(output.Section.ID) == "" {
		lines = append(lines, "Requested section: `"+section+"`")
	}
	if desc := strings.TrimSpace(output.Skill.Description); desc != "" {
		lines = append(lines, "Description: "+desc)
	}
	if source := strings.TrimSpace(output.Skill.Source); source != "" {
		lines = append(lines, "Source: "+source)
	}

	switch strings.TrimSpace(output.Mode) {
	case "toc":
		lines = append(lines, "", "## Sections")
		if len(output.Skill.Sections) == 0 {
			lines = append(lines, "_No sections._")
			return strings.Join(lines, "\n"), true
		}
		for _, section := range output.Skill.Sections {
			label := "- `" + strings.TrimSpace(section.ID) + "`"
			if title := strings.TrimSpace(section.Title); title != "" {
				label += ": " + title
			}
			lines = append(lines, label)
		}
		return strings.Join(lines, "\n"), true
	case "section":
		title := strings.TrimSpace(output.Section.Title)
		sectionID := strings.TrimSpace(output.Section.ID)
		if title != "" || sectionID != "" {
			lines = append(lines, "")
			if title != "" && sectionID != "" {
				lines = append(lines, "## Section: "+title+" (`"+sectionID+"`)")
			} else if title != "" {
				lines = append(lines, "## Section: "+title)
			} else {
				lines = append(lines, "## Section: `"+sectionID+"`")
			}
		}
		content := strings.TrimSpace(output.Content)
		if content != "" {
			lines = append(lines, "", content)
		}
		return strings.Join(lines, "\n"), true
	case "full":
		content := strings.TrimSpace(output.Skill.Content)
		if content != "" {
			lines = append(lines, "", content)
		}
		return strings.Join(lines, "\n"), true
	default:
		return "", false
	}
}

func compactNonBlank(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		out = append(out, value)
	}
	return out
}
