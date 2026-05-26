package tui

import (
	"context"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/tui/theme"
)

func TestRenderAssistantContentLinesFormatsDenseFindingsList(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context: ctx,
		Theme:   &defaultTheme,
	})

	text := strings.Join([]string{
		"1. internal/app/session_task.go: duplicate custom task IDs can still reach append.",
		"   CreateTask needs to reject an explicit existing task_id before writing a durable event.",
		"2. internal/tool/task_input.go: mutable task status validation is too weak.",
		"   The parser should reject completed or blocked because update only supports pending and in_progress.",
	}, "\n")

	rendered := ansi.Strip(strings.Join(renderAssistantContentLines(model, text, 100, ""), "\n"))
	if !strings.Contains(rendered, "• internal/app/session_task.go") {
		t.Fatalf("rendered missing findings bullet:\n%s", rendered)
	}
	if !strings.Contains(rendered, "  duplicate custom task IDs can still reach append.") {
		t.Fatalf("rendered missing indented finding detail:\n%s", rendered)
	}
	if strings.Contains(rendered, "1. internal/app/session_task.go") {
		t.Fatalf("rendered kept numbered-list format instead of findings format:\n%s", rendered)
	}
}

func TestRenderAssistantContentLinesKeepsShortOrdinaryListAsMarkdown(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context: ctx,
		Theme:   &defaultTheme,
	})

	text := "1. run tests\n2. review output"
	rendered := ansi.Strip(strings.Join(renderAssistantContentLines(model, text, 80, ""), "\n"))
	if !strings.Contains(rendered, "1. run tests") || !strings.Contains(rendered, "2. review output") {
		t.Fatalf("rendered ordinary list incorrectly:\n%s", rendered)
	}
	if strings.Contains(rendered, "• run tests") {
		t.Fatalf("ordinary list was incorrectly promoted to findings:\n%s", rendered)
	}
}

func TestRenderAssistantContentLinesPreservesFencedCodeBlocks(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context: ctx,
		Theme:   &defaultTheme,
	})

	text := strings.Join([]string{
		"Findings:",
		"",
		"1. keep the findings list readable",
		"2. preserve code blocks",
		"",
		"```go",
		"func main() {",
		"    println(\"hello\")",
		"}",
		"```",
	}, "\n")

	rendered := ansi.Strip(strings.Join(renderAssistantContentLines(model, text, 80, ""), "\n"))
	if !strings.Contains(rendered, "1. keep the findings list readable") {
		t.Fatalf("rendered lost numbered list in fenced-code fallback:\n%s", rendered)
	}
	if !strings.Contains(rendered, "│ func main() {") && !strings.Contains(rendered, "func main() {") {
		t.Fatalf("rendered lost fenced code block:\n%s", rendered)
	}
}

func TestRenderAssistantContentLinesKeepsFinalizedMarkdownReadable(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context: ctx,
		Theme:   &defaultTheme,
	})

	text := strings.Join([]string{
		"# Plan",
		"",
		"> Keep the transcript renderer, swap only the assistant body.",
	}, "\n")

	rendered := ansi.Strip(strings.Join(renderAssistantContentLines(model, text, 80, ""), "\n"))
	if !strings.Contains(rendered, "Plan") {
		t.Fatalf("rendered lost heading text:\n%s", rendered)
	}
	if !strings.Contains(rendered, "Keep the transcript renderer") {
		t.Fatalf("rendered lost quoted content:\n%s", rendered)
	}
}

func TestRenderAssistantContentLinesKeepsStreamingMarkdownReadable(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context: ctx,
		Theme:   &defaultTheme,
	})

	text := "> partial quote while streaming"
	rendered := ansi.Strip(strings.Join(renderAssistantContentLines(model, text, 80, ""), "\n"))
	if !strings.Contains(rendered, "partial quote while streaming") {
		t.Fatalf("streaming markdown should remain readable:\n%s", rendered)
	}
}

