package search

import (
	"context"
	"strings"
	"testing"

	"github.com/sageil/kodacode/internal/provider"
)

func TestBuildChunkInputsSplitGoDeclarations(t *testing.T) {
	root := t.TempDir()
	content := `package auth

// CheckPermission verifies access.
func CheckPermission(user string) bool {
	return user != ""
}

type Guard struct{}

func (g *Guard) Allow() bool {
	return true
}
`

	inputs := buildChunkInputs(root, root+"/auth.go", content)
	if len(inputs) != 3 {
		t.Fatalf("len(inputs) = %d, want 3", len(inputs))
	}
	if inputs[0].Chunk.Line != 4 {
		t.Fatalf("first line = %d, want 4", inputs[0].Chunk.Line)
	}
	if inputs[0].Chunk.StartLine != 3 || inputs[0].Chunk.EndLine != 7 {
		t.Fatalf("first chunk range = %d:%d, want 3:7", inputs[0].Chunk.StartLine, inputs[0].Chunk.EndLine)
	}
	if !strings.Contains(inputs[0].Text, "// CheckPermission verifies access.") {
		t.Fatalf("first text = %q, want leading comment context", inputs[0].Text)
	}
	if inputs[1].Chunk.Line != 8 {
		t.Fatalf("second line = %d, want 8", inputs[1].Chunk.Line)
	}
	if inputs[1].Chunk.StartLine != 8 || inputs[1].Chunk.EndLine != 9 {
		t.Fatalf("second chunk range = %d:%d, want 8:9", inputs[1].Chunk.StartLine, inputs[1].Chunk.EndLine)
	}
	if inputs[2].Chunk.Line != 10 {
		t.Fatalf("third line = %d, want 10", inputs[2].Chunk.Line)
	}
	if inputs[2].Chunk.StartLine != 10 || inputs[2].Chunk.EndLine != 13 {
		t.Fatalf("third chunk range = %d:%d, want 10:13", inputs[2].Chunk.StartLine, inputs[2].Chunk.EndLine)
	}
	if !strings.Contains(inputs[0].Text, "func CheckPermission(user string) bool") {
		t.Fatalf("first text = %q, want declaration line", inputs[0].Text)
	}
	if !strings.Contains(inputs[1].Text, "type Guard struct{}") {
		t.Fatalf("second text = %q, want type declaration", inputs[1].Text)
	}
	if !strings.Contains(inputs[2].Text, "func (g *Guard) Allow() bool") {
		t.Fatalf("third text = %q, want method declaration", inputs[2].Text)
	}
}

func TestBuildChunkInputsSplitGenericSymbolBoundaries(t *testing.T) {
	root := t.TempDir()
	content := `export function buildPayload(user) {
  return { user };
}

const version = "1";

export class Guard {
  allow() {
    return true;
  }
}
`

	inputs := buildChunkInputs(root, root+"/guard.ts", content)
	if len(inputs) < 3 {
		t.Fatalf("len(inputs) = %d, want at least 3 boundary chunks", len(inputs))
	}
	if inputs[0].Chunk.Line != 1 {
		t.Fatalf("first line = %d, want 1", inputs[0].Chunk.Line)
	}
	if inputs[1].Chunk.Line != 5 {
		t.Fatalf("second line = %d, want 5", inputs[1].Chunk.Line)
	}
	if inputs[2].Chunk.Line != 7 {
		t.Fatalf("third line = %d, want 7", inputs[2].Chunk.Line)
	}
}

