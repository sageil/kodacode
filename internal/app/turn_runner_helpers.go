package app

import (
	"strings"

	"github.com/sageil/kodacode/internal/provider"
)

type toolCallAccumulator struct {
	ToolName string
	ToolKind provider.ToolKind
	Input    strings.Builder
}

func ensureToolCallAccumulator(calls map[string]*toolCallAccumulator, callID, toolName string) *toolCallAccumulator {
	if call, ok := calls[callID]; ok {
		if toolName != "" {
			call.ToolName = toolName
		}
		return call
	}
	call := &toolCallAccumulator{ToolName: toolName}
	calls[callID] = call
	return call
}

func requestToolNames(tools []provider.Tool) []string {
	if len(tools) == 0 {
		return nil
	}
	names := make([]string, 0, len(tools))
	for _, tool := range tools {
		names = append(names, tool.Name)
	}
	return names
}

func cloneAnthropicThinkingBlock(block *provider.AnthropicThinkingBlock) *provider.AnthropicThinkingBlock {
	if block == nil {
		return nil
	}
	copyBlock := *block
	return &copyBlock
}

func conversationToolStepStartIndex(inputs []provider.Input, callID, toolName string) int {
	callID = strings.TrimSpace(callID)
	toolName = strings.TrimSpace(toolName)
	if callID == "" || toolName == "" {
		return -1
	}
	for idx := len(inputs) - 1; idx >= 0; idx-- {
		input := inputs[idx]
		if input.Kind != provider.InputKindToolCall {
			continue
		}
		if input.CallID == callID && input.ToolName == toolName {
			for idx > 0 && inputs[idx-1].Kind == provider.InputKindAnthropicThinking {
				idx--
			}
			return idx
		}
	}
	return -1
}
