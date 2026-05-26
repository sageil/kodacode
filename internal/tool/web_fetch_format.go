package tool

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/PuerkitoBio/goquery"
	"golang.org/x/net/html"
)

func formatWebFetchResponse(body []byte, contentType, format, selector string) (string, error) {
	switch format {
	case "json":
		if !webFetchJSONContent(contentType, body) && !webFetchTextLikeContent(contentType, body) {
			return "", ErrWebFetchBinaryResponse
		}
		return prettyWebFetchJSON(body), nil
	case "raw":
		if !webFetchTextLikeContent(contentType, body) {
			return "", ErrWebFetchBinaryResponse
		}
		return string(body), nil
	case "text":
		return webFetchTextOutput(body, contentType, selector)
	case "markdown":
		return webFetchMarkdownOutput(body, contentType, selector)
	case "auto":
		if webFetchJSONContent(contentType, body) {
			return prettyWebFetchJSON(body), nil
		}
		if webFetchHTMLContent(contentType, body) {
			return htmlToWebFetchMarkdown(body, selector), nil
		}
		if !webFetchTextLikeContent(contentType, body) {
			return "", ErrWebFetchBinaryResponse
		}
		return string(body), nil
	default:
		return "", ErrWebFetchFormatInvalid
	}
}

func webFetchTextOutput(body []byte, contentType, selector string) (string, error) {
	if webFetchHTMLContent(contentType, body) {
		return extractSelectedWebFetchText(body, selector), nil
	}
	if !webFetchTextLikeContent(contentType, body) {
		return "", ErrWebFetchBinaryResponse
	}
	return string(body), nil
}

func webFetchMarkdownOutput(body []byte, contentType, selector string) (string, error) {
	if webFetchHTMLContent(contentType, body) {
		return htmlToWebFetchMarkdown(body, selector), nil
	}
	if !webFetchTextLikeContent(contentType, body) {
		return "", ErrWebFetchBinaryResponse
	}
	return string(body), nil
}

func webFetchHTMLContent(contentType string, body []byte) bool {
	return strings.Contains(strings.ToLower(contentType), "text/html") ||
		bytes.Contains(bytes.ToLower(body), []byte("<html")) ||
		bytes.Contains(bytes.ToLower(body), []byte("<body"))
}

func webFetchJSONContent(contentType string, body []byte) bool {
	lower := strings.ToLower(contentType)
	if strings.Contains(lower, "application/json") || strings.Contains(lower, "+json") {
		return true
	}
	trimmed := bytes.TrimSpace(body)
	return len(trimmed) > 0 && (trimmed[0] == '{' || trimmed[0] == '[')
}

func webFetchTextLikeContent(contentType string, body []byte) bool {
	lower := strings.ToLower(contentType)
	switch {
	case lower == "":
		return utf8.Valid(body) && !bytes.Contains(body, []byte{0})
	case strings.HasPrefix(lower, "text/"):
		return true
	case strings.Contains(lower, "json"):
		return true
	case strings.Contains(lower, "xml"):
		return true
	case strings.Contains(lower, "javascript"):
		return true
	default:
		return utf8.Valid(body) && !bytes.Contains(body, []byte{0})
	}
}

func prettyWebFetchJSON(body []byte) string {
	var value any
	if err := json.Unmarshal(body, &value); err != nil {
		return string(body)
	}
	formatted, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return string(body)
	}
	return string(formatted)
}

func selectWebFetchContent(body []byte, selector string) (*goquery.Selection, error) {
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(body))
	if err != nil {
		return nil, err
	}

	var selection *goquery.Selection
	if trimmedSelector := strings.TrimSpace(selector); trimmedSelector != "" {
		selection = doc.Find(trimmedSelector)
	}
	if selection == nil || selection.Length() == 0 {
		for _, candidate := range []string{"main", "article", "[role='main']"} {
			found := doc.Find(candidate)
			if found.Length() > 0 {
				selection = found.First()
				break
			}
		}
	}
	if selection == nil || selection.Length() == 0 {
		selection = doc.Find("body")
	}
	if selection.Length() == 0 {
		selection = doc.Selection
	}

	selection.Find("script, style, nav, noscript, svg, iframe, header, footer").Remove()
	selection.Find("[role='navigation'], [role='banner'], [role='contentinfo']").Remove()
	selection.Find(".navbar, .nav, .sidebar, .menu, .breadcrumb, .pagination").Remove()
	selection.Find(".cookie-banner, .cookie-consent, .ad, .advertisement").Remove()

	return selection, nil
}

func extractSelectedWebFetchText(body []byte, selector string) string {
	selection, err := selectWebFetchContent(body, selector)
	if err != nil {
		return string(body)
	}
	return cleanWebFetchWhitespace(selection.Text())
}

func htmlToWebFetchMarkdown(body []byte, selector string) string {
	selection, err := selectWebFetchContent(body, selector)
	if err != nil {
		return string(body)
	}

	var buf strings.Builder
	selection.Contents().Each(func(_ int, child *goquery.Selection) {
		nodeToWebFetchMarkdown(&buf, child, 0)
	})
	return cleanWebFetchWhitespace(buf.String())
}

