package app

import (
	"context"
	"errors"
	"strings"

	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/provider"
	"github.com/sageil/kodacode/internal/workspace"
)

type CreateSessionInput struct {
	SessionID                string
	WorkspaceRoot            string
	AdditionalWorkspaceRoots []string
	PermissionMode           PermissionMode
}

func (s *SessionService) CreateSession(ctx context.Context, input CreateSessionInput) (events.Event, error) {
	if strings.TrimSpace(input.SessionID) == "" {
		return events.Event{}, ErrSessionIDRequired
	}
	if strings.TrimSpace(input.WorkspaceRoot) == "" {
		return events.Event{}, ErrWorkspaceRootRequired
	}

	scope, err := workspace.New(input.WorkspaceRoot)
	if err != nil {
		return events.Event{}, err
	}
	additionalRoots, err := resolveAdditionalWorkspaceRoots(scope.Root(), input.AdditionalWorkspaceRoots)
	if err != nil {
		return events.Event{}, err
	}

	return s.append(ctx, events.Draft{
		SessionID: input.SessionID,
		TurnID:    sessionTurnID,
		Type:      events.TypeSessionConfigured,
		Payload: events.SessionConfiguredPayload{
			WorkspaceRoot:            scope.Root(),
			AdditionalWorkspaceRoots: additionalRoots,
			PermissionMode:           string(normalizePermissionMode(input.PermissionMode)),
		},
	})
}

func (s *SessionService) AddWorkspaceRoots(ctx context.Context, sessionID string, roots []string) (events.Event, error) {
	if strings.TrimSpace(sessionID) == "" {
		return events.Event{}, ErrSessionIDRequired
	}
	var workspaceRoot string
	var existingRoots []string
	err := s.Inspect(ctx, sessionID, func(state events.SessionState) error {
		workspaceRoot = state.WorkspaceRoot
		existingRoots = append(existingRoots[:0], state.AdditionalWorkspaceRoots...)
		return nil
	})
	if err != nil {
		return events.Event{}, err
	}
	if strings.TrimSpace(workspaceRoot) == "" {
		return events.Event{}, ErrSessionNotConfigured
	}
	resolved, err := resolveAdditionalWorkspaceRoots(workspaceRoot, roots, existingRoots)
	if err != nil {
		return events.Event{}, err
	}
	if len(resolved) == 0 {
		return events.Event{}, nil
	}
	return s.append(ctx, events.Draft{
		SessionID: sessionID,
		TurnID:    sessionTurnID,
		Type:      events.TypeSessionWorkspaceRootsAdded,
		Payload: events.SessionWorkspaceRootsAddedPayload{
			WorkspaceRoots: resolved,
		},
	})
}

func (s *SessionService) SetPermissionMode(ctx context.Context, sessionID string, mode PermissionMode) (events.Event, error) {
	if strings.TrimSpace(sessionID) == "" {
		return events.Event{}, ErrSessionIDRequired
	}
	normalized := normalizePermissionMode(mode)
	return s.append(ctx, events.Draft{
		SessionID: sessionID,
		TurnID:    sessionTurnID,
		Type:      events.TypeSessionPermissionModeUpdated,
		Payload: events.SessionPermissionModeUpdatedPayload{
			PermissionMode: string(normalized),
		},
	})
}

func (s *SessionService) SetModelRoute(ctx context.Context, sessionID string, route provider.ModelRoute) (events.Event, error) {
	if strings.TrimSpace(sessionID) == "" {
		return events.Event{}, ErrSessionIDRequired
	}
	if err := route.Validate(); err != nil {
		return events.Event{}, err
	}
	state, err := s.Snapshot(ctx, sessionID)
	if err != nil {
		return events.Event{}, err
	}
	if strings.TrimSpace(state.WorkspaceRoot) == "" {
		return events.Event{}, ErrSessionNotConfigured
	}
	if sessionModelRouteMatches(state, route) {
		return events.Event{}, nil
	}
	return s.append(ctx, events.Draft{
		SessionID: sessionID,
		TurnID:    sessionTurnID,
		Type:      events.TypeSessionModelRouteUpdated,
		Payload: events.SessionModelRouteUpdatedPayload{
			Model: route.Primary.String(),
		},
	})
}

func (s *SessionService) SetTitle(ctx context.Context, sessionID, title string) (events.Event, error) {
	if strings.TrimSpace(sessionID) == "" {
		return events.Event{}, ErrSessionIDRequired
	}
	if strings.TrimSpace(title) == "" {
		return events.Event{}, errors.New("title is required")
	}
	return s.append(ctx, events.Draft{
		SessionID: sessionID,
		TurnID:    sessionTurnID,
		Type:      events.TypeSessionTitleUpdated,
		Payload: events.SessionTitleUpdatedPayload{
			Title: strings.TrimSpace(title),
		},
	})
}

func resolveAdditionalWorkspaceRoots(primaryRoot string, roots []string, existing ...[]string) ([]string, error) {
	seen := map[string]struct{}{}
	if trimmed := strings.TrimSpace(primaryRoot); trimmed != "" {
		seen[trimmed] = struct{}{}
	}
	for _, group := range existing {
		for _, root := range group {
			if trimmed := strings.TrimSpace(root); trimmed != "" {
				seen[trimmed] = struct{}{}
			}
		}
	}
	resolved := make([]string, 0, len(roots))
	for _, root := range roots {
		scope, err := workspace.New(root)
		if err != nil {
			return nil, err
		}
		canonical := scope.Root()
		if _, ok := seen[canonical]; ok {
			continue
		}
		seen[canonical] = struct{}{}
		resolved = append(resolved, canonical)
	}
	return resolved, nil
}

func sessionModelRouteMatches(state events.SessionState, route provider.ModelRoute) bool {
	return strings.TrimSpace(state.Model) == strings.TrimSpace(route.Primary.String())
}