func TestRenderAssistantContentLinesRendersThematicBreakAsDivider(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context: ctx,
		Theme:   &defaultTheme,
	})

	text := strings.Join([]string{
		"Virtual scrolling",
		"",
		"---",
		"",
		"Testing & CI",
	}, "\n")

	rendered := ansi.Strip(strings.Join(renderAssistantContentLines(model, text, 24, ""), "\n"))
	if !strings.Contains(rendered, "──────") {
		t.Fatalf("rendered missing divider row:\n%s", rendered)
	}
	if !strings.Contains(rendered, "Virtual scrolling") || !strings.Contains(rendered, "Testing & CI") {
		t.Fatalf("rendered lost surrounding content:\n%s", rendered)
	}
}

func TestRenderAssistantContentLinesRendersMarkdownTablesReadably(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context: ctx,
		Theme:   &defaultTheme,
	})

	text := strings.Join([]string{
		"| Area | Current state | Why it matters |",
		"| --- | --- | --- |",
		"| Pagination & Skipping | TaskRepository.findTasks uses skip for most requests and falls back to keyset only when skip > 10 000. | Large skip forces MongoDB to scan and discard many docs. |",
	}, "\n")

	rendered := ansi.Strip(strings.Join(renderAssistantContentLines(model, text, 72, ""), "\n"))
	if strings.Contains(rendered, "| Area | Current state |") {
		t.Fatalf("markdown table header should not render as raw markdown:\n%s", rendered)
	}
	for _, want := range []string{"Current state", "Why it matters", "Pagination &", "TaskRepository.find", "Large skip forces"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("rendered missing table content %q:\n%s", want, rendered)
		}
	}
}

func TestRenderAssistantContentLinesPreservesHeadingWhileRenderingTable(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context: ctx,
		Theme:   &defaultTheme,
	})

	text := strings.Join([]string{
		"## Cache Layer",
		"",
		"| Area | Current state | Why it matters |",
		"| --- | --- | --- |",
		"| Pagination & Skipping | TaskRepository.findTasks uses skip for most requests and falls back to keyset only when skip > 10 000. | Large skip forces MongoDB to scan and discard many docs. |",
	}, "\n")

	rendered := ansi.Strip(strings.Join(renderAssistantContentLines(model, text, 72, ""), "\n"))
	if !strings.Contains(rendered, "Cache Layer") {
		t.Fatalf("rendered lost heading:\n%s", rendered)
	}
	if strings.Contains(rendered, "| Area | Current state |") {
		t.Fatalf("heading + table should not render raw markdown table header:\n%s", rendered)
	}
	for _, want := range []string{"Current state", "Why it matters", "Pagination &", "TaskRepository.find"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("rendered missing heading+table content %q:\n%s", want, rendered)
		}
	}
}

func TestRenderTurnTranscriptSectionsPreferStructuredReviewOverRawJSON(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context: ctx,
		Theme:   &defaultTheme,
	})

	raw := `{"findings":[{"severity":"P1","path":"internal/app/runtime_review.go","line":57,"title":"Silent fallback drops review state","explanation":"When invalid output is accepted as plain text, structured review state is unavailable to downstream tooling."}],"overall_correctness":"incorrect","overall_summary":"The review path needs structured output to be reliable."}`
	turn := &events.TurnState{
		TurnID:         "turn-1",
		AssistantText:  raw,
		Config:         &events.TurnConfigState{AgentID: "reviewer", Model: "openai/gpt-5-mini"},
		Review:         &events.ReviewState{Title: "Security Review", Findings: []events.ReviewFindingState{{Severity: "P1", Path: "internal/app/runtime_review.go", Line: 57, Title: "Silent fallback drops review state", Explanation: "When invalid output is accepted as plain text, structured review state is unavailable to downstream tooling."}}, OverallCorrectness: "incorrect", OverallSummary: "The review path needs structured output to be reliable."},
		Transcript:     []events.TranscriptEntryState{{Kind: events.TranscriptEntryAssistant, Text: raw}, {Kind: events.TranscriptEntryReview}},
		ToolCalls:      map[string]*events.ToolCallState{},
		Handoffs:       map[string]*events.AgentHandoffState{},
		ToolCallOrder:  nil,
		HandoffOrder:   nil,
		CompletedAtSeq: 2,
	}

	sections := renderTurnTranscriptSections(model, events.SessionState{
		SessionID: "session-1",
		Turns:     map[string]*events.TurnState{"turn-1": turn},
	}, "turn-1", turn, 100)
	renderedSections := make([]string, 0, len(sections))
	for _, section := range sections {
		renderedSections = append(renderedSections, section.content)
	}
	rendered := ansi.Strip(strings.Join(renderedSections, "\n"))
	if strings.Contains(rendered, `{"findings":`) {
		t.Fatalf("rendered leaked raw review json:\n%s", rendered)
	}
	for _, want := range []string{
		"Security Review",
		"Model: openai/gpt-5-mini",
		"internal/app/runtime_review.go:57",
		"Silent fallback drops review state",
		"Overall correctness: incorrect",
		"Overall summary: The review path needs structured output to be reliable.",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("rendered missing %q:\n%s", want, rendered)
		}
	}
}

