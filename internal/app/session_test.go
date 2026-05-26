package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/permissionpolicy"
	"github.com/sageil/kodacode/internal/tool"
	"github.com/sageil/kodacode/internal/workspace"
)

func TestSessionServiceCreateSessionStoresCanonicalWorkspaceRoot(t *testing.T) {
	store := events.NewMemoryStore()
	service, err := NewSessionService(store)
	if err != nil {
		t.Fatalf("NewSessionService() error = %v", err)
	}

	root := t.TempDir()
	event, err := service.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: root,
	})
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	if event.Type != events.TypeSessionConfigured {
		t.Fatalf("event type = %q", event.Type)
	}

	state, err := service.Snapshot(context.Background(), "session-1")
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	if state.WorkspaceRoot == "" {
		t.Fatal("workspace root should be set")
	}
}

func TestSessionServiceCreateSessionStoresAdditionalWorkspaceRoots(t *testing.T) {
	store := events.NewMemoryStore()
	service, err := NewSessionService(store)
	if err != nil {
		t.Fatalf("NewSessionService() error = %v", err)
	}

	root := t.TempDir()
	extra := t.TempDir()
	extraScope, err := workspace.New(extra)
	if err != nil {
		t.Fatalf("workspace.New(extra) error = %v", err)
	}
	if _, err := service.CreateSession(context.Background(), CreateSessionInput{
		SessionID:                "session-1",
		WorkspaceRoot:            root,
		AdditionalWorkspaceRoots: []string{extra},
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	state, err := service.Snapshot(context.Background(), "session-1")
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	if len(state.AdditionalWorkspaceRoots) != 1 || state.AdditionalWorkspaceRoots[0] != extraScope.Root() {
		t.Fatalf("AdditionalWorkspaceRoots = %#v", state.AdditionalWorkspaceRoots)
	}
}

func TestSessionServiceAddWorkspaceRootsUpdatesSnapshot(t *testing.T) {
	store := events.NewMemoryStore()
	service, err := NewSessionService(store)
	if err != nil {
		t.Fatalf("NewSessionService() error = %v", err)
	}

	root := t.TempDir()
	extra := t.TempDir()
	extraScope, err := workspace.New(extra)
	if err != nil {
		t.Fatalf("workspace.New(extra) error = %v", err)
	}
	if _, err := service.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: root,
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	if _, err := service.AddWorkspaceRoots(context.Background(), "session-1", []string{extra}); err != nil {
		t.Fatalf("AddWorkspaceRoots() error = %v", err)
	}

	state, err := service.Snapshot(context.Background(), "session-1")
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	if len(state.AdditionalWorkspaceRoots) != 1 || state.AdditionalWorkspaceRoots[0] != extraScope.Root() {
		t.Fatalf("AdditionalWorkspaceRoots = %#v", state.AdditionalWorkspaceRoots)
	}
}

func TestSessionServiceSnapshotReloadPreservesReasoningTranscript(t *testing.T) {
	store := events.NewMemoryStore()
	service, err := NewSessionService(store)
	if err != nil {
		t.Fatalf("NewSessionService() error = %v", err)
	}

	if _, err := service.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: t.TempDir(),
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	if _, err := service.append(context.Background(), events.Draft{
		SessionID: "session-1",
		TurnID:    "turn-1",
		Type:      events.TypeReasoningDelta,
		Payload:   events.ReasoningDeltaPayload{Content: "Inspecting the provider contract."},
	}); err != nil {
		t.Fatalf("append(reasoning) error = %v", err)
	}
	if _, err := service.append(context.Background(), events.Draft{
		SessionID: "session-1",
		TurnID:    "turn-1",
		Type:      events.TypeAssistantCommit,
		Payload:   events.AssistantCommitPayload{Content: "Done."},
	}); err != nil {
		t.Fatalf("append(assistant) error = %v", err)
	}
	if _, err := service.append(context.Background(), events.Draft{
		SessionID: "session-1",
		TurnID:    "turn-1",
		Type:      events.TypeTurnDone,
		Payload:   events.TurnDonePayload{},
	}); err != nil {
		t.Fatalf("append(done) error = %v", err)
	}

	runtime := service.runtimeForSession("session-1")
	runtime.mu.Lock()
	if err := service.appendSessionSnapshotLocked(context.Background(), runtime, "session-1", runtime.projector.CurrentState().LastSequence); err != nil {
		runtime.mu.Unlock()
		t.Fatalf("appendSessionSnapshotLocked() error = %v", err)
	}
	runtime.mu.Unlock()

	reloaded, err := NewSessionService(store)
	if err != nil {
		t.Fatalf("NewSessionService(reloaded) error = %v", err)
	}
	state, err := reloaded.Snapshot(context.Background(), "session-1")
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	turn := state.Turns["turn-1"]
	if turn == nil {
		t.Fatal("turn = nil")
	}
	if turn.ReasoningText != "Inspecting the provider contract." {
		t.Fatalf("turn.ReasoningText = %q", turn.ReasoningText)
	}
	if len(turn.Transcript) != 2 {
		t.Fatalf("turn.Transcript = %#v", turn.Transcript)
	}
	if turn.Transcript[0].Kind != events.TranscriptEntryReasoning || turn.Transcript[0].Text != "Inspecting the provider contract." {
		t.Fatalf("turn.Transcript[0] = %#v", turn.Transcript[0])
	}
	if turn.Transcript[1].Kind != events.TranscriptEntryAssistant || turn.Transcript[1].Text != "Done." {
		t.Fatalf("turn.Transcript[1] = %#v", turn.Transcript[1])
	}
}

func TestSessionServiceInspectReadsCurrentState(t *testing.T) {
	store := events.NewMemoryStore()
	service, err := NewSessionService(store)
	if err != nil {
		t.Fatalf("NewSessionService() error = %v", err)
	}

	root := t.TempDir()
	scope, err := workspace.New(root)
	if err != nil {
		t.Fatalf("workspace.New(root) error = %v", err)
	}
	if _, err := service.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: root,
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	if _, err := service.append(context.Background(), events.Draft{
		SessionID: "session-1",
		TurnID:    "turn-1",
		Type:      events.TypeAssistantCommit,
		Payload:   events.AssistantCommitPayload{Content: "Done."},
	}); err != nil {
		t.Fatalf("append(assistant) error = %v", err)
	}

	var inspectedWorkspace string
	var inspectedTitle string
	var turnCount int
	if err := service.Inspect(context.Background(), "session-1", func(state events.SessionState) error {
		inspectedWorkspace = state.WorkspaceRoot
		inspectedTitle = state.Title
		turnCount = len(state.TurnOrder)
		return nil
	}); err != nil {
		t.Fatalf("Inspect() error = %v", err)
	}

	if inspectedWorkspace == "" || inspectedWorkspace != scope.Root() {
		t.Fatalf("inspected workspace = %q, want %q", inspectedWorkspace, scope.Root())
	}
	if inspectedTitle != "" {
		t.Fatalf("inspected title = %q, want empty", inspectedTitle)
	}
	if turnCount != 1 {
		t.Fatalf("turn count = %d, want 1", turnCount)
	}
}

func TestSessionServiceAuthorizeNetworkReturnsPendingAndEmitsRequest(t *testing.T) {
	store := events.NewMemoryStore()
	service, err := NewSessionService(store)
	if err != nil {
		t.Fatalf("NewSessionService() error = %v", err)
	}

	if _, err := service.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: t.TempDir(),
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	result, err := service.AuthorizeNetwork(context.Background(), NetworkAuthorizationInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		ToolCallID: "call-1",
		Target:     "external network",
		ToolName:   "bash",
		Command:    "curl https://example.com",
		Reason:     "requires network access for command execution",
	})
	if err != nil {
		t.Fatalf("AuthorizeNetwork() error = %v", err)
	}
	if result.Status != AuthorizationStatusPending || result.RequestID == "" {
		t.Fatalf("result = %#v", result)
	}

	state, err := service.Snapshot(context.Background(), "session-1")
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	pending := state.PendingPermissions[result.RequestID]
	if pending == nil {
		t.Fatalf("pending permission %q not found", result.RequestID)
	}
	if pending.Kind != events.PermissionRequestKindNetwork || pending.Path != "external network" {
		t.Fatalf("pending permission = %#v", pending)
	}
}

func TestSessionServiceAuthorizeNetworkAllowsWebFetchByPolicy(t *testing.T) {
	store := events.NewMemoryStore()
	service, err := NewSessionService(store)
	if err != nil {
		t.Fatalf("NewSessionService() error = %v", err)
	}
	if err := service.SetPermissionPolicy(permissionpolicy.Config{
		WebFetch: permissionpolicy.SubjectRules{
			{Pattern: "https://example.com/docs*", Action: permissionpolicy.ActionAllow},
		},
	}); err != nil {
		t.Fatalf("SetPermissionPolicy() error = %v", err)
	}
	if _, err := service.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: t.TempDir(),
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	result, err := service.AuthorizeNetwork(context.Background(), NetworkAuthorizationInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		ToolCallID: "call-1",
		Target:     "example.com",
		URL:        "https://example.com/docs?id=1",
		ToolName:   "web_fetch",
		Command:    "web_fetch https://example.com/docs?id=1",
		Reason:     "perform external HTTP request",
	})
	if err != nil {
		t.Fatalf("AuthorizeNetwork() error = %v", err)
	}
	if result.Status != AuthorizationStatusAuthorized {
		t.Fatalf("result = %#v", result)
	}
}

