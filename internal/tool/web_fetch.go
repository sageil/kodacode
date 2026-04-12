package tool

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"golang.org/x/net/html"
)

// maxBodySize is the maximum response body size (5MB).
const maxBodySize = 5 * 1024 * 1024

var webFetchParams = []byte(`{
	"type": "object",
	"properties": {
		"url": {"type": "string", "description": "The URL to fetch (http or https only)"},
		"method": {"type": "string", "enum": ["GET", "POST", "PUT", "PATCH", "DELETE", "HEAD"], "description": "HTTP method. Default: GET"},
		"headers": {"type": "object", "additionalProperties": {"type": "string"}, "description": "Custom request headers (e.g. Authorization, Cookie)"},
		"body": {"type": "string", "description": "Request body for POST/PUT/PATCH methods"},
		"format": {"type": "string", "enum": ["auto", "markdown", "text", "json", "raw"], "description": "Output format: auto detects from Content-Type (HTML→markdown, JSON→pretty-print), markdown converts HTML to readable markdown, text extracts plain text from HTML, json pretty-prints JSON, raw returns body as-is. Default: auto"},
		"selector": {"type": "string", "description": "CSS selector to extract specific content from HTML (e.g. 'main', 'article', '#content', '.post-body'). Applied before format conversion. Ignored for non-HTML responses"},
		"timeout": {"type": "number", "description": "Timeout in milliseconds (default: 30000)"}
	},
	"required": ["url"]
}`)

func NewWebFetchTool() *Tool {
	return &Tool{
		Name:        "web_fetch",
		ReadOnly:    true,
		Description: prompt("web_fetch"),
		Parameters:  webFetchParams,
		Execute:     executeWebFetch,
	}
}

func executeWebFetch(ctx context.Context, ectx ExecutionContext, args []byte) (*Result, error) {
	var params struct {
		URL      string            `json:"url"`
		Method   string            `json:"method"`
		Headers  map[string]string `json:"headers"`
		Body     string            `json:"body"`
		Format   string            `json:"format"`
		Selector string            `json:"selector"`
		Timeout  float64           `json:"timeout"`
	}
	if err := flexUnmarshal(args, &params); err != nil {
		return nil, err
	}
	if params.URL == "" {
		return nil, fmt.Errorf("url is required")
	}

	parsed, err := url.Parse(params.URL)
	if err != nil {
		return nil, fmt.Errorf("invalid url: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, fmt.Errorf("only http and https URLs are supported, got %q", parsed.Scheme)
	}
	if isSearchEngine(parsed.Host) {
		return nil, fmt.Errorf("web_fetch cannot search the web — it can only fetch specific URLs. Use a direct URL to the page you want")
	}

	if params.Method == "" {
		params.Method = http.MethodGet
	}
	params.Method = strings.ToUpper(params.Method)

	if params.Format == "" {
		params.Format = "auto"
	}

	timeout := DefaultWebFetchTimeout
	if params.Timeout > 0 {
		timeout = time.Duration(params.Timeout) * time.Millisecond
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	var reqBody io.Reader
	if params.Body != "" {
		reqBody = strings.NewReader(params.Body)
	}

	req, err := http.NewRequestWithContext(ctx, params.Method, params.URL, reqBody)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("User-Agent", "kodacode/1.0")

	for k, v := range params.Headers {
		req.Header.Set(k, v)
	}

	// If body is set and no Content-Type header was provided, default to JSON.
	if params.Body != "" && req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", "application/json")
	}

	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return fmt.Errorf("too many redirects (max 5)")
			}
			if req.URL.Host != via[0].URL.Host {
				return fmt.Errorf("redirect to different host %q blocked — re-fetch with the new URL if intended", req.URL.Host)
			}
			return nil
		},
	}

	abort := ectx.Abort
	if abort == nil {
		abort = make(chan struct{})
	}

	type fetchResult struct {
		resp *http.Response
		err  error
	}
	ch := make(chan fetchResult, 1)
	go func() {
		resp, err := client.Do(req)
		ch <- fetchResult{resp, err}
	}()

	var resp *http.Response
	select {
	case fr := <-ch:
		if fr.err != nil {
			return nil, fmt.Errorf("fetch: %w", fr.err)
		}
		resp = fr.resp
	case <-abort:
		cancel()
		return nil, fmt.Errorf("aborted")
	}
	defer resp.Body.Close() //nolint:errcheck

	title := fmt.Sprintf("%s %s", params.Method, params.URL)
	finalURL := resp.Request.URL.String()

	meta := map[string]any{
		"status_code":  resp.StatusCode,
		"content_type": resp.Header.Get("Content-Type"),
		"url":          params.URL,
	}
	if finalURL != params.URL {
		meta["final_url"] = finalURL
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		errBody := make([]byte, 1024)
		n, _ := io.ReadFull(resp.Body, errBody)
		return &Result{
			Title:    title,
			Output:   fmt.Sprintf("HTTP %d %s\n\n%s", resp.StatusCode, resp.Status, string(errBody[:n])),
			Metadata: meta,
		}, nil
	}

	limited := io.LimitReader(resp.Body, maxBodySize+1)
	body, err := io.ReadAll(limited)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}
	if len(body) > maxBodySize {
		return nil, fmt.Errorf("response body exceeds maximum size of 5MB")
	}

	meta["content_length"] = len(body)

	contentType := resp.Header.Get("Content-Type")
	output := formatBody(body, contentType, params.Format, params.Selector)

	tr := TruncateWithBudget(output, "head", ectx.ContextUsage)

	return &Result{
		Title:    title,
		Output:   tr.Content,
		Metadata: meta,
	}, nil
}

