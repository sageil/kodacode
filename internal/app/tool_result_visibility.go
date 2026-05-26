package app

import (
	"strings"

	"github.com/sageil/kodacode/internal/provider"
)

const toolResultVisibilityInstruction = "Runtime note: Tool output may be hidden or summarized in the UI. Reuse prior tool results when sufficient. Only say you showed output if you include it in assistant text."

func applyToolResultVisibilityInstruction(instructions string, inputs []provider.Input) string {
	if !hasToolResultInput(inputs) {
		return instructions
	}
	if strings.Contains(instructions, toolResultVisibilityInstruction) {
		return instructions
	}
	if strings.TrimSpace(instructions) == "" {
		return toolResultVisibilityInstruction
	}
	return strings.TrimSpace(instructions) + "\n\n" + toolResultVisibilityInstruction
}

func applyToolResultVisibilityPrompt(instructions, cacheablePrefix, dynamicSuffix string, inputs []provider.Input) (string, string) {
	if strings.TrimSpace(cacheablePrefix) == "" && strings.TrimSpace(dynamicSuffix) == "" {
		return applyToolResultVisibilityInstruction(instructions, inputs), dynamicSuffix
	}
	if !hasToolResultInput(inputs) {
		return joinInstructionSections(cacheablePrefix, dynamicSuffix), dynamicSuffix
	}
	dynamicSuffix = appendInstructionSection(dynamicSuffix, toolResultVisibilityInstruction)
	return joinInstructionSections(cacheablePrefix, dynamicSuffix), dynamicSuffix
}

func joinInstructionSections(parts ...string) string {
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return strings.Join(out, "\n\n")
}

func appendInstructionSection(base, addition string) string {
	base = strings.TrimSpace(base)
	addition = strings.TrimSpace(addition)
	switch {
	case addition == "":
		return base
	case strings.Contains(base, addition):
		return base
	case base == "":
		return addition
	default:
		return base + "\n\n" + addition
	}
}

func hasToolResultInput(inputs []provider.Input) bool {
	for _, input := range inputs {
		if input.Kind == provider.InputKindToolResult {
			return true
		}
	}
	return false
}
