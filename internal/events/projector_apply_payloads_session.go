package events

import "strings"

func (p *Projector) applySessionPayload(event Event) (bool, error) {
	switch payload := event.Payload.(type) {
	case SessionConfiguredPayload:
		p.state.WorkspaceRoot = payload.WorkspaceRoot
		p.state.AdditionalWorkspaceRoots = append([]string(nil), payload.AdditionalWorkspaceRoots...)
		if strings.TrimSpace(payload.PermissionMode) == "" {
			p.state.PermissionMode = "auto"
		} else {
			p.state.PermissionMode = payload.PermissionMode
		}
		return true, nil
	case SessionStateSnapshotPayload:
		return true, nil
	case SessionModelRouteUpdatedPayload:
		p.state.Model = payload.Model
		return true, nil
	case WorkspaceWriteRestoredPayload:
		return true, nil
	case SessionWorkspaceRootsAddedPayload:
		existing := make(map[string]struct{}, len(p.state.AdditionalWorkspaceRoots)+1)
		existing[p.state.WorkspaceRoot] = struct{}{}
		for _, root := range p.state.AdditionalWorkspaceRoots {
			existing[root] = struct{}{}
		}
		for _, root := range payload.WorkspaceRoots {
			if _, ok := existing[root]; ok {
				continue
			}
			p.state.AdditionalWorkspaceRoots = append(p.state.AdditionalWorkspaceRoots, root)
			existing[root] = struct{}{}
		}
		return true, nil
	case SessionPermissionModeUpdatedPayload:
		p.state.PermissionMode = payload.PermissionMode
		return true, nil
	case SessionProviderLimitUpdatedPayload:
		p.state.ProviderRequestLimitDisabled = payload.ProviderRequestLimitDisabled
		return true, nil
	case SessionBranchedPayload:
		p.state.Branch = &SessionBranchState{
			ParentSessionID: payload.ParentSessionID,
			ParentTurnID:    payload.ParentTurnID,
			ParentSequence:  payload.ParentSequence,
		}
		return true, nil
	case SessionMCPCatalogUpdatedPayload:
		p.state.MCP = &SessionMCPState{
			WorkspaceTrusted: payload.WorkspaceTrusted,
			Servers:          append([]SessionMCPServerPayload(nil), payload.Servers...),
			Tools:            append([]SessionMCPToolPayload(nil), payload.Tools...),
		}
		return true, nil
	case SessionTitleUpdatedPayload:
		p.state.Title = payload.Title
		return true, nil
	case SessionHistoryCheckpointPayload:
		return true, nil
	default:
		return p.applyTurnWorkPayload(event)
	}
}
