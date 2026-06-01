package app

import (
	"context"
	"errors"
	"slices"
	"sort"
	"strings"

	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/mcp"
	"github.com/sageil/kodacode/internal/tool"
	"github.com/sageil/kodacode/internal/workspace"
)

var ErrInvalidTrustScope = errors.New("invalid trust scope")

func (r *Runtime) EvaluateStartupTrust(ctx context.Context, workspaceRoot string) (StartupTrustState, error) {
	root, err := canonicalWorkspaceRoot(workspaceRoot)
	if err != nil {
		return StartupTrustState{}, err
	}
	workspaceRecord, ok, err := r.Trusts.WorkspaceTrust(ctx, root)
	if err != nil {
		return StartupTrustState{}, err
	}

	state := StartupTrustState{
		WorkspaceRoot: root,
	}
	if !ok || !workspaceRecord.Trusted {
		state.WorkspaceRequired = true
	}

	servers := phase1EnabledMCPServers(r.Config.MCP)
	for _, server := range servers {
		fingerprint, err := mcpServerFingerprint(server)
		if err != nil {
			return StartupTrustState{}, err
		}
		record, ok, err := r.Trusts.MCPTrust(ctx, root, fingerprint)
		if err != nil {
			return StartupTrustState{}, err
		}
		if ok && record.Trusted {
			continue
		}
		state.Servers = append(state.Servers, StartupTrustServer{
			Name:        strings.TrimSpace(server.Name),
			Type:        strings.TrimSpace(server.Type),
			Fingerprint: fingerprint,
			Command:     strings.TrimSpace(server.Command),
			URL:         strings.TrimSpace(server.URL),
			Args:        append([]string(nil), server.Args...),
			EnvKeys:     mcpServerEnvKeys(server),
		})
	}
	sort.Slice(state.Servers, func(i, j int) bool {
		return state.Servers[i].Name < state.Servers[j].Name
	})
	return state, nil
}

func (r *Runtime) ResolveStartupTrust(ctx context.Context, input ResolveStartupTrustInput) error {
	pending, err := r.EvaluateStartupTrust(ctx, input.WorkspaceRoot)
	if err != nil {
		return err
	}
	if pending.WorkspaceRequired {
		if err := r.Trusts.SetWorkspaceTrust(ctx, pending.WorkspaceRoot, input.TrustWorkspace); err != nil {
			return err
		}
	}
	for _, server := range pending.Servers {
		trusted := input.ServerDecisions[server.Fingerprint]
		if err := r.Trusts.SetMCPTrust(ctx, pending.WorkspaceRoot, server.Fingerprint, server.Type, server.Name, trusted); err != nil {
			return err
		}
	}
	return nil
}

func (r *Runtime) WorkspaceTrustState(ctx context.Context, workspaceRoot string) (WorkspaceTrustState, error) {
	root, err := canonicalWorkspaceRoot(workspaceRoot)
	if err != nil {
		return WorkspaceTrustState{}, err
	}
	state := WorkspaceTrustState{WorkspaceRoot: root}
	record, ok, err := r.Trusts.WorkspaceTrust(ctx, root)
	if err != nil {
		return WorkspaceTrustState{}, err
	}
	if ok && record.Trusted {
		state.Trusted = true
		state.UpdatedAt = record.UpdatedAt
	}
	records, err := r.Trusts.ListWorkspaceMCPTrust(ctx, root)
	if err != nil {
		return WorkspaceTrustState{}, err
	}
	for _, record := range records {
		if !record.Trusted {
			continue
		}
		state.Servers = append(state.Servers, WorkspaceMCPTrustState{
			Fingerprint: record.Fingerprint,
			Kind:        record.Kind,
			Label:       record.Label,
			Trusted:     true,
			UpdatedAt:   record.UpdatedAt,
		})
	}
	sort.Slice(state.Servers, func(i, j int) bool {
		if state.Servers[i].Label == state.Servers[j].Label {
			return state.Servers[i].Fingerprint < state.Servers[j].Fingerprint
		}
		return state.Servers[i].Label < state.Servers[j].Label
	})
	return state, nil
}

