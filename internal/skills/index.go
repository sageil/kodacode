package skills

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"unicode"

	"github.com/bmatcuk/doublestar/v4"
	"gopkg.in/yaml.v3"

	"github.com/sageil/kodacode/v1/internal/config"
)

type Conditions struct {
	Files     []string `yaml:"files"`
	Languages []string `yaml:"languages"`
}

type Suggests struct {
	Before []string `yaml:"before"`
	After  []string `yaml:"after"`
}

type Section struct {
	ID    string
	Title string
	Level int
}

type Skill struct {
	Name        string
	Description string
	Path        string
	Scope       string
	Triggers    []string `yaml:"triggers"`
	Conditions  Conditions
	Suggests    Suggests
	Sections    []Section
}

type AccessPolicy struct {
	AllowAll    bool
	AllowAllSet bool
	Allow       []string
	Deny        []string
}

type Match struct {
	Skill   Skill
	Score   int
	Reasons []string
}

type Index struct {
	skills       []Skill
	byName       map[string]Skill
	watchedPaths []string
}

type skillFrontmatter struct {
	Name        string     `yaml:"name"`
	Description string     `yaml:"description"`
	Triggers    []string   `yaml:"triggers"`
	Conditions  Conditions `yaml:"conditions"`
	Suggests    Suggests   `yaml:"suggests"`
}

type sectionSpan struct {
	Section
	Start int
	End   int
}

type cachedIndexEntry struct {
	idx     *Index
	watched map[string]int64
}

const (
	searchExactNameScore   = 100
	searchNameMatchScore   = 60
	searchDescriptionScore = 30
	searchTriggerScore     = 12
	searchSectionScore     = 8

	relevantTriggerOverlapScore = 10
	relevantTopicOverlapScore   = 3
	relevantFileConditionScore  = 20
	relevantLanguageScore       = 12
)

var (
	indexCacheMu sync.RWMutex
	indexCache   = make(map[string]cachedIndexEntry)
)

func LoadIndex(dirs []string) *Index {
	key := cacheKeyForDirs(dirs)
	if cached := loadCachedIndex(key); cached != nil {
		return cached
	}
	idx := &Index{
		byName: make(map[string]Skill),
	}
	seenWatch := make(map[string]bool)
	watch := func(path string) {
		if path == "" || seenWatch[path] {
			return
		}
		seenWatch[path] = true
		idx.watchedPaths = append(idx.watchedPaths, path)
	}

	for _, dir := range dirs {
		if dir == "" {
			continue
		}
		watch(dir)
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			name := entry.Name()
			if _, exists := idx.byName[name]; exists {
				continue
			}
			skillFile := filepath.Join(dir, name, "SKILL.md")
			watch(skillFile)
			data, err := os.ReadFile(skillFile)
			if err != nil {
				continue
			}
			meta, body := parseSkillFile(name, string(data))
			scope := "global"
			if strings.Contains(skillFile, string(filepath.Separator)+".kodacode"+string(filepath.Separator)+"skills"+string(filepath.Separator)) {
				scope = "project"
			}
			skill := Skill{
				Name:        meta.Name,
				Description: meta.Description,
				Path:        skillFile,
				Scope:       scope,
				Triggers:    meta.Triggers,
				Conditions:  meta.Conditions,
				Suggests:    meta.Suggests,
				Sections:    sectionList(parseSections(body)),
			}
			idx.byName[skill.Name] = skill
			idx.skills = append(idx.skills, skill)
		}
	}

	sort.Slice(idx.skills, func(i, j int) bool { return idx.skills[i].Name < idx.skills[j].Name })
	storeCachedIndex(key, idx)
	return idx
}

func (i *Index) WatchedPaths() []string {
	if i == nil {
		return nil
	}
	out := make([]string, len(i.watchedPaths))
	copy(out, i.watchedPaths)
	return out
}

