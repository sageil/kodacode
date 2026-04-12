package tool_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/sageil/kodacode/v1/internal/tool"
)

func TestWebFetchTool_html(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = fmt.Fprint(w, `<html><body><h1>Hello</h1><p>World</p><script>alert("x")</script></body></html>`)
	}))
	defer srv.Close()

	tl := tool.NewWebFetchTool()
	args, _ := json.Marshal(map[string]any{"url": srv.URL})
	res, err := tl.Execute(t.Context(), tool.ExecutionContext{}, args)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Output, "Hello") {
		t.Fatalf("expected 'Hello' in output, got: %s", res.Output)
	}
	if !strings.Contains(res.Output, "World") {
		t.Fatalf("expected 'World' in output, got: %s", res.Output)
	}
	if strings.Contains(res.Output, "alert") {
		t.Fatalf("expected script content to be stripped, got: %s", res.Output)
	}
}

func TestWebFetchTool_json(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"key":"value","num":42}`)
	}))
	defer srv.Close()

	tl := tool.NewWebFetchTool()
	args, _ := json.Marshal(map[string]any{"url": srv.URL})
	res, err := tl.Execute(t.Context(), tool.ExecutionContext{}, args)
	if err != nil {
		t.Fatal(err)
	}
	// Should be pretty-printed.
	if !strings.Contains(res.Output, "  \"key\": \"value\"") {
		t.Fatalf("expected pretty-printed JSON, got: %s", res.Output)
	}
}

func TestWebFetchTool_raw(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = fmt.Fprint(w, "plain text content")
	}))
	defer srv.Close()

	tl := tool.NewWebFetchTool()
	args, _ := json.Marshal(map[string]any{"url": srv.URL, "format": "raw"})
	res, err := tl.Execute(t.Context(), tool.ExecutionContext{}, args)
	if err != nil {
		t.Fatal(err)
	}
	if res.Output != "plain text content" {
		t.Fatalf("expected raw content, got: %s", res.Output)
	}
}

func TestWebFetchTool_nonHTTP(t *testing.T) {
	tl := tool.NewWebFetchTool()
	args, _ := json.Marshal(map[string]any{"url": "ftp://example.com/file"})
	_, err := tl.Execute(t.Context(), tool.ExecutionContext{}, args)
	if err == nil {
		t.Fatal("expected error for ftp URL")
	}
	if !strings.Contains(err.Error(), "only http and https") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestWebFetchTool_non2xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer srv.Close()

	tl := tool.NewWebFetchTool()
	args, _ := json.Marshal(map[string]any{"url": srv.URL})
	res, err := tl.Execute(t.Context(), tool.ExecutionContext{}, args)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Output, "404") {
		t.Fatalf("expected 404 in output, got: %s", res.Output)
	}
}

func TestWebFetchTool_missingURL(t *testing.T) {
	tl := tool.NewWebFetchTool()
	args := []byte(`{}`)
	_, err := tl.Execute(t.Context(), tool.ExecutionContext{}, args)
	if err == nil {
		t.Fatal("expected error for missing url")
	}
}

func TestWebFetchTool_userAgent(t *testing.T) {
	var gotUA string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUA = r.Header.Get("User-Agent")
		_, _ = fmt.Fprint(w, "ok")
	}))
	defer srv.Close()

	tl := tool.NewWebFetchTool()
	args, _ := json.Marshal(map[string]any{"url": srv.URL})
	_, err := tl.Execute(t.Context(), tool.ExecutionContext{}, args)
	if err != nil {
		t.Fatal(err)
	}
	if gotUA != "kodacode/1.0" {
		t.Fatalf("expected User-Agent 'kodacode/1.0', got: %s", gotUA)
	}
}

func TestWebFetchTool_markdownFormat(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = fmt.Fprint(w, `<html><body>
			<h1>Title</h1>
			<p>Paragraph with <strong>bold</strong> and <a href="https://example.com">a link</a>.</p>
			<ul><li>Item 1</li><li>Item 2</li></ul>
			<pre><code class="language-go">fmt.Println("hi")</code></pre>
		</body></html>`)
	}))
	defer srv.Close()

	tl := tool.NewWebFetchTool()
	args, _ := json.Marshal(map[string]any{"url": srv.URL, "format": "markdown"})
	res, err := tl.Execute(t.Context(), tool.ExecutionContext{}, args)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Output, "# Title") {
		t.Fatalf("expected markdown heading, got: %s", res.Output)
	}
	if !strings.Contains(res.Output, "**bold**") {
		t.Fatalf("expected bold markdown, got: %s", res.Output)
	}
	if !strings.Contains(res.Output, "[a link](https://example.com)") {
		t.Fatalf("expected markdown link, got: %s", res.Output)
	}
	if !strings.Contains(res.Output, "- Item 1") {
		t.Fatalf("expected markdown list, got: %s", res.Output)
	}
	if !strings.Contains(res.Output, "```go") {
		t.Fatalf("expected fenced code block, got: %s", res.Output)
	}
}

