package tui

import (
	"context"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/sageil/kodacode/internal/tui/theme"
)

func TestRenderMutationDiffOpsBlankInsertDoesNotFillRowWidth(t *testing.T) {
	model := newMutationRenderTestModel(t)

	lines := renderMutationDiffOps(model, []mutationDiffOp{
		{kind: mutationDiffInsert, text: ""},
		{kind: mutationDiffInsert, text: "hello"},
	}, 20)
	if len(lines) != 2 {
		t.Fatalf("lines = %#v", lines)
	}

	if got := ansi.StringWidth(ansi.Strip(lines[0])); got >= 20 {
		t.Fatalf("blank insert width = %d, want < 20\n%q", got, ansi.Strip(lines[0]))
	}
	if got := ansi.StringWidth(ansi.Strip(lines[1])); got != 20 {
		t.Fatalf("non-empty insert width = %d, want 20\n%q", got, ansi.Strip(lines[1]))
	}
}

func TestRenderWriteContentLinesBlankLineDoesNotFillRowWidth(t *testing.T) {
	model := newMutationRenderTestModel(t)

	lines := renderWriteContentLines(model, []string{"", "hello"}, 20)
	if len(lines) != 2 {
		t.Fatalf("lines = %#v", lines)
	}

	if got := ansi.StringWidth(ansi.Strip(lines[0])); got >= 20 {
		t.Fatalf("blank write line width = %d, want < 20\n%q", got, ansi.Strip(lines[0]))
	}
	if got := ansi.StringWidth(ansi.Strip(lines[1])); got != 20 {
		t.Fatalf("non-empty write line width = %d, want 20\n%q", got, ansi.Strip(lines[1]))
	}
}

func TestRenderMutationDiffOpsShowsSeparateOldAndNewLineNumbers(t *testing.T) {
	model := newMutationRenderTestModel(t)

	lines := renderMutationDiffOpsAt(model, []mutationDiffOp{
		{kind: mutationDiffDelete, text: "old value"},
		{kind: mutationDiffInsert, text: "new value"},
	}, 40, 73, 77)
	if len(lines) != 2 {
		t.Fatalf("lines = %#v", lines)
	}

	deleteLine := ansi.Strip(lines[0])
	insertLine := ansi.Strip(lines[1])
	if !strings.Contains(deleteLine, "73 |") || strings.Contains(deleteLine, "77 +") {
		t.Fatalf("delete line = %q", deleteLine)
	}
	if !strings.Contains(insertLine, "|") || !strings.Contains(insertLine, "77 +") || strings.Contains(insertLine, "73 |") {
		t.Fatalf("insert line = %q", insertLine)
	}
}

func TestRenderMutationSideBySideOpsAtBiasesWidthTowardInsertHeavyDiff(t *testing.T) {
	model := newMutationRenderTestModel(t)

	lines := renderMutationSideBySideOpsAt(model, []mutationDiffOp{
		{kind: mutationDiffEqual, text: "server: {"},
		{kind: mutationDiffDelete, text: "historyApiFallback: true,"},
		{kind: mutationDiffInsert, text: "port: 3001,"},
		{kind: mutationDiffInsert, text: "host: true,"},
		{kind: mutationDiffInsert, text: "strictPort: true,"},
		{kind: mutationDiffInsert, text: "compress: true,"},
		{kind: mutationDiffInsert, text: "cors: true,"},
		{kind: mutationDiffInsert, text: "hmr: { clientPort: 3001 },"},
		{kind: mutationDiffInsert, text: "}"},
	}, 120, 6, 45)
	if len(lines) == 0 {
		t.Fatal("expected rendered lines")
	}

	header := ansi.Strip(lines[0])
	divider := strings.Index(header, "│")
	if divider < 0 {
		t.Fatalf("missing divider in header %q", header)
	}
	if divider >= 45 {
		t.Fatalf("divider stayed too close to center for insert-heavy diff: %d\n%s", divider, header)
	}
}

func TestRenderMutationSideBySideOpsAtBiasesWidthTowardDeleteHeavyDiff(t *testing.T) {
	model := newMutationRenderTestModel(t)

	lines := renderMutationSideBySideOpsAt(model, []mutationDiffOp{
		{kind: mutationDiffEqual, text: "server: {"},
		{kind: mutationDiffDelete, text: "port: 3001,"},
		{kind: mutationDiffDelete, text: "host: true,"},
		{kind: mutationDiffDelete, text: "strictPort: true,"},
		{kind: mutationDiffDelete, text: "compress: true,"},
		{kind: mutationDiffDelete, text: "cors: true,"},
		{kind: mutationDiffDelete, text: "hmr: { clientPort: 3001 },"},
		{kind: mutationDiffInsert, text: "historyApiFallback: true,"},
		{kind: mutationDiffInsert, text: "}"},
	}, 120, 45, 6)
	if len(lines) == 0 {
		t.Fatal("expected rendered lines")
	}

	header := ansi.Strip(lines[0])
	divider := strings.Index(header, "│")
	if divider < 0 {
		t.Fatalf("missing divider in header %q", header)
	}
	if divider <= 70 {
		t.Fatalf("divider stayed too close to center for delete-heavy diff: %d\n%s", divider, header)
	}
}

func TestRenderMutationToolDetailDiffOpsAtFallsBackToUnifiedForAsymmetricBlock(t *testing.T) {
	model := newMutationRenderTestModel(t)

	lines := renderMutationToolDetailDiffOpsAt(model, []mutationDiffOp{
		{kind: mutationDiffEqual, text: "const uri = process.env.MONGO_URI || 'mongodb://localhost:27017/projectdb';"},
		{kind: mutationDiffDelete, text: "await mongoose.connect(uri);"},
		{kind: mutationDiffInsert, text: "await mongoose.connect(uri, {"},
		{kind: mutationDiffInsert, text: "  maxPoolSize: parseInt(process.env.MONGO_MAX_POOL_SIZE || '50'),"},
		{kind: mutationDiffInsert, text: "  minPoolSize: parseInt(process.env.MONGO_MIN_POOL_SIZE || '10'),"},
		{kind: mutationDiffInsert, text: "  maxIdleTimeMS: 30000,"},
		{kind: mutationDiffInsert, text: "  serverSelectionTimeoutMS: 5000,"},
		{kind: mutationDiffInsert, text: "  socketTimeoutMS: 45000,"},
		{kind: mutationDiffInsert, text: "});"},
	}, 120, 8, 8)
	if len(lines) == 0 {
		t.Fatal("expected rendered lines")
	}

	rendered := ansi.Strip(strings.Join(lines, "\n"))
	if strings.Contains(lines[0], "Old") || strings.Contains(lines[0], "New") {
		t.Fatalf("asymmetric diff should not use side-by-side headers\n%s", rendered)
	}
	for _, want := range []string{
		"const uri = process.env.MONGO_URI",
		"- await mongoose.connect(uri);",
		"+ await mongoose.connect(uri, {",
		"+   maxPoolSize: parseInt(process.env.MONGO_MAX_POOL_SIZE || '50'),",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("rendered missing %q\n%s", want, rendered)
		}
	}
}

func newMutationRenderTestModel(t *testing.T) Model {
	t.Helper()
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	t.Cleanup(cancel)
	return NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
	})
}
