package skill

import (
	"sort"
	"strings"
)

type Match struct {
	Definition Definition
	Score      int
	Reasons    []string
}

func (r *Registry) Search(workspaceRoot, query string, limit int) ([]Match, error) {
	catalog, err := r.Catalog(workspaceRoot)
	if err != nil {
		return nil, err
	}
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, nil
	}
	if limit <= 0 {
		limit = 5
	}
	if wildcardSkillQuery(query) {
		return listSkillMatches(catalog, limit), nil
	}

	queryLower := strings.ToLower(query)
	queryTokens := uniqueSearchTokens(query)
	matches := make([]Match, 0, len(catalog))
	for _, definition := range catalog {
		if !definition.ModelVisible() {
			continue
		}
		score, reasons := scoreDefinitionMatch(definition, queryLower, queryTokens)
		if score == 0 {
			continue
		}
		matches = append(matches, Match{
			Definition: definition,
			Score:      score,
			Reasons:    reasons,
		})
	}
	sort.Slice(matches, func(i, j int) bool {
		if matches[i].Score == matches[j].Score {
			return matches[i].Definition.ID < matches[j].Definition.ID
		}
		return matches[i].Score > matches[j].Score
	})
	if len(matches) > limit {
		matches = matches[:limit]
	}
	return matches, nil
}

func wildcardSkillQuery(query string) bool {
	query = strings.TrimSpace(query)
	if query == "" {
		return false
	}
	for _, r := range query {
		if r != '*' {
			return false
		}
	}
	return true
}

func listSkillMatches(catalog map[string]Definition, limit int) []Match {
	if len(catalog) == 0 {
		return nil
	}
	matches := make([]Match, 0, len(catalog))
	for _, definition := range catalog {
		if !definition.ModelVisible() {
			continue
		}
		matches = append(matches, Match{
			Definition: definition,
			Score:      1,
			Reasons:    []string{"all skills"},
		})
	}
	sort.Slice(matches, func(i, j int) bool {
		if matches[i].Definition.ID == matches[j].Definition.ID {
			return matches[i].Definition.Source < matches[j].Definition.Source
		}
		return matches[i].Definition.ID < matches[j].Definition.ID
	})
	if len(matches) > limit {
		matches = matches[:limit]
	}
	return matches
}

func scoreDefinitionMatch(definition Definition, queryLower string, queryTokens []string) (int, []string) {
	score := 0
	reasons := make([]string, 0, 3)
	idLower := strings.ToLower(definition.ID)
	descriptionLower := strings.ToLower(definition.Description)
	whenToUseLower := strings.ToLower(definition.WhenToUse)
	promptLower := strings.ToLower(definition.Prompt)
	searchTextLower := strings.ToLower(definition.SearchText())

	if idLower == queryLower {
		score += 100
		reasons = append(reasons, "exact id")
	}
	if idLower != "" && strings.Contains(idLower, queryLower) {
		score += 60
		reasons = appendReason(reasons, "id")
	}
	if descriptionLower != "" && strings.Contains(descriptionLower, queryLower) {
		score += 40
		reasons = appendReason(reasons, "description")
	}
	if whenToUseLower != "" && strings.Contains(whenToUseLower, queryLower) {
		score += 40
		reasons = appendReason(reasons, "when_to_use")
	}
	if promptLower != "" && strings.Contains(promptLower, queryLower) {
		score += 20
		reasons = appendReason(reasons, "content")
	}

	tokenMatches := 0
	for _, token := range queryTokens {
		if token == "" {
			continue
		}
		switch {
		case strings.Contains(idLower, token):
			tokenMatches++
		case strings.Contains(descriptionLower, token):
			tokenMatches++
		case strings.Contains(whenToUseLower, token):
			tokenMatches++
		case strings.Contains(searchTextLower, token):
			tokenMatches++
		}
	}
	if tokenMatches > 0 {
		score += tokenMatches * 5
		reasons = appendReason(reasons, "keyword overlap")
	}
	return score, reasons
}

func uniqueSearchTokens(value string) []string {
	raw := strings.Fields(strings.ToLower(value))
	if len(raw) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, len(raw))
	for _, token := range raw {
		token = strings.Trim(token, ".,:;!?()[]{}<>`\"'")
		if len(token) < 2 {
			continue
		}
		if _, ok := seen[token]; ok {
			continue
		}
		seen[token] = struct{}{}
		out = append(out, token)
	}
	return out
}

func appendReason(reasons []string, reason string) []string {
	for _, existing := range reasons {
		if existing == reason {
			return reasons
		}
	}
	return append(reasons, reason)
}
