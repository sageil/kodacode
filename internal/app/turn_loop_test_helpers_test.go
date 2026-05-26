package app

import (
	"fmt"

	"github.com/sageil/kodacode/internal/provider"
)

func repeatedInvalidWriteStreams(count int, path string) []provider.Stream {
	streams := make([]provider.Stream, 0, count)
	for i := 1; i <= count; i++ {
		callID := fmt.Sprintf("call-loop-%d", i)
		streams = append(streams, provider.NewSliceStream([]provider.Event{
			{Kind: provider.EventKindToolCallDelta, ToolCallID: callID, ToolName: "write", InputDelta: fmt.Sprintf(`{"path":%q}`, path)},
			{Kind: provider.EventKindToolCallDone, ToolCallID: callID, ToolName: "write"},
		}))
	}
	return streams
}
