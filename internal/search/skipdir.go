package search

import "strings"

var defaultSearchSkipDirs = []string{".git", "node_modules", "vendor"}

type skipDirMatcher map[string]struct{}

func newSkipDirMatcher(extra []string) skipDirMatcher {
	names := make([]string, 0, len(defaultSearchSkipDirs)+len(extra))
	names = append(names, defaultSearchSkipDirs...)
	names = append(names, extra...)
	matcher := make(skipDirMatcher, len(names))
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		matcher[name] = struct{}{}
	}
	return matcher
}

func defaultSkipDirMatcher() skipDirMatcher {
	return newSkipDirMatcher(nil)
}

func (m skipDirMatcher) shouldSkip(name string) bool {
	name = strings.TrimSpace(name)
	if name == "" {
		return false
	}
	_, ok := m[name]
	return ok
}
