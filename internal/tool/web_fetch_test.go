package tool

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestWebFetchToolDefinitionRequiresOnlyURLByDefault(t *testing.T) {
	definition := NewWebFetchTool().Definition()
	var schema struct {
		Properties map[string]json.RawMessage `json:"properties"`
		Required   []string                   `json:"required"`
	}
	if err := json.Unmarshal(definition.InputSchema, &schema); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if len(schema.Properties) != 6 {
		t.Fatalf("properties = %#v", schema.Properties)
	}
	for _, field := range []string{"url"} {
		if !containsString(schema.Required, field) {
			t.Fatalf("required = %#v, missing %q", schema.Required, field)
		}
	}
	for _, field := range []string{"method", "headers", "body", "format", "selector"} {
		if containsString(schema.Required, field) {
			t.Fatalf("required = %#v, should omit %q", schema.Required, field)
		}
	}
}

func TestWebFetchToolNetworkRequestsUsesHostTargetAndPreview(t *testing.T) {
	requests, err := NewWebFetchTool().NetworkRequests(json.RawMessage(`{"url":"https://example.com/docs?id=1","method":null,"headers":null,"body":null,"format":null,"selector":null}`))
	if err != nil {
		t.Fatalf("NetworkRequests() error = %v", err)
	}
	if len(requests) != 1 {
		t.Fatalf("requests = %#v", requests)
	}
	if requests[0].Target != "example.com" {
		t.Fatalf("Target = %q", requests[0].Target)
	}
	if requests[0].URL != "https://example.com/docs?id=1" {
		t.Fatalf("URL = %q", requests[0].URL)
	}
	if requests[0].Command != "web_fetch https://example.com/docs?id=1" {
		t.Fatalf("Command = %q", requests[0].Command)
	}
}

func TestWebFetchToolNetworkRequestsIncludesNonGETMethodInPreview(t *testing.T) {
	requests, err := NewWebFetchTool().NetworkRequests(json.RawMessage(`{"url":"https://example.com/api/tasks","method":"POST","headers":null,"body":"{}","format":null,"selector":null}`))
	if err != nil {
		t.Fatalf("NetworkRequests() error = %v", err)
	}
	if len(requests) != 1 {
		t.Fatalf("requests = %#v", requests)
	}
	if requests[0].URL != "https://example.com/api/tasks" {
		t.Fatalf("URL = %q", requests[0].URL)
	}
	if requests[0].Command != "web_fetch POST https://example.com/api/tasks" {
		t.Fatalf("Command = %q", requests[0].Command)
	}
}

func TestWebFetchToolExecuteFormatsJSONAutomatically(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"name":"kodacode"}`))
	}))
	defer server.Close()

	tl := WebFetchTool{client: server.Client()}
	result, err := tl.Execute(context.Background(), ExecutionContext{}, json.RawMessage(`{"url":"`+server.URL+`","method":null,"headers":null,"body":null,"format":"auto","selector":null}`))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !strings.Contains(result.Output, "\"ok\": true") || !strings.Contains(result.Output, "\"name\": \"kodacode\"") {
		t.Fatalf("output = %q", result.Output)
	}
}

func TestWebFetchToolExecuteFormatsHTMLAsMarkdownInAutoMode(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<html><body><main><h1>Docs</h1><p>Hello world.</p><script>ignored()</script></main></body></html>`))
	}))
	defer server.Close()

	tl := WebFetchTool{client: server.Client()}
	result, err := tl.Execute(context.Background(), ExecutionContext{}, json.RawMessage(`{"url":"`+server.URL+`","method":null,"headers":null,"body":null,"format":"auto","selector":null}`))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if strings.Contains(result.Output, "ignored()") {
		t.Fatalf("output leaked script text: %q", result.Output)
	}
	for _, want := range []string{"# Docs", "Hello world."} {
		if !strings.Contains(result.Output, want) {
			t.Fatalf("output = %q, missing %q", result.Output, want)
		}
	}
}

