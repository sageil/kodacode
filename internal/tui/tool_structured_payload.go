package tui

import (
	"encoding/json"
	"sort"
	"strings"
	"unicode"

	"github.com/charmbracelet/x/ansi"
)

type structuredPayloadField struct {
	Key   string
	Value any
}

func renderStructuredPayloadTranscript(raw string, width int) (string, bool) {
	fields, ok := parseStructuredPayloadFields(raw)
	if !ok {
		return "", false
	}
	return renderStructuredPayloadTranscriptFields(fields, width), true
}

func renderStructuredPayloadTranscriptDelta(raw string, baselineRaw string, width int) (string, bool) {
	fields, ok := parseStructuredPayloadFields(raw)
	if !ok {
		return "", false
	}
	baseline, ok := parseStructuredPayloadFields(baselineRaw)
	if !ok {
		return renderStructuredPayloadTranscriptFields(fields, width), true
	}
	seen := make(map[string]string, len(baseline))
	for _, field := range baseline {
		seen[strings.ToLower(strings.TrimSpace(field.Key))] = structuredPayloadCanonicalJSON(field.Value)
	}
	filtered := make([]structuredPayloadField, 0, len(fields))
	for _, field := range fields {
		key := strings.ToLower(strings.TrimSpace(field.Key))
		if key == "" {
			continue
		}
		if structuredPayloadCanonicalJSON(field.Value) == seen[key] {
			continue
		}
		filtered = append(filtered, field)
	}
	if len(filtered) == 0 {
		return "", false
	}
	return renderStructuredPayloadTranscriptFields(filtered, width), true
}

func renderStructuredPayloadTranscriptFields(fields []structuredPayloadField, width int) string {
	lines := make([]string, 0, len(fields)*2)
	for _, field := range fields {
		lines = append(lines, renderStructuredPayloadTranscriptField(field, width)...)
	}
	return strings.Join(lines, "\n")
}

func renderStructuredPayloadMarkdown(heading, raw string) (string, bool) {
	fields, ok := parseStructuredPayloadFields(raw)
	if !ok {
		return "", false
	}
	lines := []string{strings.TrimSpace(heading)}
	for _, field := range fields {
		rendered := renderStructuredPayloadMarkdownField(field)
		if len(rendered) == 0 {
			continue
		}
		lines = append(lines, "")
		lines = append(lines, rendered...)
	}
	return strings.Join(lines, "\n"), true
}

func parseStructuredPayloadFields(raw string) ([]structuredPayloadField, bool) {
	decoder := json.NewDecoder(strings.NewReader(strings.TrimSpace(raw)))
	decoder.UseNumber()

	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, false
	}

	object, ok := value.(map[string]any)
	if !ok || len(object) == 0 {
		return nil, false
	}

	fields := make([]structuredPayloadField, 0, len(object))
	for key, fieldValue := range object {
		fields = append(fields, structuredPayloadField{Key: key, Value: fieldValue})
	}

	sort.SliceStable(fields, func(i, j int) bool {
		leftRank := structuredPayloadFieldRank(fields[i].Value)
		rightRank := structuredPayloadFieldRank(fields[j].Value)
		if leftRank != rightRank {
			return leftRank < rightRank
		}
		return strings.ToLower(fields[i].Key) < strings.ToLower(fields[j].Key)
	})

	return fields, true
}

func structuredPayloadFieldRank(value any) int {
	if text, ok := structuredPayloadString(value); ok {
		if structuredPayloadNeedsBlock(text) {
			return 0
		}
		return 1
	}
	if _, ok := structuredPayloadScalar(value); ok {
		return 2
	}
	return 3
}

func renderStructuredPayloadTranscriptField(field structuredPayloadField, width int) []string {
	label := structuredPayloadFieldLabel(field.Key)
	if text, ok := structuredPayloadString(field.Value); ok {
		if structuredPayloadNeedsBlock(text) {
			lines := []string{label + ":"}
			lines = append(lines, indentStructuredLines(wrapStructuredText(text, max(width-2, 24)), "  ")...)
			return lines
		}
		return []string{label + ": " + strings.TrimSpace(text)}
	}
	if scalar, ok := structuredPayloadScalar(field.Value); ok {
		return []string{label + ": " + scalar}
	}
	if inline, ok := structuredPayloadInlineJSON(field.Value, max(width-len(label)-2, 16)); ok {
		return []string{label + ": " + inline}
	}
	pretty := structuredPayloadPrettyJSON(field.Value)
	if pretty == "" {
		return nil
	}
	lines := []string{label + ":"}
	for _, line := range strings.Split(pretty, "\n") {
		lines = append(lines, "  "+line)
	}
	return lines
}

