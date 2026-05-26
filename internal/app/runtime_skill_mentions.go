package app

import (
	"regexp"
	"strings"

	"github.com/sageil/kodacode/internal/skill"
)

var skillMentionPattern = regexp.MustCompile(`\$\{([A-Za-z0-9][A-Za-z0-9_.-]*)\}|\$([A-Za-z0-9][A-Za-z0-9_.-]*)`)

func skillIDsForTurn(userText string, requested []string, available []skill.Definition) []string {
	out := make([]string, 0, len(requested))
	for _, id := range requested {
		if id = strings.TrimSpace(id); id != "" {
			out = appendUniqueValue(out, id)
		}
	}
	if strings.TrimSpace(userText) == "" || len(available) == 0 {
		return out
	}

	known := make(map[string]string, len(available))
	for _, definition := range available {
		id := strings.TrimSpace(definition.ID)
		if id == "" {
			continue
		}
		known[strings.ToLower(id)] = id
	}
	for _, match := range skillMentionPattern.FindAllStringSubmatch(userText, -1) {
		candidate := strings.TrimSpace(match[1])
		if candidate == "" {
			candidate = strings.TrimSpace(match[2])
		}
		if candidate == "" {
			continue
		}
		if id, ok := known[strings.ToLower(candidate)]; ok {
			out = appendUniqueValue(out, id)
		}
	}
	return out
}
