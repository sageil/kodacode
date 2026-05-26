package app

import (
	"testing"

	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/provider"
)

func TestResolveResumeTurnDefaultsThinkingDisabledWhenConfigMissing(t *testing.T) {
	state := events.SessionState{
		Turns: map[string]*events.TurnState{
			"turn-1": {
				UserText: "continue",
			},
		},
	}

	resolved, err := resolveResumeTurn(state, ResolveSessionTurnInput{
		TurnID:       "turn-1",
		UserText:     "continue",
		AgentID:      "builder",
		AllowedTools: []string{"read"},
	})
	if err != nil {
		t.Fatalf("resolveResumeTurn() error = %v", err)
	}
	if resolved.thinkingMode != "" {
		t.Fatalf("thinkingMode = %q, want empty", resolved.thinkingMode)
	}
}

func TestResolveResumeTurnCarriesStoredPromptCompactionSavings(t *testing.T) {
	baseInstructions := "Standard:\n- Preserve the full authoritative prompt.\n- Keep provider-facing prompt compact.\n- Record savings durably."
	cacheablePrefix := "Standard:\n- Preserve prompt.\n- Keep provider prompt compact."
	dynamicSuffix := "Runtime:\n- Record savings."
	state := events.SessionState{
		Turns: map[string]*events.TurnState{
			"turn-1": {
				UserText: "continue",
				Config: &events.TurnConfigState{
					Model:         "openai/gpt-5",
					ResponseStyle: string(ResponseStyleTerse),
				},
				Prompt: &events.PromptState{
					BaseInstructions: baseInstructions,
					CacheablePrefix:  cacheablePrefix,
					DynamicSuffix:    dynamicSuffix,
				},
			},
		},
	}

	resolved, err := resolveResumeTurn(state, ResolveSessionTurnInput{
		TurnID:       "turn-1",
		UserText:     "continue",
		AgentID:      "builder",
		AllowedTools: []string{"read"},
	})
	if err != nil {
		t.Fatalf("resolveResumeTurn() error = %v", err)
	}
	want := provider.EstimateTextTokens(baseInstructions) -
		provider.EstimateTextTokens(provider.JoinPromptSections(cacheablePrefix, dynamicSuffix))
	if resolved.promptCompactionTokensSaved != want {
		t.Fatalf("promptCompactionTokensSaved = %d, want %d", resolved.promptCompactionTokensSaved, want)
	}
	if resolved.responseStyle != ResponseStyleTerse {
		t.Fatalf("responseStyle = %q, want %q", resolved.responseStyle, ResponseStyleTerse)
	}
}

func TestResolveResumeTurnPreservesExplicitEmptyAllowedTools(t *testing.T) {
	state := events.SessionState{
		Turns: map[string]*events.TurnState{
			"turn-1": {
				UserText: "continue",
			},
		},
	}

	resolved, err := resolveResumeTurn(state, ResolveSessionTurnInput{
		TurnID:       "turn-1",
		UserText:     "continue",
		AgentID:      "builder",
		AllowedTools: []string{},
	})
	if err != nil {
		t.Fatalf("resolveResumeTurn() error = %v", err)
	}
	if resolved.allowedTools == nil {
		t.Fatal("allowedTools = nil, want explicit empty list")
	}
	if len(resolved.allowedTools) != 0 {
		t.Fatalf("allowedTools = %#v, want explicit empty list", resolved.allowedTools)
	}
}

func TestResolveResumeTurnAllowsAttachmentOnlyStoredInput(t *testing.T) {
	state := events.SessionState{
		Turns: map[string]*events.TurnState{
			"turn-1": {
				UserAttachments: []events.UserAttachmentPayload{{
					Name:     "pixel.png",
					MIMEType: "image/png",
					DataURL:  "data:image/png;base64,AA==",
				}},
			},
		},
	}

	resolved, err := resolveResumeTurn(state, ResolveSessionTurnInput{
		TurnID:       "turn-1",
		AgentID:      "builder",
		AllowedTools: []string{"read"},
	})
	if err != nil {
		t.Fatalf("resolveResumeTurn() error = %v", err)
	}
	if resolved.userText != "" {
		t.Fatalf("userText = %q, want empty attachment-only message", resolved.userText)
	}
	if len(resolved.attachments) != 1 || resolved.attachments[0].Name != "pixel.png" {
		t.Fatalf("attachments = %#v", resolved.attachments)
	}
}

func TestResolveResumeTurnPreservesSessionModelFlag(t *testing.T) {
	state := events.SessionState{
		Turns: map[string]*events.TurnState{
			"turn-1": {
				UserText: "continue",
				Config: &events.TurnConfigState{
					AgentID:              "reviewer",
					Model:                "openai/gpt-5-mini",
					PreserveSessionModel: true,
					AllowedTools:         []string{"read"},
				},
			},
		},
	}

	resolved, err := resolveResumeTurn(state, ResolveSessionTurnInput{
		TurnID: "turn-1",
	})
	if err != nil {
		t.Fatalf("resolveResumeTurn() error = %v", err)
	}
	if !resolved.preserveSessionModel {
		t.Fatalf("preserveSessionModel = false, want true")
	}
}
