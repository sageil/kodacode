package search

import (
	"bufio"
	"os"
	"path"
	"path/filepath"
	"strings"
)

const gitignoreFileName = ".gitignore"

type gitignoreMatcher struct {
	root       string
	rules      []gitignoreRule
	loadedDirs map[string]struct{}
}

type gitignoreRule struct {
	baseRel       string
	pattern       string
	negated       bool
	directoryOnly bool
	anchored      bool
	hasSlash      bool
}

func newGitignoreMatcher(root string) *gitignoreMatcher {
	root = strings.TrimSpace(root)
	if root == "" {
		return nil
	}
	abs, err := filepath.Abs(root)
	if err == nil {
		root = abs
	}
	return &gitignoreMatcher{
		root:       filepath.Clean(root),
		loadedDirs: map[string]struct{}{},
	}
}

func (m *gitignoreMatcher) loadDir(dir string) error {
	if m == nil {
		return nil
	}
	if abs, err := filepath.Abs(dir); err == nil {
		dir = abs
	}
	dir = filepath.Clean(dir)
	if !pathWithinRoot(m.root, dir) {
		return nil
	}
	if _, ok := m.loadedDirs[dir]; ok {
		return nil
	}
	m.loadedDirs[dir] = struct{}{}

	file, err := os.Open(filepath.Join(dir, gitignoreFileName))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	defer file.Close() //nolint:errcheck

	baseRel := relSlashPath(m.root, dir)
	if baseRel == "." {
		baseRel = ""
	}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		rule, ok := parseGitignoreRule(baseRel, scanner.Text())
		if ok {
			m.rules = append(m.rules, rule)
		}
	}
	return scanner.Err()
}

func (m *gitignoreMatcher) ignored(target string, isDir bool) bool {
	if m == nil {
		return false
	}
	if abs, err := filepath.Abs(target); err == nil {
		target = abs
	}
	target = filepath.Clean(target)
	if !pathWithinRoot(m.root, target) || target == m.root {
		return false
	}
	rel := relSlashPath(m.root, target)
	if rel == "." || strings.HasPrefix(rel, "../") {
		return false
	}

	ignored := false
	for _, rule := range m.rules {
		if rule.matches(rel, isDir) {
			ignored = !rule.negated
		}
	}
	return ignored
}

func parseGitignoreRule(baseRel, line string) (gitignoreRule, bool) {
	line = strings.TrimRight(line, " \t\r")
	if line == "" {
		return gitignoreRule{}, false
	}
	if strings.HasPrefix(line, `\#`) {
		line = line[1:]
	} else if strings.HasPrefix(line, "#") {
		return gitignoreRule{}, false
	}

	negated := false
	if strings.HasPrefix(line, `\!`) {
		line = line[1:]
	} else if strings.HasPrefix(line, "!") {
		negated = true
		line = line[1:]
	}
	line = strings.TrimRight(line, " \t\r")
	if line == "" {
		return gitignoreRule{}, false
	}

	anchored := strings.HasPrefix(line, "/")
	directoryOnly := strings.HasSuffix(line, "/")
	line = strings.Trim(line, "/")
	if line == "" {
		return gitignoreRule{}, false
	}
	line = path.Clean(filepath.ToSlash(line))
	if line == "." {
		return gitignoreRule{}, false
	}

	return gitignoreRule{
		baseRel:       strings.Trim(path.Clean(filepath.ToSlash(baseRel)), "/"),
		pattern:       line,
		negated:       negated,
		directoryOnly: directoryOnly,
		anchored:      anchored,
		hasSlash:      strings.Contains(line, "/"),
	}, true
}

func (r gitignoreRule) matches(rel string, isDir bool) bool {
	target, ok := relWithinBase(rel, r.baseRel)
	if !ok || target == "." || target == "" {
		return false
	}
	if r.directoryOnly {
		if isDir && r.matchesTarget(target) {
			return true
		}
		for _, ancestor := range ancestorDirs(target) {
			if r.matchesTarget(ancestor) {
				return true
			}
		}
		return false
	}
	return r.matchesTarget(target)
}