func TestSessionServiceAuthorizeNetworkDeniesWebFetchHostByPolicy(t *testing.T) {
	store := events.NewMemoryStore()
	service, err := NewSessionService(store)
	if err != nil {
		t.Fatalf("NewSessionService() error = %v", err)
	}
	if err := service.SetPermissionPolicy(permissionpolicy.Config{
		NetworkTarget: permissionpolicy.SubjectRules{
			{Pattern: "example.com", Action: permissionpolicy.ActionDeny},
		},
	}); err != nil {
		t.Fatalf("SetPermissionPolicy() error = %v", err)
	}
	if _, err := service.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: t.TempDir(),
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	_, err = service.AuthorizeNetwork(context.Background(), NetworkAuthorizationInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		ToolCallID: "call-1",
		Target:     "example.com",
		URL:        "https://example.com/docs?id=1",
		ToolName:   "web_fetch",
		Command:    "web_fetch https://example.com/docs?id=1",
		Reason:     "perform external HTTP request",
	})
	var denied PermissionPolicyDeniedError
	if !errors.As(err, &denied) {
		t.Fatalf("AuthorizeNetwork() error = %v, want PermissionPolicyDeniedError", err)
	}
	if denied.Subject != "permissions.network_target" || denied.Value != "example.com" {
		t.Fatalf("denied = %#v", denied)
	}
}

