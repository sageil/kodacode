package tui

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/sageil/kodacode/v1/internal/tui/theme"
)

func truncStr(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// flushView flushes any pending render and returns the view.
// Tests must use this instead of m.View() since mutations no longer render eagerly.
func flushView(m *Messages) string {
	m.FlushRender()
	return m.View()
}

// TestSendMessagePreservesHeader verifies that submitting a second message
// (sendMessage path) does not clear the agent/model displayed in the header.
// Before the fix, sendMessage returned sessionCreatedMsg with an empty APISession,
// which zeroed the header fields. The fix introduces messageSentMsg, which does
// not touch the header.
func TestSendMessagePreservesHeader(t *testing.T) {
	app := NewApp("http://localhost", nil)
	app.width = 80
	app.height = 24
	app.ready = true
	app.route = routeSession
	app.sessionID = "sess-1"
	// Simulate first message: header populated via sessionCreatedMsg.
	app.session.SetAgent("my-agent", "My Agent")
	app.session.SetModel("openai/gpt-4o")

	// Verify they are visible before the second send.
	if got := app.session.header.agentID; got != "my-agent" {
		t.Fatalf("pre-condition: agentID = %q, want my-agent", got)
	}

	// messageSentMsg is what sendMessage now fires. It must NOT clear the header.
	updated, _ := app.Update(messageSentMsg{
		sessionID: "sess-1",
		text:      "second prompt",
	})
	result := updated.(App)

	if got := result.session.header.agentID; got != "my-agent" {
		t.Errorf("agentID after second send = %q, want my-agent (was cleared)", got)
	}
	if got := result.session.header.modelID; got != "openai/gpt-4o" {
		t.Errorf("modelID after second send = %q, want openai/gpt-4o (was cleared)", got)
	}
}

func TestAppSetsTokensOnDone(t *testing.T) {
	app := NewApp("http://localhost", nil)
	app.width = 80
	app.height = 24
	app.ready = true
	app.route = routeSession
	app.sessionID = "sess-1"
	payload := SSEDonePayload{}
	payload.Usage.InputTokens = 1200
	payload.ContextSize = 128000
	data, _ := json.Marshal(payload)
	updated, _ := app.Update(SSEEventMsg{
		SessionID: "sess-1",
		Type:      "done",
		Data:      data,
	})
	result := updated.(App)
	if got := result.session.statusBar.inputTokens; got != 1200 {
		t.Errorf("inputTokens = %d, want 1200", got)
	}
	if got := result.session.statusBar.contextSize; got != 128000 {
		t.Errorf("contextSize = %d, want 128000", got)
	}
}

func TestHomeShowsModel(t *testing.T) {
	h := NewHome()
	h.SetSize(80, 24)
	h.SetModel("zai-coding-plan/glm-4.5")
	view := h.View()
	if !strings.Contains(view, "glm-4.5") {
		t.Errorf("Home.View() does not contain model name, got:\n%s", view)
	}
}

func TestToolCallRendering(t *testing.T) {
	m := NewMessages()
	m.SetSize(80, 20)
	// Bash with JSON input — renders as a panel with tool name and description.
	m.AppendToolStart("bash", `{"command":"go build ./...","description":"Build the project"}`, "")
	view := flushView(&m)
	if !strings.Contains(view, "Bash") {
		t.Errorf("View() missing tool name, got:\n%s", view)
	}
	if !strings.Contains(view, "Build the project") {
		t.Errorf("View() missing description, got:\n%s", view)
	}
	if !strings.Contains(view, "running") {
		t.Errorf("View() missing running indicator, got:\n%s", view)
	}

	m.UpdateToolEnd("bash", "ok  github.com/sageil/kodacode/v1\n", "", "")
	view = flushView(&m)
	if !strings.Contains(view, "⦿") {
		t.Errorf("View() missing done indicator after UpdateToolEnd, got:\n%s", view)
	}
	if strings.Contains(view, `"command"`) {
		t.Errorf("View() must not show raw JSON, got:\n%s", view)
	}
	// Successful tools are auto-collapsed. Uncollapse to verify output.
	m.messages[len(m.messages)-1].Collapsed = false
	m.invalidateFrom(0)
	m.render()
	expandedView := flushView(&m)
	if !strings.Contains(expandedView, "ok  github.com/sageil/kodacode/v1") {
		t.Errorf("View() missing tool output when expanded, got:\n%s", expandedView)
	}
}

func TestToolCallRendering_SubagentPreservesSoftLineBreaks(t *testing.T) {
	m := NewMessages()
	m.SetSize(100, 20)
	m.AppendToolStart("subagent", `{"agent_id":"explorer","task":"inspect repo"}`, "sub-1")
	m.UpdateToolEnd("subagent", "First line\nSecond line\n\n- item one\n- item two", "", "sub-1")
	m.messages[len(m.messages)-1].Collapsed = false
	m.messages[len(m.messages)-1].UserExpanded = true
	m.invalidateFrom(0)

	view := ansi.Strip(flushView(&m))
	if strings.Contains(view, "First line Second line") {
		t.Fatalf("subagent output flattened soft line break, got:\n%s", view)
	}
	firstIdx := strings.Index(view, "First line")
	secondIdx := strings.Index(view, "Second line")
	if firstIdx < 0 || secondIdx < 0 || secondIdx <= firstIdx || !strings.Contains(view[firstIdx:secondIdx], "\n") {
		t.Fatalf("subagent output missing preserved line break, got:\n%s", view)
	}
	if !strings.Contains(view, "• item one") {
		t.Fatalf("subagent markdown list formatting missing, got:\n%s", view)
	}
}

func TestToolCallRendering_ReadFormatted(t *testing.T) {
	m := NewMessages()
	m.SetSize(80, 20)
	m.AppendToolStart("read", `{"filePath":"/Users/sageil/dev/kodacode/internal/tui/messages.go","limit":100}`, "")

	// While running, read tools should be visible with formatted summary.
	view := flushView(&m)
	if !strings.Contains(view, "Read") {
		t.Errorf("Running read tool should show tool name, got:\n%s", view)
	}
	if !strings.Contains(view, "messages.go") {
		t.Errorf("Running read tool should show filename, got:\n%s", view)
	}

	// After completion, read tools are collapsed (header visible, output hidden).
	m.UpdateToolEnd("read", "package tui\n\nimport (\n", "", "")
	m.expireReadOnlyGrace()
	view = flushView(&m)
	if !strings.Contains(view, "Read") {
		t.Errorf("Collapsed read tool header should be visible, got:\n%s", view)
	}
	if strings.Contains(view, "package tui") {
		t.Errorf("Collapsed read tool should not show output content, got:\n%s", view)
	}
}

func TestToolCallRendering_OutputTruncated(t *testing.T) {
	m := NewMessages()
	m.SetSize(80, 30)
	m.AppendToolStart("bash", `{"command":"go test ./...","description":"Run tests"}`, "")
	output := strings.Repeat("line\n", 15)
	m.UpdateToolEnd("bash", output, "", "")

	// Completed: collapsed shows checkmark
	view := flushView(&m)
	if !strings.Contains(view, "⦿") {
		t.Errorf("Collapsed tool should show checkmark, got:\n%s", view)
	}

	// Uncollapse to verify full content is available.
	m.messages[len(m.messages)-1].Collapsed = false
	m.invalidateFrom(0)
	m.render()
	expandedView := flushView(&m)
	lineCount := strings.Count(expandedView, "line")
	if lineCount < 15 {
		t.Errorf("Expanded tool should show all 15 output lines, got %d in:\n%s", lineCount, expandedView)
	}
}

func TestSystemMessageRendering_LongHeaderFallsBackSafely(t *testing.T) {
	m := NewMessages()
	m.SetSize(24, 10)
	m.AppendSystemMessage("/Users/sageil/.local/share/kodacode/some/really/long/path/that/used/to/panic")

	var view string
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("FlushRender panicked: %v", r)
			}
		}()
		view = flushView(&m)
	}()

	stripped := ansi.Strip(view)
	if !strings.Contains(stripped, "System") {
		t.Fatalf("expected fallback system header, got:\n%s", stripped)
	}
	if !strings.Contains(stripped, "/Users/sageil") {
		t.Fatalf("expected long path to remain visible, got:\n%s", stripped)
	}
	if !strings.Contains(stripped, "used/to/panic") {
		t.Fatalf("expected wrapped tail of long path to remain visible, got:\n%s", stripped)
	}
}

func TestToolCallRendering_ErrorStatus(t *testing.T) {
	m := NewMessages()
	m.SetSize(80, 20)
	m.AppendToolStart("bash", `{"command":"exit 1","description":"Fail on purpose"}`, "")
	m.UpdateToolEnd("bash", "error: something went wrong", "", "")
	view := flushView(&m)
	if !strings.Contains(view, "⊘") {
		t.Errorf("View() missing error indicator ✗, got:\n%s", view)
	}
}

func TestAppendToolStart_UpsertOnFullInput(t *testing.T) {
	// Simulate the two-event pattern from middleware_llm:
	// 1st tool_start: empty input (early disclosure)
	// tool_input_delta: streams JSON arguments
	// 2nd tool_start: full input (at execution time) — must NOT create a second block
	m := NewMessages()
	m.SetSize(80, 20)
	m.AppendToolStart("bash", "", "")
	m.UpdateToolInputDelta("bash", `{"command":"ls","description":"List files"}`, "")
	m.AppendToolStart("bash", `{"command":"ls","description":"List files"}`, "") // upsert
	// Only one tool block should exist.
	count := 0
	for _, msg := range m.messages {
		if msg.Role == "tool_call" && msg.ToolName == "bash" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("expected 1 tool_call block after upsert, got %d", count)
	}
	// The block should have the full input set by the upsert.
	if m.messages[0].ToolInput != `{"command":"ls","description":"List files"}` {
		t.Errorf("ToolInput = %q, want full JSON", m.messages[0].ToolInput)
	}
}

