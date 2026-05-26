package tui

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/sageil/kodacode/internal/events"
)

type readToolViewInput struct {
	Paths     []string `json:"paths"`
	Offset    int      `json:"offset"`
	Limit     int      `json:"limit"`
	HasOffset bool
	HasLimit  bool
}

func readInspectorParams(call *events.ToolCallState) []inspectorParam {
	input, ok := parseReadToolViewInput(call.Input)
	if !ok {
		return defaultInspectorParams(call)
	}

	params := []inspectorParam{
		{Label: "Path", Value: strings.Join(input.Paths, ", ")},
	}
	if failedPaths := parseReadFailurePaths(call.Error); len(failedPaths) > 0 {
		label := "Failed Path"
		if len(failedPaths) > 1 {
			label = "Failed Paths"
		}
		params = append(params, inspectorParam{Label: label, Value: strings.Join(failedPaths, ", ")})
	}
	if input.HasOffset {
		params = append(params, inspectorParam{Label: "Offset", Value: fmt.Sprintf("%d", input.Offset)})
	}
	if input.HasLimit {
		params = append(params, inspectorParam{Label: "Limit", Value: fmt.Sprintf("%d", input.Limit)})
	}
	if errorText := summarizeBlock(call.Error); errorText != "" {
		params = append(params, inspectorParam{Label: "Error", Value: errorText, Error: true})
	}
	return params
}

func parseReadFailurePaths(raw string) []string {
	lines := strings.Split(strings.TrimSpace(raw), "\n")
	if len(lines) < 2 {
		return nil
	}

	header := strings.TrimSpace(lines[0])
	if !strings.HasPrefix(header, "read failed for ") {
		return nil
	}
	if !strings.HasSuffix(header, " path:") && !strings.HasSuffix(header, " paths:") {
		return nil
	}

	failedPaths := make([]string, 0, len(lines)-1)
	for _, line := range lines[1:] {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		separator := strings.Index(line, ": ")
		if separator <= 0 {
			return nil
		}
		path := strings.TrimSpace(line[:separator])
		if path == "" {
			return nil
		}
		failedPaths = append(failedPaths, path)
	}
	if len(failedPaths) == 0 {
		return nil
	}
	return failedPaths
}

func parseReadToolViewInput(raw string) (readToolViewInput, bool) {
	var input struct {
		Path   json.RawMessage `json:"path"`
		Paths  json.RawMessage `json:"paths"`
		Offset json.RawMessage `json:"offset"`
		Limit  json.RawMessage `json:"limit"`
	}
	if json.Unmarshal([]byte(raw), &input) != nil {
		return readToolViewInput{}, false
	}
	paths, ok := parseReadToolViewPaths(input.Path, input.Paths)
	if !ok {
		return readToolViewInput{}, false
	}
	out := readToolViewInput{Paths: paths}
	offset, hasOffset := parseToolViewOptionalInt(input.Offset)
	limit, hasLimit := parseToolViewOptionalInt(input.Limit)
	if hasOffset {
		out.Offset = offset
		out.HasOffset = true
	}
	if hasLimit {
		out.Limit = limit
		out.HasLimit = true
	}
	return out, true
}

func parseReadToolViewPaths(rawPath, rawPaths json.RawMessage) ([]string, bool) {
	path, hasPath, ok := parseReadToolViewPath(rawPath)
	if !ok {
		return nil, false
	}
	paths, hasPaths, ok := parseReadToolViewPathList(rawPaths)
	if !ok {
		return nil, false
	}
	if hasPath == hasPaths {
		return nil, false
	}
	if hasPath {
		return []string{path}, true
	}
	return paths, true
}

func parseReadToolViewPath(raw json.RawMessage) (string, bool, bool) {
	if len(raw) == 0 || strings.TrimSpace(string(raw)) == "null" {
		return "", false, true
	}
	var path string
	if err := json.Unmarshal(raw, &path); err != nil {
		return "", false, false
	}
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return "", false, false
	}
	if strings.EqualFold(trimmed, "null") {
		return "", false, true
	}
	return trimmed, true, true
}

func parseReadToolViewPathList(raw json.RawMessage) ([]string, bool, bool) {
	if len(raw) == 0 || strings.TrimSpace(string(raw)) == "null" {
		return nil, false, true
	}
	items, ok := parseReadToolViewPathListItems(raw)
	if !ok {
		return nil, false, false
	}
	paths := make([]string, 0, len(items))
	for _, item := range items {
		var path string
		if err := json.Unmarshal(item, &path); err != nil {
			return nil, false, false
		}
		trimmed := strings.TrimSpace(path)
		if trimmed == "" {
			return nil, false, false
		}
		paths = append(paths, trimmed)
	}
	if len(paths) == 0 {
		return nil, false, false
	}
	return paths, true, true
}

func parseReadToolViewPathListItems(raw json.RawMessage) ([]json.RawMessage, bool) {
	var items []json.RawMessage
	if err := json.Unmarshal(raw, &items); err == nil {
		return items, true
	}
	var text string
	if err := json.Unmarshal(raw, &text); err != nil {
		return nil, false
	}
	trimmed := strings.TrimSpace(text)
	if trimmed == "" || strings.EqualFold(trimmed, "null") {
		return nil, true
	}
	if !strings.HasPrefix(trimmed, "[") {
		return []json.RawMessage{append(json.RawMessage(nil), raw...)}, true
	}
	if err := json.Unmarshal([]byte(trimmed), &items); err != nil {
		return nil, false
	}
	return items, true
}