func TestRenderTurnTranscriptSectionsHideStructuredReviewPreviewWhileStreaming(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context: ctx,
		Theme:   &defaultTheme,
	})

	raw := `{"findings":[{"severity":"P1","path":"src/routes/oauth.ts","line":17`
	turn := &events.TurnState{
		TurnID:        "turn-1",
		UserText:      "[review] Review the current workspace changes and report concrete, actionable issues.",
		StreamingText: raw,
		Config:        &events.TurnConfigState{AgentID: "reviewer", HideAssistantPreview: true},
		Transcript:    []events.TranscriptEntryState{{Kind: events.TranscriptEntryUser, Text: "[review] Review the current workspace changes and report concrete, actionable issues."}},
		ToolCalls:     map[string]*events.ToolCallState{},
		Handoffs:      map[string]*events.AgentHandoffState{},
	}

	sections := renderTurnTranscriptSections(model, events.SessionState{
		SessionID: "session-1",
		Turns:     map[string]*events.TurnState{"turn-1": turn},
	}, "turn-1", turn, 100)
	renderedSections := make([]string, 0, len(sections))
	for _, section := range sections {
		renderedSections = append(renderedSections, section.content)
	}
	rendered := ansi.Strip(strings.Join(renderedSections, "\n"))
	if strings.Contains(rendered, `{"findings":`) {
		t.Fatalf("rendered leaked structured review preview json:\n%s", rendered)
	}
	if !strings.Contains(rendered, "[review] Review the current workspace changes") {
		t.Fatalf("rendered missing user review request:\n%s", rendered)
	}
}

func TestRenderAssistantContentLinesRendersPermissionMatrixTable(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context: ctx,
		Theme:   &defaultTheme,
	})

	text := strings.Join([]string{
		"| **developer** | Developer | `['tasks:read', 'tasks:create', 'tasks:update']` |",
		"| **user** | Basic user | `['tasks:read', 'tasks:create']` |",
		"",
		"### Permission Matrix",
		"",
		"| Operation | Assignee | Project Admin | Project Manager | Admin |",
		"|----------|----------|---------------|-----------------|-------|",
		"| **Tasks** |||||",
		"| View Tasks | Own + assigned projects | Own projects | All | All |",
		"| Create Tasks | Any (with member validation) | Any | Any | Any |",
		"| Edit Tasks | Own tasks only | No | Any | Any |",
		"| **Projects** |||||",
		"| View Projects | Member/admin projects | Own | All | All |",
		"| Create Projects | Any (becomes admin) | Any | Any | Any |",
	}, "\n")

	rendered := ansi.Strip(strings.Join(renderAssistantContentLines(model, text, 100, ""), "\n"))
	for _, unwanted := range []string{
		"| Operation | Assignee | Project Admin | Project Manager | Admin |",
		"|----------|----------|---------------|-----------------|-------|",
	} {
		if strings.Contains(rendered, unwanted) {
			t.Fatalf("permission matrix table should not render as raw markdown:\n%s", rendered)
		}
	}
	for _, want := range []string{
		"Permission Matrix",
		"Tasks",
		"View Tasks",
		"Own + assigned projects",
		"Create Projects",
		"becomes admin",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("rendered missing permission matrix content %q:\n%s", want, rendered)
		}
	}
	for _, want := range []string{"┌", "│ Operation", "│ Tasks"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("permission matrix should render with table structure %q:\n%s", want, rendered)
		}
	}
}

