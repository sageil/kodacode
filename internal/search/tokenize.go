package search

import (
	"strings"
	"unicode"
)

// SplitTokens breaks a symbol name into space-separated lowercase tokens.
// Handles CamelCase, snake_case, acronyms (HTTPServer → http server),
// and mixed formats. The original name is preserved as the last token
// for exact-match boosting.
func SplitTokens(name string) string {
	var tokens []string
	var current strings.Builder

	flush := func() {
		if current.Len() > 0 {
			tokens = append(tokens, strings.ToLower(current.String()))
			current.Reset()
		}
	}

	runes := []rune(name)
	for i, r := range runes {
		switch {
		case r == '_' || r == '-' || r == '.':
			flush()
		case unicode.IsUpper(r):
			if i > 0 {
				prev := runes[i-1]
				if unicode.IsLower(prev) {
					flush()
				} else if unicode.IsUpper(prev) && i+1 < len(runes) && unicode.IsLower(runes[i+1]) {
					flush()
				}
			}
			current.WriteRune(r)
		default:
			current.WriteRune(r)
		}
	}
	flush()

	if len(tokens) == 0 {
		return strings.ToLower(name)
	}

	lower := strings.ToLower(name)
	if tokens[len(tokens)-1] != lower {
		tokens = append(tokens, lower)
	}
	return strings.Join(tokens, " ")
}
