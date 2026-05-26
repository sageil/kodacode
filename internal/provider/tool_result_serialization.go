package provider

import (
	"encoding/json"
	"strings"
)

func serializeToolResultForModel(input Input) string {
	body := rawToolResultBody(input)
	if hint := toolResultModelHint(input); hint != "" {
		return body + "\n" + hint
	}
	return body
}

func toolResultModelHint(input Input) string {
	switch {
	case shouldAppendReadPatchHint(input):
		return readPatchHint()
	case shouldAppendWebSearchReuseHint(input):
		return webSearchReuseHint()
	case shouldAppendWebFetchRecoveryHint(input):
		return webFetchRecoveryHint()
	default:
		return ""
	}
}

func rawToolResultBody(input Input) string {
	if strings.TrimSpace(input.Error) == "" {
		return input.Output
	}
	if strings.TrimSpace(input.Output) == "" {
		return input.Error
	}
	encoded, err := json.Marshal(map[string]string{
		"output": input.Output,
		"error":  input.Error,
	})
	if err != nil {
		return input.Error
	}
	return string(encoded)
}

func shouldAppendReadPatchHint(input Input) bool {
	return strings.TrimSpace(input.ToolName) == "read" &&
		strings.TrimSpace(input.CallID) != "" &&
		strings.TrimSpace(input.Error) == "" &&
		strings.TrimSpace(input.Output) != ""
}

func shouldAppendWebSearchReuseHint(input Input) bool {
	return strings.TrimSpace(input.ToolName) == "web_search" &&
		strings.TrimSpace(input.Error) == "" &&
		strings.TrimSpace(input.Output) != ""
}

func shouldAppendWebFetchRecoveryHint(input Input) bool {
	return strings.TrimSpace(input.ToolName) == "web_fetch" &&
		strings.TrimSpace(input.Error) != ""
}

func readPatchHint() string {
	return "(patch planning: use apply_patch with a structured patch for existing-file source edits. Do not include read line-number prefixes in patch lines.)"
}

func webSearchReuseHint() string {
	return "(web search reuse: reuse these candidate URLs when sufficient. If one fetched page fails, times out, or is too large, prefer another URL from these results before issuing another web_search unless the query itself needs to change.)"
}

func webFetchRecoveryHint() string {
	return "(web fetch recovery: if you already have candidate URLs from a recent web_search, prefer another returned URL before issuing another web_search. Search again only when the query itself needs to change.)"
}