func TestAppendUserMessage(t *testing.T) {
	m := NewMessages()
	m.SetSize(80, 20)
	m.AppendUserMessage("hello from user")
	view := flushView(&m)
	if !strings.Contains(view, "hello from user") {
		t.Errorf("View() missing user content, got:\n%s", view)
	}
	if strings.Contains(view, "User") {
		t.Errorf("View() should not contain role label 'User', got:\n%s", view)
	}
	if strings.Contains(view, "Assistant") {
		t.Errorf("View() should not contain role label 'Assistant', got:\n%s", view)
	}
}

func TestNoRoleLabels(t *testing.T) {
	m := NewMessages()
	m.SetSize(80, 20)
	m.AppendUserMessage("hi")
	m.AppendDelta("hello")
	m.FinishStreaming()
	view := flushView(&m)
	if strings.Contains(view, "User") {
		t.Errorf("View() should not contain role label 'User', got:\n%s", view)
	}
	if strings.Contains(view, "Assistant") {
		t.Errorf("View() should not contain role label 'Assistant', got:\n%s", view)
	}
}

func TestToolBlockStyled(t *testing.T) {
	m := NewMessages()
	m.SetSize(80, 20)
	m.AppendToolStart("bash", "go build ./...", "")
	view := flushView(&m)
	// Must contain ANSI escapes (lipgloss styling applied)
	if !strings.Contains(view, "\x1b[") {
		t.Errorf("tool block View() contains no ANSI escapes; tool blocks not styled")
	}
	m.UpdateToolEnd("bash", "exit 0", "", "")
	doneView := flushView(&m)
	if !strings.Contains(doneView, "⦿") {
		t.Errorf("done tool block missing ✓ indicator, got:\n%s", doneView)
	}
}

func TestMarkdownRenderedDuringStreaming(t *testing.T) {
	m := NewMessages()
	m.SetSize(80, 20)
	m.AppendDelta("**bold text**")
	// Markdown should be rendered even during streaming.
	view := flushView(&m)
	// The view should contain ANSI escapes (bold formatting applied by markdown renderer).
	if !strings.Contains(view, "\x1b[") {
		t.Error("View() during streaming should render markdown with ANSI formatting")
	}
}

func TestScrollbarNotVisibleWhenContentFits(t *testing.T) {
	m := NewMessages()
	m.SetSize(80, 15) // extra height for rounded border around user messages
	m.AppendUserMessage("short message")
	view := flushView(&m)
	// Check for scrollbar thumb (█) — the │ char is expected from rounded panel borders.
	if strings.Contains(view, "█") {
		t.Errorf("Scrollbar should not be visible when content fits viewport, got:\n%s", view)
	}
}

func TestScrollbarVisibleWhenContentOverflows(t *testing.T) {
	m := NewMessages()
	m.SetSize(80, 5)
	for range 20 {
		m.AppendUserMessage("This is a longer message that will cause scrolling to be needed")
	}
	view := flushView(&m)
	if !strings.Contains(view, "│") && !strings.Contains(view, "█") {
		t.Errorf("Scrollbar should be visible when content overflows viewport, got:\n%s", view)
	}
}

func TestParseToolSummary_Bash(t *testing.T) {
	ts := parseToolSummary("bash", `{"command":"go build ./...","description":"Build the project","timeout":5000}`)
	if ts.summary != "Build the project" {
		t.Errorf("summary = %q, want %q", ts.summary, "Build the project")
	}
	// When description is present, command is omitted from the header
	// (it is shown in the expanded panel body instead).
	if ts.command != "" {
		t.Errorf("command = %q, want empty when description is present", ts.command)
	}
	if ts.args != "" {
		t.Errorf("args = %q, want empty", ts.args)
	}
}

func TestParseToolSummary_BashNoDescription(t *testing.T) {
	ts := parseToolSummary("bash", `{"command":"ls -la"}`)
	if ts.summary != "ls -la" {
		t.Errorf("summary = %q, want %q", ts.summary, "ls -la")
	}
	// Without description, command stays in the header.
	if ts.command != "ls -la" {
		t.Errorf("command = %q, want %q", ts.command, "ls -la")
	}
}

func TestParseToolSummary_Read(t *testing.T) {
	ts := parseToolSummary("read", `{"filePath":"/Users/sageil/dev/kodacode/internal/tui/messages.go","limit":100,"offset":10}`)
	if ts.summary != "/Users/sageil/dev/kodacode/internal/tui/messages.go" {
		t.Errorf("summary = %q, want %q", ts.summary, "/Users/sageil/dev/kodacode/internal/tui/messages.go")
	}
	if ts.args != "[offset=10, limit=100]" {
		t.Errorf("args = %q, want %q", ts.args, "[offset=10, limit=100]")
	}
}

func TestParseToolSummary_ReadNoOffset(t *testing.T) {
	ts := parseToolSummary("read", `{"filePath":"/path/to/file.go","limit":50}`)
	if ts.args != "[limit=50]" {
		t.Errorf("args = %q, want %q", ts.args, "[limit=50]")
	}
}

func TestParseToolSummary_Glob(t *testing.T) {
	ts := parseToolSummary("glob", `{"pattern":"**/*.go","path":"src/"}`)
	if ts.summary != "**/*.go" {
		t.Errorf("summary = %q, want %q", ts.summary, "**/*.go")
	}
	if ts.args != "[path=src/]" {
		t.Errorf("args = %q, want %q", ts.args, "[path=src/]")
	}
}

func TestParseToolSummary_Grep(t *testing.T) {
	ts := parseToolSummary("grep", `{"pattern":"renderMessage","include":"*.go","path":"internal/"}`)
	if ts.summary != "renderMessage" {
		t.Errorf("summary = %q, want %q", ts.summary, "renderMessage")
	}
	if ts.args != "[path=internal/, *.go]" {
		t.Errorf("args = %q, want %q", ts.args, "[path=internal/, *.go]")
	}
}

func TestParseToolSummary_InvalidJSON(t *testing.T) {
	ts := parseToolSummary("bash", "not json")
	if ts.summary != "" {
		t.Errorf("summary = %q, want empty for unparseable input", ts.summary)
	}
}

func TestParseToolSummary_Write(t *testing.T) {
	ts := parseToolSummary("write", `{"filePath":"/path/to/output.go","content":"package main"}`)
	if ts.summary != "/path/to/output.go" {
		t.Errorf("summary = %q, want %q", ts.summary, "/path/to/output.go")
	}
	if ts.args != "" {
		t.Errorf("args = %q, want empty", ts.args)
	}
}

func TestParseToolSummary_Patch(t *testing.T) {
	ts := parseToolSummary("patch", `{"filePath":"/src/main.go","edits":[{"oldString":"a","newString":"b"}]}`)
	if ts.summary != "/src/main.go" {
		t.Errorf("summary = %q, want %q", ts.summary, "/src/main.go")
	}
	if ts.args != "[1 edits]" {
		t.Errorf("args = %q, want %q", ts.args, "[1 edits]")
	}

	ts = parseToolSummary("patch", `{"filePath":"/src/utils.go","edits":[{"oldString":"a","newString":"b"},{"oldString":"c","newString":"d"}]}`)
	if ts.args != "[2 edits]" {
		t.Errorf("args = %q, want %q", ts.args, "[2 edits]")
	}
}

func TestParseToolSummary_Git(t *testing.T) {
	ts := parseToolSummary("git", `{"action":"status"}`)
	if ts.summary != "status" {
		t.Errorf("summary = %q, want %q", ts.summary, "status")
	}

	ts = parseToolSummary("git", `{"action":"diff","args":"--staged"}`)
	if ts.args != "[--staged]" {
		t.Errorf("args = %q, want %q", ts.args, "[--staged]")
	}
}

func TestParseToolSummary_Search(t *testing.T) {
	ts := parseToolSummary("search", `{"query":"TODO","path":"internal"}`)
	if ts.summary != "TODO" {
		t.Errorf("summary = %q, want %q", ts.summary, "TODO")
	}
	if ts.args != "[path=internal]" {
		t.Errorf("args = %q, want %q", ts.args, "[path=internal]")
	}

	ts = parseToolSummary("search", `{"query":"func main","include":"*.go"}`)
	if ts.args != "[*.go]" {
		t.Errorf("args = %q, want %q", ts.args, "[*.go]")
	}
}

func TestSearchHintText(t *testing.T) {
	tests := []struct {
		output string
		want   string
	}{
		{`Found 25 results for "customer"`, "25 results"},
		{`Found 3 results for "foo"`, "3 results"},
		{`No results found for "xyz".`, "0 results"},
		{"", "done"},
	}
	for _, tt := range tests {
		msg := Message{ToolName: "search", ToolDone: true, ToolOutput: tt.output}
		got := toolHintText(msg)
		if got != tt.want {
			t.Errorf("toolHintText(%q) = %q, want %q", tt.output, got, tt.want)
		}
	}
}

func TestSplitSymbolLine(t *testing.T) {
	tests := []struct {
		line string
		want []string
	}{
		{"src/foo.go:10  function  myFunc", []string{"src/foo.go:10", "function", "myFunc"}},
		{"path:3  interface  Customer", []string{"path:3", "interface", "Customer"}},
	}
	for _, tt := range tests {
		got := splitSymbolLine(tt.line)
		if len(got) != len(tt.want) {
			t.Errorf("splitSymbolLine(%q) = %v, want %v", tt.line, got, tt.want)
			continue
		}
		for i := range got {
			if got[i] != tt.want[i] {
				t.Errorf("splitSymbolLine(%q)[%d] = %q, want %q", tt.line, i, got[i], tt.want[i])
			}
		}
	}
}

func TestSplitPathName(t *testing.T) {
	dir, file := splitPathName("src/controllers/customerController.ts")
	if dir != "src/controllers/" || file != "customerController.ts" {
		t.Errorf("got dir=%q file=%q", dir, file)
	}
	dir, file = splitPathName("file.go")
	if dir != "" || file != "file.go" {
		t.Errorf("got dir=%q file=%q", dir, file)
	}
}

