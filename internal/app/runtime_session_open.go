package app

import (
	"context"
	"strings"

	"github.com/sageil/kodacode/internal/events"
)

type OpenWorkspaceSessionResult struct {
	SessionID string
	Resumed   bool
}

func (r *Runtime) OpenWorkspaceSession(ctx context.Context, workspaceRoot string, additionalRoots []string, resume bool) (OpenWorkspaceSessionResult, error) {
	if strings.TrimSpace(workspaceRoot) == "" {
		return OpenWorkspaceSessionResult{}, ErrWorkspaceRootRequired
	}
	if !resume {
		sessionID, err := r.CreateSessionWithRoots(ctx, workspaceRoot, additionalRoots)
		if err != nil {
			return OpenWorkspaceSessionResult{}, err
		}
		if err := r.activateWorkspaceMCP(ctx, workspaceRoot); err != nil {
			return OpenWorkspaceSessionResult{}, err
		}
		if err := r.appendSessionMCPCatalog(ctx, sessionID, workspaceRoot); err != nil {
			return OpenWorkspaceSessionResult{}, err
		}
		return OpenWorkspaceSessionResult{SessionID: sessionID}, nil
	}

	sessionID, ok, err := latestWorkspaceSessionID(ctx, r.Store, workspaceRoot)
	if err != nil {
		return OpenWorkspaceSessionResult{}, err
	}
	if !ok {
		sessionID, err = r.CreateSessionWithRoots(ctx, workspaceRoot, additionalRoots)
		if err != nil {
			return OpenWorkspaceSessionResult{}, err
		}
		if err := r.activateWorkspaceMCP(ctx, workspaceRoot); err != nil {
			return OpenWorkspaceSessionResult{}, err
		}
		if err := r.appendSessionMCPCatalog(ctx, sessionID, workspaceRoot); err != nil {
			return OpenWorkspaceSessionResult{}, err
		}
		return OpenWorkspaceSessionResult{SessionID: sessionID}, nil
	}

	if _, err := r.Sessions.AddWorkspaceRoots(ctx, sessionID, additionalRoots); err != nil {
		return OpenWorkspaceSessionResult{}, err
	}
	resumedWorkspaceRoot := ""
	if err := r.Sessions.Inspect(ctx, sessionID, func(state events.SessionState) error {
		resumedWorkspaceRoot = strings.TrimSpace(state.WorkspaceRoot)
		return nil
	}); err != nil {
		return OpenWorkspaceSessionResult{}, err
	}
	if r.Tools != nil {
		if err := r.Tools.ResumeBackgroundExecutions(ctx, sessionID); err != nil {
			return OpenWorkspaceSessionResult{}, err
		}
	}
	if r.Search != nil && resumedWorkspaceRoot != "" {
		r.Search.TrackWorkspace(resumedWorkspaceRoot, searchTrackOptions(r.Config.Search))
	}
	if err := r.activateWorkspaceMCP(ctx, resumedWorkspaceRoot); err != nil {
		return OpenWorkspaceSessionResult{}, err
	}
	if err := r.appendSessionMCPCatalog(ctx, sessionID, resumedWorkspaceRoot); err != nil {
		return OpenWorkspaceSessionResult{}, err
	}
	r.log("runtime").Op("session resumed", "session_id", sessionID, "workspace_root", resumedWorkspaceRoot)
	return OpenWorkspaceSessionResult{
		SessionID: sessionID,
		Resumed:   true,
	}, nil
}
