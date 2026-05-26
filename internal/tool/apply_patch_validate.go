package tool

import (
	"fmt"
	"strconv"
	"strings"
)

func detectSequentialReadLinePrefix(lines []string) (string, bool) {
	run := 0
	prev := -1
	first := ""
	for _, line := range lines {
		number, prefix, ok := parseReadLinePrefix(line)
		if !ok {
			run = 0
			prev = -1
			first = ""
			continue
		}
		if run == 0 || number != prev+1 {
			run = 1
			first = prefix
		} else {
			run++
		}
		prev = number
		if run >= 3 {
			return first, true
		}
	}
	return "", false
}

func detectAnyReadLinePrefix(lines []string) (string, bool) {
	for _, line := range lines {
		_, prefix, ok := parseReadLinePrefix(line)
		if ok {
			return prefix, true
		}
	}
	return "", false
}

func parseReadLinePrefix(line string) (int, string, bool) {
	trimmed := strings.TrimLeft(line, " \t")
	if trimmed == "" {
		return 0, "", false
	}
	colon := strings.IndexByte(trimmed, ':')
	if colon <= 0 || colon == len(trimmed)-1 || trimmed[colon+1] != ' ' {
		return 0, "", false
	}
	numberText := trimmed[:colon]
	for _, ch := range numberText {
		if ch < '0' || ch > '9' {
			return 0, "", false
		}
	}
	number, err := strconv.Atoi(numberText)
	if err != nil || number <= 0 {
		return 0, "", false
	}
	return number, fmt.Sprintf("%d:", number), true
}
