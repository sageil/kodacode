package app

import (
	"context"
	"os"
	"strings"
	"testing"
)

func TestRuntimeEvaluateAndResolveStartupTrust(t *testing.T) {
	root := t.TempDir()
	store, err := newStartupTrustStore(root + "/kodacode.db")
	if err != nil {
		t.Fatalf("newStartupTrustStore() error = %v", err)
	}
	defer func() {
		_ = store.Close()
	}()

	runtime := &Runtime{
		Config: Config{
			MCP: MCPConfig{
				Servers: []MCPServerConfig{{
					Name:    "filesystem",
					Type:    "stdio",
					Command: "npx",
					Args:    []string{"-y", "@modelcontextprotocol/server-filesystem", root},
				}},
			},
		},
		Trusts: store,
	}

	pending, err := runtime.EvaluateStartupTrust(context.Background(), root)
	if err != nil {
		t.Fatalf("EvaluateStartupTrust() error = %v", err)
	}
	if !pending.WorkspaceRequired {
		t.Fatalf("pending = %#v, want workspace trust prompt", pending)
	}
	if len(pending.Servers) != 1 {
		t.Fatalf("pending servers = %#v", pending.Servers)
	}

	if err := runtime.ResolveStartupTrust(context.Background(), ResolveStartupTrustInput{
		WorkspaceRoot:  root,
		TrustWorkspace: true,
		ServerDecisions: map[string]bool{
			pending.Servers[0].Fingerprint: false,
		},
	}); err != nil {
		t.Fatalf("ResolveStartupTrust() error = %v", err)
	}

	pending, err = runtime.EvaluateStartupTrust(context.Background(), root)
	if err != nil {
		t.Fatalf("EvaluateStartupTrust() after resolve error = %v", err)
	}
	if pending.WorkspaceRequired {
		t.Fatalf("pending after resolve = %#v, want workspace already trusted", pending)
	}
	if len(pending.Servers) != 1 {
		t.Fatalf("pending servers after resolve = %#v, want denied MCP to prompt again", pending.Servers)
	}

	active, fingerprints, err := runtime.trustedPhase1MCPServers(context.Background(), root)
	if err != nil {
		t.Fatalf("trustedPhase1MCPServers() error = %v", err)
	}
	if len(active) != 0 || len(fingerprints) != 0 {
		t.Fatalf("trusted servers = %#v fingerprints = %#v, want none", active, fingerprints)
	}
}

func TestRuntimeEvaluateStartupTrustPromptsAgainWhenWorkspaceDenied(t *testing.T) {
	root := t.TempDir()
	canonicalRoot, err := canonicalWorkspaceRoot(root)
	if err != nil {
		t.Fatalf("canonicalWorkspaceRoot() error = %v", err)
	}
	store, err := newStartupTrustStore(root + "/kodacode.db")
	if err != nil {
		t.Fatalf("newStartupTrustStore() error = %v", err)
	}
	defer func() {
		_ = store.Close()
	}()

	runtime := &Runtime{
		Config: Config{
			MCP: MCPConfig{
				Servers: []MCPServerConfig{{
					Name:    "filesystem",
					Type:    "stdio",
					Command: "npx",
					Args:    []string{"-y", "@modelcontextprotocol/server-filesystem", root},
				}},
			},
		},
		Trusts: store,
	}
	if err := store.SetWorkspaceTrust(context.Background(), canonicalRoot, false); err != nil {
		t.Fatalf("SetWorkspaceTrust() error = %v", err)
	}

	pending, err := runtime.EvaluateStartupTrust(context.Background(), canonicalRoot)
	if err != nil {
		t.Fatalf("EvaluateStartupTrust() error = %v", err)
	}
	if !pending.WorkspaceRequired {
		t.Fatalf("pending = %#v, want workspace trust prompt", pending)
	}
	if len(pending.Servers) != 1 {
		t.Fatalf("pending servers = %#v, want server prompt alongside workspace trust", pending.Servers)
	}
}

