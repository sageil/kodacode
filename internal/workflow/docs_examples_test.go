package workflow

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDocumentationWorkflowExamplesParseAndValidate(t *testing.T) {
	path := filepath.Join("..", "..", "site", "src", "content", "docs", "getting-started", "workflows.mdx")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", path, err)
	}
	blocks := workflowYAMLBlocks(string(data))
	if len(blocks) < 4 {
		t.Fatalf("workflowYAMLBlocks() = %d, want at least 4 documented examples", len(blocks))
	}
	for index, block := range blocks {
		definition, err := LoadBytes([]byte(block), testValidationContext())
		if err != nil {
			t.Fatalf("documented workflow YAML block %d failed validation: %v\n%s", index+1, err, block)
		}
		if definition.ID == "" || len(definition.Phases) == 0 {
			t.Fatalf("documented workflow YAML block %d parsed empty definition: %#v", index+1, definition)
		}
	}
}

func workflowYAMLBlocks(markdown string) []string {
	lines := strings.Split(markdown, "\n")
	blocks := make([]string, 0)
	var current []string
	inYAML := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !inYAML {
			if trimmed == "```yaml" || trimmed == "```yml" {
				inYAML = true
				current = nil
			}
			continue
		}
		if trimmed == "```" {
			block := strings.TrimSpace(strings.Join(current, "\n"))
			if strings.Contains(block, "id:") && strings.Contains(block, "phases:") {
				blocks = append(blocks, block)
			}
			inYAML = false
			current = nil
			continue
		}
		current = append(current, line)
	}
	return blocks
}