func formatBody(body []byte, contentType, format, selector string) string {
	ct := strings.ToLower(contentType)
	isHTML := strings.Contains(ct, "text/html")

	switch format {
	case "raw":
		return string(body)
	case "text":
		if isHTML {
			return extractSelectedText(body, selector)
		}
		return string(body)
	case "markdown":
		if isHTML {
			return htmlToMarkdown(body, selector)
		}
		return string(body)
	case "json":
		return prettyJSON(body)
	case "auto":
		if isHTML {
			return htmlToMarkdown(body, selector)
		}
		if strings.Contains(ct, "json") {
			return prettyJSON(body)
		}
		return string(body)
	default:
		return string(body)
	}
}

func prettyJSON(body []byte) string {
	var v any
	if err := json.Unmarshal(body, &v); err != nil {
		return string(body)
	}
	out, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return string(body)
	}
	return string(out)
}

func selectContent(body []byte, selector string) (*goquery.Selection, error) {
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	var sel *goquery.Selection
	if selector != "" {
		sel = doc.Find(selector)
		if sel.Length() == 0 {
			sel = doc.Find("body")
		}
	} else {
		// Prefer <main> or <article> over <body> for cleaner content.
		for _, s := range []string{"main", "article", "[role='main']"} {
			if found := doc.Find(s); found.Length() > 0 {
				sel = found.First()
				break
			}
		}
		if sel == nil {
			sel = doc.Find("body")
		}
	}
	sel.Find("script, style, nav, noscript, svg, iframe, header, footer").Remove()
	sel.Find("[role='navigation'], [role='banner'], [role='contentinfo']").Remove()
	sel.Find(".navbar, .nav, .sidebar, .menu, .breadcrumb, .pagination").Remove()
	sel.Find(".cookie-banner, .cookie-consent, .ad, .advertisement").Remove()
	return sel, nil
}

func extractSelectedText(body []byte, selector string) string {
	sel, err := selectContent(body, selector)
	if err != nil {
		return string(body)
	}
	return cleanWhitespace(sel.Text())
}

func htmlToMarkdown(body []byte, selector string) string {
	sel, err := selectContent(body, selector)
	if err != nil {
		return string(body)
	}

	var buf strings.Builder
	sel.Contents().Each(func(_ int, s *goquery.Selection) {
		nodeToMarkdown(&buf, s, 0)
	})

	return cleanWhitespace(buf.String())
}