func TestRuntimeEvaluateStartupTrustPromptsDeniedMCPAgain(t *testing.T) {
	root := t.TempDir()
	store, err := newStartupTrustStore(root + "/kodacode.db")
	if err != nil {
		t.Fatalf("newStartupTrustStore() error = %v", err)
	}
	defer func() {
		_ = store.Close()
	}()

	runtime := &Runtime{
		Config: Config{
			MCP: MCPConfig{
				Servers: []MCPServerConfig{{
					Name:    "filesystem",
					Type:    "stdio",
					Command: "npx",
					Args:    []string{"-y", "@modelcontextprotocol/server-filesystem", root},
				}},
			},
		},
		Trusts: store,
	}
	pending, err := runtime.EvaluateStartupTrust(context.Background(), root)
	if err != nil {
		t.Fatalf("EvaluateStartupTrust() error = %v", err)
	}
	if len(pending.Servers) != 1 {
		t.Fatalf("pending servers = %#v, want one server", pending.Servers)
	}
	if err := store.SetWorkspaceTrust(context.Background(), pending.WorkspaceRoot, true); err != nil {
		t.Fatalf("SetWorkspaceTrust() error = %v", err)
	}
	if err := store.SetMCPTrust(context.Background(), pending.WorkspaceRoot, pending.Servers[0].Fingerprint, pending.Servers[0].Type, pending.Servers[0].Name, false); err != nil {
		t.Fatalf("SetMCPTrust() error = %v", err)
	}

	pending, err = runtime.EvaluateStartupTrust(context.Background(), root)
	if err != nil {
		t.Fatalf("EvaluateStartupTrust() after deny error = %v", err)
	}
	if pending.WorkspaceRequired {
		t.Fatalf("pending = %#v, want workspace already trusted", pending)
	}
	if len(pending.Servers) != 1 {
		t.Fatalf("pending servers after deny = %#v, want denied server to remain promptable", pending.Servers)
	}
}

func TestRuntimeEvaluateStartupTrustExcludesDisabledMCPServers(t *testing.T) {
	root := t.TempDir()
	disabled := false
	store, err := newStartupTrustStore(root + "/kodacode.db")
	if err != nil {
		t.Fatalf("newStartupTrustStore() error = %v", err)
	}
	defer func() {
		_ = store.Close()
	}()

	runtime := &Runtime{
		Config: Config{
			MCP: MCPConfig{
				Servers: []MCPServerConfig{
					{
						Name:    "enabled-server",
						Type:    "stdio",
						Command: "npx",
						Args:    []string{"-y", "@modelcontextprotocol/server-filesystem", root},
					},
					{
						Name:    "disabled-server",
						Type:    "stdio",
						Command: "npx",
						Args:    []string{"-y", "@modelcontextprotocol/server-filesystem", root},
						Enabled: &disabled,
					},
				},
			},
		},
		Trusts: store,
	}

	pending, err := runtime.EvaluateStartupTrust(context.Background(), root)
	if err != nil {
		t.Fatalf("EvaluateStartupTrust() error = %v", err)
	}
	if len(pending.Servers) != 1 {
		t.Fatalf("pending servers = %#v, want only enabled MCP servers in prompt", pending.Servers)
	}
	if got := pending.Servers[0].Name; got != "enabled-server" {
		t.Fatalf("pending server name = %q, want enabled-server", got)
	}
}

func TestMCPServerFingerprintIgnoresDisplayName(t *testing.T) {
	serverA := MCPServerConfig{
		Name:    "filesystem",
		Type:    "stdio",
		Command: "npx",
		Args:    []string{"-y", "@modelcontextprotocol/server-filesystem"},
		Env: map[string]string{
			"ROOT": "/tmp/workspace",
		},
	}
	serverB := MCPServerConfig{
		Name:    "fs-local",
		Type:    "stdio",
		Command: "npx",
		Args:    []string{"-y", "@modelcontextprotocol/server-filesystem"},
		Env: map[string]string{
			"ROOT": "/tmp/workspace",
		},
	}
	fingerprintA, err := mcpServerFingerprint(serverA)
	if err != nil {
		t.Fatalf("mcpServerFingerprint(serverA) error = %v", err)
	}
	fingerprintB, err := mcpServerFingerprint(serverB)
	if err != nil {
		t.Fatalf("mcpServerFingerprint(serverB) error = %v", err)
	}
	if fingerprintA != fingerprintB {
		t.Fatalf("fingerprints differ for renamed server: %q != %q", fingerprintA, fingerprintB)
	}
}

