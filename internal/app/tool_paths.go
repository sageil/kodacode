package app

import (
	"encoding/json"

	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/tool"
	"github.com/sageil/kodacode/internal/workspace"
)

func resolvedToolPathsFromState(state events.SessionState, tl tool.Tool, args json.RawMessage, extraGrants ...workspace.Grant) ([]string, bool) {
	introspector, ok := tl.(tool.PathIntrospector)
	if !ok {
		return nil, false
	}
	requests, err := introspector.PathRequests(args)
	if err != nil || len(requests) == 0 {
		return nil, false
	}
	scope, err := scopeFromState(state, extraGrants...)
	if err != nil {
		return nil, false
	}
	paths := make([]string, 0, len(requests))
	seen := make(map[string]struct{}, len(requests))
	for _, request := range requests {
		decision, err := scope.Check(request.Access, request.Path)
		if err != nil {
			return nil, false
		}
		resolved := decision.ResolvedPath
		if _, ok := seen[resolved]; ok {
			continue
		}
		seen[resolved] = struct{}{}
		paths = append(paths, resolved)
	}
	return paths, true
}
