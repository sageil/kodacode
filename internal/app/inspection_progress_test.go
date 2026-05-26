package app

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/sageil/kodacode/internal/agent"
	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/tool"
)

func TestCollectInspectionProgressPromptEntriesIncludesSearchAndLocate(t *testing.T) {
	state := events.SessionState{
		TurnOrder: []string{"turn-1", "turn-2"},
		Turns: map[string]*events.TurnState{
			"turn-1": {
				ToolCallOrder: []string{"search-1"},
				ToolCalls: map[string]*events.ToolCallState{
					"search-1": {
						CallID:         "search-1",
						ToolName:       tool.SearchToolName,
						Input:          `{"query":"calculateProjectStats","path":"src","mode":"lexical","max_matches":20}`,
						Completed:      true,
						Succeeded:      true,
						LastUpdatedSeq: 20,
						StructuredResult: json.RawMessage(`{
							"mode": "lexical",
							"results": [
								{"path":"src/services/ProjectStatsService.ts","line":88},
								{"path":"src/controllers/ProjectController.ts","line":140}
							]
						}`),
					},
				},
			},
			"turn-2": {
				ToolCallOrder: []string{"locate-1"},
				ToolCalls: map[string]*events.ToolCallState{
					"locate-1": {
						CallID:         "locate-1",
						ToolName:       tool.LocateToolName,
						Input:          `{"query":"vite","path":".","max_matches":20}`,
						Output:         "client/vite.config.ts\nclient/package.json\n",
						Completed:      true,
						Succeeded:      true,
						LastUpdatedSeq: 30,
					},
				},
			},
		},
	}

	entries := collectInspectionProgressPromptEntries(&state)
	if len(entries) != 2 {
		t.Fatalf("entries = %#v, want 2", entries)
	}
	if entries[0].ToolName != tool.LocateToolName {
		t.Fatalf("entries[0].ToolName = %q, want locate", entries[0].ToolName)
	}
	for _, want := range []string{
		`locate "vite" under .`,
		"client/vite.config.ts",
		`search "calculateProjectStats" under src`,
		"src/services/ProjectStatsService.ts:88",
	} {
		if !strings.Contains(entries[0].Summary+"\n"+entries[1].Summary, want) {
			t.Fatalf("entries missing %q: %#v", want, entries)
		}
	}
}

func TestCollectCompactedInspectionProgressPromptEntriesRequiresHistoryCompaction(t *testing.T) {
	state := events.SessionState{
		TurnOrder: []string{"turn-1"},
		Turns: map[string]*events.TurnState{
			"turn-1": {
				ToolCallOrder: []string{"search-1"},
				ToolCalls: map[string]*events.ToolCallState{
					"search-1": {
						CallID:    "search-1",
						ToolName:  tool.SearchToolName,
						Input:     `{"query":"cache","path":"src","mode":"lexical"}`,
						Completed: true,
						Succeeded: true,
					},
				},
			},
		},
	}

	if entries := collectCompactedInspectionProgressPromptEntries(&state); len(entries) != 0 {
		t.Fatalf("entries without compaction = %#v, want none", entries)
	}

	state.TurnOrder = append([]string{"compaction-turn"}, state.TurnOrder...)
	state.Turns["compaction-turn"] = &events.TurnState{
		Continuation: &events.HistoryContinuationState{
			ConsolidatedTurnCount: 1,
			RenderedSummary:       "History continuation",
		},
	}
	if entries := collectCompactedInspectionProgressPromptEntries(&state); len(entries) != 1 {
		t.Fatalf("entries with compaction = %#v, want 1", entries)
	}
}

func TestRuntimeInspectionProgressFragmentContentDiscouragesCompactionReplay(t *testing.T) {
	content, ok := runtimeInspectionProgressFragmentContent([]inspectionProgressPromptEntry{{
		ToolName: tool.SearchToolName,
		Summary:  `search "Task.find" under src; returned src/services/ProjectStatsService.ts:88.`,
	}})
	if !ok {
		t.Fatal("ok = false, want true")
	}
	for _, want := range []string{
		"Search/locate progress",
		"Compaction may hide raw tool output",
		"Do not rerun exact search/locate calls after compaction",
		"read specific result paths",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("content missing %q:\n%s", want, content)
		}
	}
}

func TestDefaultTurnFragmentsIncludesInspectionProgress(t *testing.T) {
	fragments, err := defaultTurnFragments(
		agent.Definition{},
		"/repo",
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		ResponseStyleDefault,
		ExecutionConfig{},
		[]inspectionProgressPromptEntry{{
			ToolName: tool.SearchToolName,
			Summary:  `search "cache" under src; returned src/cache.ts:1.`,
		}},
	)
	if err != nil {
		t.Fatalf("defaultTurnFragments() error = %v", err)
	}
	for _, fragment := range fragments {
		if fragment.Key == "inspection-progress" && strings.Contains(fragment.Content, `search "cache" under src`) {
			return
		}
	}
	t.Fatalf("inspection-progress fragment missing: %#v", fragments)
}