func (i *Index) Filter(policy AccessPolicy) []Skill {
	if i == nil {
		return nil
	}
	allow := make(map[string]bool, len(policy.Allow))
	for _, name := range policy.Allow {
		allow[name] = true
	}
	deny := make(map[string]bool, len(policy.Deny))
	for _, name := range policy.Deny {
		deny[name] = true
	}

	out := make([]Skill, 0, len(i.skills))
	allowAll := policy.AllowAll
	if !policy.AllowAllSet {
		allowAll = true
	}
	for _, skill := range i.skills {
		if len(allow) > 0 && !allow[skill.Name] {
			continue
		}
		if !allowAll && len(allow) == 0 {
			continue
		}
		if deny[skill.Name] {
			continue
		}
		out = append(out, skill)
	}
	return out
}

func (i *Index) Get(name string, policy AccessPolicy) (Skill, bool) {
	if i == nil {
		return Skill{}, false
	}
	skill, ok := i.byName[name]
	if !ok {
		return Skill{}, false
	}
	filtered := i.Filter(policy)
	for _, candidate := range filtered {
		if candidate.Name == skill.Name {
			return skill, true
		}
	}
	return Skill{}, false
}

func (i *Index) Search(query string, policy AccessPolicy, limit int) []Match {
	query = strings.TrimSpace(query)
	if query == "" || i == nil {
		return nil
	}
	if limit <= 0 {
		limit = 5
	}
	queryTokens := tokenSet(query)
	var matches []Match
	for _, skill := range i.Filter(policy) {
		score := 0
		var reasons []string
		nameLower := strings.ToLower(skill.Name)
		descLower := strings.ToLower(skill.Description)
		if nameLower == strings.ToLower(query) {
			score += searchExactNameScore
			reasons = append(reasons, "exact name match")
		} else if strings.Contains(nameLower, strings.ToLower(query)) {
			score += searchNameMatchScore
			reasons = append(reasons, "name match")
		}
		if descLower != "" && strings.Contains(descLower, strings.ToLower(query)) {
			score += searchDescriptionScore
			reasons = append(reasons, "description match")
		}
		triggerHits := 0
		for _, trigger := range skill.Triggers {
			triggerTokens := tokenSet(trigger)
			overlap := overlapCount(queryTokens, triggerTokens)
			if overlap > 0 {
				triggerHits += overlap
			}
		}
		if triggerHits > 0 {
			score += triggerHits * searchTriggerScore
			reasons = append(reasons, "trigger overlap")
		}
		for _, section := range skill.Sections {
			if strings.Contains(strings.ToLower(section.Title), strings.ToLower(query)) {
				score += searchSectionScore
				reasons = append(reasons, "section match")
				break
			}
		}
		if score == 0 {
			continue
		}
		matches = append(matches, Match{Skill: skill, Score: score, Reasons: uniqueStrings(reasons)})
	}
	sortMatches(matches)
	if len(matches) > limit {
		matches = matches[:limit]
	}
	return matches
}

func (i *Index) Relevant(query string, touchedFiles []string, policy AccessPolicy, limit int) []Match {
	if i == nil {
		return nil
	}
	queryTokens := tokenSet(query)
	languages := languagesFromFiles(touchedFiles)
	var matches []Match
	for _, skill := range i.Filter(policy) {
		score := 0
		var reasons []string
		if len(queryTokens) > 0 {
			for _, trigger := range skill.Triggers {
				overlap := overlapCount(queryTokens, tokenSet(trigger))
				if overlap > 0 {
					score += overlap * relevantTriggerOverlapScore
					reasons = append(reasons, "trigger match")
				}
			}
			nameDescTokens := tokenSet(skill.Name + " " + skill.Description)
			if overlap := overlapCount(queryTokens, nameDescTokens); overlap > 0 {
				score += overlap * relevantTopicOverlapScore
				reasons = append(reasons, "topic overlap")
			}
		}
		if len(touchedFiles) > 0 && len(skill.Conditions.Files) > 0 {
			for _, file := range touchedFiles {
				for _, pattern := range skill.Conditions.Files {
					if matched, _ := doublestar.Match(pattern, file); matched {
						score += relevantFileConditionScore
						reasons = append(reasons, "file condition")
						goto fileMatched
					}
				}
			}
		}
	fileMatched:
		if len(languages) > 0 && len(skill.Conditions.Languages) > 0 {
			for _, lang := range skill.Conditions.Languages {
				if languages[strings.ToLower(lang)] {
					score += relevantLanguageScore
					reasons = append(reasons, "language condition")
					break
				}
			}
		}
		if score == 0 {
			continue
		}
		matches = append(matches, Match{Skill: skill, Score: score, Reasons: uniqueStrings(reasons)})
	}
	sortMatches(matches)
	if limit <= 0 {
		limit = 5
	}
	if len(matches) > limit {
		matches = matches[:limit]
	}
	return matches
}