func TestRuntimeResolveStartupTrustWhenOnlyServersArePending(t *testing.T) {
	root := t.TempDir()
	canonicalRoot, err := canonicalWorkspaceRoot(root)
	if err != nil {
		t.Fatalf("canonicalWorkspaceRoot() error = %v", err)
	}
	store, err := newStartupTrustStore(root + "/kodacode.db")
	if err != nil {
		t.Fatalf("newStartupTrustStore() error = %v", err)
	}
	defer func() {
		_ = store.Close()
	}()
	if err := store.SetWorkspaceTrust(context.Background(), canonicalRoot, true); err != nil {
		t.Fatalf("SetWorkspaceTrust() error = %v", err)
	}

	runtime := &Runtime{
		Config: Config{
			MCP: MCPConfig{
				Servers: []MCPServerConfig{{
					Name:    "filesystem",
					Type:    "stdio",
					Command: "npx",
					Args:    []string{"-y", "@modelcontextprotocol/server-filesystem", root},
				}},
			},
		},
		Trusts: store,
	}

	pending, err := runtime.EvaluateStartupTrust(context.Background(), root)
	if err != nil {
		t.Fatalf("EvaluateStartupTrust() error = %v", err)
	}
	if pending.WorkspaceRequired {
		t.Fatalf("pending = %#v, want workspace already trusted", pending)
	}
	if strings.TrimSpace(pending.WorkspaceRoot) != canonicalRoot {
		t.Fatalf("pending workspace root = %q, want %q", pending.WorkspaceRoot, canonicalRoot)
	}
	if len(pending.Servers) != 1 {
		t.Fatalf("pending servers = %#v", pending.Servers)
	}

	if err := runtime.ResolveStartupTrust(context.Background(), ResolveStartupTrustInput{
		WorkspaceRoot: canonicalRoot,
		ServerDecisions: map[string]bool{
			pending.Servers[0].Fingerprint: true,
		},
	}); err != nil {
		t.Fatalf("ResolveStartupTrust() error = %v", err)
	}

	active, fingerprints, err := runtime.trustedPhase1MCPServers(context.Background(), canonicalRoot)
	if err != nil {
		t.Fatalf("trustedPhase1MCPServers() error = %v", err)
	}
	if len(active) != 1 || len(fingerprints) != 1 {
		t.Fatalf("trusted servers = %#v fingerprints = %#v, want one trusted server", active, fingerprints)
	}
}

func TestRuntimeMCPTrustIsScopedPerWorkspace(t *testing.T) {
	root := t.TempDir()
	workspaceA := root + "/project-a"
	workspaceB := root + "/project-b"
	for _, dir := range []string{workspaceA, workspaceB} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("MkdirAll(%q) error = %v", dir, err)
		}
	}
	canonicalA, err := canonicalWorkspaceRoot(workspaceA)
	if err != nil {
		t.Fatalf("canonicalWorkspaceRoot(project-a) error = %v", err)
	}
	canonicalB, err := canonicalWorkspaceRoot(workspaceB)
	if err != nil {
		t.Fatalf("canonicalWorkspaceRoot(project-b) error = %v", err)
	}

	store, err := newStartupTrustStore(root + "/kodacode.db")
	if err != nil {
		t.Fatalf("newStartupTrustStore() error = %v", err)
	}
	defer func() {
		_ = store.Close()
	}()

	runtime := &Runtime{
		Config: Config{
			MCP: MCPConfig{
				Servers: []MCPServerConfig{{
					Name:    "filesystem",
					Type:    "stdio",
					Command: "npx",
					Args:    []string{"-y", "@modelcontextprotocol/server-filesystem", "/shared"},
				}},
			},
		},
		Trusts: store,
	}

	pendingA, err := runtime.EvaluateStartupTrust(context.Background(), canonicalA)
	if err != nil {
		t.Fatalf("EvaluateStartupTrust(project-a) error = %v", err)
	}
	if len(pendingA.Servers) != 1 {
		t.Fatalf("pending servers for project-a = %#v, want one server", pendingA.Servers)
	}
	if err := runtime.ResolveStartupTrust(context.Background(), ResolveStartupTrustInput{
		WorkspaceRoot:  canonicalA,
		TrustWorkspace: true,
		ServerDecisions: map[string]bool{
			pendingA.Servers[0].Fingerprint: true,
		},
	}); err != nil {
		t.Fatalf("ResolveStartupTrust(project-a) error = %v", err)
	}

	activeA, fingerprintsA, err := runtime.trustedPhase1MCPServers(context.Background(), canonicalA)
	if err != nil {
		t.Fatalf("trustedPhase1MCPServers(project-a) error = %v", err)
	}
	if len(activeA) != 1 || len(fingerprintsA) != 1 {
		t.Fatalf("trusted servers for project-a = %#v fingerprints = %#v, want one trusted server", activeA, fingerprintsA)
	}

	pendingB, err := runtime.EvaluateStartupTrust(context.Background(), canonicalB)
	if err != nil {
		t.Fatalf("EvaluateStartupTrust(project-b) error = %v", err)
	}
	if !pendingB.WorkspaceRequired {
		t.Fatalf("pending for project-b = %#v, want workspace trust prompt", pendingB)
	}
	if len(pendingB.Servers) != 1 {
		t.Fatalf("pending servers for project-b = %#v, want same server to prompt again", pendingB.Servers)
	}

	activeB, fingerprintsB, err := runtime.trustedPhase1MCPServers(context.Background(), canonicalB)
	if err != nil {
		t.Fatalf("trustedPhase1MCPServers(project-b) error = %v", err)
	}
	if len(activeB) != 0 || len(fingerprintsB) != 0 {
		t.Fatalf("trusted servers for project-b = %#v fingerprints = %#v, want none", activeB, fingerprintsB)
	}
}

