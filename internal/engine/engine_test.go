package engine

import (
	"context"
	"strings"
	"testing"

	"github.com/sageil/kodacode/internal/prompt"
)

func TestNewRequiresCompiler(t *testing.T) {
	if _, err := New(Dependencies{}); err == nil {
		t.Fatal("New() error = nil, want compiler error")
	}
}

func TestPrepareTurnCompilesFragments(t *testing.T) {
	eng, err := New(Dependencies{
		Compiler: prompt.NewStaticCompiler(),
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	got, err := eng.PrepareTurn(context.Background(), TurnRequest{
		SessionID: "session-1",
		TurnID:    "turn-1",
		AgentID:   "builder",
		UserText:  "inspect this repo",
		Fragments: []prompt.Fragment{
			{Kind: prompt.KindPolicy, Source: prompt.SourceBuiltin, Stability: prompt.StabilityStable, Content: "base"},
			{Kind: prompt.KindRole, Source: prompt.SourceProject, Stability: prompt.StabilityStable, Content: "agent"},
		},
	})
	if err != nil {
		t.Fatalf("PrepareTurn() error = %v", err)
	}
	if !strings.Contains(got.Prompt.Document, "base") || !strings.Contains(got.Prompt.Document, "agent") {
		t.Fatalf("compiled prompt document = %q", got.Prompt.Document)
	}
}

func TestPrepareTurnAllowsEmptyUserText(t *testing.T) {
	eng, err := New(Dependencies{
		Compiler: prompt.NewStaticCompiler(),
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if _, err := eng.PrepareTurn(context.Background(), TurnRequest{
		SessionID: "session-1",
		TurnID:    "turn-1",
		AgentID:   "builder",
		Fragments: []prompt.Fragment{
			{Kind: prompt.KindPolicy, Source: prompt.SourceBuiltin, Stability: prompt.StabilityStable, Content: "base"},
		},
	}); err != nil {
		t.Fatalf("PrepareTurn() error = %v", err)
	}
}
