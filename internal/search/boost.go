package search

import "strings"

type pathBoostConfig struct {
	enabled   bool
	penalties []pathBoostRule
	bonuses   []pathBoostRule
}

type pathBoostRule struct {
	pattern string
	factor  float64
}

func defaultPathBoostConfig() pathBoostConfig {
	return pathBoostConfig{
		enabled: true,
		penalties: []pathBoostRule{
			{pattern: "/tests/", factor: 0.50},
			{pattern: "/test/", factor: 0.50},
			{pattern: "__tests__", factor: 0.50},
			{pattern: "_test.", factor: 0.50},
			{pattern: ".spec.", factor: 0.50},
			{pattern: "/mocks/", factor: 0.40},
			{pattern: "/fixtures/", factor: 0.40},
			{pattern: "/testdata/", factor: 0.40},
			{pattern: "/generated/", factor: 0.40},
			{pattern: "/docs/", factor: 0.60},
			{pattern: ".md", factor: 0.60},
		},
		bonuses: []pathBoostRule{
			{pattern: "/src/", factor: 1.10},
			{pattern: "/lib/", factor: 1.10},
			{pattern: "/app/", factor: 1.10},
		},
	}
}

func normalizeSearchPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	return strings.ReplaceAll(path, "\\", "/")
}

func pathBoostMultiplier(path string, config pathBoostConfig) float64 {
	if !config.enabled {
		return 1
	}
	normalizedPath := normalizeSearchPath(path)
	if normalizedPath == "" {
		return 1
	}
	multiplier := 1.0
	for _, rule := range config.penalties {
		if pathBoostRuleMatches(normalizedPath, rule.pattern) {
			multiplier *= rule.factor
		}
	}
	for _, rule := range config.bonuses {
		if pathBoostRuleMatches(normalizedPath, rule.pattern) {
			multiplier *= rule.factor
		}
	}
	return multiplier
}

func pathBoostRuleMatches(normalizedPath, pattern string) bool {
	if normalizedPath == "" || pattern == "" {
		return false
	}
	if strings.HasPrefix(pattern, "/") && strings.HasSuffix(pattern, "/") && len(pattern) > 1 {
		padded := "/" + strings.Trim(normalizedPath, "/") + "/"
		return strings.Contains(padded, pattern)
	}
	if strings.HasSuffix(normalizedPath, pattern) {
		return true
	}
	return strings.Contains(normalizedPath, pattern)
}