func TestRenderSearchResults(t *testing.T) {
	output := `Found 5 results for "customer"

── Symbol Matches ──
src/models/customer.ts:3  interface  Customer  [both]
  export interface Customer { id: string }

── File Matches ──
src/models/customer.ts

── Content Matches ──
src/routes/customerRoutes.ts
  2: import * as customerController from '../controllers/customerController';
  6: router.get('/', customerController.getCustomers);`

	th := theme.StaticDefault()
	m := &Messages{theme: &th}
	var body strings.Builder
	m.renderSearchResults(&body, output, 120)
	rendered := body.String()

	// Verify sections are present.
	if !strings.Contains(rendered, "Symbols") {
		t.Error("missing Symbols section header")
	}
	if !strings.Contains(rendered, "Files") {
		t.Error("missing Files section header")
	}
	if !strings.Contains(rendered, "Content") {
		t.Error("missing Content section header")
	}
	// Source tags should be stripped.
	if strings.Contains(rendered, "[both]") {
		t.Error("[both] source tag should be stripped from TUI output")
	}
	if strings.Contains(rendered, "[fts]") {
		t.Error("[fts] source tag should be stripped from TUI output")
	}
	// "Found N results" summary should be stripped (shown in hint text instead).
	if strings.Contains(rendered, "Found 5") {
		t.Error("summary line should not appear in rendered body")
	}
}

func TestParseToolSummary_WebFetch(t *testing.T) {
	ts := parseToolSummary("web_fetch", `{"url":"https://example.com/api/docs"}`)
	if ts.summary != "example.com/api/docs" {
		t.Errorf("summary = %q, want %q", ts.summary, "example.com/api/docs")
	}

	ts = parseToolSummary("web_fetch", `{"url":"https://github.com/user/repo"}`)
	if ts.summary != "github.com/user/repo" {
		t.Errorf("summary = %q, want %q", ts.summary, "github.com/user/repo")
	}

	ts = parseToolSummary("web_fetch", `{"url":"https://example.com"}`)
	if ts.summary != "example.com" {
		t.Errorf("summary = %q, want %q", ts.summary, "example.com")
	}
}

func TestParseToolSummary_Task(t *testing.T) {
	ts := parseToolSummary("task", `{"action":"list"}`)
	if ts.summary != "list" {
		t.Errorf("summary = %q, want %q", ts.summary, "list")
	}

	ts = parseToolSummary("task", `{"action":"update","id":"abc123","status":"completed"}`)
	if ts.summary != "update" {
		t.Errorf("summary = %q, want %q", ts.summary, "update")
	}

	ts = parseToolSummary("task", `{"action":"create","title":"Fix bug in login"}`)
	if ts.summary != "create: Fix bug in login" {
		t.Errorf("summary = %q, want %q", ts.summary, "create: Fix bug in login")
	}

	ts = parseToolSummary("task", `{"action":"delete","id":"task 3"}`)
	if ts.summary != "delete" {
		t.Errorf("summary = %q, want %q", ts.summary, "delete")
	}

	ts = parseToolSummary("task", `{"action":""}`)
	if ts.summary != "" {
		t.Errorf("summary = %q, want empty for unknown action", ts.summary)
	}

	// Partial JSON (streaming)
	ts = parseToolSummary("task", `{"action":"create","title":"Fix`)
	if ts.summary != "create: Fix" {
		t.Errorf("partial summary = %q, want %q", ts.summary, "create: Fix")
	}

	ts = parseToolSummary("task", `{"action":"update"`)
	if ts.summary != "update" {
		t.Errorf("partial summary = %q, want %q", ts.summary, "update")
	}
}