func TestWebFetchToolExecuteSelectorLimitsHTMLExtraction(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<html><body><header>Navigation</header><main><h1>Docs</h1><p>Main content.</p></main><footer>Footer</footer></body></html>`))
	}))
	defer server.Close()

	tl := WebFetchTool{client: server.Client()}
	result, err := tl.Execute(context.Background(), ExecutionContext{}, json.RawMessage(`{"url":"`+server.URL+`","method":null,"headers":null,"body":null,"format":"text","selector":"main"}`))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if strings.Contains(result.Output, "Navigation") || strings.Contains(result.Output, "Footer") {
		t.Fatalf("output = %q", result.Output)
	}
	for _, want := range []string{"Docs", "Main content."} {
		if !strings.Contains(result.Output, want) {
			t.Fatalf("output = %q, missing %q", result.Output, want)
		}
	}
}

func TestWebFetchToolExecuteMarkdownFormatConvertsHTMLStructure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<html><body><article><h1>Title</h1><p>Paragraph with <a href="https://example.com">a link</a>.</p><ul><li>Item 1</li><li>Item 2</li></ul><pre><code class="language-go">fmt.Println("hi")</code></pre></article></body></html>`))
	}))
	defer server.Close()

	tl := WebFetchTool{client: server.Client()}
	result, err := tl.Execute(context.Background(), ExecutionContext{}, json.RawMessage(`{"url":"`+server.URL+`","method":null,"headers":null,"body":null,"format":"markdown","selector":null}`))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	for _, want := range []string{"# Title", "[a link](https://example.com)", "- Item 1", "```go"} {
		if !strings.Contains(result.Output, want) {
			t.Fatalf("output = %q, missing %q", result.Output, want)
		}
	}
}

func TestWebFetchToolExecuteSupportsExplicitMethodHeadersAndBody(t *testing.T) {
	var gotMethod, gotBody, gotAuth, gotContentType string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotAuth = r.Header.Get("Authorization")
		gotContentType = r.Header.Get("Content-Type")
		body, _ := io.ReadAll(r.Body)
		gotBody = string(body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	tl := WebFetchTool{client: server.Client()}
	result, err := tl.Execute(context.Background(), ExecutionContext{}, json.RawMessage(`{"url":"`+server.URL+`","method":"POST","headers":{"Authorization":"Bearer tok123"},"body":"{\"task\":\"sync\"}","format":"auto","selector":null}`))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if gotMethod != http.MethodPost {
		t.Fatalf("Method = %q", gotMethod)
	}
	if gotAuth != "Bearer tok123" {
		t.Fatalf("Authorization = %q", gotAuth)
	}
	if gotContentType != "application/json" {
		t.Fatalf("Content-Type = %q", gotContentType)
	}
	if gotBody != `{"task":"sync"}` {
		t.Fatalf("Body = %q", gotBody)
	}
	if !strings.Contains(result.Output, `"ok": true`) {
		t.Fatalf("output = %q", result.Output)
	}
}

func TestWebFetchToolRejectsBodyWithGetOrHead(t *testing.T) {
	for _, method := range []string{"GET", "HEAD"} {
		_, _, err := parseWebFetchInput(json.RawMessage(`{"url":"https://example.com/docs","method":"` + method + `","body":"{}"}`))
		if !errors.Is(err, ErrWebFetchBodyMethod) {
			t.Fatalf("parseWebFetchInput(%s) error = %v, want ErrWebFetchBodyMethod", method, err)
		}
		if !errors.Is(err, ErrInvalidArguments) {
			t.Fatalf("parseWebFetchInput(%s) errors.Is ErrInvalidArguments = false, err = %v", method, err)
		}
	}
}

func TestWebFetchToolExecuteRejectsBinaryContent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = w.Write([]byte{0x00, 0x01, 0x02})
	}))
	defer server.Close()

	tl := WebFetchTool{client: server.Client()}
	_, err := tl.Execute(context.Background(), ExecutionContext{}, json.RawMessage(`{"url":"`+server.URL+`","method":null,"headers":null,"body":null,"format":"auto","selector":null}`))
	if err != ErrWebFetchBinaryResponse {
		t.Fatalf("error = %v, want %v", err, ErrWebFetchBinaryResponse)
	}
}

func TestWebFetchToolExecuteTruncatesOversizedHTMLResponse(t *testing.T) {
	main := `<html><body><main><h1>Apple Inc.</h1><p>Current quote page.</p></main>`
	filler := strings.Repeat("<div>market-data</div>", webFetchMaxResponseBytes/8)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(main + filler + "</body></html>"))
	}))
	defer server.Close()

	tl := WebFetchTool{client: server.Client()}
	result, err := tl.Execute(context.Background(), ExecutionContext{}, json.RawMessage(`{"url":"`+server.URL+`","method":null,"headers":null,"body":null,"format":"text","selector":"main"}`))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !strings.Contains(result.Output, "[source response truncated to first ") {
		t.Fatalf("output = %q", result.Output)
	}
	for _, want := range []string{"Apple Inc.", "Current quote page."} {
		if !strings.Contains(result.Output, want) {
			t.Fatalf("output = %q, missing %q", result.Output, want)
		}
	}
}

func TestWebFetchToolExecuteRejectsOversizedJSONResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":"` + strings.Repeat("x", webFetchMaxResponseBytes) + `"}`))
	}))
	defer server.Close()

	tl := WebFetchTool{client: server.Client()}
	_, err := tl.Execute(context.Background(), ExecutionContext{}, json.RawMessage(`{"url":"`+server.URL+`","method":null,"headers":null,"body":null,"format":"auto","selector":null}`))
	if err != ErrWebFetchBodyTooLarge {
		t.Fatalf("error = %v, want %v", err, ErrWebFetchBodyTooLarge)
	}
}

