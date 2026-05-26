package events

import "strings"

type ToolResultBlobRef struct {
	Ref   string
	Bytes int
}

func (r ToolResultBlobRef) valid() bool {
	return strings.TrimSpace(r.Ref) != "" && r.Bytes > 0
}
