package tui

import "strings"

func messageContentForTest(messages Messages) string {
	return strings.Join(messages.RawLines(), "\n")
}
