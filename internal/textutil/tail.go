package textutil

import "unicode/utf8"

// AppendRuneTail appends chunk and keeps the last limit runes.
func AppendRuneTail(current, chunk string, limit int) string {
	if limit <= 0 {
		return current + chunk
	}
	combined := current + chunk
	if len(combined) <= limit {
		return combined
	}
	if utf8.RuneCountInString(combined) <= limit {
		return combined
	}
	start := len(combined)
	for kept := 0; kept < limit && start > 0; kept++ {
		_, size := utf8.DecodeLastRuneInString(combined[:start])
		start -= size
	}
	return combined[start:]
}