func TestSessionServiceAuthorizeNetworkAllowsCommandScopedTargetByPolicy(t *testing.T) {
	store := events.NewMemoryStore()
	service, err := NewSessionService(store)
	if err != nil {
		t.Fatalf("NewSessionService() error = %v", err)
	}
	target := tool.CommandScopedNetworkTarget("go test ./...", []string{"go", "test"})
	if err := service.SetPermissionPolicy(permissionpolicy.Config{
		NetworkTarget: permissionpolicy.SubjectRules{
			{Pattern: target, Action: permissionpolicy.ActionAllow},
		},
	}); err != nil {
		t.Fatalf("SetPermissionPolicy() error = %v", err)
	}
	if _, err := service.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: t.TempDir(),
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	result, err := service.AuthorizeNetwork(context.Background(), NetworkAuthorizationInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		ToolCallID: "call-1",
		Target:     target,
		ToolName:   "bash",
		Command:    "go test ./...",
		Reason:     "requires network access",
	})
	if err != nil {
		t.Fatalf("AuthorizeNetwork() error = %v", err)
	}
	if result.Status != AuthorizationStatusAuthorized {
		t.Fatalf("result = %#v", result)
	}
}

func TestSessionServiceAuthorizeNetworkDeniesCommandScopedTargetByPolicy(t *testing.T) {
	store := events.NewMemoryStore()
	service, err := NewSessionService(store)
	if err != nil {
		t.Fatalf("NewSessionService() error = %v", err)
	}
	target := tool.CommandScopedNetworkTarget("go test ./...", []string{"go", "test"})
	if err := service.SetPermissionPolicy(permissionpolicy.Config{
		NetworkTarget: permissionpolicy.SubjectRules{
			{Pattern: target, Action: permissionpolicy.ActionDeny},
		},
	}); err != nil {
		t.Fatalf("SetPermissionPolicy() error = %v", err)
	}
	if _, err := service.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: t.TempDir(),
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	_, err = service.AuthorizeNetwork(context.Background(), NetworkAuthorizationInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		ToolCallID: "call-1",
		Target:     target,
		ToolName:   "bash",
		Command:    "go test ./...",
		Reason:     "requires network access",
	})
	var denied PermissionPolicyDeniedError
	if !errors.As(err, &denied) {
		t.Fatalf("AuthorizeNetwork() error = %v, want PermissionPolicyDeniedError", err)
	}
	if denied.Subject != "permissions.network_target" || denied.Value != target {
		t.Fatalf("denied = %#v", denied)
	}
}

