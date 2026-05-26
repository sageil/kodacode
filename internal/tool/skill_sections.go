package tool

import (
	"regexp"
	"strings"
	"unicode"
)

const largeSkillContentChars = 1600
const largeSkillContentLines = 60

type parsedSkillSection struct {
	ID      string
	Title   string
	Content string
}

var skillHeadingPattern = regexp.MustCompile(`^(#{1,6})\s+(.+?)\s*$`)

func parseSkillSections(content string) []parsedSkillSection {
	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
	sections := make([]parsedSkillSection, 0, 8)
	current := -1
	for _, line := range lines {
		matches := skillHeadingPattern.FindStringSubmatch(line)
		if len(matches) == 3 {
			title := strings.TrimSpace(matches[2])
			current = len(sections)
			sections = append(sections, parsedSkillSection{
				ID:    slugifySkillHeading(title),
				Title: title,
			})
			continue
		}
		if current < 0 {
			continue
		}
		section := &sections[current]
		if strings.TrimSpace(section.Content) == "" {
			section.Content = strings.TrimRight(line, "\n")
			continue
		}
		section.Content += "\n" + strings.TrimRight(line, "\n")
	}
	for i := range sections {
		sections[i].Content = strings.TrimSpace(sections[i].Content)
	}
	return sections
}

func sectionsToOutput(sections []parsedSkillSection) []SkillSection {
	if len(sections) == 0 {
		return nil
	}
	out := make([]SkillSection, 0, len(sections))
	for _, section := range sections {
		if section.ID == "" || section.Title == "" {
			continue
		}
		out = append(out, SkillSection{ID: section.ID, Title: section.Title})
	}
	return out
}

func findSkillSection(sections []parsedSkillSection, id string) (parsedSkillSection, bool) {
	normalized := slugifySkillHeading(id)
	for _, section := range sections {
		if section.ID == normalized {
			return section, true
		}
	}
	return parsedSkillSection{}, false
}

func shouldReturnSkillTOC(content string, sections []parsedSkillSection) bool {
	if len(sections) == 0 {
		return false
	}
	if len(content) > largeSkillContentChars {
		return true
	}
	return strings.Count(content, "\n") > largeSkillContentLines
}

func slugifySkillHeading(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return ""
	}
	var b strings.Builder
	lastDash := false
	for _, r := range value {
		switch {
		case unicode.IsLetter(r), unicode.IsDigit(r):
			b.WriteRune(r)
			lastDash = false
		case !lastDash:
			b.WriteByte('-')
			lastDash = true
		}
	}
	return strings.Trim(b.String(), "-")
}
