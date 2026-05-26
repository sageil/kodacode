package tui

import (
	"testing"

	"github.com/sageil/kodacode/internal/events"
)

func TestSearchInspectorParamsIncludeStructuredResultMetadata(t *testing.T) {
	call := &events.ToolCallState{
		ToolName:         "search",
		Input:            `{"query":"permission","path":".","mode":"auto","glob":"","regex":false,"case_sensitive":false,"max_matches":10}`,
		StructuredResult: []byte(`{"mode":"lexical","notice":"warming workspace index in background","fallback":true,"results":[{"path":"main.go","line":12,"snippet":"permission check","source":"lexical"},{"path":"guard.go","line":7,"snippet":"permission denied","source":"lexical"}]}`),
	}

	params := searchInspectorParams(call)
	if got := inspectorParamValue(params, "Result Mode"); got != "lexical" {
		t.Fatalf("Result Mode = %q", got)
	}
	if got := inspectorParamValue(params, "Matches"); got != "2" {
		t.Fatalf("Matches = %q", got)
	}
	if got := inspectorParamValue(params, "Fallback"); got != "on" {
		t.Fatalf("Fallback = %q", got)
	}
	if got := inspectorParamValue(params, "Sources"); got != "lexical: 2" {
		t.Fatalf("Sources = %q", got)
	}
	if got := inspectorParamValue(params, "Notice"); got != "warming workspace index in background" {
		t.Fatalf("Notice = %q", got)
	}
	if got := groupedToolItemResultDetail(call); got != "2 matches · fallback" {
		t.Fatalf("groupedToolItemResultDetail() = %q", got)
	}
}

func TestTraceInspectorParamsIncludeStructuredResultMetadata(t *testing.T) {
	call := &events.ToolCallState{
		ToolName:         "trace",
		Input:            `{"path":"main.go","line":2,"character":5,"mode":"graph","depth":2,"max_nodes":50}`,
		StructuredResult: []byte(`{"supported":true,"found":true,"root_id":"root","nodes":[{"id":"root","name":"run","kind":"Function","path":"main.go","line":2,"character":5},{"id":"caller","name":"main","kind":"Function","path":"entry.go","line":9,"character":0}],"edges":[{"from_id":"caller","to_id":"root"}],"truncated":true}`),
	}

	params := traceInspectorParams(call)
	if got := inspectorParamValue(params, "Status"); got != "truncated" {
		t.Fatalf("Status = %q", got)
	}
	if got := inspectorParamValue(params, "Root"); got != "run" {
		t.Fatalf("Root = %q", got)
	}
	if got := inspectorParamValue(params, "Nodes"); got != "2" {
		t.Fatalf("Nodes = %q", got)
	}
	if got := inspectorParamValue(params, "Edges"); got != "1" {
		t.Fatalf("Edges = %q", got)
	}
	if got := inspectorParamValue(params, "Truncated"); got != "on" {
		t.Fatalf("Truncated = %q", got)
	}
	if got := groupedToolItemResultDetail(call); got != "2 nodes · 1 edge · truncated" {
		t.Fatalf("groupedToolItemResultDetail() = %q", got)
	}
}

func TestRefsInspectorParamsIncludeStructuredResultMetadata(t *testing.T) {
	call := &events.ToolCallState{
		ToolName:         "refs",
		Input:            `{"path":"store.ts","line":8,"character":2,"mode":"all","max_results":20,"include_declaration":false}`,
		StructuredResult: []byte(`{"supported":true,"found":true,"target":{"name":"value","kind":"Variable","path":"store.ts","line":1,"character":6},"references":[{"kind":"read","path":"store.ts","line":8,"character":2,"snippet":"return state.value"},{"kind":"write","path":"store.ts","line":12,"character":3,"snippet":"state.value = nextValue"}],"classification_supported":true,"classification_incomplete":true,"truncated":true}`),
	}

	params := refsInspectorParams(call)
	if got := inspectorParamValue(params, "Status"); got != "truncated" {
		t.Fatalf("Status = %q", got)
	}
	if got := inspectorParamValue(params, "Target"); got != "value" {
		t.Fatalf("Target = %q", got)
	}
	if got := inspectorParamValue(params, "References"); got != "2" {
		t.Fatalf("References = %q", got)
	}
	if got := inspectorParamValue(params, "Kinds"); got != "read: 1, write: 1" {
		t.Fatalf("Kinds = %q", got)
	}
	if got := inspectorParamValue(params, "Classified"); got != "on" {
		t.Fatalf("Classified = %q", got)
	}
	if got := inspectorParamValue(params, "Partial"); got != "on" {
		t.Fatalf("Partial = %q", got)
	}
	if got := inspectorParamValue(params, "Truncated"); got != "on" {
		t.Fatalf("Truncated = %q", got)
	}
	if got := groupedToolItemResultDetail(call); got != "2 refs · truncated · partial" {
		t.Fatalf("groupedToolItemResultDetail() = %q", got)
	}
}