func TestRenderAssistantContentLinesConvertsHTMLBreaksBeforeWrapping(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context: ctx,
		Theme:   &defaultTheme,
	})

	text := strings.Join([]string{
		"Database indexing",
		"Recommended change: Add compound indexes for common query shapes.<br>Task collection:<br>js<br>TaskSchema.index({ project: 1, status: 1, updatedAt: -1 });<br>TaskSchema.index({ assignee: 1, updatedAt: -1 });",
	}, "\n\n")

	rendered := ansi.Strip(strings.Join(renderAssistantContentLines(model, text, 72, ""), "\n"))
	normalized := strings.Join(strings.Fields(rendered), " ")
	if strings.Contains(rendered, "<br>") {
		t.Fatalf("rendered still contains raw HTML breaks:\n%s", rendered)
	}
	if strings.Contains(rendered, "…") {
		t.Fatalf("rendered still truncated wrapped markdown lines:\n%s", rendered)
	}
	if !strings.Contains(normalized, "Task collection: js TaskSchema.index({ project: 1, status: 1, updatedAt: -1 });") {
		t.Fatalf("rendered lost normalized line break content:\n%s", rendered)
	}
}

func TestRenderAssistantMarkdownBlockPreservesValidMarkdownWithinWidth(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context: ctx,
		Theme:   &defaultTheme,
	})

	text := strings.Join([]string{
		"# Plan",
		"",
		"- keep width ownership in one layer",
		"- swap only the assistant body renderer",
	}, "\n")

	lines := renderAssistantMarkdownBlock(model, text, 40, "")
	rendered := ansi.Strip(strings.Join(lines, "\n"))
	normalized := strings.Join(strings.Fields(rendered), " ")
	if !strings.Contains(rendered, "Plan") {
		t.Fatalf("assistant markdown rendering lost heading text:\n%s", rendered)
	}
	if !strings.Contains(normalized, "keep width ownership in one layer") {
		t.Fatalf("assistant markdown rendering lost list content:\n%s", rendered)
	}
	for _, line := range lines {
		if got := ansi.StringWidth(line); got > 40 {
			t.Fatalf("assistant markdown line width = %d, want <= 40\n%q", got, ansi.Strip(line))
		}
	}
}

func TestRenderAssistantMarkdownBlockKeepsMalformedMarkdownWithinWidth(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context: ctx,
		Theme:   &defaultTheme,
	})

	text := strings.Join([]string{
		"**Priority Implementation Order (Quick Wins)**",
		"",
		"1",
		"  Task: Add .lean() to all queries",
		"  Backend/Frontend: Backend",
		"  Time: 1-2h",
		"  Impact: -30-80ms",
		"",
		"2",
		"  Task: Fix Kanban filtering",
		"  Backend/Frontend: Frontend",
		"  Time: 1-2h",
		"  Impact: -50% re-renders",
	}, "\n")

	lines := renderAssistantMarkdownBlock(model, text, 52, "")
	rendered := ansi.Strip(strings.Join(lines, "\n"))
	normalized := strings.Join(strings.Fields(rendered), " ")
	if strings.Contains(rendered, "…") {
		t.Fatalf("assistant markdown rendering unexpectedly truncated malformed markdown:\n%s", rendered)
	}
	if !strings.Contains(rendered, "Priority Implementation Order") {
		t.Fatalf("assistant markdown rendering lost malformed heading text:\n%s", rendered)
	}
	if !strings.Contains(normalized, "Task: Add .lean() to all queries") {
		t.Fatalf("assistant markdown rendering lost malformed list body:\n%s", rendered)
	}
	for _, line := range lines {
		if got := ansi.StringWidth(line); got > 52 {
			t.Fatalf("assistant markdown line width = %d, want <= 52\n%q", got, ansi.Strip(line))
		}
	}
}