func TestSessionServiceAuthorizePathUsesPendingApprovalByDefaultOutsideWorkspace(t *testing.T) {
	store := events.NewMemoryStore()
	service, err := NewSessionService(store)
	if err != nil {
		t.Fatalf("NewSessionService() error = %v", err)
	}

	root := t.TempDir()
	outside := t.TempDir()
	if _, err := service.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: root,
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	result, err := service.AuthorizePath(context.Background(), PathAuthorizationInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		ToolCallID: "call-1",
		Path:       outside,
		Access:     workspace.AccessList,
		ToolName:   "locate",
		Command:    "locate " + outside,
		Reason:     "find filesystem paths",
	})
	if err != nil {
		t.Fatalf("AuthorizePath() error = %v", err)
	}
	if result.Status != AuthorizationStatusPending || result.RequestID == "" {
		t.Fatalf("result = %#v", result)
	}
}

func TestSessionServiceAuthorizePathAllowsExternalDirectoryByPolicy(t *testing.T) {
	store := events.NewMemoryStore()
	service, err := NewSessionService(store)
	if err != nil {
		t.Fatalf("NewSessionService() error = %v", err)
	}

	root := t.TempDir()
	outside := t.TempDir()
	outsideScope, err := workspace.New(outside)
	if err != nil {
		t.Fatalf("workspace.New(outside) error = %v", err)
	}
	if err := service.SetPermissionPolicy(permissionpolicy.Config{
		ExternalDirectory: permissionpolicy.SubjectRules{
			{Pattern: outsideScope.Root(), Action: permissionpolicy.ActionAllow},
		},
	}); err != nil {
		t.Fatalf("SetPermissionPolicy() error = %v", err)
	}
	if _, err := service.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: root,
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	result, err := service.AuthorizePath(context.Background(), PathAuthorizationInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		ToolCallID: "call-1",
		Path:       outside,
		Access:     workspace.AccessList,
		ToolName:   "locate",
		Command:    "locate " + outside,
		Reason:     "find filesystem paths",
	})
	if err != nil {
		t.Fatalf("AuthorizePath() error = %v", err)
	}
	if result.Status != AuthorizationStatusAuthorized {
		t.Fatalf("result = %#v", result)
	}
	if len(result.Grants) != 1 || result.Grants[0].Path != outsideScope.Root() || !result.Grants[0].Recursive {
		t.Fatalf("grants = %#v", result.Grants)
	}
}

func TestSessionServiceAuthorizePathDeniesExternalDirectoryByPolicy(t *testing.T) {
	store := events.NewMemoryStore()
	service, err := NewSessionService(store)
	if err != nil {
		t.Fatalf("NewSessionService() error = %v", err)
	}

	root := t.TempDir()
	outside := t.TempDir()
	outsideScope, err := workspace.New(outside)
	if err != nil {
		t.Fatalf("workspace.New(outside) error = %v", err)
	}
	if err := service.SetPermissionPolicy(permissionpolicy.Config{
		ExternalDirectory: permissionpolicy.SubjectRules{
			{Pattern: outsideScope.Root(), Action: permissionpolicy.ActionDeny},
		},
	}); err != nil {
		t.Fatalf("SetPermissionPolicy() error = %v", err)
	}
	if _, err := service.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: root,
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	_, err = service.AuthorizePath(context.Background(), PathAuthorizationInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		ToolCallID: "call-1",
		Path:       outside,
		Access:     workspace.AccessList,
		ToolName:   "locate",
		Command:    "locate " + outside,
		Reason:     "find filesystem paths",
	})
	var denied PermissionPolicyDeniedError
	if !errors.As(err, &denied) {
		t.Fatalf("AuthorizePath() error = %v, want PermissionPolicyDeniedError", err)
	}
}

