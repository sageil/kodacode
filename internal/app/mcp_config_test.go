package app

import "testing"

func TestLoadRuntimeConfigWithSourcesLoadsMCPServers(t *testing.T) {
	config, err := LoadRuntimeConfigWithSources(
		func(string) string { return "" },
		fakeStoredConfigLoader{config: StoredConfig{
			MCP: StoredMCPConfig{
				Servers: []StoredMCPServerConfig{{
					Name:    "filesystem",
					Type:    "stdio",
					Command: "npx",
					Args:    []string{"-y", "@modelcontextprotocol/server-filesystem", "/tmp/project"},
					Env: map[string]string{
						"MCP_TOKEN": "abc123",
					},
				}},
			},
		}},
		fakeAuthLookup{},
	)
	if err != nil {
		t.Fatalf("LoadRuntimeConfigWithSources() error = %v", err)
	}
	if len(config.MCP.Servers) != 1 {
		t.Fatalf("len(config.MCP.Servers) = %d, want 1", len(config.MCP.Servers))
	}
	server := config.MCP.Servers[0]
	if server.Name != "filesystem" || server.Type != "stdio" || server.Command != "npx" {
		t.Fatalf("server = %#v", server)
	}
	if len(server.Args) != 3 || server.Args[2] != "/tmp/project" {
		t.Fatalf("args = %#v", server.Args)
	}
	if got := server.Env["MCP_TOKEN"]; got != "abc123" {
		t.Fatalf("env[MCP_TOKEN] = %q", got)
	}
	if !server.IsEnabled() {
		t.Fatal("server should default to enabled")
	}
}
