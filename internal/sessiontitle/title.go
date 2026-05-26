package sessiontitle

import (
	"strings"
	"unicode"
)

var removableTitleChars = strings.NewReplacer(
	"*", "",
	"_", "",
	"`", "",
	"~", "",
	"“", "",
	"”", "",
	"\"", "",
	"‘", "",
	"’", "",
	"'", "",
)

// Normalize reduces a model-produced title to plain words so it renders cleanly
// in headers and persists without markdown or punctuation noise.
func Normalize(raw string) string {
	title := strings.Join(strings.Fields(strings.ReplaceAll(raw, "\n", " ")), " ")
	title = removableTitleChars.Replace(title)
	title = strings.TrimLeft(title, "#>- ")
	title = trimCaseInsensitivePrefix(title, "title:")
	title = trimCaseInsensitivePrefix(title, "kodacode:")
	title = lettersAndSpacesOnly(title)
	return strings.TrimSpace(strings.Join(strings.Fields(title), " "))
}

func lettersAndSpacesOnly(value string) string {
	var b strings.Builder
	lastSpace := true
	for _, r := range value {
		switch {
		case unicode.IsLetter(r):
			b.WriteRune(r)
			lastSpace = false
		case unicode.IsSpace(r):
			if !lastSpace {
				b.WriteByte(' ')
				lastSpace = true
			}
		default:
			if !lastSpace {
				b.WriteByte(' ')
				lastSpace = true
			}
		}
	}
	return b.String()
}

func trimCaseInsensitivePrefix(value, prefix string) string {
	if len(value) < len(prefix) {
		return value
	}
	if strings.EqualFold(value[:len(prefix)], prefix) {
		return strings.TrimSpace(value[len(prefix):])
	}
	return value
}