func (r *Runtime) RevokeTrust(ctx context.Context, input RevokeTrustInput) (WorkspaceTrustState, error) {
	root, err := canonicalWorkspaceRoot(input.WorkspaceRoot)
	if err != nil {
		return WorkspaceTrustState{}, err
	}
	switch input.Scope {
	case RevokeTrustScopeWorkspace:
		if err := r.Trusts.DeleteWorkspaceTrust(ctx, root); err != nil {
			return WorkspaceTrustState{}, err
		}
	case RevokeTrustScopeServer:
		if err := r.Trusts.DeleteMCPTrust(ctx, root, strings.TrimSpace(input.Fingerprint)); err != nil {
			return WorkspaceTrustState{}, err
		}
	case RevokeTrustScopeWorkspaceAll:
		if err := r.Trusts.DeleteWorkspaceMCPTrust(ctx, root); err != nil {
			return WorkspaceTrustState{}, err
		}
		if err := r.Trusts.DeleteWorkspaceTrust(ctx, root); err != nil {
			return WorkspaceTrustState{}, err
		}
	case RevokeTrustScopeAll:
		if err := r.Trusts.DeleteAllTrust(ctx); err != nil {
			return WorkspaceTrustState{}, err
		}
	default:
		return WorkspaceTrustState{}, ErrInvalidTrustScope
	}
	if err := r.activateWorkspaceMCP(ctx, root); err != nil {
		return WorkspaceTrustState{}, err
	}
	if err := r.appendSessionMCPCatalog(ctx, input.SessionID, root); err != nil {
		return WorkspaceTrustState{}, err
	}
	return r.WorkspaceTrustState(ctx, root)
}

func (r *Runtime) activateWorkspaceMCP(ctx context.Context, workspaceRoot string) error {
	root, err := canonicalWorkspaceRoot(workspaceRoot)
	if err != nil {
		return err
	}
	servers, fingerprints, err := r.trustedPhase1MCPServers(ctx, root)
	if err != nil {
		return err
	}

	r.mcpMu.Lock()
	defer r.mcpMu.Unlock()

	if root == strings.TrimSpace(r.mcpActiveWorkspace) && slices.Equal(fingerprints, r.mcpActiveFingerprints) {
		return nil
	}
	r.deactivateWorkspaceMCPLocked()
	if len(servers) == 0 {
		return nil
	}

	logger := r.log("runtime")
	registry, regErr := mcp.NewRegistry(ctx, mapMCPServers(servers))
	if regErr != nil {
		logger.Error("mcp init warning", regErr, "workspace_root", root)
	}
	tools, toolErr := registry.Tools(ctx)
	if toolErr != nil {
		logger.Error("mcp tool discovery warning", toolErr, "workspace_root", root)
	}
	r.Tools.ReplaceMCPTools(tools)
	r.mcpRegistry = registry
	r.mcpActiveWorkspace = root
	r.mcpActiveFingerprints = append([]string(nil), fingerprints...)
	return nil
}

func (r *Runtime) currentMCPTools(ctx context.Context) []tool.Tool {
	r.mcpMu.Lock()
	defer r.mcpMu.Unlock()
	if r.mcpRegistry == nil {
		return nil
	}
	tools, err := r.mcpRegistry.Tools(ctx)
	if err != nil {
		if logger := r.log("runtime"); logger != nil {
			logger.Error("mcp tool discovery failed", err, "workspace_root", r.mcpActiveWorkspace)
		}
		return nil
	}
	return tools
}

func (r *Runtime) deactivateWorkspaceMCPLocked() {
	if r.Tools != nil {
		r.Tools.ReplaceMCPTools(nil)
	}
	if r.mcpRegistry != nil {
		_ = r.mcpRegistry.Close()
	}
	r.mcpRegistry = nil
	r.mcpActiveWorkspace = ""
	r.mcpActiveFingerprints = nil
}