func TestWebFetchToolExecuteRejectsCrossHostRedirect(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
	defer target.Close()
	redirector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusFound)
	}))
	defer redirector.Close()

	tl := WebFetchTool{client: redirector.Client()}
	_, err := tl.Execute(context.Background(), ExecutionContext{}, json.RawMessage(`{"url":"`+redirector.URL+`","method":null,"headers":null,"body":null,"format":"auto","selector":null}`))
	if !errors.Is(err, ErrWebFetchCrossHostRedir) {
		t.Fatalf("error = %v, want %v", err, ErrWebFetchCrossHostRedir)
	}
}

func TestWebFetchToolExecuteRejectsSearchEngineURLs(t *testing.T) {
	_, err := NewWebFetchTool().Execute(context.Background(), ExecutionContext{}, json.RawMessage(`{"url":"https://www.google.com/search?q=kodacode","method":null,"headers":null,"body":null,"format":"auto","selector":null}`))
	if !errors.Is(err, ErrWebFetchURLSearch) {
		t.Fatalf("error = %v, want %v", err, ErrWebFetchURLSearch)
	}
	if !errors.Is(err, ErrInvalidArguments) {
		t.Fatalf("errors.Is(err, ErrInvalidArguments) = false, err = %v", err)
	}
}

func TestWebFetchToolNetworkRequestsAllowsGoogleFinancePage(t *testing.T) {
	requests, err := NewWebFetchTool().NetworkRequests(json.RawMessage(`{"url":"https://www.google.com/finance/quote/AAPL:NASDAQ","method":null,"headers":null,"body":null,"format":"markdown","selector":null}`))
	if err != nil {
		t.Fatalf("NetworkRequests() error = %v", err)
	}
	if len(requests) != 1 {
		t.Fatalf("requests = %#v", requests)
	}
	if requests[0].URL != "https://www.google.com/finance/quote/AAPL:NASDAQ" {
		t.Fatalf("URL = %q", requests[0].URL)
	}
}

func TestWebFetchSearchResultsURLRejectsKnownSearchPathsButAllowsContentPages(t *testing.T) {
	tests := []struct {
		rawURL string
		want   bool
	}{
		{rawURL: "https://www.google.com/search?q=kodacode", want: true},
		{rawURL: "https://www.google.com/finance/quote/AAPL:NASDAQ", want: false},
		{rawURL: "https://www.bing.com/search?q=kodacode", want: true},
		{rawURL: "https://duckduckgo.com/?q=kodacode", want: true},
		{rawURL: "https://duckduckgo.com/about", want: false},
		{rawURL: "https://search.yahoo.com/search?p=kodacode", want: true},
	}

	for _, tt := range tests {
		parsed, err := url.Parse(tt.rawURL)
		if err != nil {
			t.Fatalf("url.Parse(%q) error = %v", tt.rawURL, err)
		}
		if got := webFetchSearchResultsURL(parsed); got != tt.want {
			t.Fatalf("webFetchSearchResultsURL(%q) = %v, want %v", tt.rawURL, got, tt.want)
		}
	}
}

func TestWebFetchToolNormalizedInputKeyNormalizesEquivalentArguments(t *testing.T) {
	tool := NewWebFetchTool()
	first, err := tool.NormalizedInputKey(json.RawMessage(`{"url":"https://example.com/docs#intro","method":"get","headers":null,"body":null,"format":null,"selector":" main "}`))
	if err != nil {
		t.Fatalf("NormalizedInputKey(first) error = %v", err)
	}
	second, err := tool.NormalizedInputKey(json.RawMessage(`{"url":"https://EXAMPLE.com/docs","method":"GET","headers":{},"body":"","format":"auto","selector":"main"}`))
	if err != nil {
		t.Fatalf("NormalizedInputKey(second) error = %v", err)
	}
	if first != second {
		t.Fatalf("input keys differ:\nfirst=%s\nsecond=%s", first, second)
	}
}

func TestWebFetchToolRejectsTimeoutArgument(t *testing.T) {
	_, err := NewWebFetchTool().NetworkRequests(json.RawMessage(`{"url":"https://example.com/docs","timeout":5000}`))
	if !errors.Is(err, ErrInvalidArguments) {
		t.Fatalf("error = %v, want invalid arguments", err)
	}
	if !strings.Contains(err.Error(), "unknown field") || !strings.Contains(err.Error(), `"timeout"`) {
		t.Fatalf("error = %q", err)
	}
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
