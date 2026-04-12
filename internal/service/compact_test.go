package service

import (
	"encoding/json"
	"testing"

	"github.com/sageil/kodacode/v1/internal/provider"
)

func TestProviderSupportsCaching(t *testing.T) {
	tests := []struct {
		id   string
		want bool
	}{
		{"anthropic", true},
		{"google", true},
		{"openai", false},
		{"lmstudio", false},
		{"ollama", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := providerSupportsCaching(tt.id); got != tt.want {
			t.Errorf("providerSupportsCaching(%q) = %v, want %v", tt.id, got, tt.want)
		}
	}
}

func TestCompactToolSchemas(t *testing.T) {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"file_path": map[string]any{
				"type":        "string",
				"description": "The absolute path to the file",
			},
			"limit": map[string]any{
				"type":        "integer",
				"description": "Maximum number of lines to read",
			},
		},
		"required": []string{"file_path"},
	}
	raw, _ := json.Marshal(schema)

	tools := []provider.Tool{
		{
			Name:        "read",
			Description: "Read a file from disk",
			Parameters:  raw,
		},
		{
			Name:        "glob",
			Description: "Find files by pattern",
		},
	}

	compacted := compactToolSchemas(tools)

	if compacted[0].Description != "Read one file or a specific range" {
		t.Errorf("tool-level Description = %q, want compact summary", compacted[0].Description)
	}
	if compacted[1].Description != "Find files by path pattern" {
		t.Errorf("tool without parameters Description = %q, want compact summary", compacted[1].Description)
	}

	var result map[string]any
	if err := json.Unmarshal(compacted[0].Parameters, &result); err != nil {
		t.Fatalf("failed to unmarshal compacted schema: %v", err)
	}

	props := result["properties"].(map[string]any)
	for name, v := range props {
		pm := v.(map[string]any)
		if _, hasDesc := pm["description"]; hasDesc {
			t.Errorf("parameter %q still has description after compaction", name)
		}
		if _, hasType := pm["type"]; !hasType {
			t.Errorf("parameter %q lost its type after compaction", name)
		}
	}

	if _, ok := result["required"]; !ok {
		t.Error("required field should be preserved")
	}
}

func TestCompactToolSchemas_Idempotent(t *testing.T) {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{"type": "string"},
		},
	}
	raw, _ := json.Marshal(schema)

	tools := []provider.Tool{{Name: "test", Parameters: raw}}
	once := compactToolSchemas(tools)
	twice := compactToolSchemas(once)

	if string(once[0].Parameters) != string(twice[0].Parameters) {
		t.Error("compactToolSchemas should be idempotent")
	}
}

func TestCompactToolSchemas_PreservesCriticalWorkflowToolDocs(t *testing.T) {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"question": map[string]any{
				"type":        "string",
				"description": "A short question shown to the user",
			},
			"options": map[string]any{
				"type":        "array",
				"description": "Choices for the user",
			},
		},
	}
	raw, _ := json.Marshal(schema)

	compacted := compactToolSchemas([]provider.Tool{{
		Name:        "question",
		Description: "Ask the user a short question with selectable options.",
		Parameters:  raw,
	}})

	if compacted[0].Description == "Ask the user to choose between options" {
		t.Fatalf("question description was over-compacted: %q", compacted[0].Description)
	}
	if string(compacted[0].Parameters) != string(raw) {
		t.Fatalf("question parameters should keep descriptions in compact mode")
	}
}

func TestCompactToolSchemas_PreservesBashPurposeDocs(t *testing.T) {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"command": map[string]any{
				"type":        "string",
				"description": "The command to execute",
			},
			"purpose": map[string]any{
				"type":        "string",
				"description": "Why this command is being run",
				"enum":        []string{"verification", "build", "diagnostic", "other"},
			},
		},
		"required": []string{"command", "purpose"},
	}
	raw, _ := json.Marshal(schema)

	compacted := compactToolSchemas([]provider.Tool{{
		Name:        "bash",
		Description: "Run a shell command.",
		Parameters:  raw,
	}})

	if string(compacted[0].Parameters) != string(raw) {
		t.Fatalf("bash parameters should keep descriptions in compact mode")
	}
}