func (r *Runtime) trustedPhase1MCPServers(ctx context.Context, workspaceRoot string) ([]MCPServerConfig, []string, error) {
	servers := phase1EnabledMCPServers(r.Config.MCP)
	if len(servers) == 0 {
		return nil, nil, nil
	}
	workspaceRecord, ok, err := r.Trusts.WorkspaceTrust(ctx, workspaceRoot)
	if err != nil {
		return nil, nil, err
	}
	if !ok || !workspaceRecord.Trusted {
		return nil, nil, nil
	}
	active := make([]MCPServerConfig, 0, len(servers))
	fingerprints := make([]string, 0, len(servers))
	for _, server := range servers {
		fingerprint, err := mcpServerFingerprint(server)
		if err != nil {
			return nil, nil, err
		}
		record, ok, err := r.Trusts.MCPTrust(ctx, workspaceRoot, fingerprint)
		if err != nil {
			return nil, nil, err
		}
		if !ok || !record.Trusted {
			continue
		}
		active = append(active, server)
		fingerprints = append(fingerprints, fingerprint)
	}
	sort.Strings(fingerprints)
	return active, fingerprints, nil
}

func phase1EnabledMCPServers(config MCPConfig) []MCPServerConfig {
	if len(config.Servers) == 0 {
		return nil
	}
	out := make([]MCPServerConfig, 0, len(config.Servers))
	for _, server := range config.Servers {
		if !server.IsEnabled() || !supportedMCPServerType(server.Type) {
			continue
		}
		out = append(out, server)
	}
	sort.Slice(out, func(i, j int) bool {
		return strings.TrimSpace(out[i].Name) < strings.TrimSpace(out[j].Name)
	})
	return out
}

func mapMCPServers(servers []MCPServerConfig) []mcp.ServerConfig {
	if len(servers) == 0 {
		return nil
	}
	out := make([]mcp.ServerConfig, 0, len(servers))
	for _, server := range servers {
		out = append(out, mcp.ServerConfig{
			Name:    strings.TrimSpace(server.Name),
			Type:    strings.TrimSpace(server.Type),
			Command: strings.TrimSpace(server.Command),
			Args:    append([]string(nil), server.Args...),
			Env:     cloneStringMap(server.Env),
			URL:     strings.TrimSpace(server.URL),
			Headers: cloneStringMap(server.Headers),
		})
	}
	return out
}

func supportedMCPServerType(serverType string) bool {
	switch strings.ToLower(strings.TrimSpace(serverType)) {
	case "stdio", "http", "sse":
		return true
	default:
		return false
	}
}

func canonicalWorkspaceRoot(root string) (string, error) {
	scope, err := workspace.New(root)
	if err != nil {
		return "", err
	}
	return scope.Root(), nil
}

func (r *Runtime) appendSessionMCPCatalog(ctx context.Context, sessionID, workspaceRoot string) error {
	if strings.TrimSpace(sessionID) == "" {
		return nil
	}
	payload, err := r.sessionMCPCatalogPayload(ctx, workspaceRoot)
	if err != nil {
		return err
	}
	return r.appendSessionMCPCatalogPayload(ctx, sessionID, payload)
}

func (r *Runtime) appendSessionMCPCatalogPayload(ctx context.Context, sessionID string, payload events.SessionMCPCatalogUpdatedPayload) error {
	if strings.TrimSpace(sessionID) == "" {
		return nil
	}
	_, err := r.Sessions.append(ctx, events.Draft{
		SessionID: sessionID,
		TurnID:    sessionTurnID,
		Type:      events.TypeSessionMCPCatalogUpdated,
		Payload:   payload,
	})
	return err
}

func (r *Runtime) syncSessionMCPState(ctx context.Context, sessionID, workspaceRoot string, current *events.SessionMCPState) (*events.SessionMCPState, error) {
	if err := r.activateWorkspaceMCP(ctx, workspaceRoot); err != nil {
		return nil, err
	}
	payload, err := r.sessionMCPCatalogPayload(ctx, workspaceRoot)
	if err != nil {
		return nil, err
	}
	next := sessionMCPStateFromPayload(payload)
	if !sameSessionMCPState(current, next) {
		if err := r.appendSessionMCPCatalogPayload(ctx, sessionID, payload); err != nil {
			return nil, err
		}
	}
	return next, nil
}

