package permissionpolicy

import (
	"path/filepath"
	"strings"
)

func (c Config) MatchExternalDirectory(path string) (Action, bool) {
	return c.ExternalDirectory.Match(normalizePathSubject(path))
}

func (c Config) MatchBash(command string) (Action, bool) {
	return c.Bash.Match(normalizeTextSubject(command))
}

func (c Config) MatchWebFetch(rawURL string) (Action, bool) {
	return c.WebFetch.Match(normalizeTextSubject(rawURL))
}

func (c Config) MatchNetworkTarget(target string) (Action, bool) {
	return c.NetworkTarget.Match(normalizeLowerSubject(target))
}

func (s SubjectRules) Match(subject string) (Action, bool) {
	subject = normalizeTextSubject(subject)
	if subject == "" {
		return "", false
	}
	var (
		action Action
		ok     bool
	)
	for _, rule := range s {
		pattern := normalizeTextSubject(rule.Pattern)
		if pattern == "" {
			continue
		}
		if wildcardMatch(pattern, subject) {
			action = rule.Action
			ok = true
		}
	}
	return action, ok
}

func normalizePathSubject(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	return filepath.Clean(path)
}

func normalizeTextSubject(value string) string {
	return strings.TrimSpace(value)
}

func normalizeLowerSubject(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func wildcardMatch(pattern, subject string) bool {
	patternRunes := []rune(pattern)
	subjectRunes := []rune(subject)
	patternIndex := 0
	subjectIndex := 0
	starIndex := -1
	matchIndex := 0

	for subjectIndex < len(subjectRunes) {
		if patternIndex < len(patternRunes) && (patternRunes[patternIndex] == '?' || patternRunes[patternIndex] == subjectRunes[subjectIndex]) {
			patternIndex++
			subjectIndex++
			continue
		}
		if patternIndex < len(patternRunes) && patternRunes[patternIndex] == '*' {
			starIndex = patternIndex
			matchIndex = subjectIndex
			patternIndex++
			continue
		}
		if starIndex >= 0 {
			patternIndex = starIndex + 1
			matchIndex++
			subjectIndex = matchIndex
			continue
		}
		return false
	}

	for patternIndex < len(patternRunes) && patternRunes[patternIndex] == '*' {
		patternIndex++
	}
	return patternIndex == len(patternRunes)
}
