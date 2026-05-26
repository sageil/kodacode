package tool

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	WebFetchToolName         = "web_fetch"
	webFetchDefaultTimeout   = 20 * time.Second
	webFetchMaxResponseBytes = 4 << 20
	webFetchMaxOutputChars   = 32 * 1024
)

var (
	ErrWebFetchURLRequired    = errors.New("url is required")
	ErrWebFetchURLInvalid     = errors.New("url must be a valid http or https URL")
	ErrWebFetchURLSearch      = errors.New("url must be a direct page URL, not a search engine")
	ErrWebFetchMethodInvalid  = errors.New(`method must be one of "GET", "POST", "PUT", "PATCH", "DELETE", or "HEAD"`)
	ErrWebFetchFormatInvalid  = errors.New(`format must be one of "auto", "markdown", "text", "json", or "raw"`)
	ErrWebFetchBodyMethod     = errors.New("body is only supported for POST, PUT, PATCH, or DELETE")
	ErrWebFetchBinaryResponse = errors.New("response content type is not supported for web_fetch")
	ErrWebFetchBodyTooLarge   = errors.New("response body exceeds the maximum size")
	ErrWebFetchCrossHostRedir = errors.New("redirect to a different host is not allowed")
)

type WebFetchTool struct {
	client *http.Client
}

type webFetchInput struct {
	URL      string
	Method   string
	Headers  map[string]string
	Body     string
	Format   string
	Selector string
}

type webFetchBodyData struct {
	Body            []byte
	SourceTruncated bool
}

func NewWebFetchTool() WebFetchTool {
	return WebFetchTool{}
}

func (WebFetchTool) Definition() Definition {
	return Definition{
		Name:                WebFetchToolName,
		Description:         "Fetch a specific http or https URL for research, documentation, and API inspection work. This tool follows only same-host redirects, supports direct HTTP methods when explicitly requested, and can format HTML as markdown or text plus pretty-print JSON. Use a direct URL, not a search engine results page.",
		ProviderDescription: "Fetch one direct http or https URL. Same-host redirects only; can format HTML, text, or JSON.",
		InputSchema:         json.RawMessage(`{"type":"object","properties":{"url":{"type":"string","description":"Direct http or https URL to fetch."},"method":{"type":["string","null"],"enum":["GET","POST","PUT","PATCH","DELETE","HEAD",null],"description":"Optional HTTP method. Use null or omit this field to accept the default GET method."},"headers":{"type":["object","null"],"additionalProperties":{"type":"string"},"description":"Optional HTTP headers such as Authorization or Cookie. Use null or omit this field when not needed."},"body":{"type":["string","null"],"description":"Optional request body for POST, PUT, PATCH, or DELETE. Do not send body with GET or HEAD. Use null or omit this field when not needed."},"format":{"type":["string","null"],"enum":["auto","markdown","text","json","raw",null],"description":"Use null or omit this field to accept the default auto mode. Auto converts HTML to readable markdown and pretty-prints JSON."},"selector":{"type":["string","null"],"description":"Optional CSS selector to extract a specific HTML region before formatting, such as main, article, #content, or .post-body. Use null or omit this field when not needed."}},"required":["url"],"additionalProperties":false}`),
		ArgumentExamples:    []string{`{"url":"https://example.com/docs","method":"GET","headers":null,"body":null,"format":"markdown","selector":"main"}`},
	}
}

func (WebFetchTool) NormalizedInputKey(args json.RawMessage) (string, error) {
	input, parsed, err := parseWebFetchInput(args)
	if err != nil {
		return "", err
	}
	key, err := json.Marshal(struct {
		URL      string            `json:"url"`
		Method   string            `json:"method"`
		Headers  map[string]string `json:"headers,omitempty"`
		Body     string            `json:"body,omitempty"`
		Format   string            `json:"format"`
		Selector string            `json:"selector,omitempty"`
	}{
		URL:      canonicalWebFetchURL(parsed),
		Method:   input.Method,
		Headers:  input.Headers,
		Body:     input.Body,
		Format:   input.Format,
		Selector: input.Selector,
	})
	if err != nil {
		return "", err
	}
	return string(key), nil
}