func TestBuildChunkInputsAttachDecoratorAndModifiers(t *testing.T) {
	root := t.TempDir()
	content := `@injectable
// BuildPayload prepares the response.
export async function buildPayload(user) {
  return { user };
}

public final class Guard {
}
`

	inputs := buildChunkInputs(root, root+"/service.ts", content)
	if len(inputs) != 2 {
		t.Fatalf("len(inputs) = %d, want 2", len(inputs))
	}
	if inputs[0].Chunk.Line != 3 {
		t.Fatalf("first line = %d, want 3", inputs[0].Chunk.Line)
	}
	if !strings.Contains(inputs[0].Text, "@injectable") || !strings.Contains(inputs[0].Text, "// BuildPayload prepares the response.") {
		t.Fatalf("first text = %q, want decorator and comment context", inputs[0].Text)
	}
	if inputs[1].Chunk.Line != 7 {
		t.Fatalf("second line = %d, want 7", inputs[1].Chunk.Line)
	}
	if !strings.Contains(inputs[1].Text, "public final class Guard") {
		t.Fatalf("second text = %q, want modifier-heavy declaration", inputs[1].Text)
	}
}

func TestBuildChunkInputsDetectCallableAndAssignedFunctions(t *testing.T) {
	root := t.TempDir()
	content := `public async Task<Result> Handle(Request request) {
	return Result.Ok();
}

handler := func(value string) bool {
	return value != ""
}

handler = (value) => {
	return value;
}
`

	inputs := buildChunkInputs(root, root+"/handler.txt", content)
	if len(inputs) != 3 {
		t.Fatalf("len(inputs) = %d, want 3", len(inputs))
	}
	if inputs[0].Chunk.Line != 1 {
		t.Fatalf("first line = %d, want 1", inputs[0].Chunk.Line)
	}
	if inputs[1].Chunk.Line != 5 {
		t.Fatalf("second line = %d, want 5", inputs[1].Chunk.Line)
	}
	if inputs[2].Chunk.Line != 9 {
		t.Fatalf("third line = %d, want 9", inputs[2].Chunk.Line)
	}
	if !strings.Contains(inputs[0].Text, "public async Task<Result> Handle") {
		t.Fatalf("first text = %q, want callable block signature", inputs[0].Text)
	}
	if !strings.Contains(inputs[1].Text, "handler := func") {
		t.Fatalf("second text = %q, want assigned func boundary", inputs[1].Text)
	}
	if !strings.Contains(inputs[2].Text, "handler = (value) =>") {
		t.Fatalf("third text = %q, want arrow-function boundary", inputs[2].Text)
	}
}

func TestBuildChunkInputsSplitBracketSections(t *testing.T) {
	root := t.TempDir()
	content := `[server]
port = 8080

[client]
timeout = "5s"
`

	inputs := buildChunkInputs(root, root+"/config.ini", content)
	if len(inputs) != 2 {
		t.Fatalf("len(inputs) = %d, want 2", len(inputs))
	}
	if inputs[0].Chunk.Line != 1 || inputs[1].Chunk.Line != 4 {
		t.Fatalf("lines = %d,%d, want 1,4", inputs[0].Chunk.Line, inputs[1].Chunk.Line)
	}
}

func TestHybridSearchUsesLanguageAgnosticBoundaryChunks(t *testing.T) {
	root := t.TempDir()
	path := root + "/auth.go"
	content := `package auth

// CheckPermission verifies access.
func CheckPermission(user string) bool {
	return user != ""
}
`
	writeSearchFile(t, path, content)
	inputs := buildChunkInputs(root, path, content)
	if len(inputs) != 1 || !strings.Contains(inputs[0].Text, "func CheckPermission") {
		t.Fatalf("inputs = %#v", inputs)
	}

	embedder := &fakeEmbedder{
		vectors: map[string][]float32{
			"authorization gate": {1, 0},
			inputs[0].Text:       {1, 0},
		},
	}
	service := NewService(embedder, provider.ModelRef{ProviderID: "openai", ModelID: "text-embedding-3-small"}, 0, t.TempDir(), nil)

	response, err := service.Search(context.Background(), Request{
		Query:         "authorization gate",
		RootPath:      root,
		WorkspaceRoot: root,
		MaxResults:    5,
		Mode:          ModeHybrid,
	})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(response.Results) != 1 {
		t.Fatalf("results = %#v", response.Results)
	}
	if response.Results[0].Path != "auth.go" || response.Results[0].Line != 4 {
		t.Fatalf("first result = %#v", response.Results[0])
	}
}