func (r *Runtime) sessionMCPCatalogPayload(ctx context.Context, workspaceRoot string) (events.SessionMCPCatalogUpdatedPayload, error) {
	root, err := canonicalWorkspaceRoot(workspaceRoot)
	if err != nil {
		return events.SessionMCPCatalogUpdatedPayload{}, err
	}
	workspaceRecord, ok, err := r.Trusts.WorkspaceTrust(ctx, root)
	if err != nil {
		return events.SessionMCPCatalogUpdatedPayload{}, err
	}
	workspaceTrusted := ok && workspaceRecord.Trusted
	activeTools := r.currentMCPTools(ctx)
	r.mcpMu.Lock()
	activeFingerprints := append([]string(nil), r.mcpActiveFingerprints...)
	r.mcpMu.Unlock()

	servers := phase1EnabledMCPServers(r.Config.MCP)
	serverPayloads := make([]events.SessionMCPServerPayload, 0, len(servers))
	for _, server := range servers {
		fingerprint, err := mcpServerFingerprint(server)
		if err != nil {
			return events.SessionMCPCatalogUpdatedPayload{}, err
		}
		record, ok, err := r.Trusts.MCPTrust(ctx, root, fingerprint)
		if err != nil {
			return events.SessionMCPCatalogUpdatedPayload{}, err
		}
		serverPayloads = append(serverPayloads, events.SessionMCPServerPayload{
			Name:        strings.TrimSpace(server.Name),
			Type:        strings.TrimSpace(server.Type),
			Fingerprint: fingerprint,
			Trusted:     ok && record.Trusted,
			Active:      slices.Contains(activeFingerprints, fingerprint),
		})
	}
	toolPayloads := make([]events.SessionMCPToolPayload, 0, len(activeTools))
	for _, tl := range activeTools {
		definition := tl.Definition()
		serverName, remoteName := sessionMCPToolIdentity(definition.Name, serverPayloads)
		toolPayloads = append(toolPayloads, events.SessionMCPToolPayload{
			Name:        definition.Name,
			Description: definition.Description,
			InputSchema: string(definition.InputSchema),
			ServerName:  serverName,
			RemoteName:  remoteName,
		})
	}
	sort.Slice(serverPayloads, func(i, j int) bool {
		return serverPayloads[i].Name < serverPayloads[j].Name
	})
	sort.Slice(toolPayloads, func(i, j int) bool {
		return toolPayloads[i].Name < toolPayloads[j].Name
	})
	return events.SessionMCPCatalogUpdatedPayload{
		WorkspaceTrusted: workspaceTrusted,
		Servers:          serverPayloads,
		Tools:            toolPayloads,
	}, nil
}

func sessionMCPToolIdentity(toolName string, servers []events.SessionMCPServerPayload) (string, string) {
	name := strings.TrimSpace(toolName)
	serverComponent, remoteComponent, ok := tool.ParseMCPToolName(name)
	if !ok {
		return "", ""
	}
	for _, server := range servers {
		serverName := strings.TrimSpace(server.Name)
		if serverName == "" {
			continue
		}
		if tool.MCPToolNameComponent(serverName) == serverComponent {
			return serverName, remoteComponent
		}
	}
	return serverComponent, remoteComponent
}

func sessionMCPStateFromPayload(payload events.SessionMCPCatalogUpdatedPayload) *events.SessionMCPState {
	return &events.SessionMCPState{
		WorkspaceTrusted: payload.WorkspaceTrusted,
		Servers:          append([]events.SessionMCPServerPayload(nil), payload.Servers...),
		Tools:            append([]events.SessionMCPToolPayload(nil), payload.Tools...),
	}
}

func sameSessionMCPState(current, next *events.SessionMCPState) bool {
	switch {
	case current == nil || next == nil:
		return current == next
	case current.WorkspaceTrusted != next.WorkspaceTrusted:
		return false
	case len(current.Servers) != len(next.Servers):
		return false
	case len(current.Tools) != len(next.Tools):
		return false
	}
	for idx := range current.Servers {
		left := current.Servers[idx]
		right := next.Servers[idx]
		if left.Name != right.Name ||
			left.Type != right.Type ||
			left.Fingerprint != right.Fingerprint ||
			left.Trusted != right.Trusted ||
			left.Active != right.Active {
			return false
		}
	}
	for idx := range current.Tools {
		left := current.Tools[idx]
		right := next.Tools[idx]
		if left.Name != right.Name ||
			left.Description != right.Description ||
			left.InputSchema != right.InputSchema ||
			left.ServerName != right.ServerName ||
			left.RemoteName != right.RemoteName {
			return false
		}
	}
	return true
}