func TestSessionServiceResolveNetworkPermissionAddsGrantAndAllowsFutureAccess(t *testing.T) {
	store := events.NewMemoryStore()
	service, err := NewSessionService(store)
	if err != nil {
		t.Fatalf("NewSessionService() error = %v", err)
	}

	if _, err := service.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: t.TempDir(),
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	pending, err := service.AuthorizeNetwork(context.Background(), NetworkAuthorizationInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		ToolCallID: "call-1",
		Target:     "external network",
		ToolName:   "bash",
		Command:    "curl https://example.com",
		Reason:     "requires network access for command execution",
	})
	if err != nil {
		t.Fatalf("AuthorizeNetwork() error = %v", err)
	}

	if _, err := service.ResolvePermission(context.Background(), ResolvePermissionInput{
		SessionID: "session-1",
		TurnID:    "turn-1",
		RequestID: pending.RequestID,
		Decision:  events.PermissionDecisionApproved,
		Scope:     events.PermissionScopeSession,
	}); err != nil {
		t.Fatalf("ResolvePermission() error = %v", err)
	}

	allowed, err := service.AuthorizeNetwork(context.Background(), NetworkAuthorizationInput{
		SessionID:  "session-1",
		TurnID:     "turn-2",
		ToolCallID: "call-2",
		Target:     "external network",
		ToolName:   "bash",
		Command:    "curl https://example.com",
		Reason:     "requires network access for command execution",
	})
	if err != nil {
		t.Fatalf("AuthorizeNetwork(after approval) error = %v", err)
	}
	if allowed.Status != AuthorizationStatusAuthorized {
		t.Fatalf("status = %q, want authorized", allowed.Status)
	}

	state, err := service.Snapshot(context.Background(), "session-1")
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	if len(state.NetworkGrants) != 1 || state.NetworkGrants[0].Target != "external network" {
		t.Fatalf("network grants = %#v", state.NetworkGrants)
	}
}

func TestSessionServiceAuthorizeNetworkAllowsTemporaryTarget(t *testing.T) {
	store := events.NewMemoryStore()
	service, err := NewSessionService(store)
	if err != nil {
		t.Fatalf("NewSessionService() error = %v", err)
	}

	if _, err := service.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: t.TempDir(),
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	allowed, err := service.AuthorizeNetwork(context.Background(), NetworkAuthorizationInput{
		SessionID:               "session-1",
		TurnID:                  "turn-1",
		ToolCallID:              "call-1",
		Target:                  "example.com",
		ToolName:                "web_fetch",
		Command:                 "web_fetch https://example.com/docs",
		Reason:                  "perform external HTTP request",
		TemporaryNetworkTargets: []string{"example.com"},
	})
	if err != nil {
		t.Fatalf("AuthorizeNetwork() error = %v", err)
	}
	if allowed.Status != AuthorizationStatusAuthorized {
		t.Fatalf("status = %q, want authorized", allowed.Status)
	}

	state, err := service.Snapshot(context.Background(), "session-1")
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	if len(state.PendingPermissions) != 0 {
		t.Fatalf("pending permissions = %#v", state.PendingPermissions)
	}
	if len(state.NetworkGrants) != 0 {
		t.Fatalf("network grants = %#v", state.NetworkGrants)
	}
}

func TestSessionServiceResolveExecutionNetworkApprovalForSessionDoesNotGrantPaths(t *testing.T) {
	store := events.NewMemoryStore()
	service, err := NewSessionService(store)
	if err != nil {
		t.Fatalf("NewSessionService() error = %v", err)
	}

	root := t.TempDir()
	if _, err := service.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: root,
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	pending, err := service.AuthorizeExecution(context.Background(), ExecutionAuthorizationInput{
		SessionID:             "session-1",
		TurnID:                "turn-1",
		ExecutionID:           "exec-call-1",
		ToolCallID:            "call-1",
		ToolName:              "bash",
		Command:               "./probe.sh",
		WorkingDir:            t.TempDir(),
		Reason:                "requires approval for network access",
		NetworkTargets:        []string{"command: ./probe.sh"},
		AvailableDecisions:    []events.ExecutionApprovalDecision{events.ExecutionApprovalDecisionAccept, events.ExecutionApprovalDecisionAcceptForSession, events.ExecutionApprovalDecisionDecline},
		ProposedNetworkPolicy: &events.ExecutionNetworkPolicyAmendment{Enabled: true},
	})
	if err != nil {
		t.Fatalf("AuthorizeExecution() error = %v", err)
	}
	if pending.Status != AuthorizationStatusPending {
		t.Fatalf("status = %q, want pending", pending.Status)
	}

	if _, err := service.ResolvePermission(context.Background(), ResolvePermissionInput{
		SessionID: "session-1",
		TurnID:    "turn-1",
		RequestID: pending.RequestID,
		Decision:  events.PermissionDecisionApproved,
		Scope:     events.PermissionScopeSession,
	}); err != nil {
		t.Fatalf("ResolvePermission() error = %v", err)
	}

	state, err := service.Snapshot(context.Background(), "session-1")
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	if len(state.WorkspaceGrants) != 0 {
		t.Fatalf("workspace grants = %#v, want none", state.WorkspaceGrants)
	}
	if len(state.ExecutionGrants) != 1 {
		t.Fatalf("execution grants = %#v", state.ExecutionGrants)
	}
	if len(state.ExecutionGrants[0].NetworkTargets) != 1 || state.ExecutionGrants[0].NetworkTargets[0] != "command: ./probe.sh" {
		t.Fatalf("execution grants = %#v", state.ExecutionGrants)
	}
	if len(state.NetworkGrants) != 1 || state.NetworkGrants[0].Target != "command: ./probe.sh" {
		t.Fatalf("network grants = %#v", state.NetworkGrants)
	}
}