func nodeToWebFetchMarkdown(buf *strings.Builder, selection *goquery.Selection, depth int) {
	if len(selection.Nodes) == 0 {
		return
	}
	node := selection.Nodes[0]

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

	tag := strings.ToLower(node.Data)
	children := selection.Contents()

	switch tag {
	case "h1", "h2", "h3", "h4", "h5", "h6":
		level := int(tag[1] - '0')
		buf.WriteString("\n")
		buf.WriteString(strings.Repeat("#", level))
		buf.WriteString(" ")
		buf.WriteString(strings.TrimSpace(selection.Text()))
		buf.WriteString("\n\n")
	case "p", "div", "section", "article", "main", "aside", "header", "footer":
		buf.WriteString("\n")
		children.Each(func(_ int, child *goquery.Selection) {
			nodeToWebFetchMarkdown(buf, child, depth)
		})
		buf.WriteString("\n")
	case "br":
		buf.WriteString("\n")
	case "strong", "b":
		text := strings.TrimSpace(selection.Text())
		if text != "" {
			buf.WriteString("**")
			buf.WriteString(text)
			buf.WriteString("**")
		}
	case "em", "i":
		text := strings.TrimSpace(selection.Text())
		if text != "" {
			buf.WriteString("*")
			buf.WriteString(text)
			buf.WriteString("*")
		}
	case "code":
		text := selection.Text()
		if text != "" {
			buf.WriteString("`")
			buf.WriteString(text)
			buf.WriteString("`")
		}
	case "pre":
		code := selection.Find("code")
		content := selection.Text()
		language := ""
		if code.Length() > 0 {
			content = code.Text()
			if classNames, ok := code.Attr("class"); ok {
				for _, className := range strings.Fields(classNames) {
					if after, ok := strings.CutPrefix(className, "language-"); ok {
						language = after
						break
					}
					if after, ok := strings.CutPrefix(className, "highlight-"); ok {
						language = after
						break
					}
				}
			}
		}
		buf.WriteString("\n```")
		buf.WriteString(language)
		buf.WriteString("\n")
		buf.WriteString(strings.TrimSpace(content))
		buf.WriteString("\n```\n")
	case "a":
		text := strings.TrimSpace(selection.Text())
		href, ok := selection.Attr("href")
		switch {
		case text != "" && ok && href != "" && !strings.HasPrefix(href, "#"):
			buf.WriteString("[")
			buf.WriteString(text)
			buf.WriteString("](")
			buf.WriteString(href)
			buf.WriteString(")")
		case text != "":
			buf.WriteString(text)
		}
	case "img":
		alt, _ := selection.Attr("alt")
		src, ok := selection.Attr("src")
		if ok && src != "" {
			buf.WriteString("![")
			buf.WriteString(alt)
			buf.WriteString("](")
			buf.WriteString(src)
			buf.WriteString(")")
		}
	case "ul", "ol":
		buf.WriteString("\n")
		selection.Children().Each(func(i int, item *goquery.Selection) {
			if strings.ToLower(goquery.NodeName(item)) != "li" {
				return
			}
			if tag == "ol" {
				fmt.Fprintf(buf, "%d. ", i+1)
			} else {
				buf.WriteString("- ")
			}
			var itemBuf strings.Builder
			item.Contents().Each(func(_ int, child *goquery.Selection) {
				nodeToWebFetchMarkdown(&itemBuf, child, depth+1)
			})
			buf.WriteString(strings.TrimSpace(itemBuf.String()))
			buf.WriteString("\n")
		})
		buf.WriteString("\n")
	case "blockquote":
		var blockquoteBuf strings.Builder
		children.Each(func(_ int, child *goquery.Selection) {
			nodeToWebFetchMarkdown(&blockquoteBuf, child, depth)
		})
		for _, line := range strings.Split(strings.TrimSpace(blockquoteBuf.String()), "\n") {
			buf.WriteString("> ")
			buf.WriteString(line)
			buf.WriteString("\n")
		}
		buf.WriteString("\n")
	case "table":
		renderWebFetchTable(buf, selection)
	case "hr":
		buf.WriteString("\n---\n\n")
	default:
		children.Each(func(_ int, child *goquery.Selection) {
			nodeToWebFetchMarkdown(buf, child, depth)
		})
	}
}

func renderWebFetchTable(buf *strings.Builder, table *goquery.Selection) {
	buf.WriteString("\n")

	var headers []string
	table.Find("thead tr").First().Find("th").Each(func(_ int, header *goquery.Selection) {
		headers = append(headers, strings.TrimSpace(header.Text()))
	})
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

	table.Find("tbody tr, tr").Each(func(i int, row *goquery.Selection) {
		if i == 0 && len(headers) > 0 && row.Find("th").Length() > 0 {
			return
		}
		var cells []string
		row.Find("td, th").Each(func(_ int, cell *goquery.Selection) {
			cells = append(cells, strings.TrimSpace(cell.Text()))
		})
		if len(cells) == 0 {
			return
		}
		buf.WriteString("| ")
		buf.WriteString(strings.Join(cells, " | "))
		buf.WriteString(" |\n")
	})
	buf.WriteString("\n")
}

func cleanWebFetchWhitespace(text string) string {
	lines := strings.Split(text, "\n")
	cleaned := make([]string, 0, len(lines))
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
			continue
		}
		cleaned = append(cleaned, trimmed)
	}
	return strings.TrimSpace(strings.Join(cleaned, "\n"))
}

func truncateWebFetchOutput(output string, maxChars int) (string, bool) {
	if maxChars <= 0 || utf8.RuneCountInString(output) <= maxChars {
		return output, false
	}
	runes := []rune(output)
	if maxChars == 1 {
		return string(runes[:1]), true
	}
	return string(runes[:maxChars-1]) + "…", true
}