func ResolveAccess(cfg *config.Config, providerID, modelID string, agentCfg config.AgentConfig) AccessPolicy {
	policy := AccessPolicy{
		AllowAll:    true,
		AllowAllSet: false,
		Allow:       uniqueStrings(agentCfg.Skills.Allow),
		Deny:        uniqueStrings(agentCfg.Skills.Deny),
	}
	if cfg == nil {
		return policy
	}
	modelKey := providerID
	if modelID != "" {
		modelKey += "/" + modelID
	}
	if mc, ok := cfg.Skills.Models[modelKey]; ok {
		policy.AllowAll = mc.AllowAll
		policy.AllowAllSet = true
		policy.Deny = uniqueStrings(append(policy.Deny, mc.Deny...))
	}
	return policy
}

func LoadSkill(idx *Index, name, section string, policy AccessPolicy) (Skill, string, error) {
	skill, ok := idx.Get(name, policy)
	if !ok {
		return Skill{}, "", fmt.Errorf("skill %q not found", name)
	}
	data, err := os.ReadFile(skill.Path)
	if err != nil {
		return Skill{}, "", fmt.Errorf("read skill %q: %w", name, err)
	}
	_, body := parseSkillFile(skill.Name, string(data))
	if section == "" {
		return skill, body, nil
	}
	for _, span := range parseSections(body) {
		if span.ID == section {
			return skill, strings.TrimSpace(body[span.Start:span.End]), nil
		}
	}
	return Skill{}, "", fmt.Errorf("section %q not found in skill %q", section, name)
}

func parseSkillFile(defaultName, raw string) (skillFrontmatter, string) {
	meta := skillFrontmatter{Name: defaultName}
	body := strings.TrimSpace(raw)
	if strings.HasPrefix(raw, "---") {
		rest := raw[3:]
		if before, after, ok := strings.Cut(rest, "\n---"); ok {
			if err := yaml.Unmarshal([]byte(before), &meta); err == nil {
				body = strings.TrimSpace(strings.TrimPrefix(after, "\n"))
			}
		}
	}
	if meta.Name == "" {
		meta.Name = defaultName
	}
	if meta.Description == "" {
		meta.Description = firstDescriptionLine(body)
	}
	return meta, body
}

func parseSections(body string) []sectionSpan {
	lines := strings.Split(body, "\n")
	var spans []sectionSpan
	type pending struct {
		Section
		startLine int
	}
	var sections []pending
	usedIDs := make(map[string]int)
	bytePos := 0
	lineStarts := make([]int, 0, len(lines))
	for _, line := range lines {
		lineStarts = append(lineStarts, bytePos)
		bytePos += len(line) + 1
	}
	for idx, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "#") {
			continue
		}
		level := 0
		for level < len(trimmed) && trimmed[level] == '#' {
			level++
		}
		if level == 0 || level >= len(trimmed) || trimmed[level] != ' ' {
			continue
		}
		title := strings.TrimSpace(trimmed[level:])
		if title == "" {
			continue
		}
		id := slug(title)
		if count := usedIDs[id]; count > 0 {
			id = fmt.Sprintf("%s-%d", id, count+1)
		}
		usedIDs[slug(title)]++
		sections = append(sections, pending{
			Section:   Section{ID: id, Title: title, Level: level},
			startLine: idx,
		})
	}
	if len(sections) == 0 {
		return nil
	}
	for idx, section := range sections {
		start := lineStarts[section.startLine]
		end := len(body)
		if idx+1 < len(sections) {
			end = lineStarts[sections[idx+1].startLine]
		}
		spans = append(spans, sectionSpan{
			Section: section.Section,
			Start:   start,
			End:     end,
		})
	}
	return spans
}