func (WebFetchTool) NetworkRequests(args json.RawMessage) ([]NetworkRequest, error) {
	input, parsed, err := parseWebFetchInput(args)
	if err != nil {
		return nil, err
	}
	target := webFetchNetworkTarget(parsed)
	if target == "" {
		return nil, ErrWebFetchURLInvalid
	}
	return []NetworkRequest{{
		Target:  target,
		URL:     canonicalWebFetchURL(parsed),
		Command: webFetchPermissionPreview(input, parsed),
		Reason:  "perform external HTTP request",
	}}, nil
}

func (t WebFetchTool) Execute(ctx context.Context, _ ExecutionContext, args json.RawMessage) (Result, error) {
	input, parsed, err := parseWebFetchInput(args)
	if err != nil {
		return Result{}, err
	}

	client := t.client
	if client == nil {
		client = &http.Client{}
	}
	client = cloneWebFetchClient(client)
	client.Timeout = webFetchDefaultTimeout
	client.CheckRedirect = sameHostRedirectPolicy(parsed)

	var reqBody io.Reader
	if input.Body != "" {
		reqBody = strings.NewReader(input.Body)
	}

	req, err := http.NewRequestWithContext(ctx, input.Method, parsed.String(), reqBody)
	if err != nil {
		return Result{}, err
	}
	req.Header.Set("User-Agent", "kodacode/web_fetch")
	for key, value := range input.Headers {
		req.Header.Set(key, value)
	}
	if input.Body != "" && req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := client.Do(req)
	if err != nil {
		return Result{}, err
	}
	defer resp.Body.Close() //nolint:errcheck

	bodyData, err := readWebFetchBody(resp.Body, resp.Header.Get("Content-Type"), input.Format)
	if err != nil {
		return Result{}, err
	}

	output, err := formatWebFetchResponse(bodyData.Body, resp.Header.Get("Content-Type"), input.Format, input.Selector)
	if err != nil {
		return Result{}, err
	}
	if bodyData.SourceTruncated {
		output = webFetchSourceTruncationNotice(webFetchMaxResponseBytes) + "\n\n" + output
	}
	output, truncated := truncateWebFetchOutput(output, webFetchMaxOutputChars)
	if truncated {
		output += "\n\n[output truncated to protect context budget]"
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		if output == "" {
			output = http.StatusText(resp.StatusCode)
		}
		return Result{Error: fmt.Sprintf("HTTP %d %s\n\n%s", resp.StatusCode, resp.Status, output)}, nil
	}
	return Result{Output: output}, nil
}

func parseWebFetchInput(args json.RawMessage) (_ webFetchInput, _ *url.URL, err error) {
	defer func() {
		err = normalizeToolInputError(WebFetchToolName, err)
	}()
	var raw struct {
		URL      string            `json:"url"`
		Method   *string           `json:"method"`
		Headers  map[string]string `json:"headers"`
		Body     *string           `json:"body"`
		Format   *string           `json:"format"`
		Selector *string           `json:"selector"`
	}
	if err := decodeWebFetchArgs(args, &raw); err != nil {
		return webFetchInput{}, nil, err
	}
	if strings.TrimSpace(raw.URL) == "" {
		return webFetchInput{}, nil, ErrWebFetchURLRequired
	}
	parsed, err := url.Parse(strings.TrimSpace(raw.URL))
	if err != nil || parsed == nil || parsed.Host == "" {
		return webFetchInput{}, nil, ErrWebFetchURLInvalid
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return webFetchInput{}, nil, ErrWebFetchURLInvalid
	}
	if webFetchSearchResultsURL(parsed) {
		return webFetchInput{}, nil, ErrWebFetchURLSearch
	}

	method := http.MethodGet
	if raw.Method != nil && strings.TrimSpace(*raw.Method) != "" {
		method = strings.ToUpper(strings.TrimSpace(*raw.Method))
	}
	switch method {
	case http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete, http.MethodHead:
	default:
		return webFetchInput{}, nil, ErrWebFetchMethodInvalid
	}

	format := "auto"
	if raw.Format != nil && strings.TrimSpace(*raw.Format) != "" {
		format = strings.TrimSpace(*raw.Format)
	}
	switch format {
	case "auto", "markdown", "text", "json", "raw":
	default:
		return webFetchInput{}, nil, ErrWebFetchFormatInvalid
	}

	headers := normalizeWebFetchHeaders(raw.Headers)
	selector := ""
	if raw.Selector != nil {
		selector = strings.TrimSpace(*raw.Selector)
	}
	body := ""
	if raw.Body != nil {
		body = *raw.Body
	}
	if body != "" && (method == http.MethodGet || method == http.MethodHead) {
		return webFetchInput{}, nil, ErrWebFetchBodyMethod
	}

	return webFetchInput{
		URL:      strings.TrimSpace(raw.URL),
		Method:   method,
		Headers:  headers,
		Body:     body,
		Format:   format,
		Selector: selector,
	}, parsed, nil
}

