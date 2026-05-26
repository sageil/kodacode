package search

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
)

func TestLexicalObservedUsesExactFileBytesForObservedVersion(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "notes.txt")
	content := []byte("first line\r\nTODO second line")
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	response, err := LexicalObserved(Request{
		Query:         "TODO",
		RootPath:      path,
		WorkspaceRoot: root,
		MaxResults:    10,
		Mode:          ModeLexical,
	})
	if err != nil {
		t.Fatalf("LexicalObserved() error = %v", err)
	}
	if response.Observation == nil || !response.Observation.Complete {
		t.Fatalf("observation = %#v", response.Observation)
	}
	if len(response.Observation.Resources) != 1 {
		t.Fatalf("observed resources = %#v", response.Observation.Resources)
	}

	expected := sha256.Sum256(content)
	resource := response.Observation.Resources[0]
	if resource.Kind != observedResourceFileContent || resource.Path != path {
		t.Fatalf("resource = %#v", resource)
	}
	if resource.Version != hex.EncodeToString(expected[:]) {
		t.Fatalf("resource version = %q, want %q", resource.Version, hex.EncodeToString(expected[:]))
	}
	if resource.State == "" {
		t.Fatalf("resource state = %#v", resource)
	}
}