func sectionList(spans []sectionSpan) []Section {
	if len(spans) == 0 {
		return nil
	}
	out := make([]Section, 0, len(spans))
	for _, span := range spans {
		out = append(out, span.Section)
	}
	return out
}

func firstDescriptionLine(body string) string {
	for _, line := range strings.Split(body, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if strings.HasPrefix(trimmed, "#") {
			return strings.TrimSpace(strings.TrimLeft(trimmed, "# "))
		}
		return trimmed
	}
	return ""
}

func tokenSet(text string) map[string]bool {
	out := make(map[string]bool)
	var current strings.Builder
	flush := func() {
		if current.Len() == 0 {
			return
		}
		out[current.String()] = true
		current.Reset()
	}
	for _, r := range strings.ToLower(text) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '-' || r == '.' || r == '/' {
			current.WriteRune(r)
			continue
		}
		flush()
	}
	flush()
	return out
}

func overlapCount(a, b map[string]bool) int {
	count := 0
	for token := range a {
		if b[token] {
			count++
		}
	}
	return count
}

func languagesFromFiles(files []string) map[string]bool {
	out := make(map[string]bool)
	for _, file := range files {
		switch strings.ToLower(filepath.Ext(file)) {
		case ".go":
			out["go"] = true
		case ".py":
			out["python"] = true
		case ".js", ".mjs", ".cjs":
			out["javascript"] = true
		case ".ts", ".tsx":
			out["typescript"] = true
		case ".jsx":
			out["javascript"] = true
		case ".rb":
			out["ruby"] = true
		case ".rs":
			out["rust"] = true
		case ".java":
			out["java"] = true
		case ".kt":
			out["kotlin"] = true
		case ".swift":
			out["swift"] = true
		case ".php":
			out["php"] = true
		case ".cs":
			out["csharp"] = true
		}
	}
	return out
}

func uniqueStrings(in []string) []string {
	seen := make(map[string]bool, len(in))
	var out []string
	for _, item := range in {
		item = strings.TrimSpace(item)
		if item == "" || seen[item] {
			continue
		}
		seen[item] = true
		out = append(out, item)
	}
	return out
}

func cacheKeyForDirs(dirs []string) string {
	clean := make([]string, 0, len(dirs))
	for _, dir := range dirs {
		if dir == "" {
			continue
		}
		clean = append(clean, filepath.Clean(dir))
	}
	return strings.Join(clean, "\x00")
}

func loadCachedIndex(key string) *Index {
	if key == "" {
		return nil
	}
	indexCacheMu.RLock()
	entry, ok := indexCache[key]
	indexCacheMu.RUnlock()
	if !ok || !indexCacheValid(entry) {
		return nil
	}
	return entry.idx
}

func storeCachedIndex(key string, idx *Index) {
	if key == "" || idx == nil {
		return
	}
	watched := make(map[string]int64, len(idx.watchedPaths))
	for _, path := range idx.watchedPaths {
		watched[path] = statUnixNano(path)
	}
	indexCacheMu.Lock()
	indexCache[key] = cachedIndexEntry{idx: idx, watched: watched}
	indexCacheMu.Unlock()
}

func indexCacheValid(entry cachedIndexEntry) bool {
	if entry.idx == nil {
		return false
	}
	for path, want := range entry.watched {
		if statUnixNano(path) != want {
			return false
		}
	}
	return true
}

func statUnixNano(path string) int64 {
	info, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return info.ModTime().UnixNano()
}

func sortMatches(matches []Match) {
	sort.Slice(matches, func(i, j int) bool {
		if matches[i].Score != matches[j].Score {
			return matches[i].Score > matches[j].Score
		}
		return matches[i].Skill.Name < matches[j].Skill.Name
	})
}

func slug(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var out strings.Builder
	lastDash := false
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			out.WriteRune(r)
			lastDash = false
			continue
		}
		if lastDash || out.Len() == 0 {
			continue
		}
		out.WriteByte('-')
		lastDash = true
	}
	return strings.Trim(out.String(), "-")
}