func nodeToMarkdown(buf *strings.Builder, s *goquery.Selection, depth int) {
	if len(s.Nodes) == 0 {
		return
	}
	node := s.Nodes[0]

	if node.Type == html.TextNode {
		text := strings.TrimSpace(node.Data)
		if text != "" {
			buf.WriteString(text)
			buf.WriteString(" ")
		}
		return
	}

	if node.Type != html.ElementNode {
		return
	}

	{
		sel := s
		tag := strings.ToLower(node.Data)
		children := sel.Contents()

		switch tag {
		case "h1", "h2", "h3", "h4", "h5", "h6":
			level := int(tag[1] - '0')
			buf.WriteString("\n")
			buf.WriteString(strings.Repeat("#", level))
			buf.WriteString(" ")
			buf.WriteString(strings.TrimSpace(sel.Text()))
			buf.WriteString("\n\n")

		case "p", "div", "section", "article", "main", "aside", "header", "footer":
			buf.WriteString("\n")
			children.Each(func(_ int, child *goquery.Selection) {
				nodeToMarkdown(buf, child, depth)
			})
			buf.WriteString("\n")

		case "br":
			buf.WriteString("\n")

		case "strong", "b":
			text := strings.TrimSpace(sel.Text())
			if text != "" {
				buf.WriteString("**")
				buf.WriteString(text)
				buf.WriteString("**")
			}

		case "em", "i":
			text := strings.TrimSpace(sel.Text())
			if text != "" {
				buf.WriteString("*")
				buf.WriteString(text)
				buf.WriteString("*")
			}

		case "code":
			text := sel.Text()
			if text != "" {
				buf.WriteString("`")
				buf.WriteString(text)
				buf.WriteString("`")
			}

		case "pre":
			code := sel.Find("code")
			var content string
			if code.Length() > 0 {
				content = code.Text()
			} else {
				content = sel.Text()
			}
			lang := ""
			if code.Length() > 0 {
				if cls, exists := code.Attr("class"); exists {
					for _, c := range strings.Fields(cls) {
						if after, ok := strings.CutPrefix(c, "language-"); ok {
							lang = after
							break
						}
						if after, ok := strings.CutPrefix(c, "highlight-"); ok {
							lang = after
							break
						}
					}
				}
			}
			buf.WriteString("\n```")
			buf.WriteString(lang)
			buf.WriteString("\n")
			buf.WriteString(strings.TrimSpace(content))
			buf.WriteString("\n```\n")

		case "a":
			text := strings.TrimSpace(sel.Text())
			href, exists := sel.Attr("href")
			if text != "" && exists && href != "" && !strings.HasPrefix(href, "#") {
				buf.WriteString("[")
				buf.WriteString(text)
				buf.WriteString("](")
				buf.WriteString(href)
				buf.WriteString(")")
			} else if text != "" {
				buf.WriteString(text)
			}

		case "img":
			alt, _ := sel.Attr("alt")
			src, exists := sel.Attr("src")
			if exists && src != "" {
				buf.WriteString("![")
				buf.WriteString(alt)
				buf.WriteString("](")
				buf.WriteString(src)
				buf.WriteString(")")
			}

		case "ul", "ol":
			buf.WriteString("\n")
			sel.Children().Each(func(i int, li *goquery.Selection) {
				if strings.ToLower(goquery.NodeName(li)) == "li" {
					if tag == "ol" {
						fmt.Fprintf(buf, "%d. ", i+1)
					} else {
						buf.WriteString("- ")
					}
					var liBuf strings.Builder
					li.Contents().Each(func(_ int, child *goquery.Selection) {
						nodeToMarkdown(&liBuf, child, depth+1)
					})
					buf.WriteString(strings.TrimSpace(liBuf.String()))
					buf.WriteString("\n")
				}
			})
			buf.WriteString("\n")

		case "blockquote":
			var bqBuf strings.Builder
			children.Each(func(_ int, child *goquery.Selection) {
				nodeToMarkdown(&bqBuf, child, depth)
			})
			for _, line := range strings.Split(strings.TrimSpace(bqBuf.String()), "\n") {
				buf.WriteString("> ")
				buf.WriteString(line)
				buf.WriteString("\n")
			}
			buf.WriteString("\n")

		case "table":
			renderTable(buf, sel)

		case "hr":
			buf.WriteString("\n---\n\n")

		default:
			children.Each(func(_ int, child *goquery.Selection) {
				nodeToMarkdown(buf, child, depth)
			})
		}
	}
}

func renderTable(buf *strings.Builder, table *goquery.Selection) {
	buf.WriteString("\n")

	// Collect header row.
	var headers []string
	table.Find("thead tr").First().Find("th").Each(func(_ int, th *goquery.Selection) {
		headers = append(headers, strings.TrimSpace(th.Text()))
	})

	// If no thead, try first tr.
	if len(headers) == 0 {
		table.Find("tr").First().Find("th, td").Each(func(_ int, cell *goquery.Selection) {
			headers = append(headers, strings.TrimSpace(cell.Text()))
		})
	}

	if len(headers) > 0 {
		buf.WriteString("| ")
		buf.WriteString(strings.Join(headers, " | "))
		buf.WriteString(" |\n|")
		for range headers {
			buf.WriteString(" --- |")
		}
		buf.WriteString("\n")
	}

	// Collect body rows.
	table.Find("tbody tr, tr").Each(func(i int, tr *goquery.Selection) {
		// Skip header row if it was already rendered.
		if i == 0 && len(headers) > 0 && tr.Find("th").Length() > 0 {
			return
		}
		var cells []string
		tr.Find("td, th").Each(func(_ int, cell *goquery.Selection) {
			cells = append(cells, strings.TrimSpace(cell.Text()))
		})
		if len(cells) > 0 {
			buf.WriteString("| ")
			buf.WriteString(strings.Join(cells, " | "))
			buf.WriteString(" |\n")
		}
	})
	buf.WriteString("\n")
}

func cleanWhitespace(text string) string {
	lines := strings.Split(text, "\n")
	var cleaned []string
	blankCount := 0
	inCodeBlock := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") {
			inCodeBlock = !inCodeBlock
		}
		if trimmed == "" {
			blankCount++
			if blankCount <= 1 {
				cleaned = append(cleaned, "")
			}
			continue
		}
		blankCount = 0
		if inCodeBlock {
			cleaned = append(cleaned, line)
		} else {
			cleaned = append(cleaned, trimmed)
		}
	}
	return strings.TrimSpace(strings.Join(cleaned, "\n"))
}

func isSearchEngine(host string) bool {
	host = strings.ToLower(host)
	for _, s := range []string{
		"google.com", "www.google.com",
		"bing.com", "www.bing.com",
		"search.yahoo.com",
		"duckduckgo.com",
		"baidu.com", "www.baidu.com",
		"yandex.com", "yandex.ru",
	} {
		if host == s {
			return true
		}
	}
	return false
}
