package app

import (
	"strings"

	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/tool"
)

func augmentTextMutationObservedResources(resources []tool.ObservedResource, mutation *events.WriteMutation, args textMutationArguments, ranges []tool.MutationRange) []tool.ObservedResource {
	if mutation == nil || !args.HasAfterContent || strings.TrimSpace(mutation.Path) == "" {
		return resources
	}

	version, ok, err := tool.CurrentObservedResourceVersion(tool.ObservedResourceFileContent, mutation.Path)
	if err != nil || !ok || strings.TrimSpace(version) == "" {
		return resources
	}
	state, _, _ := tool.CurrentObservedResourceState(tool.ObservedResourceFileContent, mutation.Path)
	totalLines := textMutationObservedLineCount(args.AfterContent)

	resource := tool.ObservedResource{
		Kind:       tool.ObservedResourceFileContent,
		Path:       mutation.Path,
		Version:    version,
		State:      strings.TrimSpace(state),
		TotalLines: totalLines,
	}
	switch strings.TrimSpace(args.ToolName) {
	case tool.WriteToolName:
		resource.Complete = true
	default:
		return resources
	}
	if resource.Complete && totalLines > 0 {
		resource.StartLine = 1
		resource.EndLine = totalLines
	}
	return append(resources, resource)
}

func textMutationObservedLineCount(content string) int {
	if content == "" {
		return 0
	}
	lines := strings.Count(content, "\n")
	if strings.HasSuffix(content, "\n") {
		return lines
	}
	return lines + 1
}
