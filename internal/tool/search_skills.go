package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

const SearchSkillsToolName = "search_skills"
const searchSkillsDefaultLimit = 5
const searchSkillsMaxLimit = 10

type SearchSkillsTool struct{}

func NewSearchSkillsTool() SearchSkillsTool {
	return SearchSkillsTool{}
}

func (SearchSkillsTool) Definition() Definition {
	return Definition{
		Name:                SearchSkillsToolName,
		Description:         "Search available project and global skills by topic, workflow, description, and instruction content. Use query \"*\" to list all available skills when you need discovery instead of topic search.",
		ProviderDescription: "Search available skills by topic, or use query `*` to list them.",
		InputSchema:         json.RawMessage(`{"type":"object","properties":{"query":{"type":"string","description":"Topic, workflow, or phrase to search for. Use \"*\" to list all available skills."},"limit":{"type":["integer","string","null"],"minimum":1,"description":"Use null or omit this field to accept the default limit of 5 results. Values above 10 are clamped to 10."}},"required":["query"],"additionalProperties":false}`),
		ArgumentExamples:    []string{`{"query":"oauth login","limit":5}`, `{"query":"*","limit":10}`},
	}
}

func (SearchSkillsTool) Execute(_ context.Context, ectx ExecutionContext, args json.RawMessage) (Result, error) {
	catalog, err := ectx.Skills()
	if err != nil {
		return Result{}, err
	}
	input, err := parseSearchSkillsInput(args)
	if err != nil {
		return Result{}, err
	}
	matches, err := catalog.SearchSkills(input.Query, input.Limit)
	if err != nil {
		return Result{}, err
	}
	return Result{Output: searchSkillsOutput(matches)}, nil
}

type searchSkillsInput struct {
	Query string
	Limit int
}

func parseSearchSkillsInput(args json.RawMessage) (_ searchSkillsInput, err error) {
	defer func() {
		err = normalizeToolInputError(SearchSkillsToolName, err)
	}()
	var raw struct {
		Query string          `json:"query"`
		Limit json.RawMessage `json:"limit"`
	}
	if err := DecodeArgs(SearchSkillsToolName, args, &raw); err != nil {
		return searchSkillsInput{}, err
	}
	query := strings.TrimSpace(raw.Query)
	if query == "" {
		return searchSkillsInput{}, fmt.Errorf("query is required")
	}
	limit := searchSkillsDefaultLimit
	if value, ok, err := decodeOptionalIntArg(SearchSkillsToolName, raw.Limit, "limit"); err != nil {
		return searchSkillsInput{}, err
	} else if ok {
		limit = value
	}
	if limit <= 0 {
		return searchSkillsInput{}, fmt.Errorf("limit must be between 1 and %d", searchSkillsMaxLimit)
	}
	if limit > searchSkillsMaxLimit {
		limit = searchSkillsMaxLimit
	}
	return searchSkillsInput{Query: query, Limit: limit}, nil
}

func searchSkillsOutput(matches []SkillMatch) string {
	if matches == nil {
		matches = []SkillMatch{}
	}
	payload, err := json.Marshal(struct {
		Matches []SkillMatch `json:"matches"`
	}{Matches: matches})
	if err != nil {
		return `{"matches":[]}`
	}
	return string(payload)
}