func TestRuntimeEvaluateStartupTrustRequiresWorkspaceWithoutMCP(t *testing.T) {
	root := t.TempDir()
	store, err := newStartupTrustStore(root + "/kodacode.db")
	if err != nil {
		t.Fatalf("newStartupTrustStore() error = %v", err)
	}
	defer func() {
		_ = store.Close()
	}()

	runtime := &Runtime{
		Config: Config{},
		Trusts: store,
	}

	pending, err := runtime.EvaluateStartupTrust(context.Background(), root)
	if err != nil {
		t.Fatalf("EvaluateStartupTrust() error = %v", err)
	}
	if !pending.WorkspaceRequired {
		t.Fatalf("pending = %#v, want workspace trust prompt", pending)
	}
	if len(pending.Servers) != 0 {
		t.Fatalf("pending servers = %#v, want none", pending.Servers)
	}
}

func TestRuntimeWorkspaceTrustStateListsTrustedServersOnly(t *testing.T) {
	root := t.TempDir()
	canonicalRoot, err := canonicalWorkspaceRoot(root)
	if err != nil {
		t.Fatalf("canonicalWorkspaceRoot() error = %v", err)
	}
	store, err := newStartupTrustStore(root + "/kodacode.db")
	if err != nil {
		t.Fatalf("newStartupTrustStore() error = %v", err)
	}
	defer func() {
		_ = store.Close()
	}()
	if err := store.SetWorkspaceTrust(context.Background(), canonicalRoot, true); err != nil {
		t.Fatalf("SetWorkspaceTrust() error = %v", err)
	}
	if err := store.SetMCPTrust(context.Background(), canonicalRoot, "trusted-server", "stdio", "trusted", true); err != nil {
		t.Fatalf("SetMCPTrust(trusted) error = %v", err)
	}
	if err := store.SetMCPTrust(context.Background(), canonicalRoot, "denied-server", "stdio", "denied", false); err != nil {
		t.Fatalf("SetMCPTrust(denied) error = %v", err)
	}

	runtime := &Runtime{Trusts: store}
	state, err := runtime.WorkspaceTrustState(context.Background(), canonicalRoot)
	if err != nil {
		t.Fatalf("WorkspaceTrustState() error = %v", err)
	}
	if !state.Trusted {
		t.Fatalf("state = %#v, want trusted workspace", state)
	}
	if len(state.Servers) != 1 {
		t.Fatalf("state.Servers = %#v, want one trusted server", state.Servers)
	}
	if state.Servers[0].Fingerprint != "trusted-server" {
		t.Fatalf("state.Servers[0] = %#v, want trusted-server", state.Servers[0])
	}
}