func TestRenderAssistantContentLinesKeepsShellRecipeCommentsLiteralWithinWidth(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context: ctx,
		Theme:   &defaultTheme,
	})

	text := strings.Join([]string{
		"cat > src/routes/health.ts <<'EOS'",
		"import { Router } from 'express';",
		"const router = Router();",
		"router.get('/', (req, res) => res.json({ status: 'ok' }));",
		"export default router;",
		"EOS",
		"",
		"# 3 Install dotenv-safe and add example env",
		"npm install -D dotenv-safe",
		"cat > .env.example <<'EOS'",
		"MONGO_URI=mongodb://localhost:27017/kairo",
		"JWT_SECRET=your-secret-here",
		"EOS",
		"",
		"# 4 Enforce Jest coverage thresholds (add to jest.config.js)",
		"#   coverageThreshold: { global: { branches: 80, functions: 80, lines: 80, statements: 80 } },",
		"",
		"# 5 Add a workspace lint script",
		`npm pkg set scripts.lint:all="npm run lint --prefix . && npm run lint --prefix client"`,
	}, "\n")

	lines := renderAssistantContentLines(model, text, 52, "")
	rendered := ansi.Strip(strings.Join(lines, "\n"))
	normalized := strings.Join(strings.Fields(rendered), " ")
	for _, want := range []string{
		"# 3 Install dotenv-safe and add example env",
		"# 4 Enforce Jest coverage thresholds (add to jest.config.js)",
		"# 5 Add a workspace lint script",
		"npm pkg set scripts.lint:all=",
	} {
		if !strings.Contains(normalized, want) {
			t.Fatalf("assistant shell recipe rendering lost %q:\n%s", want, rendered)
		}
	}
	if strings.Contains(rendered, "…") {
		t.Fatalf("assistant shell recipe rendering unexpectedly truncated content:\n%s", rendered)
	}
	for _, line := range lines {
		if got := ansi.StringWidth(line); got > 52 {
			t.Fatalf("assistant shell recipe line width = %d, want <= 52\n%q", got, ansi.Strip(line))
		}
	}
}

func TestSplitAssistantBlocksKeepsFencedCodeWithBlankLinesIntact(t *testing.T) {
	text := strings.Join([]string{
		"# Intro",
		"",
		"```go",
		"func main() {",
		"",
		`    println("hello")`,
		"}",
		"```",
		"",
		"> quoted follow-up",
	}, "\n")

	blocks := splitAssistantBlocks(text)
	if len(blocks) != 3 {
		t.Fatalf("splitAssistantBlocks() block count = %d, want 3\n%q", len(blocks), blocks)
	}
	if !strings.Contains(blocks[1], "func main() {") || !strings.Contains(blocks[1], `println("hello")`) {
		t.Fatalf("fenced code block was not preserved intact:\n%q", blocks[1])
	}
	if strings.Contains(blocks[2], "```") {
		t.Fatalf("quote block unexpectedly absorbed fenced code terminator:\n%q", blocks[2])
	}
}

func TestRenderAssistantContentLinesKeepsBlockQuoteFormattingInsideLongMessage(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context: ctx,
		Theme:   &defaultTheme,
	})

	fillerParagraphs := make([]string, 0, 40)
	for i := 0; i < 40; i++ {
		fillerParagraphs = append(fillerParagraphs, "This filler paragraph is here to make the full assistant message exceed the glamour size threshold while each individual block remains small and readable.")
	}
	text := strings.Join(append([]string{
		"# Plan",
		"",
		"> quoted insight should keep rich markdown rendering",
		"",
	}, fillerParagraphs...), "\n\n")

	rendered := ansi.Strip(strings.Join(renderAssistantContentLines(model, text, 72, ""), "\n"))
	if !strings.Contains(rendered, "Plan") {
		t.Fatalf("rendered lost heading text:\n%s", rendered)
	}
	if !strings.Contains(rendered, "│ quoted insight should keep rich markdown rendering") {
		t.Fatalf("rendered long multi-block message lost glamour quote rendering:\n%s", rendered)
	}
}