func TestWebFetchTool_selector(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = fmt.Fprint(w, `<html><body>
			<header>Navigation stuff</header>
			<main><p>Main content here</p></main>
			<footer>Footer stuff</footer>
		</body></html>`)
	}))
	defer srv.Close()

	tl := tool.NewWebFetchTool()
	args, _ := json.Marshal(map[string]any{"url": srv.URL, "selector": "main"})
	res, err := tl.Execute(t.Context(), tool.ExecutionContext{}, args)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Output, "Main content") {
		t.Fatalf("expected main content, got: %s", res.Output)
	}
	if strings.Contains(res.Output, "Navigation") {
		t.Fatalf("expected header to be excluded, got: %s", res.Output)
	}
	if strings.Contains(res.Output, "Footer") {
		t.Fatalf("expected footer to be excluded, got: %s", res.Output)
	}
}

func TestWebFetchTool_postMethod(t *testing.T) {
	var gotMethod, gotBody, gotCT string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotCT = r.Header.Get("Content-Type")
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"ok":true}`)
	}))
	defer srv.Close()

	tl := tool.NewWebFetchTool()
	args, _ := json.Marshal(map[string]any{
		"url":    srv.URL,
		"method": "POST",
		"body":   `{"key":"value"}`,
	})
	res, err := tl.Execute(t.Context(), tool.ExecutionContext{}, args)
	if err != nil {
		t.Fatal(err)
	}
	if gotMethod != "POST" {
		t.Fatalf("expected POST, got: %s", gotMethod)
	}
	if gotBody != `{"key":"value"}` {
		t.Fatalf("expected body, got: %s", gotBody)
	}
	if gotCT != "application/json" {
		t.Fatalf("expected JSON content-type default, got: %s", gotCT)
	}
	if !strings.Contains(res.Output, `"ok": true`) {
		t.Fatalf("expected pretty-printed JSON, got: %s", res.Output)
	}
}

func TestWebFetchTool_customHeaders(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		_, _ = fmt.Fprint(w, "ok")
	}))
	defer srv.Close()

	tl := tool.NewWebFetchTool()
	args, _ := json.Marshal(map[string]any{
		"url":     srv.URL,
		"headers": map[string]string{"Authorization": "Bearer tok123"},
	})
	_, err := tl.Execute(t.Context(), tool.ExecutionContext{}, args)
	if err != nil {
		t.Fatal(err)
	}
	if gotAuth != "Bearer tok123" {
		t.Fatalf("expected auth header, got: %s", gotAuth)
	}
}

func TestWebFetchTool_metadata(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = fmt.Fprint(w, "hello")
	}))
	defer srv.Close()

	tl := tool.NewWebFetchTool()
	args, _ := json.Marshal(map[string]any{"url": srv.URL})
	res, err := tl.Execute(t.Context(), tool.ExecutionContext{}, args)
	if err != nil {
		t.Fatal(err)
	}
	if res.Metadata == nil {
		t.Fatal("expected metadata")
	}
	if res.Metadata["status_code"] != 200 {
		t.Fatalf("expected status 200, got: %v", res.Metadata["status_code"])
	}
	if !strings.Contains(res.Metadata["content_type"].(string), "text/plain") {
		t.Fatalf("expected text/plain content type, got: %v", res.Metadata["content_type"])
	}
}