func TestRuntimeRevokeTrustRemovesSelectedServerOnlyInCurrentWorkspace(t *testing.T) {
	root := t.TempDir()
	workspaceA := root + "/project-a"
	workspaceB := root + "/project-b"
	for _, dir := range []string{workspaceA, workspaceB} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("MkdirAll(%q) error = %v", dir, err)
		}
	}
	canonicalA, err := canonicalWorkspaceRoot(workspaceA)
	if err != nil {
		t.Fatalf("canonicalWorkspaceRoot(project-a) error = %v", err)
	}
	canonicalB, err := canonicalWorkspaceRoot(workspaceB)
	if err != nil {
		t.Fatalf("canonicalWorkspaceRoot(project-b) error = %v", err)
	}
	store, err := newStartupTrustStore(root + "/kodacode.db")
	if err != nil {
		t.Fatalf("newStartupTrustStore() error = %v", err)
	}
	defer func() {
		_ = store.Close()
	}()
	for _, workspaceRoot := range []string{canonicalA, canonicalB} {
		if err := store.SetWorkspaceTrust(context.Background(), workspaceRoot, true); err != nil {
			t.Fatalf("SetWorkspaceTrust(%q) error = %v", workspaceRoot, err)
		}
	}
	if err := store.SetMCPTrust(context.Background(), canonicalA, "server-a", "stdio", "server-a", true); err != nil {
		t.Fatalf("SetMCPTrust(project-a) error = %v", err)
	}
	if err := store.SetMCPTrust(context.Background(), canonicalB, "server-a", "stdio", "server-a", true); err != nil {
		t.Fatalf("SetMCPTrust(project-b) error = %v", err)
	}

	runtime := &Runtime{Config: Config{}, Trusts: store}
	state, err := runtime.RevokeTrust(context.Background(), RevokeTrustInput{
		WorkspaceRoot: canonicalA,
		Scope:         RevokeTrustScopeServer,
		Fingerprint:   "server-a",
	})
	if err != nil {
		t.Fatalf("RevokeTrust() error = %v", err)
	}
	if len(state.Servers) != 0 {
		t.Fatalf("state.Servers = %#v, want revoked server removed from current workspace", state.Servers)
	}

	recordA, ok, err := store.MCPTrust(context.Background(), canonicalA, "server-a")
	if err != nil {
		t.Fatalf("MCPTrust(project-a) error = %v", err)
	}
	if ok || recordA.Trusted {
		t.Fatalf("project-a server trust = %#v exists=%v, want deleted", recordA, ok)
	}

	recordB, ok, err := store.MCPTrust(context.Background(), canonicalB, "server-a")
	if err != nil {
		t.Fatalf("MCPTrust(project-b) error = %v", err)
	}
	if !ok || !recordB.Trusted {
		t.Fatalf("project-b server trust = %#v exists=%v, want preserved", recordB, ok)
	}
}

func TestRuntimeRevokeTrustAllClearsWorkspaceAndMCPTrust(t *testing.T) {
	root := t.TempDir()
	workspaceA := root + "/project-a"
	workspaceB := root + "/project-b"
	for _, dir := range []string{workspaceA, workspaceB} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("MkdirAll(%q) error = %v", dir, err)
		}
	}
	canonicalA, err := canonicalWorkspaceRoot(workspaceA)
	if err != nil {
		t.Fatalf("canonicalWorkspaceRoot(project-a) error = %v", err)
	}
	canonicalB, err := canonicalWorkspaceRoot(workspaceB)
	if err != nil {
		t.Fatalf("canonicalWorkspaceRoot(project-b) error = %v", err)
	}
	store, err := newStartupTrustStore(root + "/kodacode.db")
	if err != nil {
		t.Fatalf("newStartupTrustStore() error = %v", err)
	}
	defer func() {
		_ = store.Close()
	}()
	for _, workspaceRoot := range []string{canonicalA, canonicalB} {
		if err := store.SetWorkspaceTrust(context.Background(), workspaceRoot, true); err != nil {
			t.Fatalf("SetWorkspaceTrust(%q) error = %v", workspaceRoot, err)
		}
		if err := store.SetMCPTrust(context.Background(), workspaceRoot, "shared-server", "stdio", "shared", true); err != nil {
			t.Fatalf("SetMCPTrust(%q) error = %v", workspaceRoot, err)
		}
	}

	runtime := &Runtime{Config: Config{}, Trusts: store}
	state, err := runtime.RevokeTrust(context.Background(), RevokeTrustInput{
		WorkspaceRoot: canonicalA,
		Scope:         RevokeTrustScopeAll,
	})
	if err != nil {
		t.Fatalf("RevokeTrust(all) error = %v", err)
	}
	if state.Trusted || len(state.Servers) != 0 {
		t.Fatalf("state after revoke all = %#v, want empty trust state", state)
	}
	if _, ok, err := store.WorkspaceTrust(context.Background(), canonicalA); err != nil || ok {
		t.Fatalf("WorkspaceTrust(project-a) = exists:%v err:%v, want deleted", ok, err)
	}
	if _, ok, err := store.WorkspaceTrust(context.Background(), canonicalB); err != nil || ok {
		t.Fatalf("WorkspaceTrust(project-b) = exists:%v err:%v, want deleted", ok, err)
	}
	if _, ok, err := store.MCPTrust(context.Background(), canonicalA, "shared-server"); err != nil || ok {
		t.Fatalf("MCPTrust(project-a) = exists:%v err:%v, want deleted", ok, err)
	}
	if _, ok, err := store.MCPTrust(context.Background(), canonicalB, "shared-server"); err != nil || ok {
		t.Fatalf("MCPTrust(project-b) = exists:%v err:%v, want deleted", ok, err)
	}
}
