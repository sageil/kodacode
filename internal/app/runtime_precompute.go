package app

import (
	"context"
	"fmt"
	"strings"
	"time"
)

const runtimePrecomputeTimeout = 2 * time.Minute

type RuntimePrecomputeHint struct {
	SessionID     string
	TurnID        string
	WorkspaceRoot string
	Status        TurnRunStatus
	ChangedAt     time.Time
	Tags          []string
}

type RuntimePrecomputeHook interface {
	Precompute(context.Context, RuntimePrecomputeHint) error
}

type searchPrecomputeHook struct {
	search interface {
		RefreshWorkspace(context.Context, string) bool
	}
}

func (h searchPrecomputeHook) Precompute(ctx context.Context, hint RuntimePrecomputeHint) error {
	if h.search == nil || strings.TrimSpace(hint.WorkspaceRoot) == "" {
		return nil
	}
	h.search.RefreshWorkspace(ctx, hint.WorkspaceRoot)
	return nil
}

func (r *Runtime) triggerTurnPrecompute(ctx context.Context, sessionID, turnID string, status TurnRunStatus) {
	if r == nil || !shouldPrecomputeForTurnStatus(status) {
		return
	}
	hooks := r.runtimePrecomputeHooks()
	if len(hooks) == 0 {
		return
	}
	state, err := r.Sessions.Snapshot(ctx, sessionID)
	if err != nil {
		r.log("runtime").Error("turn precompute snapshot failed", err, "session_id", sessionID, "turn_id", turnID)
		return
	}
	workspaceRoot := strings.TrimSpace(state.WorkspaceRoot)
	if workspaceRoot == "" {
		return
	}
	hint := RuntimePrecomputeHint{
		SessionID:     sessionID,
		TurnID:        turnID,
		WorkspaceRoot: workspaceRoot,
		Status:        status,
		ChangedAt:     time.Now(),
		Tags:          []string{fmt.Sprintf("turn:%s", status)},
	}
	go r.runPrecomputeHooks(context.WithoutCancel(ctx), hooks, hint)
}

func (r *Runtime) runPrecomputeHooks(ctx context.Context, hooks []RuntimePrecomputeHook, hint RuntimePrecomputeHint) {
	ctx, cancel := context.WithTimeout(ctx, runtimePrecomputeTimeout)
	defer cancel()
	for _, hook := range hooks {
		if hook == nil {
			continue
		}
		if err := hook.Precompute(ctx, hint); err != nil {
			r.log("runtime").Error("turn precompute failed", err,
				"session_id", hint.SessionID,
				"turn_id", hint.TurnID,
				"workspace_root", hint.WorkspaceRoot,
				"status", hint.Status,
			)
		}
	}
}

func (r *Runtime) runtimePrecomputeHooks() []RuntimePrecomputeHook {
	if r == nil {
		return nil
	}
	if len(r.precomputeHooks) > 0 {
		return append([]RuntimePrecomputeHook(nil), r.precomputeHooks...)
	}
	hooks := append([]RuntimePrecomputeHook(nil), r.extensionPrecomputeHooks...)
	if r.Search != nil {
		hooks = append([]RuntimePrecomputeHook{searchPrecomputeHook{search: r.Search}}, hooks...)
	}
	return hooks
}

func shouldPrecomputeForTurnStatus(status TurnRunStatus) bool {
	switch status {
	case TurnRunStatusCompleted, TurnRunStatusFailed:
		return true
	default:
		return false
	}
}
