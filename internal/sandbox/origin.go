// Package sandbox is the single dispatch point for all tool calls.
// It enforces path confinement, delete permissions, and tool name normalization.
package sandbox

// Origin identifies whether a request came from the TUI or the HTTP API.
type Origin int

const (
	OriginTUI Origin = iota // local CLI — 0, matches TurnRequest.Origin
	OriginAPI               // remote web client — 1
)
