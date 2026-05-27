package tui

import (
	"slices"
	"testing"

	"github.com/sageil/kodacode/internal/events"
)

func TestToolPresenterRegistryCoversKnownTools(t *testing.T) {
	want := []string{
		"apply_patch",
		"bash",
		"code_action",
		"definition",
		"delegate",
		"diagnostics",
		"git_diff",
		"git_show",
		"git_status",
		"list",
		"locate",
		"memory",
		"mkdir",
		"question",
		"read",
		"refs",
		"rename_symbol",
		"search",
		"search_skills",
		"skill",
		"symbols",
		"task",
		"task_review",
		"task_workflow",
		"test",
		"trace",
		"tree",
		"web_fetch",
		"web_search",
		"write",
	}

	got := registeredToolPresenterNames()
	slices.Sort(got)
	if !slices.Equal(got, want) {
		t.Fatalf("registered presenter names = %#v, want %#v", got, want)
	}

	for _, name := range want {
		if _, ok := toolPresenterForCall(&events.ToolCallState{ToolName: name}); !ok {
			t.Fatalf("toolPresenterForCall(%q) = false", name)
		}
	}
}

func TestToolPresentersDoNotDeclareDuplicateNames(t *testing.T) {
	seen := map[string]struct{}{}
	for _, presenter := range allToolPresenters() {
		if len(presenter.Names) == 0 {
			t.Fatal("presenter declared without names")
		}
		for _, name := range presenter.Names {
			if name == "" {
				t.Fatal("presenter declared blank name")
			}
			if _, ok := seen[name]; ok {
				t.Fatalf("duplicate presenter name %q", name)
			}
			seen[name] = struct{}{}
		}
	}
}

func TestToolPresenterOutcomeAndPathContracts(t *testing.T) {
	tests := []struct {
		name              string
		call              *events.ToolCallState
		wantCategory      toolOutcomeKind
		wantOutcomePaths  []string
		wantMutationPaths []string
	}{
		{
			name:         "read is exploration",
			call:         &events.ToolCallState{ToolName: "read", Input: `{"path":"README.md"}`},
			wantCategory: toolOutcomeExploration,
		},
		{
			name:         "test is command",
			call:         &events.ToolCallState{ToolName: "test", Input: `{"cmd":"go test ./...","path":"."}`},
			wantCategory: toolOutcomeCommand,
		},
		{
			name:         "web_search remains generic",
			call:         &events.ToolCallState{ToolName: "web_search", Input: `{"query":"golang"}`},
			wantCategory: toolOutcomeGeneric,
		},
		{
			name:              "write mutation path",
			call:              &events.ToolCallState{ToolName: "write", Input: `{"path":"internal/app.go","content":"package app\n"}`},
			wantCategory:      toolOutcomeMutation,
			wantOutcomePaths:  []string{"internal/app.go"},
			wantMutationPaths: []string{"internal/app.go"},
		},
		{
			name:             "mkdir outcome path does not become retry mutation path",
			call:             &events.ToolCallState{ToolName: "mkdir", Input: `{"path":"internal/newpkg"}`},
			wantCategory:     toolOutcomeMutation,
			wantOutcomePaths: []string{"internal/newpkg"},
		},
		{
			name:         "bash without write mutation is exploration",
			call:         &events.ToolCallState{ToolName: "bash", Input: `{"cmd":"ls","workdir":"."}`},
			wantCategory: toolOutcomeExploration,
		},
		{
			name: "bash with write mutation is mutation",
			call: &events.ToolCallState{
				ToolName: "bash",
				Input:    `{"cmd":"cat > internal/app.go","workdir":"."}`,
				WriteMutation: &events.WriteMutation{
					Path: "internal/app.go",
				},
			},
			wantCategory:      toolOutcomeMutation,
			wantOutcomePaths:  []string{"internal/app.go"},
			wantMutationPaths: []string{"internal/app.go"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := outcomeCategoryForTool(tt.call); got != tt.wantCategory {
				t.Fatalf("outcomeCategoryForTool() = %q, want %q", got, tt.wantCategory)
			}
			if got := mutationOutcomePaths(tt.call); !slices.Equal(got, tt.wantOutcomePaths) {
				t.Fatalf("mutationOutcomePaths() = %#v, want %#v", got, tt.wantOutcomePaths)
			}
			if got := mutationCallPaths(tt.call); !slices.Equal(got, tt.wantMutationPaths) {
				t.Fatalf("mutationCallPaths() = %#v, want %#v", got, tt.wantMutationPaths)
			}
		})
	}
}

func TestToolPresenterRoutesSummaryDisplayAndInspector(t *testing.T) {
	call := &events.ToolCallState{ToolName: "read", Input: `{"path":"README.md","offset":1,"limit":5}`}

	if got := toolPrimaryListSummary(call); got != "README.md\noffset: 1\nlimit: 5" {
		t.Fatalf("toolPrimaryListSummary() = %q", got)
	}
	if got := toolDisplayNameForWorkspace("/repo", call); got != "read README.md" {
		t.Fatalf("toolDisplayNameForWorkspace() = %q", got)
	}
	params := toolInspectorParams(call)
	if len(params) < 3 {
		t.Fatalf("toolInspectorParams() = %#v, want path/offset/limit params", params)
	}
	if params[0].Label != "Path" || params[0].Value != "README.md" {
		t.Fatalf("first inspector param = %#v", params[0])
	}
}

func TestApplyPatchPresenterUsesEditLanguage(t *testing.T) {
	call := &events.ToolCallState{
		ToolName: "apply_patch",
		WriteMutations: []events.WriteMutation{{
			Path:    "/repo/src/cache.ts",
			Existed: true,
		}},
	}

	if got := toolDisplayNameForWorkspace("/repo", call); got != "Edit cache.ts" {
		t.Fatalf("toolDisplayNameForWorkspace() = %q, want Edit cache.ts", got)
	}
	display, ok := mutationDisplayFromCall("/repo", call)
	if !ok {
		t.Fatal("mutationDisplayFromCall() = false")
	}
	if got := display.Summary; got != "edited src/cache.ts" {
		t.Fatalf("display.Summary = %q, want edited src/cache.ts", got)
	}
}

func TestApplyPatchNoopIsHiddenFromMutationTranscript(t *testing.T) {
	call := &events.ToolCallState{
		ToolName:  "apply_patch",
		Output:    "Patch already applied successfully. No file changes needed.",
		Completed: true,
	}

	if !isApplyPatchNoop(call) {
		t.Fatal("isApplyPatchNoop() = false")
	}
	if showMutationToolInTranscript(call) {
		t.Fatal("showMutationToolInTranscript() = true, want false")
	}
	if got := mutationToolFallbackBody(call); got != "" {
		t.Fatalf("mutationToolFallbackBody() = %q, want empty", got)
	}
}
