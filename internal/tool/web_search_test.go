package tool

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	websearchsvc "github.com/sageil/kodacode/internal/websearch"
)

type stubWebSearchBackend struct {
	id       string
	lastReq  websearchsvc.Request
	response websearchsvc.Response
	err      error
}

func (s *stubWebSearchBackend) ID() string {
	return s.id
}

func (s *stubWebSearchBackend) Search(_ context.Context, req websearchsvc.Request) (websearchsvc.Response, error) {
	s.lastReq = req
	if s.err != nil {
		return websearchsvc.Response{}, s.err
	}
	return s.response, nil
}

func TestNormalizeWebSearchArgumentsInjectsConfiguredProvider(t *testing.T) {
	service, err := websearchsvc.NewService("exa", &stubWebSearchBackend{id: "exa"})
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	normalized, err := NormalizeWebSearchArguments(json.RawMessage(`{"query":"code agents","limit":"3"}`), service)
	if err != nil {
		t.Fatalf("NormalizeWebSearchArguments() error = %v", err)
	}
	if !strings.Contains(string(normalized), `"provider":"exa"`) {
		t.Fatalf("normalized = %s, want provider injected", normalized)
	}
}

func TestWebSearchToolNormalizedInputKeyNormalizesStringArgsAndDomains(t *testing.T) {
	tl := NewWebSearchTool()

	first, err := tl.NormalizedInputKey(json.RawMessage(`{"query":"code agents","limit":"5","domains":["OpenAI.com","arxiv.org"],"exclude_domains":"YouTube.com","freshness_days":"30","provider":"exa"}`))
	if err != nil {
		t.Fatalf("NormalizedInputKey(first) error = %v", err)
	}
	second, err := tl.NormalizedInputKey(json.RawMessage(`{"query":"code agents","limit":5,"domains":["arxiv.org","openai.com"],"exclude_domains":["youtube.com"],"freshness_days":30,"provider":"exa"}`))
	if err != nil {
		t.Fatalf("NormalizedInputKey(second) error = %v", err)
	}
	if first != second {
		t.Fatalf("input keys differ:\nfirst=%s\nsecond=%s", first, second)
	}
}

func TestWebSearchToolNormalizedInputKeyNormalizesMaxResultsAlias(t *testing.T) {
	tl := NewWebSearchTool()

	alias, err := tl.NormalizedInputKey(json.RawMessage(`{"query":"code agents","max_results":"5","provider":"exa"}`))
	if err != nil {
		t.Fatalf("NormalizedInputKey(alias) error = %v", err)
	}
	canonical, err := tl.NormalizedInputKey(json.RawMessage(`{"query":"code agents","limit":5,"provider":"exa"}`))
	if err != nil {
		t.Fatalf("NormalizedInputKey(canonical) error = %v", err)
	}
	if alias != canonical {
		t.Fatalf("input keys differ:\nalias=%s\ncanonical=%s", alias, canonical)
	}
}

func TestWebSearchToolExecuteFormatsStructuredResults(t *testing.T) {
	backend := &stubWebSearchBackend{
		id: "exa",
		response: websearchsvc.Response{
			Provider: "exa",
			Results: []websearchsvc.Result{{
				Title:       "Code Agents Survey",
				URL:         "https://example.com/code-agents",
				Snippet:     "A survey of agent systems.",
				Domain:      "example.com",
				PublishedAt: "2026-05-01T00:00:00Z",
				Author:      "Ada",
			}},
		},
	}
	service, err := websearchsvc.NewService("exa", backend)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	result, err := NewWebSearchTool().Execute(context.Background(), ExecutionContext{
		WebSearch: service,
	}, json.RawMessage(`{"query":"code agents","limit":5,"provider":"exa"}`))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !strings.Contains(result.Output, `Search results for "code agents"`) {
		t.Fatalf("output = %q", result.Output)
	}
	if !strings.Contains(result.Output, "Provider: exa") {
		t.Fatalf("output = %q", result.Output)
	}
	if !strings.Contains(result.Output, "https://example.com/code-agents") {
		t.Fatalf("output = %q", result.Output)
	}
	if !strings.Contains(result.Output, "example.com") {
		t.Fatalf("output = %q", result.Output)
	}
	if strings.Contains(result.Output, "Author:") || strings.Contains(result.Output, "Published:") {
		t.Fatalf("output should omit verbose metadata, got %q", result.Output)
	}
	if backend.lastReq.Query != "code agents" || backend.lastReq.Limit != 5 {
		t.Fatalf("backend request = %#v", backend.lastReq)
	}
	var structured struct {
		Provider string `json:"provider"`
		Query    string `json:"query"`
		Results  []struct {
			URL string `json:"url"`
		} `json:"results"`
	}
	if err := json.Unmarshal(result.StructuredResult, &structured); err != nil {
		t.Fatalf("structured result unmarshal error = %v", err)
	}
	if structured.Provider != "exa" || structured.Query != "code agents" || len(structured.Results) != 1 {
		t.Fatalf("structured = %#v", structured)
	}
}

func TestWebSearchToolRejectsExplicitZeroFreshnessDays(t *testing.T) {
	_, err := NewWebSearchTool().NormalizedInputKey(json.RawMessage(`{"query":"code agents","freshness_days":0,"provider":"exa"}`))
	if err == nil {
		t.Fatal("NormalizedInputKey() error = nil, want invalid arguments")
	}
	if !strings.Contains(err.Error(), "freshness_days") {
		t.Fatalf("err = %v", err)
	}
}
