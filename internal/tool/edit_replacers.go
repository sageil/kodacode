package tool

import "strings"

func simpleReplacer(content, old, new string) (string, bool, bool) {
	if !strings.Contains(content, old) {
		return "", false, false
	}
	idx := strings.Index(content, old)
	last := strings.LastIndex(content, old)
	if idx != last {
		return "", true, true
	}
	return strings.Replace(content, old, new, 1), true, false
}

func multiOccurrenceReplacer(content, old, new string) (string, bool, bool) {
	if !strings.Contains(content, old) {
		return "", false, false
	}
	return strings.ReplaceAll(content, old, new), true, false
}

func levenshtein(a, b string) int {
	la := len(a)
	lb := len(b)

	if la == 0 {
		return lb
	}
	if lb == 0 {
		return la
	}

	prev := make([]int, lb+1)
	curr := make([]int, lb+1)

	for j := 0; j <= lb; j++ {
		prev[j] = j
	}

	for i := 1; i <= la; i++ {
		curr[0] = i
		for j := 1; j <= lb; j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			curr[j] = min(
				prev[j]+1,
				curr[j-1]+1,
				prev[j-1]+cost,
			)
		}
		prev, curr = curr, prev
	}
	return prev[lb]
}
