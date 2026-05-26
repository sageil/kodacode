package tool

import "testing"

func TestMCPToolNameNormalizesComponents(t *testing.T) {
	got := MCPToolName("sequential-thinking", "sequentialthinking")
	want := "mcp_sequential_thinking__sequentialthinking"
	if got != want {
		t.Fatalf("MCPToolName() = %q, want %q", got, want)
	}
}

func TestParseMCPToolNameSupportsCurrentAndDashedFormats(t *testing.T) {
	tests := []struct {
		name       string
		toolName   string
		wantServer string
		wantRemote string
		wantOK     bool
	}{
		{
			name:       "current",
			toolName:   "mcp_sequential_thinking__sequentialthinking",
			wantServer: "sequential_thinking",
			wantRemote: "sequentialthinking",
			wantOK:     true,
		},
		{
			name:       "dashed",
			toolName:   "mcp_sequential-thinking_sequentialthinking",
			wantServer: "sequential-thinking",
			wantRemote: "sequentialthinking",
			wantOK:     true,
		},
		{
			name:     "non-mcp",
			toolName: "read",
			wantOK:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotServer, gotRemote, gotOK := ParseMCPToolName(tt.toolName)
			if gotOK != tt.wantOK {
				t.Fatalf("ParseMCPToolName() ok = %v, want %v", gotOK, tt.wantOK)
			}
			if gotServer != tt.wantServer || gotRemote != tt.wantRemote {
				t.Fatalf("ParseMCPToolName() = (%q, %q), want (%q, %q)", gotServer, gotRemote, tt.wantServer, tt.wantRemote)
			}
		})
	}
}
