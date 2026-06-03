package tool

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"
)

func TestCurrentToolDefinitionsUseStrictSchemaShape(t *testing.T) {
	for _, tl := range AllBuiltInTools() {
		definition := tl.Definition()
		assertStrictSchemaIncludesAllProperties(t, definition.Name, definition.InputSchema, optionalSchemaProperties(definition.Name))
	}
}

func TestCurrentToolDefinitionsAllowStringFormsForBooleanAndIntegerProperties(t *testing.T) {
	for _, tl := range AllBuiltInTools() {
		definition := tl.Definition()
		var schema struct {
			Properties map[string]json.RawMessage `json:"properties"`
		}
		if err := json.Unmarshal(definition.InputSchema, &schema); err != nil {
			t.Fatalf("%s schema unmarshal error = %v", definition.Name, err)
		}
		for name, property := range schema.Properties {
			var raw struct {
				Type any `json:"type"`
			}
			if err := json.Unmarshal(property, &raw); err != nil {
				t.Fatalf("%s property %s schema unmarshal error = %v", definition.Name, name, err)
			}
			types := schemaTypeNames(raw.Type)
			if len(types) == 0 {
				continue
			}
			if (slices.Contains(types, "boolean") || slices.Contains(types, "integer")) && !slices.Contains(types, "string") {
				t.Fatalf("%s schema property %q must allow string forms for boolean/integer values; types=%v", definition.Name, name, types)
			}
		}
	}
}

func TestCurrentToolDefinitionsProvideArgumentExamples(t *testing.T) {
	for _, tl := range AllBuiltInTools() {
		definition := tl.Definition()
		if len(definition.ArgumentExamples) == 0 {
			t.Fatalf("%s definition missing argument examples", definition.Name)
		}
		for _, example := range definition.ArgumentExamples {
			if len(example) == 0 {
				t.Fatalf("%s definition contains empty argument example", definition.Name)
			}
			if definition.InputKindOrDefault() == InputKindCustom {
				continue
			}
			var payload any
			if err := json.Unmarshal([]byte(example), &payload); err != nil {
				t.Fatalf("%s argument example is not valid JSON: %v\nexample=%s", definition.Name, err, example)
			}
		}
	}
}

func TestCurrentToolDefinitionsProvideExplicitProviderDescriptions(t *testing.T) {
	for _, tl := range AllBuiltInTools() {
		definition := tl.Definition()
		if definition.ProviderDescriptionText() != definition.ProviderDescription {
			t.Fatalf("%s definition missing explicit provider description", definition.Name)
		}
		if definition.ProviderDescription == "" {
			t.Fatalf("%s definition has empty provider description", definition.Name)
		}
		if len(definition.ProviderDescription) > len(definition.Description) {
			t.Fatalf("%s provider description longer than description", definition.Name)
		}
	}
}

var auditedParallelSafeToolReasons = map[string]string{
	DefinitionToolName:  "read-only LSP definition lookup after workspace read authorization",
	DiagnosticsToolName: "read-only LSP diagnostics after workspace read authorization",
	"git_diff":          "workspace-scoped git diff process captured as an ordinary read result",
	"git_show":          "workspace-scoped git show process captured as an ordinary read result",
	"git_status":        "workspace-scoped git status process captured as an ordinary read result",
	LocateToolName:      "workspace list-only filesystem walk after list authorization",
	ReadToolName:        "workspace read-only file windows after read authorization",
	SearchToolName:      "workspace read-only lexical or indexed search after read authorization",
	SymbolsToolName:     "read-only workspace-symbol LSP query",
}

var auditedProviderRichGuidanceReasons = map[string]string{
	CodeActionToolName:           "LSP code actions need range and action-selection guidance",
	DefinitionToolName:           "LSP definition lookup benefits from symbol-vs-character guidance",
	DiagnosticsToolName:          "diagnostics should be batched on existing concrete files",
	ReadToolName:                 "read batching and range guidance materially reduces tool churn",
	RefsToolName:                 "semantic reference lookup has mode-specific refactor guidance",
	RenameSymbolToolName:         "semantic rename should be preferred over manual text replacement",
	SymbolsToolName:              "workspace symbols are the semantic declaration lookup path",
	TraceToolName:                "call hierarchy modes need concise selection guidance",
	WorkflowPhaseOutputToolName:  "workflow phase output must strongly prefer structured evidence over final prose",
	WorkflowReviewResultToolName: "workflow review result must strongly prefer typed evidence over assistant JSON",
	WriteToolName:                "whole-file replacement semantics need strong deletion guidance",
}

func TestCurrentToolDefinitionsParallelSafetyMatchesAudit(t *testing.T) {
	seen := make(map[string]struct{}, len(auditedParallelSafeToolReasons))
	for _, tl := range AllBuiltInTools() {
		definition := tl.Definition()
		reason, audited := auditedParallelSafeToolReasons[definition.Name]
		if definition.ParallelSafe && !audited {
			t.Fatalf("%s is ParallelSafe without an audit reason", definition.Name)
		}
		if audited {
			seen[definition.Name] = struct{}{}
			if strings.TrimSpace(reason) == "" {
				t.Fatalf("%s has an empty ParallelSafe audit reason", definition.Name)
			}
			if !definition.ParallelSafe {
				t.Fatalf("%s has a ParallelSafe audit reason but ParallelSafe=false", definition.Name)
			}
		}
	}
	for name := range auditedParallelSafeToolReasons {
		if _, ok := seen[name]; !ok {
			t.Fatalf("%s has a ParallelSafe audit reason but is not a built-in tool", name)
		}
	}
}

