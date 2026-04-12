package provider

import "strings"

// SystemPartsToString joins SystemParts into a single system prompt string.
// Used by providers that don't support Anthropic-style two-part caching.
func SystemPartsToString(parts []string) string {
	var b strings.Builder
	for i, p := range parts {
		if i > 0 && p != "" {
			b.WriteString("\n\n")
		}
		b.WriteString(p)
	}
	return b.String()
}

// IsImageMIME returns true if the MIME type is an image type supported by providers.
func IsImageMIME(mime string) bool {
	switch mime {
	case "image/png", "image/jpeg", "image/gif", "image/webp":
		return true
	default:
		return false
	}
}

// ExtractBase64Data strips the data URI prefix (e.g. "data:image/png;base64,")
// and returns only the base64-encoded data.
func ExtractBase64Data(dataURI string) string {
	if _, after, ok := strings.Cut(dataURI, ","); ok {
		return after
	}
	return dataURI
}

// TextFromParts extracts concatenated text from a message's parts.
// Used by adapters that only support plain text content.
func TextFromParts(parts []MessagePart) string {
	var b strings.Builder
	for _, p := range parts {
		if tp, ok := p.(TextPart); ok {
			b.WriteString(tp.Text)
		}
	}
	return b.String()
}