func TestSessionServiceSetPermissionModeUpdatesSnapshot(t *testing.T) {
	store := events.NewMemoryStore()
	service, err := NewSessionService(store)
	if err != nil {
		t.Fatalf("NewSessionService() error = %v", err)
	}

	root := t.TempDir()
	if _, err := service.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: root,
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	if _, err := service.SetPermissionMode(context.Background(), "session-1", PermissionModeReadOnly); err != nil {
		t.Fatalf("SetPermissionMode() error = %v", err)
	}

	state, err := service.Snapshot(context.Background(), "session-1")
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	if state.PermissionMode != string(PermissionModeReadOnly) {
		t.Fatalf("permission mode = %q", state.PermissionMode)
	}
}

func TestSessionServiceColdAppendReloadsExistingStateBeforeMutation(t *testing.T) {
	store := events.NewMemoryStore()
	service, err := NewSessionService(store)
	if err != nil {
		t.Fatalf("NewSessionService() error = %v", err)
	}

	root := t.TempDir()
	scope, err := workspace.New(root)
	if err != nil {
		t.Fatalf("workspace.New(root) error = %v", err)
	}
	if _, err := service.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: root,
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	reloaded, err := NewSessionService(store)
	if err != nil {
		t.Fatalf("NewSessionService(reloaded) error = %v", err)
	}
	if _, err := reloaded.SetPermissionMode(context.Background(), "session-1", PermissionModeReadOnly); err != nil {
		t.Fatalf("SetPermissionMode(reloaded) error = %v", err)
	}

	state, err := reloaded.Snapshot(context.Background(), "session-1")
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	if state.WorkspaceRoot != scope.Root() {
		t.Fatalf("workspace root = %q, want %q", state.WorkspaceRoot, scope.Root())
	}
	if state.PermissionMode != string(PermissionModeReadOnly) {
		t.Fatalf("permission mode = %q, want %q", state.PermissionMode, PermissionModeReadOnly)
	}
}

func TestSessionServiceWatchStreamsPermissionEvents(t *testing.T) {
	store := events.NewMemoryStore()
	service, err := NewSessionService(store)
	if err != nil {
		t.Fatalf("NewSessionService() error = %v", err)
	}

	root := t.TempDir()
	if _, err := service.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: root,
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	watchCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	stream, err := service.Watch(watchCtx, "session-1", 0)
	if err != nil {
		t.Fatalf("Watch() error = %v", err)
	}

	if _, err := service.AuthorizeNetwork(context.Background(), NetworkAuthorizationInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		ToolCallID: "call-1",
		Target:     "example.com",
		ToolName:   "bash",
		Command:    `curl https://example.com`,
		Reason:     "requires network access",
	}); err != nil {
		t.Fatalf("AuthorizeNetwork() error = %v", err)
	}

	select {
	case event := <-stream:
		if event.Type != events.TypePermissionRequested {
			t.Fatalf("event type = %q, want permission_requested", event.Type)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for permission_requested event")
	}
}

func TestSessionServiceRequiresExistingRequestToResolve(t *testing.T) {
	store := events.NewMemoryStore()
	service, err := NewSessionService(store)
	if err != nil {
		t.Fatalf("NewSessionService() error = %v", err)
	}

	if _, err := service.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: t.TempDir(),
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	_, err = service.ResolvePermission(context.Background(), ResolvePermissionInput{
		SessionID: "session-1",
		TurnID:    "turn-1",
		RequestID: "missing",
		Decision:  events.PermissionDecisionDenied,
	})
	if !errors.Is(err, ErrPermissionRequestMissing) {
		t.Fatalf("ResolvePermission() error = %v, want ErrPermissionRequestMissing", err)
	}
}
