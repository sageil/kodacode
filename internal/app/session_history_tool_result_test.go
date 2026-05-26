package app

import (
	"strings"
	"testing"

	"github.com/sageil/kodacode/internal/provider"
	"github.com/sageil/kodacode/internal/tool"
)

func TestSessionHistoryToolResultInputKeepsLargeOutputVerbatim(t *testing.T) {
	large := strings.Repeat("x", 8192)
	input := sessionHistoryToolResultInputWithOutcome("call-1", tool.WebFetchToolName, large, "", true)
	if input.Output != large {
		t.Fatalf("input.Output length = %d, want %d", len(input.Output), len(large))
	}
}

func TestProviderToolResultInputKeepsReadErrorVerbatim(t *testing.T) {
	raw := "read failed for 1 path:\nsrc/repositories/ProposalRepository.ts: stat /repo/src/repositories/ProposalRepository.ts: no such file or directory"
	input := providerToolResultInput(
		"call-1",
		tool.ReadToolName,
		provider.ToolKindFunction,
		"",
		raw,
		false,
	)
	if input.Error != raw {
		t.Fatalf("input.Error = %q, want %q", input.Error, raw)
	}
}

func TestSessionHistoryToolResultInputKeepsReadErrorVerbatim(t *testing.T) {
	raw := strings.Join([]string{
		"read failed for 2 paths:",
		"src/repositories/ProposalRepository.ts: stat /repo/src/repositories/ProposalRepository.ts: no such file or directory",
		"src/repositories/TaskRepository.ts: stat /repo/src/repositories/TaskRepository.ts: no such file or directory",
	}, "\n")
	input := sessionHistoryToolResultInputWithOutcome(
		"call-1",
		tool.ReadToolName,
		"",
		raw,
		false,
	)
	if input.Error != raw {
		t.Fatalf("input.Error = %q, want %q", input.Error, raw)
	}
}

func TestProviderAndSessionHistoryToolResultErrorsMatch(t *testing.T) {
	raw := "read failed for 1 path:\nsrc: src is a directory, not a file"
	live := providerToolResultInput("call-1", tool.ReadToolName, provider.ToolKindFunction, "", raw, false)
	replayed := sessionHistoryToolResultInputWithOutcome("call-1", tool.ReadToolName, "", raw, false)
	if live.Error != raw {
		t.Fatalf("live Error = %q, want %q", live.Error, raw)
	}
	if replayed.Error != raw {
		t.Fatalf("replayed Error = %q, want %q", replayed.Error, raw)
	}
	if live.Error != replayed.Error {
		t.Fatalf("live Error = %q, replayed Error = %q", live.Error, replayed.Error)
	}
}