func decodeWebFetchArgs(args json.RawMessage, dst any) error {
	decoder := json.NewDecoder(bytes.NewReader(args))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		err = withArgumentDetail(args, err)
		return NormalizeArgumentError(WebFetchToolName, err)
	}
	return nil
}

func cloneWebFetchClient(client *http.Client) *http.Client {
	if client == nil {
		return &http.Client{}
	}
	copyClient := *client
	return &copyClient
}

func sameHostRedirectPolicy(origin *url.URL) func(*http.Request, []*http.Request) error {
	originTarget := webFetchNetworkTarget(origin)
	return func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return errors.New("too many redirects")
		}
		if webFetchNetworkTarget(req.URL) != originTarget {
			return ErrWebFetchCrossHostRedir
		}
		return nil
	}
}

func readWebFetchBody(body io.Reader, contentType, format string) (webFetchBodyData, error) {
	limited := io.LimitReader(body, webFetchMaxResponseBytes+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return webFetchBodyData{}, err
	}
	if len(data) > webFetchMaxResponseBytes {
		truncated := data[:webFetchMaxResponseBytes]
		if !webFetchCanTruncateBody(contentType, format, truncated) {
			return webFetchBodyData{}, ErrWebFetchBodyTooLarge
		}
		return webFetchBodyData{Body: truncated, SourceTruncated: true}, nil
	}
	return webFetchBodyData{Body: data}, nil
}

func webFetchCanTruncateBody(contentType, format string, body []byte) bool {
	switch format {
	case "json":
		return false
	case "auto":
		if webFetchJSONContent(contentType, body) {
			return false
		}
		return webFetchHTMLContent(contentType, body) || webFetchTextLikeContent(contentType, body)
	case "markdown":
		return webFetchHTMLContent(contentType, body) || webFetchTextLikeContent(contentType, body)
	case "text", "raw":
		return webFetchTextLikeContent(contentType, body)
	default:
		return false
	}
}

func webFetchSourceTruncationNotice(maxBytes int) string {
	return fmt.Sprintf("[source response truncated to first %d bytes before formatting]", maxBytes)
}

func canonicalWebFetchURL(u *url.URL) string {
	copyURL := *u
	copyURL.Fragment = ""
	copyURL.Host = strings.ToLower(copyURL.Host)
	return copyURL.String()
}

func webFetchPermissionPreview(input webFetchInput, parsed *url.URL) string {
	if input.Method == http.MethodGet {
		return "web_fetch " + canonicalWebFetchURL(parsed)
	}
	return "web_fetch " + input.Method + " " + canonicalWebFetchURL(parsed)
}

func webFetchNetworkTarget(u *url.URL) string {
	if u == nil {
		return ""
	}
	host := strings.ToLower(strings.TrimSpace(u.Hostname()))
	if host == "" {
		return ""
	}
	port := strings.TrimSpace(u.Port())
	if port == "" {
		return host
	}
	return host + ":" + port
}

func normalizeWebFetchHeaders(headers map[string]string) map[string]string {
	if len(headers) == 0 {
		return nil
	}
	normalized := make(map[string]string, len(headers))
	for key, value := range headers {
		trimmedKey := strings.TrimSpace(key)
		if trimmedKey == "" {
			continue
		}
		normalized[trimmedKey] = value
	}
	if len(normalized) == 0 {
		return nil
	}
	return normalized
}

func webFetchSearchResultsURL(u *url.URL) bool {
	if u == nil {
		return false
	}
	host := strings.ToLower(strings.TrimSpace(u.Hostname()))
	path := strings.Trim(strings.ToLower(strings.TrimSpace(u.EscapedPath())), "/")
	query := u.Query()
	switch host {
	case "google.com", "www.google.com":
		return path == "search"
	case "bing.com", "www.bing.com":
		return path == "search"
	case "search.yahoo.com":
		return true
	case "duckduckgo.com":
		return (path == "" || path == "html" || path == "lite") && strings.TrimSpace(query.Get("q")) != ""
	case "baidu.com", "www.baidu.com":
		return path == "s"
	case "yandex.com", "yandex.ru":
		return path == "search"
	default:
		return false
	}
}