func TestStructuredToolResultDetailsHandleUnsupportedStates(t *testing.T) {
	traceCall := &events.ToolCallState{
		ToolName:         "trace",
		StructuredResult: []byte(`{"supported":false,"found":false}`),
	}
	if got := groupedToolItemResultDetail(traceCall); got != "unsupported" {
		t.Fatalf("trace groupedToolItemResultDetail() = %q", got)
	}

	refsCall := &events.ToolCallState{
		ToolName:         "refs",
		StructuredResult: []byte(`{"supported":true,"found":false}`),
	}
	if got := groupedToolItemResultDetail(refsCall); got != "not found" {
		t.Fatalf("refs groupedToolItemResultDetail() = %q", got)
	}
}

func TestWebSearchInspectorParamsIncludeStructuredResultMetadata(t *testing.T) {
	call := &events.ToolCallState{
		ToolName:         "web_search",
		Input:            `{"query":"dependency injection","limit":5,"domains":["martinfowler.com","wikipedia.org"],"exclude_domains":["youtube.com"],"freshness_days":30,"provider":"exa"}`,
		StructuredResult: []byte(`{"provider":"exa","query":"dependency injection","notice":"limit clamped to 10","results":[{"title":"Dependency Injection","url":"https://en.wikipedia.org/wiki/Dependency_Injection","domain":"en.wikipedia.org","snippet":"Overview."},{"title":"Inversion of Control Containers and the Dependency Injection pattern","url":"https://martinfowler.com/articles/injection.html","domain":"martinfowler.com","snippet":"Martin Fowler guide."}]}`),
	}

	params := webSearchInspectorParams(call)
	if got := inspectorParamValue(params, "Query"); got != "dependency injection" {
		t.Fatalf("Query = %q", got)
	}
	if got := inspectorParamValue(params, "Limit"); got != "5" {
		t.Fatalf("Limit = %q", got)
	}
	if got := inspectorParamValue(params, "Domains"); got != "martinfowler.com, wikipedia.org" {
		t.Fatalf("Domains = %q", got)
	}
	if got := inspectorParamValue(params, "Exclude"); got != "youtube.com" {
		t.Fatalf("Exclude = %q", got)
	}
	if got := inspectorParamValue(params, "Freshness"); got != "30 days" {
		t.Fatalf("Freshness = %q", got)
	}
	if got := inspectorParamValue(params, "Provider"); got != "exa" {
		t.Fatalf("Provider = %q", got)
	}
	if got := inspectorParamValue(params, "Results"); got != "2" {
		t.Fatalf("Results = %q", got)
	}
	if got := inspectorParamValue(params, "Result Domains"); got != "en.wikipedia.org, martinfowler.com" {
		t.Fatalf("Result Domains = %q", got)
	}
	if got := inspectorParamValue(params, "Notice"); got != "limit clamped to 10" {
		t.Fatalf("Notice = %q", got)
	}
	if got := groupedToolItemResultDetail(call); got != "2 results · exa · notice" {
		t.Fatalf("groupedToolItemResultDetail() = %q", got)
	}
}

func inspectorParamValue(params []inspectorParam, label string) string {
	for _, param := range params {
		if param.Label == label {
			return param.Value
		}
	}
	return ""
}