func (r gitignoreRule) matchesTarget(target string) bool {
	target = strings.Trim(target, "/")
	if target == "" {
		return false
	}
	if r.hasSlash || r.anchored {
		return matchGitignorePath(r.pattern, target)
	}
	for _, segment := range strings.Split(target, "/") {
		if matchGitignoreSegment(r.pattern, segment) {
			return true
		}
	}
	return false
}

func relWithinBase(rel, base string) (string, bool) {
	rel = strings.Trim(path.Clean(filepath.ToSlash(rel)), "/")
	base = strings.Trim(path.Clean(filepath.ToSlash(base)), "/")
	if base == "." {
		base = ""
	}
	if base == "" {
		return rel, true
	}
	if rel == base {
		return ".", true
	}
	prefix := base + "/"
	if !strings.HasPrefix(rel, prefix) {
		return "", false
	}
	return strings.TrimPrefix(rel, prefix), true
}

func ancestorDirs(target string) []string {
	parts := strings.Split(strings.Trim(target, "/"), "/")
	if len(parts) <= 1 {
		return nil
	}
	out := make([]string, 0, len(parts)-1)
	for idx := 1; idx < len(parts); idx++ {
		out = append(out, strings.Join(parts[:idx], "/"))
	}
	return out
}

func matchGitignorePath(pattern, target string) bool {
	patternParts := strings.Split(strings.Trim(pattern, "/"), "/")
	targetParts := strings.Split(strings.Trim(target, "/"), "/")
	return matchGitignorePathParts(patternParts, targetParts, map[[2]int]bool{})
}

func matchGitignorePathParts(patternParts, targetParts []string, memo map[[2]int]bool) bool {
	key := [2]int{len(patternParts), len(targetParts)}
	if failed := memo[key]; failed {
		return false
	}
	if len(patternParts) == 0 {
		return len(targetParts) == 0
	}
	if patternParts[0] == "**" {
		if matchGitignorePathParts(patternParts[1:], targetParts, memo) {
			return true
		}
		for idx := range targetParts {
			if matchGitignorePathParts(patternParts[1:], targetParts[idx+1:], memo) {
				return true
			}
		}
		memo[key] = true
		return false
	}
	if len(targetParts) == 0 || !matchGitignoreSegment(patternParts[0], targetParts[0]) {
		memo[key] = true
		return false
	}
	if matchGitignorePathParts(patternParts[1:], targetParts[1:], memo) {
		return true
	}
	memo[key] = true
	return false
}

func matchGitignoreSegment(pattern, segment string) bool {
	pattern = strings.ReplaceAll(pattern, "**", "*")
	ok, err := path.Match(pattern, segment)
	return err == nil && ok
}

func pathWithinRoot(root, target string) bool {
	root = filepath.Clean(root)
	target = filepath.Clean(target)
	if target == root {
		return true
	}
	rel, err := filepath.Rel(root, target)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func relSlashPath(root, target string) string {
	rel, err := filepath.Rel(root, target)
	if err != nil {
		return filepath.ToSlash(filepath.Base(target))
	}
	return filepath.ToSlash(rel)
}

func gitignoreRoot(rootPath, workspaceRoot string) string {
	root := strings.TrimSpace(workspaceRoot)
	target := strings.TrimSpace(rootPath)
	if root != "" && target != "" {
		absRoot, rootErr := filepath.Abs(root)
		absTarget, targetErr := filepath.Abs(target)
		if rootErr == nil && targetErr == nil && pathWithinRoot(absRoot, absTarget) {
			return absRoot
		}
	}
	if target == "" {
		return root
	}
	info, err := os.Stat(target)
	if err == nil && !info.IsDir() {
		return filepath.Dir(target)
	}
	return target
}

func newRequestGitignoreMatcher(req Request) *gitignoreMatcher {
	return newGitignoreMatcher(gitignoreRoot(req.RootPath, req.WorkspaceRoot))
}