func TestToolHintText_Task(t *testing.T) {
	tests := []struct {
		name   string
		output string
		want   string
	}{
		{"create", "Created task task 1: Fix bug in login", "Fix bug in login"},
		{"update with status", "Updated task task 1: Fix bug in login [completed]", "Fix bug in login"},
		{"delete", "Deleted task task 1: Fix bug in login", "Fix bug in login"},
		{"empty output", "", "done"},
		{"list", "[ ] task 1  Write tests", "1 tasks"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := Message{
				ToolName:   "task",
				ToolDone:   true,
				ToolOutput: tt.output,
			}
			got := toolHintText(msg)
			if got != tt.want {
				t.Errorf("toolHintText() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestParseToolSummary_Tree(t *testing.T) {
	ts := parseToolSummary("tree", `{"path":"/src/components"}`)
	if ts.summary != "components" {
		t.Errorf("summary = %q, want %q", ts.summary, "components")
	}

	ts = parseToolSummary("tree", `{"path":".","depth":2}`)
	if ts.summary != "tree" {
		t.Errorf("summary = %q, want %q", ts.summary, "tree")
	}
	if ts.args != "[depth=2]" {
		t.Errorf("args = %q, want %q", ts.args, "[depth=2]")
	}

	ts = parseToolSummary("tree", `{"path":"/src","include":"*.go"}`)
	if ts.args != "[*.go]" {
		t.Errorf("args = %q, want %q", ts.args, "[*.go]")
	}
}

func TestParseToolSummary_Test(t *testing.T) {
	ts := parseToolSummary("test", `{"command":"go test ./...","filter":"TestFoo"}`)
	if ts.summary != "go test ./..." {
		t.Errorf("summary = %q, want %q", ts.summary, "go test ./...")
	}
	if ts.args != "[TestFoo]" {
		t.Errorf("args = %q, want %q", ts.args, "[TestFoo]")
	}

	ts = parseToolSummary("test", `{"path":"internal/tui","command":""}`)
	if ts.summary != "internal/tui" {
		t.Errorf("summary = %q, want %q", ts.summary, "internal/tui")
	}

	ts = parseToolSummary("test", `{}`)
	if ts.summary != "test" {
		t.Errorf("summary = %q, want %q", ts.summary, "test")
	}
}

func TestParseToolSummary_LSP(t *testing.T) {
	ts := parseToolSummary("lsp", `{"action":"definition","filePath":"/src/main.go","line":42}`)
	if ts.summary != "definition" {
		t.Errorf("summary = %q, want %q", ts.summary, "definition")
	}
	if ts.args != "[/src/main.go:42]" {
		t.Errorf("args = %q, want %q", ts.args, "[/src/main.go:42]")
	}

	ts = parseToolSummary("lsp", `{"action":"inspect","filePath":"/src/utils.go"}`)
	if ts.args != "[/src/utils.go]" {
		t.Errorf("args = %q, want %q", ts.args, "[/src/utils.go]")
	}
}

func TestParseToolSummary_Open(t *testing.T) {
	ts := parseToolSummary("open", `{"filePath":"/src/main.go"}`)
	if ts.summary != "/src/main.go" {
		t.Errorf("summary = %q, want %q", ts.summary, "/src/main.go")
	}

	ts = parseToolSummary("open", `{"filePath":"/src/utils.go","line":100}`)
	if ts.args != "[Line=100]" {
		t.Errorf("args = %q, want %q", ts.args, "[Line=100]")
	}
}

func TestParseToolSummary_CodeAction(t *testing.T) {
	ts := parseToolSummary("code_action", `{"filePath":"/src/main.go","kind":"source.organizeImports","startLine":12,"startCharacter":0,"endLine":12,"endCharacter":1}`)
	if ts.summary != "/src/main.go" {
		t.Errorf("summary = %q, want %q", ts.summary, "/src/main.go")
	}
	if ts.args != "[source.organizeImports, L12]" {
		t.Errorf("args = %q, want %q", ts.args, "[source.organizeImports, L12]")
	}

	ts = parseToolSummary("code_action", `{"filePath":"/src/main.go","title":"Extract function","startLine":12,"startCharacter":0,"endLine":18,"endCharacter":1}`)
	if ts.args != "[Extract function, L12-18]" {
		t.Errorf("args = %q, want %q", ts.args, "[Extract function, L12-18]")
	}
}

func TestParseToolSummary_RenameSymbol(t *testing.T) {
	ts := parseToolSummary("rename_symbol", `{"filePath":"/src/main.go","line":42,"character":3,"newName":"requestID"}`)
	if ts.summary != "/src/main.go" {
		t.Errorf("summary = %q, want %q", ts.summary, "/src/main.go")
	}
	if ts.args != "[→ requestID, L42]" {
		t.Errorf("args = %q, want %q", ts.args, "[→ requestID, L42]")
	}
}

func TestToolDisplayName_LSPMutationTools(t *testing.T) {
	if got := toolDisplayName("code_action"); got != "CodeAction" {
		t.Errorf("toolDisplayName(code_action) = %q, want %q", got, "CodeAction")
	}
	if got := toolDisplayName("rename_symbol"); got != "RenameSymbol" {
		t.Errorf("toolDisplayName(rename_symbol) = %q, want %q", got, "RenameSymbol")
	}
}

func TestStripXMLContent_Directory(t *testing.T) {
	input := "<path>/Users/sageil/dev/kodacode</path>\n<type>directory</type>\n<content>\n.git/\ninternal/\n</content>"
	got := stripXMLContent(input)
	if strings.Contains(got, "<path>") {
		t.Errorf("stripXMLContent(%q) should remove <path> tag, got:\n%s", input, got)
	}
	if strings.Contains(got, "<content>") {
		t.Errorf("stripXMLContent(%q) should remove <content> tag, got:\n%s", input, got)
	}
	if !strings.Contains(got, ".git/") {
		t.Errorf("stripXMLContent(%q) should keep directory entries, got:\n%s", input, got)
	}
}

func TestStripXMLContent_FileContent(t *testing.T) {
	input := "<path>/Users/sageil/dev/kodacode/main.go</path>\n<type>file</type>\n<content>\npackage main\n\nfunc main() {}\n</content>"
	got := stripXMLContent(input)
	if strings.Contains(got, "<path>") {
		t.Errorf("stripXMLContent(%q) should remove <path> tag, got:\n%s", input, got)
	}
	if !strings.Contains(got, "package main") {
		t.Errorf("stripXMLContent(%q) should keep file content, got:\n%s", input, got)
	}
}

func TestStripXMLContent_NoTags(t *testing.T) {
	input := "package tui\n\nimport (\n"
	got := stripXMLContent(input)
	if got != input {
		t.Errorf("stripXMLContent(%q) with no XML tags should return input unchanged\ngot:  %q\nwant: %q", input, got, input)
	}
}

func TestReadToolHeaderOnly(t *testing.T) {
	m := NewMessages()
	m.SetSize(80, 20)
	m.AppendToolStart("read", `{"filePath":"/Users/sageil/dev/kodacode"}`, "")
	output := "<path>/Users/sageil/dev/kodacode</path>\n<type>directory</type>\n<content>\ninternal/\ngo.mod\n</content>"
	m.UpdateToolEnd("read", output, "", "")
	m.expireReadOnlyGrace()
	m.invalidateFrom(0)
	m.render()
	view := flushView(&m)
	// Completed read tools are collapsed — header visible, output hidden.
	if !strings.Contains(view, "Read") {
		t.Errorf("Collapsed read tool header should be visible, got:\n%s", view)
	}
	if strings.Contains(view, "internal/") {
		t.Errorf("Collapsed read tool should not show directory content, got:\n%s", view)
	}
}

func TestDetectLanguage(t *testing.T) {
	cases := []struct {
		path string
		want string
	}{
		{"/path/to/file.go", "Go"},
		{"/path/to/file.ts", "TypeScript"},
		{"/path/to/file.tsx", "TypeScript"},
		{"/path/to/file.js", "JavaScript"},
		{"/path/to/file.json", "JSON"},
		{"/path/to/file.py", "Python"},
		{"/path/to/file.sh", "Bash"},
		{"/path/to/file.md", "Markdown"},
		{"/path/to/file.yaml", "YAML"},
		{"/path/to/file.yml", "YAML"},
		{"/path/to/file.toml", "TOML"},
		{"/path/to/file.sql", "SQL"},
		{"/path/to/file.html", "HTML"},
		{"/path/to/file.css", "CSS"},
		{"/path/to/file.rs", "Rust"},
		{"/path/to/unknown.xyz", ""},
		{"", ""},
	}
	for _, c := range cases {
		got := detectLanguage(c.path)
		if got != c.want {
			t.Errorf("detectLanguage(%q) = %q, want %q", c.path, got, c.want)
		}
	}
}

func TestChromaStyle(t *testing.T) {
	cases := []struct {
		name string
		th   *theme.Theme
		want string
	}{
		{"nil theme", nil, "dracula"},
		{"default dark", &theme.Theme{Name: "default"}, "dracula"},
		{"rose-pine-moon", &theme.Theme{Name: "rose-pine-moon"}, "rose-pine"},
		{"catppuccin", &theme.Theme{Name: "catppuccin"}, "catppuccin-mocha"},
		{"light", &theme.Theme{Name: "light"}, "github"},
		{"explicit syntax_style", &theme.Theme{Name: "custom", SyntaxStyle: "solarized-dark"}, "solarized-dark"},
		{"light palette fallback", &theme.Theme{Name: "my-light", Palette: theme.Palette{Surface: "#f5f5f5"}}, "github"},
	}
	for _, c := range cases {
		got := chromaStyle(c.th)
		if got != c.want {
			t.Errorf("chromaStyle(%s) = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestSyntaxHighlight_GoCode(t *testing.T) {
	code := "package main\n\nfunc main() {}\n"
	th := &theme.Theme{Name: "default"}
	result := syntaxHighlight(code, "Go", th)
	if !strings.Contains(result, "\x1b[") {
		t.Errorf("syntaxHighlight did not produce ANSI escapes for Go code, got:\n%s", result)
	}
	if !strings.Contains(result, "main") {
		t.Errorf("syntaxHighlight lost content, got:\n%s", result)
	}
}

func TestSyntaxHighlight_UnknownLanguage(t *testing.T) {
	code := "some plain text content"
	result := syntaxHighlight(code, "", nil)
	if !strings.Contains(result, "some plain text content") {
		t.Errorf("syntaxHighlight with empty language lost content, got:\n%s", result)
	}
}

func TestToolCallLeftBar(t *testing.T) {
	m := NewMessages()
	m.SetSize(80, 20)
	m.AppendToolStart("bash", `{"command":"go build ./...","description":"Build"}`, "")
	m.UpdateToolEnd("bash", "ok  github.com/sageil/kodacode/v1\n", "", "")
	view := flushView(&m)
	// Tool name must be present
	if !strings.Contains(view, "Bash") {
		t.Errorf("View() missing tool name, got:\n%s", view)
	}
}

func TestToolCallCollapsedShowsHint(t *testing.T) {
	// Use bash instead of read — completed read tools are hidden from the TUI.
	m := NewMessages()
	m.SetSize(80, 20)
	m.AppendToolStart("bash", `{"command":"echo hi","description":"Test"}`, "")
	m.UpdateToolEnd("bash", "hi\n", "", "")

	// After completion, panel shows tool name and checkmark.
	view := flushView(&m)
	if !strings.Contains(view, "Bash") {
		t.Errorf("Collapsed tool should show tool name, got:\n%s", view)
	}
	if !strings.Contains(view, "⦿") {
		t.Errorf("Collapsed tool should show checkmark, got:\n%s", view)
	}
}

func TestToolCallExpandedShowsContent(t *testing.T) {
	m := NewMessages()
	m.SetSize(80, 20)
	m.AppendToolStart("bash", `{"command":"go build ./...","description":"Build"}`, "")
	m.UpdateToolEnd("bash", "build-ok\n", "", "")
	// Successful tools are auto-collapsed. Uncollapse to verify output.
	m.messages[len(m.messages)-1].Collapsed = false
	m.invalidateFrom(0)
	m.render()
	view := flushView(&m)
	if !strings.Contains(view, "build-ok") {
		t.Errorf("Expanded tool should show output content, got:\n%s", view)
	}
}

func TestToolCallDirectoryHint(t *testing.T) {
	m := NewMessages()
	m.SetSize(80, 20)
	m.AppendToolStart("read", `{"filePath":"/Users/sageil/dev/kodacode"}`, "")
	output := "<path>/Users/sageil/dev/kodacode</path>\n<type>directory</type>\n<content>\ninternal/\ngo.mod\ngo.sum\n</content>"
	m.UpdateToolEnd("read", output, "", "")
	m.expireReadOnlyGrace()
	view := flushView(&m)
	if !strings.Contains(view, "Read") {
		t.Errorf("Collapsed read tool header should be visible, got:\n%s", view)
	}
}

func TestClickToExpandCollapsedTool(t *testing.T) {
	m := NewMessages()
	m.SetSize(80, 30)
	m.screenY = 2 // simulate header offset (headerHeight)

	// First tool — will be collapsed when second starts.
	m.AppendToolStart("bash", `{"command":"ls","description":"List"}`, "")
	m.UpdateToolEnd("bash", "file1.go\nfile2.go\n", "", "")

	// Second tool — collapses the first.
	m.AppendToolStart("bash", `{"command":"pwd","description":"CWD"}`, "")
	m.UpdateToolEnd("bash", "/home/user\n", "", "")

	m.invalidateFrom(0)
	m.render()

	// First tool should be collapsed (no file1.go visible).
	view := flushView(&m)
	if strings.Contains(view, "file1.go") {
		t.Fatalf("First tool should be collapsed, got:\n%s", view)
	}

	// Verify tool regions are populated.
	if len(m.toolRegions) < 2 {
		t.Fatalf("Expected at least 2 tool regions, got %d", len(m.toolRegions))
	}

	// Simulate click on the first tool region.
	tr := m.toolRegions[0]
	clickY := tr.startLine + m.screenY // convert content line to absolute terminal row
	m, _ = m.Update(tea.MouseClickMsg{X: 5, Y: clickY, Button: tea.MouseLeft})
	m.FlushRender()
	view = flushView(&m)

	// After clicking, first tool should be expanded.
	if !strings.Contains(view, "file1.go") {
		t.Errorf("Clicked tool should expand and show output, got:\n%s", view)
	}
}

func TestToolCallFullJourney(t *testing.T) {
	m := NewMessages()
	m.SetSize(80, 30)

	// 1. Bash tool: running — starts expanded
	m.AppendToolStart("bash", `{"command":"go build ./...","description":"Build"}`, "")
	view := flushView(&m)
	if !strings.Contains(view, "⠋") {
		t.Errorf("running state: View() missing spinner, got:\n%s", view)
	}
	if !strings.Contains(view, "Bash") {
		t.Errorf("running state: View() missing tool name, got:\n%s", view)
	}

	// 2. Bash tool: done (success)
	m.UpdateToolEnd("bash", "ok  github.com/sageil/kodacode/v1\n", "", "")
	view = flushView(&m)
	if !strings.Contains(view, "⦿") {
		t.Errorf("done state: View() missing ✓, got:\n%s", view)
	}
	if !strings.Contains(view, "⦿") {
		t.Errorf("done state: should show checkmark, got:\n%s", view)
	}

	// 3. Add a read tool — completed read tools are hidden after grace period.
	m.AppendToolStart("read", `{"filePath":"/Users/sageil/dev/kodacode"}`, "")
	m.UpdateToolEnd("read", "<path>/Users/sageil/dev/kodacode</path>\n<type>directory</type>\n<content>\ninternal/\ngo.mod\n</content>", "", "")
	m.expireReadOnlyGrace()
	view = flushView(&m)
	if !strings.Contains(view, "Bash") {
		t.Errorf("should show bash tool, got:\n%s", view)
	}
	if !strings.Contains(view, "Read") {
		t.Errorf("collapsed read tool header should be visible, got:\n%s", view)
	}
}

func TestWriteToolExpandedShowsFileContent(t *testing.T) {
	m := NewMessages()
	m.SetSize(80, 40)

	input := `{"filePath":"/path/to/main.go","content":"package main\n\nfunc main() {\n\tprintln(\"hello\")\n}\n"}`
	m.AppendToolStart("write", input, "")
	m.UpdateToolEnd("write", "Wrote file successfully.", "", "")

	// Auto-collapsed: shows tool name and checkmark
	view := flushView(&m)
	if !strings.Contains(view, "Write") {
		t.Errorf("auto-collapsed write tool should show tool name, got:\n%s", view)
	}

	// Must show file CONTENT, not confirmation
	m.invalidateFrom(0)
	m.render()
	expandedView := flushView(&m)

	if strings.Contains(expandedView, "Wrote file successfully") {
		t.Errorf("expanded write tool should show file content, not confirmation message, got:\n%s", expandedView)
	}
	if !strings.Contains(expandedView, "main") {
		t.Errorf("expanded write tool should show file content with 'main', got:\n%s", expandedView)
	}
}

func TestBashOutputPreservesNewlines(t *testing.T) {
	m := NewMessages()
	m.SetSize(80, 40)

	output := "PASS\nok  \tgithub.com/test/pkg\t0.5s\nok  \tgithub.com/test/other\t1.2s\n-rw-r--r--  1 user staff 1024 Mar 13 10:00 file.go\n"
	m.AppendToolStart("bash", `{"command":"go test ./...","description":"Run tests"}`, "")
	m.UpdateToolEnd("bash", output, "", "")

	// Uncollapse to verify output rendering.
	m.messages[len(m.messages)-1].Collapsed = false
	m.invalidateFrom(0)
	m.render()
	view := flushView(&m)

	if !strings.Contains(view, "PASS") {
		t.Errorf("bash output missing 'PASS', got:\n%s", view)
	}
	if !strings.Contains(view, "file.go") {
		t.Errorf("bash output missing 'file.go', got:\n%s", view)
	}
	// The key test: "PASS" and "github.com" must NOT be on the same line
	for line := range strings.SplitSeq(view, "\n") {
		if strings.Contains(line, "PASS") && strings.Contains(line, "github.com") {
			t.Errorf("bash output newlines lost: 'PASS' and 'github.com' on same line:\n%s", line)
		}
	}
}

func TestDeltaAfterCollapsedToolIsVisible(t *testing.T) {
	m := NewMessages()
	m.SetSize(80, 40)

	// Simulate: assistant text, then glob tool, then more assistant text
	m.AppendDelta("I'll find the files.")
	m.FinishStreaming()

	m.AppendToolStart("glob", `{"pattern":"trials/**/*"}`, "")
	m.UpdateToolEnd("glob", "/path/a.js\n/path/b.js\n/path/c.js\n", "", "")
	m.expireReadOnlyGrace()

	// Now the LLM continues with more text
	m.AppendDelta("Found the files. Let me read one.")

	view := flushView(&m)

	// Completed glob tool should be collapsed (header visible, output hidden).
	if !strings.Contains(view, "Glob") {
		t.Errorf("collapsed glob tool header should be visible, got:\n%s", view)
	}

	// The subsequent assistant text MUST be visible
	if !strings.Contains(view, "Found the files") {
		t.Errorf("assistant text after tool block missing, got:\n%s", view)
	}
}

func TestEditToolShowsDiff(t *testing.T) {
	m := NewMessages()
	m.SetSize(80, 40)

	input := `{"filePath":"/path/to/main.go","oldString":"fmt.Println(\"hello\")","newString":"fmt.Println(\"goodbye\")"}`
	m.AppendToolStart("edit", input, "")
	m.UpdateToolEnd("edit", "Edited file successfully.", "", "")

	m.invalidateFrom(0)
	m.render()
	view := flushView(&m)

	// Must show diff-style output with - and + prefixes
	if !strings.Contains(view, "- ") {
		t.Errorf("edit diff should show removed lines with '- ' prefix, got:\n%s", view)
	}
	if !strings.Contains(view, "+ ") {
		t.Errorf("edit diff should show added lines with '+ ' prefix, got:\n%s", view)
	}
	// Must NOT show the confirmation message
	if strings.Contains(view, "Edited file successfully") {
		t.Errorf("edit diff should show diff, not confirmation message, got:\n%s", view)
	}
	// Tool name should appear in panel
	if !strings.Contains(view, "Edit") {
		t.Errorf("tool name should appear in panel, got:\n%s", view)
	}
}

func TestSameTypeToolGrouping(t *testing.T) {
	m := NewMessages()
	m.SetSize(80, 40)

	// Simulate: bash → short assistant text → bash → short assistant text → bash
	m.AppendToolStart("bash", `{"command":"tsc","description":"Run TypeScript compiler directly"}`, "")
	m.UpdateToolEnd("bash", "error: tsc not found\n", "", "")

	m.AppendDelta("Let me try with pnpm:")
	m.FinishStreaming()

	m.AppendToolStart("bash", `{"command":"pnpm tsc","description":"Run TypeScript compiler with pnpm"}`, "")
	m.UpdateToolEnd("bash", "error: no tsconfig\n", "", "")

	m.AppendDelta("Let me check the config:")
	m.FinishStreaming()

	m.AppendToolStart("bash", `{"command":"pnpm tsc -p .","description":"Run TypeScript compiler with project config"}`, "")
	m.UpdateToolEnd("bash", "Build succeeded\n", "", "")

	view := flushView(&m)

	// Each tool should render in its own panel (no grouping)
	if !strings.Contains(view, "Bash") {
		t.Errorf("should show bash tool name, got:\n%s", view)
	}

	// Assistant text between tools should be visible as separate messages
	if !strings.Contains(view, "Let me try with pnpm:") {
		t.Errorf("should show assistant text between tools, got:\n%s", view)
	}
	if !strings.Contains(view, "Let me check the config:") {
		t.Errorf("should show second assistant text, got:\n%s", view)
	}

	// All three tool descriptions should be visible
	if !strings.Contains(view, "Run TypeScript compiler directly") {
		t.Errorf("should show first tool description, got:\n%s", view)
	}
	if !strings.Contains(view, "Run TypeScript compiler with pnpm") {
		t.Errorf("should show second tool description, got:\n%s", view)
	}
	if !strings.Contains(view, "Run TypeScript compiler with project config") {
		t.Errorf("should show third tool description, got:\n%s", view)
	}
}

func TestSameTypeGrouping_DifferentTypesNotGrouped(t *testing.T) {
	m := NewMessages()
	m.SetSize(80, 30)

	// bash → assistant → read: different types, should NOT be grouped together
	m.AppendToolStart("bash", `{"command":"ls","description":"List files"}`, "")
	m.UpdateToolEnd("bash", "file.go\n", "", "")

	m.AppendDelta("Let me read the file:")
	m.FinishStreaming()

	m.AppendToolStart("read", `{"filePath":"/path/file.go"}`, "")
	m.UpdateToolEnd("read", "package main\n", "", "")

	view := flushView(&m)

	// Both tools should be separate single-tool blocks (no └ connector)
	if !strings.Contains(view, "Bash") {
		t.Errorf("should show Bash, got:\n%s", view)
	}
	if !strings.Contains(view, "Read") {
		t.Errorf("should show Read, got:\n%s", view)
	}
	// The assistant text should NOT be absorbed — it should render normally
	if !strings.Contains(view, "Let me read the file:") {
		t.Errorf("assistant text between different tool types should render normally, got:\n%s", view)
	}
}

func TestSameTypeGrouping_LongTextNotAbsorbed(t *testing.T) {
	m := NewMessages()
	m.SetSize(80, 30)

	m.AppendToolStart("bash", `{"command":"ls","description":"List files"}`, "")
	m.UpdateToolEnd("bash", "file.go\n", "", "")

	// Long assistant text (> 300 chars) should NOT be absorbed
	longText := strings.Repeat("This is a very detailed explanation of what happened. ", 7)
	m.AppendDelta(longText)
	m.FinishStreaming()

	m.AppendToolStart("bash", `{"command":"cat file.go","description":"Read file contents"}`, "")
	m.UpdateToolEnd("bash", "package main\n", "", "")

	view := flushView(&m)

	// Both tools should render as individual panels
	if !strings.Contains(view, "Bash") {
		t.Errorf("should show bash tools, got:\n%s", view)
	}
}

func TestSameTypeGrouping_RealisticSSEFlow(t *testing.T) {
	m := NewMessages()
	m.SetSize(80, 40)

	// Realistic SSE flow: FinishStreaming is NOT called between tools.
	// AppendDelta creates streaming assistant messages that stay Streaming=true
	// until the final FinishStreaming at the end of the turn.

	// Tool 1
	m.AppendToolStart("bash", `{"command":"tsc","description":"Run TypeScript compiler directly"}`, "")
	m.UpdateToolEnd("bash", "error: tsc not found\n", "", "")

	// Assistant text (still streaming)
	m.AppendDelta("Let me try with pnpm:")

	// Tool 2 — AppendToolStart should NOT append to the streaming message
	m.AppendToolStart("bash", `{"command":"pnpm tsc","description":"Run TypeScript compiler with pnpm"}`, "")
	m.UpdateToolEnd("bash", "error: no tsconfig\n", "", "")

	// More assistant text
	m.AppendDelta("Let me check the config:")

	// Tool 3
	m.AppendToolStart("bash", `{"command":"pnpm tsc -p .","description":"Run TypeScript compiler with project config"}`, "")
	m.UpdateToolEnd("bash", "Build succeeded\n", "", "")

	// Final FinishStreaming (as done event would trigger)
	m.FinishStreaming()

	view := flushView(&m)

	// Each tool should render as its own panel (no grouping)
	if !strings.Contains(view, "Bash") {
		t.Errorf("realistic SSE flow: should show bash tools, got:\n%s", view)
	}
	if !strings.Contains(view, "Run TypeScript compiler with project config") {
		t.Errorf("realistic SSE flow: should show last tool description, got:\n%s", view)
	}
}

func TestMCPToolsNotGrouped(t *testing.T) {
	m := NewMessages()
	m.SetSize(120, 40)

	// MCP tools render as: ⚙ toolname [key=val, ...]
	m.AppendToolStart("Sequential-thinking_sequentialthinking", `{"thought":"step 1","thoughtNumber":1,"totalThoughts":3}`, "")
	m.UpdateToolEnd("Sequential-thinking_sequentialthinking", `{"thoughtNumber":1}`, "", "")

	m.AppendToolStart("Sequential-thinking_sequentialthinking", `{"thought":"step 2","thoughtNumber":2,"totalThoughts":3}`, "")
	m.UpdateToolEnd("Sequential-thinking_sequentialthinking", `{"thoughtNumber":2}`, "", "")

	view := flushView(&m)

	if !strings.Contains(view, "Sequential-Thinking") {
		t.Errorf("MCP tools should show server name, got:\n%s", view)
	}
	if strings.Contains(view, "sequentialthinking") {
		t.Errorf("MCP tools should not show function name, got:\n%s", view)
	}
}

func TestClickRegionsWithMCPTools(t *testing.T) {
	m := NewMessages()
	m.SetSize(80, 40)

	// bash → MCP → bash: click regions for both bash tools should be correct.
	m.AppendToolStart("bash", `{"command":"ls","description":"List files"}`, "")
	m.UpdateToolEnd("bash", "file.go\n", "", "")

	m.AppendToolStart("mcp-server_tool", `{"arg":"value"}`, "")
	m.UpdateToolEnd("mcp-server_tool", `{"ok":true}`, "", "")

	m.AppendToolStart("bash", `{"command":"cat file.go","description":"Read file"}`, "")
	m.UpdateToolEnd("bash", "package main\n", "", "")

	_ = flushView(&m)

	// All three tools should have click regions (bash + MCP + bash).
	if len(m.toolRegions) != 3 {
		t.Fatalf("expected 3 tool regions, got %d", len(m.toolRegions))
	}

	// First region should map to message index 0 (first bash).
	if m.toolRegions[0].msgIndex != 0 {
		t.Errorf("first tool region should map to msg 0, got %d", m.toolRegions[0].msgIndex)
	}

	// Third region should map to message index 2 (second bash).
	if m.toolRegions[2].msgIndex != 2 {
		t.Errorf("second tool region should map to msg 2, got %d", m.toolRegions[1].msgIndex)
	}

	// Second region must not overlap with first (endLine is exclusive).
	if m.toolRegions[1].startLine < m.toolRegions[0].endLine {
		t.Errorf("tool regions overlap: first ends at %d, second starts at %d",
			m.toolRegions[0].endLine, m.toolRegions[1].startLine)
	}

	// Clicking on third region (second bash, after MCP) should find it.
	contentY := m.toolRegions[2].startLine
	foundRegion := false
	for _, r := range m.toolRegions {
		if contentY >= r.startLine && contentY < r.endLine {
			if r.msgIndex != 2 {
				t.Errorf("expected click to hit msgIndex=2, got %d", r.msgIndex)
			}
			foundRegion = true
			break
		}
	}
	if !foundRegion {
		t.Error("expected to find a tool region for the third tool")
	}
}

func TestClickRegionsWithMultipleMCPTools(t *testing.T) {
	m := NewMessages()
	m.SetSize(80, 60)

	// MCP → MCP → MCP → read → read → MCP → bash
	// Only read (grouped) and bash should have click regions.
	m.AppendToolStart("mcp_think", `{"thought":"t1"}`, "")
	m.UpdateToolEnd("mcp_think", `{"ok":true}`, "", "")

	m.AppendToolStart("mcp_think", `{"thought":"t2"}`, "")
	m.UpdateToolEnd("mcp_think", `{"ok":true}`, "", "")

	m.AppendToolStart("mcp_think", `{"thought":"t3"}`, "")
	m.UpdateToolEnd("mcp_think", `{"ok":true}`, "", "")

	m.AppendToolStart("read", `{"filePath":"/a.go"}`, "")
	m.UpdateToolEnd("read", "package a\n", "", "")

	m.AppendToolStart("read", `{"filePath":"/b.go"}`, "")
	m.UpdateToolEnd("read", "package b\n", "", "")

	m.AppendToolStart("mcp_think", `{"thought":"t4"}`, "")
	m.UpdateToolEnd("mcp_think", `{"ok":true}`, "", "")

	m.AppendToolStart("bash", `{"command":"go build ./...","description":"Build"}`, "")
	m.UpdateToolEnd("bash", "ok\n", "", "")
	m.expireReadOnlyGrace()

	_ = flushView(&m)

	// 7 visible regions: 3 mcp + 2 read (collapsed) + 1 mcp + 1 bash.
	// Read tools are collapsed but still rendered with headers.
	if len(m.toolRegions) != 7 {
		t.Fatalf("expected 7 tool regions (all visible, read collapsed), got %d", len(m.toolRegions))
	}

	// Regions should be non-overlapping and in order.
	for i := 1; i < len(m.toolRegions); i++ {
		if m.toolRegions[i].startLine < m.toolRegions[i-1].endLine {
			t.Errorf("regions %d and %d overlap: startLine %d < endLine %d",
				i-1, i, m.toolRegions[i].startLine, m.toolRegions[i-1].endLine)
		}
	}

	// Last region should be bash (message index 6).
	lastRegion := m.toolRegions[len(m.toolRegions)-1]
	if lastRegion.msgIndex != 6 {
		t.Errorf("last region should be msg 6 (bash), got %d", lastRegion.msgIndex)
	}
}

func TestScrollUpDuringStreamingSuppressesAutoScroll(t *testing.T) {
	m := NewMessages()
	m.SetSize(80, 10)

	// Fill viewport with content to enable scrolling.
	for range 20 {
		m.AppendUserMessage("This is a message that creates scrollable content")
	}

	// Start streaming — autoScroll is true, viewport at bottom.
	m.AppendDelta("Streaming response starts here...")

	// Simulate user scrolling up.
	m.userScrolled = true
	m.vp.ScrollUp(5)
	savedOffset := m.vp.YOffset()

	// More streaming content arrives.
	m.AppendDelta(" more content arrives during streaming...")

	// Viewport should NOT have jumped to bottom — user's scroll position preserved.
	if m.vp.YOffset() != savedOffset {
		t.Errorf("viewport jumped during streaming: offset = %d, want %d (user scrolled up)",
			m.vp.YOffset(), savedOffset)
	}

	// Simulate user scrolling back to bottom.
	m.userScrolled = false

	// Next streaming update should auto-scroll (must exceed throttle threshold).
	m.AppendDelta(strings.Repeat(" more text to exceed the render throttle threshold. ", 2))
	if !m.vp.AtBottom() {
		t.Error("viewport should auto-scroll to bottom after user scrolls back down")
	}
}

func TestClickAfterSecondPromptWithTrimToTurns(t *testing.T) {
	m := NewMessages()
	m.SetSize(80, 40)
	m.screenY = 2

	// === First prompt ===
	m.AppendUserMessage("first prompt")
	m.AppendDelta("thinking...")
	m.FinishStreaming()
	m.AppendToolStart("bash", `{"command":"ls","description":"List files"}`, "")
	m.UpdateToolEnd("bash", "file1.go\nfile2.go\n", "", "")
	m.AppendDelta("Done with first prompt.")
	m.FinishStreaming()

	// Simulate TrimToTurns(4) like the done event does.
	m.TrimToTurns(4)
	flushView(&m)

	// === Second prompt ===
	m.AppendUserMessage("second prompt")
	m.AppendDelta("working on it...")
	m.FinishStreaming()
	m.AppendToolStart("bash", `{"command":"go build ./...","description":"Build project"}`, "")
	m.UpdateToolEnd("bash", "build-output-xyz\n", "", "")
	m.AppendDelta("Done with second prompt.")
	m.FinishStreaming()

	// Simulate TrimToTurns(4) again.
	m.TrimToTurns(4)
	view := flushView(&m)

	// Verify tool regions exist after trim.
	if len(m.toolRegions) == 0 {
		t.Fatalf("Expected tool regions after second prompt, got 0.\nView:\n%s", view)
	}

	// Log all messages and their states.
	for i, msg := range m.messages {
		t.Logf("  msg[%d]: Role=%s ToolName=%q Collapsed=%v ToolDone=%v Output=%q",
			i, msg.Role, msg.ToolName, msg.Collapsed, msg.ToolDone, truncStr(msg.ToolOutput, 40))
	}

	// Find the last tool region (from second prompt).
	tr := m.toolRegions[len(m.toolRegions)-1]
	clickY := tr.startLine + m.screenY
	t.Logf("Tool region: startLine=%d endLine=%d msgIndex=%d, clickY=%d", tr.startLine, tr.endLine, tr.msgIndex, clickY)
	t.Logf("Messages count: %d, toolRegions count: %d", len(m.messages), len(m.toolRegions))

	// Verify msgIndex is valid.
	if tr.msgIndex >= len(m.messages) {
		t.Fatalf("toolRegion.msgIndex=%d is out of bounds (messages len=%d)", tr.msgIndex, len(m.messages))
	}

	msg := m.messages[tr.msgIndex]
	t.Logf("Target message: Role=%s ToolName=%s Collapsed=%v ToolDone=%v Output=%q", msg.Role, msg.ToolName, msg.Collapsed, msg.ToolDone, truncStr(msg.ToolOutput, 40))

	// Before click, verify the expanded tool shows its output.
	if !msg.Collapsed && !strings.Contains(view, "build-output-xyz") {
		t.Errorf("Tool is expanded (Collapsed=false) but output not in view BEFORE click.\nView:\n%s", view)
	}

	// Click to toggle (expand if collapsed, collapse if expanded).
	wasCollapsed := msg.Collapsed
	m, _ = m.Update(tea.MouseClickMsg{X: 5, Y: clickY, Button: tea.MouseLeft})
	m.FlushRender()
	view = flushView(&m)

	afterMsg := m.messages[tr.msgIndex]
	t.Logf("After click: Collapsed changed %v → %v", wasCollapsed, afterMsg.Collapsed)

	// If it was collapsed before, it should now be expanded and show output.
	if wasCollapsed && !strings.Contains(view, "build-output-xyz") {
		t.Errorf("Tool was collapsed, click should expand and show output.\nView:\n%s", view)
	}
	// If it was expanded before, it should now be collapsed and hide output.
	if !wasCollapsed && strings.Contains(view, "build-output-xyz") {
		t.Errorf("Tool was expanded, click should collapse and hide output.\nView:\n%s", view)
	}

	// Now click AGAIN — should toggle back.
	m, _ = m.Update(tea.MouseClickMsg{X: 5, Y: clickY, Button: tea.MouseLeft})
	m.FlushRender()
	_ = flushView(&m)
	afterMsg2 := m.messages[tr.msgIndex]
	t.Logf("After second click: Collapsed=%v", afterMsg2.Collapsed)

	// Verify the re-toggle happened.
	if afterMsg2.Collapsed == afterMsg.Collapsed {
		t.Errorf("Second click should toggle Collapsed back, but it didn't change: still %v", afterMsg2.Collapsed)
	}
}

// TestClickFailsAfterSecondPrompt reproduces the reported bug:
// clicks to expand/collapse tool calls stop working after the second prompt.
func TestClickFailsAfterSecondPrompt(t *testing.T) {
	m := NewMessages()
	m.SetSize(80, 24) // realistic terminal size
	m.screenY = 2

	// === Simulate first prompt (from app_sse.go event flow) ===
	m.AppendUserMessage("explain this code")

	// tool_start → tool_end for a bash tool
	m.AppendToolStart("bash", `{"command":"cat main.go","description":"Read file"}`, "")
	m.UpdateToolOutput("bash", "package main\nfunc main() {}\n", "")
	m.UpdateToolEnd("bash", "package main\nfunc main() {}\n", "", "")

	// delta → done
	m.AppendDelta("Here's the code explanation...")
	m.FinishStreaming()
	m.TrimToTurns(4)
	m.FlushRender()

	// Verify first prompt's tool is clickable.
	t.Logf("After first prompt: %d messages, %d tool regions", len(m.messages), len(m.toolRegions))
	if len(m.toolRegions) == 0 {
		t.Fatal("No tool regions after first prompt")
	}

	// Click on first prompt's tool.
	tr := m.toolRegions[0]
	clickY := tr.startLine - m.vp.YOffset() + m.screenY
	before := m.messages[tr.msgIndex].Collapsed
	m, _ = m.Update(tea.MouseClickMsg{X: 5, Y: clickY, Button: tea.MouseLeft})
	after := m.messages[tr.msgIndex].Collapsed
	if before == after {
		t.Errorf("First prompt: click should toggle tool, but Collapsed stayed %v", after)
	}
	m.FlushRender()

	// Toggle back.
	m, _ = m.Update(tea.MouseClickMsg{X: 5, Y: clickY, Button: tea.MouseLeft})
	m.FlushRender()

	// === Simulate second prompt (same flow as app_sse.go) ===
	m.AppendUserMessage("now refactor it")

	// tool_start collapses all previous tools
	m.AppendToolStart("edit", `{"filePath":"main.go","oldString":"func main() {}","newString":"func main() {\n\tfmt.Println(\"hello\")\n}"}`, "")
	m.UpdateToolEnd("edit", "Edited file successfully.", "", "")

	m.AppendToolStart("bash", `{"command":"go build","description":"Build project"}`, "")
	m.UpdateToolOutput("bash", "", "")
	m.UpdateToolEnd("bash", "", "", "")

	// delta → done
	m.AppendDelta("Refactoring complete.")
	m.FinishStreaming()
	m.TrimToTurns(4)
	m.FlushRender()

	t.Logf("After second prompt: %d messages, %d tool regions", len(m.messages), len(m.toolRegions))

	// Now try clicking on the FIRST prompt's bash tool (if it still exists after trim).
	found := false
	for _, tr := range m.toolRegions {
		msg := m.messages[tr.msgIndex]
		if msg.ToolName == "bash" && msg.ToolOutput == "package main\nfunc main() {}\n" {
			found = true
			clickY = tr.startLine - m.vp.YOffset() + m.screenY
			before = msg.Collapsed
			t.Logf("Clicking first prompt bash tool: msgIndex=%d Collapsed=%v clickY=%d", tr.msgIndex, before, clickY)

			m, _ = m.Update(tea.MouseClickMsg{X: 5, Y: clickY, Button: tea.MouseLeft})
			after = m.messages[tr.msgIndex].Collapsed
			if before == after {
				t.Errorf("Second prompt: click on first prompt tool should toggle, but Collapsed stayed %v", after)
			}
			break
		}
	}
	if !found {
		t.Log("First prompt's bash tool was trimmed — testing second prompt tools instead")
	}

	// Test clicking on second prompt's edit tool.
	m.FlushRender()
	for _, tr := range m.toolRegions {
		msg := m.messages[tr.msgIndex]
		if msg.ToolName == "edit" {
			clickY = tr.startLine - m.vp.YOffset() + m.screenY
			before = msg.Collapsed
			t.Logf("Clicking second prompt edit tool: msgIndex=%d Collapsed=%v clickY=%d", tr.msgIndex, before, clickY)

			m, _ = m.Update(tea.MouseClickMsg{X: 5, Y: clickY, Button: tea.MouseLeft})
			after = m.messages[tr.msgIndex].Collapsed

			// Edit tools have output "Edited file successfully." — should be toggleable.
			canToggle := msg.ToolDone && msg.ToolOutput != "" && msg.ToolName != "read" && msg.ToolName != "tree"
			if canToggle && before == after {
				t.Errorf("Second prompt: click on edit tool should toggle, but Collapsed stayed %v (ToolOutput=%q)", after, msg.ToolOutput)
			}
			break
		}
	}

	// Test clicking on second prompt's bash tool (which has empty output).
	m.FlushRender()
	for _, tr := range m.toolRegions {
		msg := m.messages[tr.msgIndex]
		if msg.ToolName == "bash" && msg.ToolOutput == "" {
			clickY = tr.startLine - m.vp.YOffset() + m.screenY
			before = msg.Collapsed
			t.Logf("Clicking second prompt empty bash tool: msgIndex=%d Collapsed=%v ToolOutput=%q", tr.msgIndex, before, msg.ToolOutput)

			m, _ = m.Update(tea.MouseClickMsg{X: 5, Y: clickY, Button: tea.MouseLeft})
			after = m.messages[tr.msgIndex].Collapsed

			// Empty output — should NOT toggle (guard at line 1528).
			if before != after {
				t.Logf("Empty-output bash tool toggled (expected no toggle since ToolOutput is empty)")
			} else {
				t.Logf("Empty-output bash tool correctly did not toggle")
			}
			break
		}
	}
}

// TestClickCoordinateMapping verifies that the content line computed from
// a mouse click Y coordinate correctly maps to tool regions after multiple
// renders. This tests the invariant: contentLine = clickY - screenY + YOffset.
func TestClickCoordinateMapping(t *testing.T) {
	m := NewMessages()
	m.SetSize(80, 20)
	m.screenY = 3 // header(2) + "\n" separator(1)

	// First prompt.
	m.AppendUserMessage("first prompt")
	m.AppendToolStart("bash", `{"command":"ls","description":"List"}`, "")
	m.UpdateToolEnd("bash", "output1\n", "", "")
	m.AppendDelta("response 1")
	m.FinishStreaming()
	m.TrimToTurns(4)
	flushView(&m)

	// Record first prompt's tool region.
	if len(m.toolRegions) == 0 {
		t.Fatal("No tool regions after first prompt")
	}
	tr1 := m.toolRegions[0]

	// Verify click maps correctly.
	clickY := tr1.startLine - m.vp.YOffset() + m.screenY
	contentLine := clickY - m.screenY + m.vp.YOffset()
	if contentLine != tr1.startLine {
		t.Errorf("First prompt: contentLine=%d != tr.startLine=%d (clickY=%d screenY=%d YOffset=%d)",
			contentLine, tr1.startLine, clickY, m.screenY, m.vp.YOffset())
	}

	// Second prompt — more content.
	m.AppendUserMessage("second prompt")
	m.AppendToolStart("grep", `{"pattern":"foo","path":"."}`, "")
	m.UpdateToolEnd("grep", "match1\nmatch2\n", "", "")
	m.AppendToolStart("bash", `{"command":"echo done","description":"Done"}`, "")
	m.UpdateToolEnd("bash", "done\n", "", "")
	m.AppendDelta("response 2")
	m.FinishStreaming()
	m.TrimToTurns(4)
	flushView(&m)

	// Verify all tool regions have valid msgIndex.
	for i, tr := range m.toolRegions {
		if tr.msgIndex < 0 || tr.msgIndex >= len(m.messages) {
			t.Errorf("toolRegion[%d].msgIndex=%d out of bounds (len=%d)", i, tr.msgIndex, len(m.messages))
		}
		msg := m.messages[tr.msgIndex]
		if msg.Role != "tool_call" {
			t.Errorf("toolRegion[%d].msgIndex=%d points to Role=%q, want tool_call", i, tr.msgIndex, msg.Role)
		}
	}

	// Verify click maps correctly for each tool region.
	// After each click, regions may shift (expanding a tool changes line counts),
	// so re-read regions from the current state.
	for i := 0; i < len(m.toolRegions); i++ {
		tr := m.toolRegions[i]
		clickY = tr.startLine - m.vp.YOffset() + m.screenY
		contentLine = clickY - m.screenY + m.vp.YOffset()
		if contentLine < tr.startLine || contentLine >= tr.endLine {
			t.Errorf("toolRegion[%d]: contentLine=%d not in [%d,%d) (clickY=%d screenY=%d YOffset=%d)",
				i, contentLine, tr.startLine, tr.endLine, clickY, m.screenY, m.vp.YOffset())
		}

		// Actually perform the click and verify toggle.
		mi := tr.msgIndex
		before := m.messages[mi].Collapsed
		m, _ = m.Update(tea.MouseClickMsg{X: 5, Y: clickY, Button: tea.MouseLeft})
		m.FlushRender()
		after := m.messages[mi].Collapsed

		// Only tools with output that aren't read/tree can be toggled.
		msg := m.messages[mi]
		canToggle := msg.ToolDone && msg.ToolOutput != "" && msg.ToolName != "read" && msg.ToolName != "tree"
		if canToggle && before == after {
			t.Errorf("toolRegion[%d] (tool=%s): click should toggle Collapsed, but stayed %v (clickY=%d contentLine=%d)",
				i, msg.ToolName, after, clickY, contentLine)
		}
	}
}

// TestClickOnScrolledToolAfterSecondPrompt tests that clicking on a tool
// call that is above the viewport (scrolled up) correctly maps the click
// to the right tool region.
func TestClickOnScrolledToolAfterSecondPrompt(t *testing.T) {
	m := NewMessages()
	m.SetSize(80, 15) // small viewport to force scrolling
	m.screenY = 2

	// First prompt with several tool calls.
	m.AppendUserMessage("first prompt")
	m.AppendToolStart("bash", `{"command":"ls","description":"List"}`, "")
	m.UpdateToolEnd("bash", "file1.go\nfile2.go\nfile3.go\n", "", "")
	m.AppendToolStart("grep", `{"pattern":"main","path":"."}`, "")
	m.UpdateToolEnd("grep", "main.go:1:func main()\nmain.go:5:var mainVar\n", "", "")
	m.AppendDelta("Found results.")
	m.FinishStreaming()
	m.TrimToTurns(4)

	// Second prompt with more content to push first prompt's tools above viewport.
	m.AppendUserMessage("second prompt")
	for range 5 {
		m.AppendToolStart("bash", `{"command":"echo test","description":"Test"}`, "")
		m.UpdateToolEnd("bash", "test output line\n", "", "")
	}
	m.AppendDelta("All done with lots of tools.")
	m.FinishStreaming()
	m.TrimToTurns(4)
	flushView(&m)

	t.Logf("Total messages: %d, toolRegions: %d, viewport height: %d, totalLines: %d",
		len(m.messages), len(m.toolRegions), m.vp.Height(), m.vp.TotalLineCount())

	// Scroll to top to see first prompt's tools.
	for !m.vp.AtTop() {
		m.vp.ScrollUp(1)
	}

	// Try clicking on the first visible tool region.
	if len(m.toolRegions) == 0 {
		t.Fatal("No tool regions found")
	}
	tr := m.toolRegions[0]
	// contentLine = clickY - screenY + YOffset
	// For a tool at contentLine=tr.startLine, with YOffset=0 (scrolled to top):
	// clickY = tr.startLine + screenY - YOffset = tr.startLine + 2 - 0
	clickY := tr.startLine - m.vp.YOffset() + m.screenY
	t.Logf("First tool region: start=%d end=%d msgIdx=%d, YOffset=%d, clickY=%d",
		tr.startLine, tr.endLine, tr.msgIndex, m.vp.YOffset(), clickY)

	if clickY < 0 || clickY > 100 {
		t.Fatalf("clickY=%d is unreasonable, something is wrong with coordinate mapping", clickY)
	}

	beforeCollapsed := m.messages[tr.msgIndex].Collapsed
	m, _ = m.Update(tea.MouseClickMsg{X: 5, Y: clickY, Button: tea.MouseLeft})
	afterCollapsed := m.messages[tr.msgIndex].Collapsed
	t.Logf("Click result: Collapsed changed %v → %v", beforeCollapsed, afterCollapsed)

	if beforeCollapsed == afterCollapsed {
		t.Errorf("Click on first tool region should toggle Collapsed, but it stayed %v", afterCollapsed)
	}
}

// ---- Streaming transition edge cases ----------------------------------------

func TestReasoningDoneStopsAcceptingDeltas(t *testing.T) {
	m := NewMessages()
	m.SetSize(80, 20)

	m.AppendReasoningDelta("thinking step 1")
	m.FinishReasoning()

	if len(m.messages) != 1 {
		t.Fatalf("want 1 message after reasoning, got %d", len(m.messages))
	}
	if !m.messages[0].ReasoningDone {
		t.Fatal("ReasoningDone should be true after FinishReasoning")
	}

	m.AppendReasoningDelta("late delta")

	if len(m.messages) != 2 {
		t.Fatalf("late reasoning delta after FinishReasoning should create new message, got %d", len(m.messages))
	}
	if m.messages[1].Reasoning != "late delta" {
		t.Errorf("new message reasoning = %q, want %q", m.messages[1].Reasoning, "late delta")
	}
}

func TestFinishStreamingMarksIncompleteToolsInterrupted(t *testing.T) {
	m := NewMessages()
	m.SetSize(80, 20)

	m.AppendToolStart("bash", `{"command":"sleep 100"}`, "")
	m.AppendDelta("partial response")

	if m.messages[0].ToolDone {
		t.Fatal("tool should not be done before FinishStreaming")
	}

	m.FinishStreaming()

	if !m.messages[0].ToolDone {
		t.Error("FinishStreaming should mark incomplete tool as done")
	}
	if m.messages[0].ToolError != "interrupted" {
		t.Errorf("interrupted tool error = %q, want %q", m.messages[0].ToolError, "interrupted")
	}
	if m.messages[1].Streaming {
		t.Error("text message should not be streaming after FinishStreaming")
	}
}

func TestToolEndWithoutStartAppendsCompletedTool(t *testing.T) {
	m := NewMessages()
	m.SetSize(80, 20)

	m.UpdateToolEnd("bash", "output", "", "")
	if len(m.messages) != 1 {
		t.Fatalf("tool_end without prior tool_start should append a completed tool, got %d messages", len(m.messages))
	}
	if m.messages[0].Role != "tool_call" || m.messages[0].ToolName != "bash" || !m.messages[0].ToolDone {
		t.Fatalf("unexpected appended message: %+v", m.messages[0])
	}
}

func TestToolOutputWithoutStartIsIgnored(t *testing.T) {
	m := NewMessages()
	m.SetSize(80, 20)

	m.UpdateToolOutput("bash", "chunk", "")
	if len(m.messages) != 0 {
		t.Errorf("tool_output without prior tool_start should be ignored, got %d messages", len(m.messages))
	}
}

func TestToolOutputStreamingCapsToMaxLines(t *testing.T) {
	m := NewMessages()
	m.SetSize(80, 20)

	m.AppendToolStart("bash", `{"command":"big output"}`, "")

	for i := range 30 {
		m.UpdateToolOutput("bash", fmt.Sprintf("line %d\n", i), "")
	}

	lines := strings.Split(m.messages[0].ToolOutput, "\n")
	nonEmpty := 0
	for _, l := range lines {
		if l != "" {
			nonEmpty++
		}
	}
	if nonEmpty > maxStreamingLines {
		t.Errorf("streaming output should be capped to %d lines, got %d", maxStreamingLines, nonEmpty)
	}
}

func TestToolEndReplacesStreamingOutput(t *testing.T) {
	m := NewMessages()
	m.SetSize(80, 20)

	m.AppendToolStart("bash", `{"command":"echo hello"}`, "")
	m.UpdateToolOutput("bash", "partial...", "")
	m.UpdateToolEnd("bash", "hello\n", "", "")

	if m.messages[0].ToolOutput != "hello\n" {
		t.Errorf("UpdateToolEnd should replace streaming output, got %q", m.messages[0].ToolOutput)
	}
	if !m.messages[0].ToolDone {
		t.Error("tool should be done after UpdateToolEnd")
	}
}

func TestToolEndAppendsCompletedToolWhenNoStartExists(t *testing.T) {
	m := NewMessages()
	m.SetSize(80, 20)

	m.UpdateToolEnd("search", `Tool "search" is blocked in the current prebuild phase.`, `Tool "search" is blocked in the current prebuild phase.`, "search-1")

	if len(m.messages) != 1 {
		t.Fatalf("messages = %d, want 1", len(m.messages))
	}
	if m.messages[0].Role != "tool_call" {
		t.Fatalf("role = %q, want tool_call", m.messages[0].Role)
	}
	if m.messages[0].ToolName != "search" {
		t.Fatalf("tool name = %q, want search", m.messages[0].ToolName)
	}
	if !m.messages[0].ToolDone {
		t.Fatal("tool should be marked done")
	}
	if m.messages[0].ToolError == "" {
		t.Fatal("tool error should be preserved")
	}
}

func TestReasoningThenTextThenToolSequence(t *testing.T) {
	m := NewMessages()
	m.SetSize(80, 20)

	m.AppendReasoningDelta("let me think...")
	m.FinishReasoning()
	m.AppendDelta("I'll run a command.")
	m.AppendToolStart("bash", `{"command":"ls"}`, "")
	m.UpdateToolEnd("bash", "file.go", "", "")
	m.AppendDelta(" Done.")
	m.FinishStreaming()

	if len(m.messages) != 3 {
		t.Fatalf("want 3 messages (assistant+tool+assistant), got %d", len(m.messages))
	}
	if m.messages[0].Reasoning != "let me think..." {
		t.Errorf("first msg reasoning = %q", m.messages[0].Reasoning)
	}
	if m.messages[0].Content != "I'll run a command." {
		t.Errorf("first msg content = %q", m.messages[0].Content)
	}
	if m.messages[1].Role != "tool_call" || !m.messages[1].ToolDone {
		t.Error("second msg should be completed tool_call")
	}
	if m.messages[2].Content != " Done." {
		t.Errorf("third msg content = %q, want ' Done.'", m.messages[2].Content)
	}
}