func renderStructuredPayloadMarkdownField(field structuredPayloadField) []string {
	label := structuredPayloadFieldLabel(field.Key)
	if text, ok := structuredPayloadString(field.Value); ok {
		text = strings.TrimSpace(text)
		if structuredPayloadNeedsBlock(text) {
			return []string{"**" + label + ":**", "", text}
		}
		return []string{"- " + label + ": " + text}
	}
	if scalar, ok := structuredPayloadScalar(field.Value); ok {
		return []string{"- " + label + ": " + scalar}
	}
	if inline, ok := structuredPayloadInlineJSON(field.Value, 48); ok {
		return []string{"- " + label + ": " + inline}
	}
	pretty := structuredPayloadPrettyJSON(field.Value)
	if pretty == "" {
		return nil
	}
	return []string{"**" + label + ":**", fencedToolDetailBlock("json", pretty)}
}

func structuredPayloadString(value any) (string, bool) {
	text, ok := value.(string)
	if !ok {
		return "", false
	}
	return strings.TrimSpace(text), true
}

func structuredPayloadScalar(value any) (string, bool) {
	switch typed := value.(type) {
	case nil:
		return "null", true
	case bool:
		if typed {
			return "true", true
		}
		return "false", true
	case json.Number:
		return typed.String(), true
	}
	return "", false
}

func structuredPayloadInlineJSON(value any, width int) (string, bool) {
	body, err := json.Marshal(value)
	if err != nil {
		return "", false
	}
	inline := string(body)
	if strings.Contains(inline, "\n") || ansi.StringWidth(inline) > max(width, 16) {
		return "", false
	}
	return inline, true
}

func structuredPayloadPrettyJSON(value any) string {
	body, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return ""
	}
	return string(body)
}

func structuredPayloadCanonicalJSON(value any) string {
	body, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	return string(body)
}

func structuredPayloadNeedsBlock(text string) bool {
	text = strings.TrimSpace(text)
	return strings.Contains(text, "\n") || ansi.StringWidth(text) > 56
}

func wrapStructuredText(text string, width int) []string {
	paragraphs := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	lines := make([]string, 0, len(paragraphs))
	for _, paragraph := range paragraphs {
		paragraph = strings.TrimSpace(paragraph)
		if paragraph == "" {
			if len(lines) == 0 || lines[len(lines)-1] == "" {
				continue
			}
			lines = append(lines, "")
			continue
		}
		wrapped := ansi.Wrap(paragraph, max(width, 1), "")
		lines = append(lines, strings.Split(strings.TrimRight(wrapped, "\n"), "\n")...)
	}
	return lines
}

func indentStructuredLines(lines []string, prefix string) []string {
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		if line == "" {
			out = append(out, "")
			continue
		}
		out = append(out, prefix+line)
	}
	return out
}

func structuredPayloadFieldLabel(key string) string {
	if strings.TrimSpace(key) == "" {
		return ""
	}
	var tokens []string
	var current []rune
	flush := func() {
		if len(current) == 0 {
			return
		}
		tokens = append(tokens, string(current))
		current = current[:0]
	}
	for _, r := range key {
		switch {
		case r == '_' || r == '-' || unicode.IsSpace(r):
			flush()
		case unicode.IsUpper(r) && len(current) > 0 && (unicode.IsLower(current[len(current)-1]) || unicode.IsDigit(current[len(current)-1])):
			flush()
			current = append(current, unicode.ToLower(r))
		default:
			current = append(current, unicode.ToLower(r))
		}
	}
	flush()
	for i, token := range tokens {
		runes := []rune(token)
		if len(runes) == 0 {
			continue
		}
		runes[0] = unicode.ToUpper(runes[0])
		tokens[i] = string(runes)
	}
	return strings.Join(tokens, " ")
}