func TestCurrentToolDefinitionsProviderRichGuidanceMatchesAudit(t *testing.T) {
	seen := make(map[string]struct{}, len(auditedProviderRichGuidanceReasons))
	for _, tl := range AllBuiltInTools() {
		definition := tl.Definition()
		reason, audited := auditedProviderRichGuidanceReasons[definition.Name]
		if definition.ProviderRichGuidance && !audited {
			t.Fatalf("%s has ProviderRichGuidance without an audit reason", definition.Name)
		}
		if audited {
			seen[definition.Name] = struct{}{}
			if strings.TrimSpace(reason) == "" {
				t.Fatalf("%s has an empty ProviderRichGuidance audit reason", definition.Name)
			}
			if !definition.ProviderRichGuidance {
				t.Fatalf("%s has a ProviderRichGuidance audit reason but ProviderRichGuidance=false", definition.Name)
			}
		}
	}
	for name := range auditedProviderRichGuidanceReasons {
		if _, ok := seen[name]; !ok {
			t.Fatalf("%s has a ProviderRichGuidance audit reason but is not a built-in tool", name)
		}
	}
}

func optionalSchemaProperties(toolName string) map[string]struct{} {
	switch toolName {
	case "bash":
		return map[string]struct{}{
			"workdir":           {},
			"login":             {},
			"max_output_tokens": {},
			"prefix_rule":       {},
			"shell":             {},
			"tty":               {},
			"yield_time_ms":     {},
		}
	case ReadToolName:
		return map[string]struct{}{
			"path":       {},
			"paths":      {},
			"offset":     {},
			"limit":      {},
			"line_start": {},
			"line_end":   {},
		}
	case SearchToolName:
		return map[string]struct{}{
			"mode":           {},
			"glob":           {},
			"regex":          {},
			"case_sensitive": {},
			"max_matches":    {},
			"limit":          {},
			"max_results":    {},
		}
	case WebSearchToolName:
		return map[string]struct{}{
			"limit":           {},
			"max_results":     {},
			"domains":         {},
			"exclude_domains": {},
			"freshness_days":  {},
		}
	case SearchSkillsToolName:
		return map[string]struct{}{
			"limit": {},
		}
	case SkillToolName:
		return map[string]struct{}{
			"section": {},
		}
	case MemoryToolName:
		return map[string]struct{}{
			"content": {},
			"id":      {},
		}
	case QuestionToolName:
		return map[string]struct{}{
			"purpose": {},
		}
	case DelegateToolName:
		return map[string]struct{}{
			"source_handoff_ids": {},
		}
	case LocateToolName:
		return map[string]struct{}{
			"query":          {},
			"include_hidden": {},
			"max_matches":    {},
			"limit":          {},
			"max_results":    {},
		}
	case TestToolName:
		return map[string]struct{}{
			"command": {},
			"path":    {},
			"filter":  {},
			"timeout": {},
		}
	case CodeActionToolName:
		return map[string]struct{}{
			"title":          {},
			"kind":           {},
			"only_preferred": {},
		}
	case DefinitionToolName:
		return map[string]struct{}{
			"character": {},
			"symbol":    {},
		}
	case TraceToolName:
		return map[string]struct{}{
			"character": {},
			"symbol":    {},
			"depth":     {},
			"max_nodes": {},
		}
	case RefsToolName:
		return map[string]struct{}{
			"character":           {},
			"symbol":              {},
			"mode":                {},
			"max_results":         {},
			"include_declaration": {},
		}
	case RenameSymbolToolName:
		return map[string]struct{}{
			"character": {},
			"symbol":    {},
		}
	case WebFetchToolName:
		return map[string]struct{}{
			"method":   {},
			"headers":  {},
			"body":     {},
			"format":   {},
			"selector": {},
		}
	case DiagnosticsToolName:
		return map[string]struct{}{
			"path":  {},
			"paths": {},
		}
	case "git_show":
		return map[string]struct{}{
			"rev":      {},
			"revision": {},
			"commit":   {},
			"ref":      {},
		}
	case TaskWorkflowToolName:
		return map[string]struct{}{
			"task_id":        {},
			"parent_task_id": {},
			"title":          {},
			"kind":           {},
			"status":         {},
			"notes":          {},
			"progress":       {},
			"block_reason":   {},
			"summary":        {},
		}
	case TaskReviewToolName:
		return map[string]struct{}{
			"task_id":        {},
			"review_status":  {},
			"review_summary": {},
		}
	default:
		return nil
	}
}

func assertStrictSchemaIncludesAllProperties(t *testing.T, toolName string, schemaJSON json.RawMessage, optional map[string]struct{}) {
	t.Helper()

	var schema struct {
		Type       string                     `json:"type"`
		Properties map[string]json.RawMessage `json:"properties"`
		Required   []string                   `json:"required"`
	}
	if err := json.Unmarshal(schemaJSON, &schema); err != nil {
		t.Fatalf("%s schema unmarshal error = %v", toolName, err)
	}
	if schema.Type != "object" || len(schema.Properties) == 0 {
		return
	}

	required := make(map[string]struct{}, len(schema.Required))
	for _, name := range schema.Required {
		required[name] = struct{}{}
	}
	for name := range schema.Properties {
		if _, ok := optional[name]; ok {
			continue
		}
		if _, ok := required[name]; !ok {
			t.Fatalf("%s schema missing required property %q", toolName, name)
		}
	}
}

func schemaTypeNames(value any) []string {
	switch typed := value.(type) {
	case string:
		return []string{typed}
	case []any:
		names := make([]string, 0, len(typed))
		for _, item := range typed {
			name, ok := item.(string)
			if ok {
				names = append(names, name)
			}
		}
		return names
	default:
		return nil
	}
}
