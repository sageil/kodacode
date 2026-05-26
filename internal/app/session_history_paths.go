package app

import (
	"encoding/json"
	"strings"

	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/tool"
	"github.com/sageil/kodacode/internal/workspace"
)

type sessionHistoryMutationPathResolver func(toolName, rawArgs string) []string

var defaultSessionHistoryMutationTools = sessionHistoryToolIndex(tool.DefaultRuntimeTools())

func defaultSessionHistoryMutationPathResolver() sessionHistoryMutationPathResolver {
	return func(toolName, rawArgs string) []string {
		return mutationPathsFromToolMap(defaultSessionHistoryMutationTools, toolName, json.RawMessage(rawArgs))
	}
}

func sessionHistoryMutationPathResolverForExecutor(executor *ToolExecutor) sessionHistoryMutationPathResolver {
	if executor == nil {
		return defaultSessionHistoryMutationPathResolver()
	}
	return func(toolName, rawArgs string) []string {
		return executor.MutationPaths(toolName, json.RawMessage(rawArgs))
	}
}

func sessionHistoryToolIndex(tools []tool.Tool) map[string]tool.Tool {
	index := make(map[string]tool.Tool, len(tools))
	for _, tl := range tools {
		if tl == nil {
			continue
		}
		name := strings.TrimSpace(tl.Definition().Name)
		if name == "" {
			continue
		}
		index[name] = tl
	}
	return index
}

func mutationPathsFromToolMap(tools map[string]tool.Tool, toolName string, args json.RawMessage) []string {
	if len(tools) == 0 {
		return nil
	}
	tl, ok := tools[strings.TrimSpace(toolName)]
	if !ok {
		return nil
	}
	return mutationPathsFromTool(tl, args)
}

func mutationPathsFromTool(tl tool.Tool, args json.RawMessage) []string {
	introspector, ok := tl.(tool.PathIntrospector)
	if !ok {
		return nil
	}
	requests, err := introspector.PathRequests(args)
	if err != nil || len(requests) == 0 {
		return nil
	}
	paths := make([]string, 0, len(requests))
	for _, request := range requests {
		if request.Access != workspace.AccessWrite {
			continue
		}
		paths = appendUniqueValues(paths, []string{strings.TrimSpace(request.Path)})
	}
	return paths
}

func sessionHistoryMutationPathsFromCheckpoint(payload events.SessionHistoryTurnPayload) []string {
	return append([]string(nil), payload.WorkspacePaths...)
}
