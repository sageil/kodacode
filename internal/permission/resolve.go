package permission

import (
	"path"
	"path/filepath"
	"strings"
)

// Defaults returns the built-in default permission Config, matching the
// OpenCode defaults:
//
//   - bash: allow
//   - read: allow except *.env and *.env.* → ask; *.env.example → allow
//   - edit: allow
//   - glob: allow
//   - external_directory: ask
func Defaults() Config {
	return Config{
		"bash": {Action: ActionAllow},
		"read": {
			Patterns: []Pattern{
				{Glob: "*", Action: ActionAllow},
				{Glob: "*.env", Action: ActionAsk},
				{Glob: "*.env.*", Action: ActionAsk},
				{Glob: "*.env.example", Action: ActionAllow},
			},
		},
		"write":              {Action: ActionAllow},
		"edit":               {Action: ActionAllow},
		"glob":               {Action: ActionAllow},
		"external_directory": {Action: ActionAsk},
	}
}

// Resolve returns the action for (toolName, keyArgument) according to cfg.
//
// If cfg has no rule for toolName, ActionAsk is returned.
// For a string-shorthand rule the action is returned directly.
// For a pattern-list rule, patterns are evaluated in order and the last
// matching pattern wins. If no pattern matches, ActionAsk is returned.
//
// Pattern matching:
//   - For "bash": the full command string is matched with matchGlobAny, which
//     allows '*' to span any character including '/'.
//   - For all other tools: path.Match is applied against path.Base(keyArgument).
func Resolve(cfg Config, toolName, keyArgument string) Action {
	if cfg == nil {
		return ActionAsk
	}
	rule, ok := cfg[toolName]
	if !ok {
		return ActionAsk
	}
	// String shorthand.
	if rule.Action != "" {
		return rule.Action
	}
	// Pattern list: last match wins.
	var last *Action
	for i := range rule.Patterns {
		p := &rule.Patterns[i]
		if matchPattern(toolName, p.Glob, keyArgument) {
			last = &p.Action
		}
	}
	if last != nil {
		return *last
	}
	return ActionAsk
}

// Merge combines base and overlay at the pattern level (base first, overlay
// appended). Last-match-wins gives overlay priority. String shorthand rules
// are expanded to [* → action] before concatenation.
func Merge(base, overlay Config) Config {
	result := make(Config, len(base)+len(overlay))
	for k, v := range base {
		cp := *v
		result[k] = &cp
	}
	for k, ov := range overlay {
		bv, exists := result[k]
		if !exists {
			cp := *ov
			result[k] = &cp
			continue
		}
		result[k] = mergeRules(bv, ov)
	}
	return result
}

func mergeRules(base, overlay *Rule) *Rule {
	basePatterns := rulePatterns(base)
	overlayPatterns := rulePatterns(overlay)
	merged := make([]Pattern, 0, len(basePatterns)+len(overlayPatterns))
	merged = append(merged, basePatterns...)
	merged = append(merged, overlayPatterns...)
	return &Rule{Patterns: merged}
}

func rulePatterns(r *Rule) []Pattern {
	if r.Action != "" {
		return []Pattern{{Glob: "*", Action: r.Action}}
	}
	return r.Patterns
}

// matchPattern returns true when pattern matches keyArgument for the given tool.
//
// For "bash", matchGlobAny is used (allows '*' to span '/').
// For all other tools, path.Match is applied against path.Base(keyArgument).
func matchPattern(toolName, pattern, keyArgument string) bool {
	if toolName == "bash" {
		return matchGlobAny(pattern, keyArgument)
	}
	for _, candidate := range pathMatchCandidates(keyArgument) {
		ok, err := path.Match(pattern, candidate)
		if err == nil && ok {
			return true
		}
	}
	return false
}

func pathMatchCandidates(keyArgument string) []string {
	normalized := filepath.ToSlash(filepath.Clean(keyArgument))
	if normalized == "." || normalized == "" {
		return []string{normalized}
	}

	seen := make(map[string]bool)
	var out []string
	appendCandidate := func(v string) {
		if v == "" || seen[v] {
			return
		}
		seen[v] = true
		out = append(out, v)
	}

	appendCandidate(normalized)
	appendCandidate(strings.TrimPrefix(normalized, "/"))

	trimmed := strings.TrimPrefix(normalized, "/")
	for {
		idx := strings.IndexByte(trimmed, '/')
		if idx < 0 {
			break
		}
		trimmed = trimmed[idx+1:]
		appendCandidate(trimmed)
	}

	appendCandidate(path.Base(normalized))
	return out
}

// matchGlobAny matches pattern against s where '*' matches any sequence of
// characters including path separators. Only '*' and '?' wildcards are
// supported; character classes '[...]' are not handled.
func matchGlobAny(pattern, s string) bool {
	pi, si := 0, 0
	starIdx, matchIdx := -1, 0

	for si < len(s) {
		if pi < len(pattern) && (pattern[pi] == '?' || pattern[pi] == s[si]) {
			pi++
			si++
		} else if pi < len(pattern) && pattern[pi] == '*' {
			starIdx = pi
			matchIdx = si
			pi++
		} else if starIdx != -1 {
			pi = starIdx + 1
			matchIdx++
			si = matchIdx
		} else {
			return false
		}
	}
	for pi < len(pattern) && pattern[pi] == '*' {
		pi++
	}
	return pi == len(pattern)
}
