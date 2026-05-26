package events

import (
	"math"
	"testing"
	"time"
)

func TestTurnProviderUsageRecordedPayloadRequiresPositiveStepAndAttempt(t *testing.T) {
	event := Event{
		SessionID: "session-1",
		TurnID:    "turn-1",
		Sequence:  0,
		Time:      time.Now().UTC(),
		Type:      TypeTurnProviderUsageRecorded,
		Payload: TurnProviderUsageRecordedPayload{
			Step:    0,
			Attempt: 1,
		},
	}

	if err := event.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want validation error")
	}
}

func TestProjectorAccumulatesTurnProviderUsage(t *testing.T) {
	projector := NewProjector("session-1")

	eventsToApply := []Event{
		{
			SessionID: "session-1",
			TurnID:    "turn-1",
			Sequence:  0,
			Time:      time.Now().UTC(),
			Type:      TypeTurnProviderUsageRecorded,
			Payload: TurnProviderUsageRecordedPayload{
				Model:                                 "openai/gpt-5",
				Step:                                  1,
				Attempt:                               1,
				EstimatedRequestTokens:                1000,
				EstimatedPromptTokens:                 220,
				EstimatedConversationTokens:           340,
				EstimatedToolNameTokens:               20,
				EstimatedToolDescriptionTokens:        120,
				EstimatedToolSchemaTokens:             300,
				EstimatedPromptCompactionTokensSaved:  140,
				EstimatedHistoryCompactionTokensSaved: 900,
				EstimatedToolDescriptionTokensSaved:   20,
				EstimatedToolSchemaTokensSaved:        180,
				EstimatedInputSavingsCost:             0.002125,
				ToolCount:                             4,
				EstimatedCompletionTokens:             200,
				EstimatedInputCost:                    0.00125,
				EstimatedOutputCost:                   0.002,
			},
		},
		{
			SessionID: "session-1",
			TurnID:    "turn-1",
			Sequence:  1,
			Time:      time.Now().UTC(),
			Type:      TypeTurnProviderUsageRecorded,
			Payload: TurnProviderUsageRecordedPayload{
				Model:                     "openai/gpt-5",
				Step:                      2,
				Attempt:                   1,
				EstimatedRequestTokens:    800,
				EstimatedCompletionTokens: 120,
				EstimatedInputCost:        0.001,
				EstimatedOutputCost:       0.0012,
			},
		},
	}
	for _, event := range eventsToApply {
		if err := projector.Apply(event); err != nil {
			t.Fatalf("Apply(%s) error = %v", event.Type, err)
		}
	}

	state := projector.Snapshot()
	turn := state.Turns["turn-1"]
	if turn == nil || turn.ProviderUsage == nil {
		t.Fatal("turn provider usage missing")
	}
	if turn.ProviderUsage.Model != "openai/gpt-5" {
		t.Fatalf("Model = %q", turn.ProviderUsage.Model)
	}
	if turn.ProviderUsage.Steps != 2 {
		t.Fatalf("Steps = %d, want 2", turn.ProviderUsage.Steps)
	}
	if turn.ProviderUsage.Attempts != 2 {
		t.Fatalf("Attempts = %d, want 2", turn.ProviderUsage.Attempts)
	}
	if turn.ProviderUsage.RequestTokens != 1800 {
		t.Fatalf("RequestTokens = %d, want 1800", turn.ProviderUsage.RequestTokens)
	}
	if turn.ProviderUsage.CompletionTokens != 320 {
		t.Fatalf("CompletionTokens = %d, want 320", turn.ProviderUsage.CompletionTokens)
	}
	if math.Abs(turn.ProviderUsage.EstimatedInputCost-0.00225) > 1e-9 {
		t.Fatalf("EstimatedInputCost = %f, want 0.002250", turn.ProviderUsage.EstimatedInputCost)
	}
	if math.Abs(turn.ProviderUsage.EstimatedOutputCost-0.0032) > 1e-9 {
		t.Fatalf("EstimatedOutputCost = %f, want 0.003200", turn.ProviderUsage.EstimatedOutputCost)
	}
	if len(turn.ProviderAttempts) != 2 {
		t.Fatalf("ProviderAttempts = %d, want 2", len(turn.ProviderAttempts))
	}
	if turn.ProviderAttempts[0].Step != 1 || turn.ProviderAttempts[0].Attempt != 1 {
		t.Fatalf("first provider attempt = %#v", turn.ProviderAttempts[0])
	}
	if turn.ProviderAttempts[0].PromptTokens != 220 || turn.ProviderAttempts[0].ConversationTokens != 340 {
		t.Fatalf("first provider attempt request mix = %#v", turn.ProviderAttempts[0])
	}
	if turn.ProviderAttempts[0].ToolNameTokens != 20 || turn.ProviderAttempts[0].ToolDescriptionTokens != 120 || turn.ProviderAttempts[0].ToolSchemaTokens != 300 {
		t.Fatalf("first provider attempt tool mix = %#v", turn.ProviderAttempts[0])
	}
	if turn.ProviderAttempts[0].PromptCompactionTokensSaved != 140 ||
		turn.ProviderAttempts[0].HistoryCompactionTokensSaved != 900 ||
		turn.ProviderAttempts[0].ToolDescriptionTokensSaved != 20 ||
		turn.ProviderAttempts[0].ToolSchemaTokensSaved != 180 {
		t.Fatalf("first provider attempt savings = %#v", turn.ProviderAttempts[0])
	}
	if math.Abs(turn.ProviderAttempts[0].EstimatedInputSavingsCost-0.002125) > 1e-9 {
		t.Fatalf("first provider attempt EstimatedInputSavingsCost = %f, want 0.002125", turn.ProviderAttempts[0].EstimatedInputSavingsCost)
	}
	if turn.ProviderAttempts[0].ToolCount != 4 {
		t.Fatalf("first provider attempt ToolCount = %d, want 4", turn.ProviderAttempts[0].ToolCount)
	}
	if turn.ProviderAttempts[1].Step != 2 || turn.ProviderAttempts[1].Attempt != 1 {
		t.Fatalf("second provider attempt = %#v", turn.ProviderAttempts[1])
	}
}

func TestProjectorIgnoresRejectedProviderAttemptUsage(t *testing.T) {
	projector := NewProjector("session-1")

	event := Event{
		SessionID: "session-1",
		TurnID:    "turn-1",
		Sequence:  0,
		Time:      time.Now().UTC(),
		Type:      TypeTurnProviderUsageRecorded,
		Payload: TurnProviderUsageRecordedPayload{
			Model:                                     "mistral/devstral-small-2505",
			Step:                                      1,
			Attempt:                                   1,
			RequestStarted:                            false,
			EstimatedRequestTokens:                    900,
			EstimatedPromptTokens:                     220,
			EstimatedConversationTokens:               340,
			EstimatedToolNameTokens:                   12,
			EstimatedToolDescriptionTokens:            34,
			EstimatedToolSchemaTokens:                 56,
			EstimatedPromptCompactionTokensSaved:      100,
			EstimatedHistoryCompactionTokensSaved:     200,
			EstimatedCurrentTurnProjectionTokensSaved: 300,
			EstimatedToolDescriptionTokensSaved:       40,
			EstimatedToolSchemaTokensSaved:            50,
			EstimatedInputSavingsCost:                 0.00001,
			ToolCount:                                 3,
			EstimatedCompletionTokens:                 0,
			EstimatedInputCost:                        0.00009,
			EstimatedOutputCost:                       0,
			Error:                                     "invalid model",
		},
	}
	if err := projector.Apply(event); err != nil {
		t.Fatalf("Apply(%s) error = %v", event.Type, err)
	}

	state := projector.Snapshot()
	turn := state.Turns["turn-1"]
	if turn == nil || turn.ProviderUsage == nil {
		t.Fatal("turn provider usage missing")
	}
	if turn.ProviderUsage.RequestTokens != 0 || turn.ProviderUsage.CompletionTokens != 0 {
		t.Fatalf("ProviderUsage tokens = %d/%d, want 0/0", turn.ProviderUsage.RequestTokens, turn.ProviderUsage.CompletionTokens)
	}
	if turn.ProviderUsage.EstimatedInputCost != 0 || turn.ProviderUsage.EstimatedOutputCost != 0 {
		t.Fatalf("ProviderUsage estimated cost = %f/%f, want 0/0", turn.ProviderUsage.EstimatedInputCost, turn.ProviderUsage.EstimatedOutputCost)
	}
	if len(turn.ProviderAttempts) != 1 {
		t.Fatalf("ProviderAttempts = %d, want 1", len(turn.ProviderAttempts))
	}
	attempt := turn.ProviderAttempts[0]
	if attempt.RequestTokens != 0 || attempt.CompletionTokens != 0 {
		t.Fatalf("attempt tokens = %d/%d, want 0/0", attempt.RequestTokens, attempt.CompletionTokens)
	}
	if attempt.EstimatedInputCost != 0 || attempt.EstimatedOutputCost != 0 {
		t.Fatalf("attempt cost = %f/%f, want 0/0", attempt.EstimatedInputCost, attempt.EstimatedOutputCost)
	}
	if attempt.PromptTokens != 0 || attempt.ConversationTokens != 0 ||
		attempt.ToolNameTokens != 0 || attempt.ToolDescriptionTokens != 0 || attempt.ToolSchemaTokens != 0 ||
		attempt.ToolCount != 0 {
		t.Fatalf("attempt request mix = %#v, want unstarted estimate cleared", attempt)
	}
	if attempt.PromptCompactionTokensSaved != 0 || attempt.HistoryCompactionTokensSaved != 0 ||
		attempt.CurrentTurnProjectionTokensSaved != 0 || attempt.ToolDescriptionTokensSaved != 0 ||
		attempt.ToolSchemaTokensSaved != 0 || attempt.EstimatedInputSavingsCost != 0 {
		t.Fatalf("attempt savings = %#v, want unstarted estimate cleared", attempt)
	}
}

func TestProjectorRetainsStartedRejectedProviderAttemptInputUsage(t *testing.T) {
	projector := NewProjector("session-1")

	event := Event{
		SessionID: "session-1",
		TurnID:    "turn-1",
		Sequence:  0,
		Time:      time.Now().UTC(),
		Type:      TypeTurnProviderUsageRecorded,
		Payload: TurnProviderUsageRecordedPayload{
			Model:                       "mistral/devstral-small-2505",
			Step:                        1,
			Attempt:                     1,
			RequestStarted:              true,
			EstimatedRequestTokens:      900,
			EstimatedPromptTokens:       220,
			EstimatedConversationTokens: 340,
			EstimatedCompletionTokens:   0,
			EstimatedInputCost:          0.00009,
			EstimatedOutputCost:         0,
			Error:                       "invalid model",
		},
	}
	if err := projector.Apply(event); err != nil {
		t.Fatalf("Apply(%s) error = %v", event.Type, err)
	}

	state := projector.Snapshot()
	turn := state.Turns["turn-1"]
	if turn == nil || turn.ProviderUsage == nil {
		t.Fatal("turn provider usage missing")
	}
	if turn.ProviderUsage.RequestTokens != 900 || turn.ProviderUsage.CompletionTokens != 0 {
		t.Fatalf("ProviderUsage tokens = %d/%d, want 900/0", turn.ProviderUsage.RequestTokens, turn.ProviderUsage.CompletionTokens)
	}
	if math.Abs(turn.ProviderUsage.EstimatedInputCost-0.00009) > 1e-9 || turn.ProviderUsage.EstimatedOutputCost != 0 {
		t.Fatalf("ProviderUsage estimated cost = %f/%f, want 0.00009/0", turn.ProviderUsage.EstimatedInputCost, turn.ProviderUsage.EstimatedOutputCost)
	}
	if len(turn.ProviderAttempts) != 1 {
		t.Fatalf("ProviderAttempts = %d, want 1", len(turn.ProviderAttempts))
	}
	attempt := turn.ProviderAttempts[0]
	if !attempt.RequestStarted {
		t.Fatalf("attempt RequestStarted = false, want true")
	}
	if attempt.RequestTokens != 900 || attempt.CompletionTokens != 0 {
		t.Fatalf("attempt tokens = %d/%d, want 900/0", attempt.RequestTokens, attempt.CompletionTokens)
	}
	if math.Abs(attempt.EstimatedInputCost-0.00009) > 1e-9 || attempt.EstimatedOutputCost != 0 {
		t.Fatalf("attempt cost = %f/%f, want 0.00009/0", attempt.EstimatedInputCost, attempt.EstimatedOutputCost)
	}
	if attempt.PromptTokens != 220 || attempt.ConversationTokens != 340 {
		t.Fatalf("attempt request mix = %#v, want prompt diagnostics retained", attempt)
	}
}

func TestProjectorNormalizesRejectedProviderUsageFromSnapshot(t *testing.T) {
	projector := NewProjectorFromSnapshot(SessionState{
		SessionID: "session-1",
		TurnOrder: []string{"turn-1"},
		Turns: map[string]*TurnState{
			"turn-1": {
				TurnID: "turn-1",
				ProviderUsage: &TurnProviderUsageState{
					Model:               "mistral/devstral-small-2505",
					Steps:               1,
					Attempts:            1,
					RequestTokens:       900,
					CompletionTokens:    0,
					EstimatedInputCost:  0.00009,
					EstimatedOutputCost: 0,
				},
				ProviderAttempts: []TurnProviderAttemptState{{
					Model:                            "mistral/devstral-small-2505",
					Step:                             1,
					Attempt:                          1,
					RequestStarted:                   false,
					RequestTokens:                    900,
					PromptTokens:                     220,
					ConversationTokens:               340,
					ToolNameTokens:                   12,
					ToolDescriptionTokens:            34,
					ToolSchemaTokens:                 56,
					PromptCompactionTokensSaved:      100,
					HistoryCompactionTokensSaved:     200,
					CurrentTurnProjectionTokensSaved: 300,
					ToolDescriptionTokensSaved:       40,
					ToolSchemaTokensSaved:            50,
					EstimatedInputSavingsCost:        0.00001,
					ToolCount:                        3,
					CompletionTokens:                 0,
					EstimatedInputCost:               0.00009,
					EstimatedOutputCost:              0,
					Error:                            "invalid model",
				}},
			},
		},
	})

	turn := projector.Snapshot().Turns["turn-1"]
	if turn == nil || turn.ProviderUsage == nil {
		t.Fatal("turn provider usage missing")
	}
	if turn.ProviderUsage.RequestTokens != 0 || turn.ProviderUsage.CompletionTokens != 0 {
		t.Fatalf("ProviderUsage tokens = %d/%d, want 0/0", turn.ProviderUsage.RequestTokens, turn.ProviderUsage.CompletionTokens)
	}
	if turn.ProviderUsage.EstimatedInputCost != 0 || turn.ProviderUsage.EstimatedOutputCost != 0 {
		t.Fatalf("ProviderUsage estimated cost = %f/%f, want 0/0", turn.ProviderUsage.EstimatedInputCost, turn.ProviderUsage.EstimatedOutputCost)
	}
	if len(turn.ProviderAttempts) != 1 {
		t.Fatalf("ProviderAttempts = %d, want 1", len(turn.ProviderAttempts))
	}
	if turn.ProviderAttempts[0].RequestTokens != 0 ||
		turn.ProviderAttempts[0].PromptTokens != 0 ||
		turn.ProviderAttempts[0].ConversationTokens != 0 ||
		turn.ProviderAttempts[0].ToolCount != 0 ||
		turn.ProviderAttempts[0].EstimatedInputCost != 0 {
		t.Fatalf("attempt = %#v, want rejected snapshot usage normalized", turn.ProviderAttempts[0])
	}
	if turn.ProviderAttempts[0].PromptCompactionTokensSaved != 0 ||
		turn.ProviderAttempts[0].HistoryCompactionTokensSaved != 0 ||
		turn.ProviderAttempts[0].CurrentTurnProjectionTokensSaved != 0 ||
		turn.ProviderAttempts[0].EstimatedInputSavingsCost != 0 {
		t.Fatalf("attempt savings = %#v, want rejected snapshot usage normalized", turn.ProviderAttempts[0])
	}
}

func TestProjectorRetainsStartedRejectedProviderUsageFromSnapshot(t *testing.T) {
	projector := NewProjectorFromSnapshot(SessionState{
		SessionID: "session-1",
		TurnOrder: []string{"turn-1"},
		Turns: map[string]*TurnState{
			"turn-1": {
				TurnID: "turn-1",
				ProviderUsage: &TurnProviderUsageState{
					Model:               "mistral/devstral-small-2505",
					Steps:               1,
					Attempts:            1,
					RequestTokens:       900,
					CompletionTokens:    0,
					EstimatedInputCost:  0.00009,
					EstimatedOutputCost: 0,
				},
				ProviderAttempts: []TurnProviderAttemptState{{
					Model:               "mistral/devstral-small-2505",
					Step:                1,
					Attempt:             1,
					RequestStarted:      true,
					RequestTokens:       900,
					PromptTokens:        220,
					ConversationTokens:  340,
					CompletionTokens:    0,
					EstimatedInputCost:  0.00009,
					EstimatedOutputCost: 0,
					Error:               "invalid model",
				}},
			},
		},
	})

	turn := projector.Snapshot().Turns["turn-1"]
	if turn == nil || turn.ProviderUsage == nil {
		t.Fatal("turn provider usage missing")
	}
	if turn.ProviderUsage.RequestTokens != 900 || turn.ProviderUsage.CompletionTokens != 0 {
		t.Fatalf("ProviderUsage tokens = %d/%d, want 900/0", turn.ProviderUsage.RequestTokens, turn.ProviderUsage.CompletionTokens)
	}
	if math.Abs(turn.ProviderUsage.EstimatedInputCost-0.00009) > 1e-9 || turn.ProviderUsage.EstimatedOutputCost != 0 {
		t.Fatalf("ProviderUsage estimated cost = %f/%f, want 0.00009/0", turn.ProviderUsage.EstimatedInputCost, turn.ProviderUsage.EstimatedOutputCost)
	}
	if len(turn.ProviderAttempts) != 1 {
		t.Fatalf("ProviderAttempts = %d, want 1", len(turn.ProviderAttempts))
	}
	if !turn.ProviderAttempts[0].RequestStarted {
		t.Fatalf("attempt RequestStarted = false, want true")
	}
	if turn.ProviderAttempts[0].RequestTokens != 900 || turn.ProviderAttempts[0].EstimatedInputCost != 0.00009 {
		t.Fatalf("attempt = %#v, want started snapshot usage retained", turn.ProviderAttempts[0])
	}
}

func TestProjectorAccumulatesReportedProviderUsage(t *testing.T) {
	projector := NewProjector("session-1")

	eventsToApply := []Event{
		{
			SessionID: "session-1",
			TurnID:    "turn-1",
			Sequence:  0,
			Time:      time.Now().UTC(),
			Type:      TypeTurnProviderUsageRecorded,
			Payload: TurnProviderUsageRecordedPayload{
				Model:                     "openai/gpt-5",
				Step:                      1,
				Attempt:                   1,
				EstimatedRequestTokens:    1400,
				EstimatedCompletionTokens: 120,
				EstimatedInputCost:        0.0042,
				EstimatedOutputCost:       0.001,
			},
		},
		{
			SessionID: "session-1",
			TurnID:    "turn-1",
			Sequence:  1,
			Time:      time.Now().UTC(),
			Type:      TypeTurnProviderUsageReported,
			Payload: TurnProviderUsageReportedPayload{
				Model:                     "openai/gpt-5",
				RequestID:                 "resp_123",
				Step:                      1,
				Attempt:                   1,
				InputTokens:               1200,
				CacheReadInputTokens:      320,
				OutputTokens:              180,
				ReasoningTokens:           60,
				TotalTokens:               1380,
				EstimatedInputCost:        0.00162,
				EstimatedOutputCost:       0.0018,
				EstimatedCacheSavingsCost: 0.00038,
				CachePricingApplied:       true,
			},
		},
	}
	for _, event := range eventsToApply {
		if err := projector.Apply(event); err != nil {
			t.Fatalf("Apply(%s) error = %v", event.Type, err)
		}
	}

	state := projector.Snapshot()
	turn := state.Turns["turn-1"]
	if turn == nil || turn.ProviderReportedUsage == nil {
		t.Fatal("turn provider reported usage missing")
	}
	if turn.ProviderReportedUsage.Model != "openai/gpt-5" {
		t.Fatalf("Model = %q", turn.ProviderReportedUsage.Model)
	}
	if turn.ProviderReportedUsage.RequestID != "resp_123" {
		t.Fatalf("RequestID = %q", turn.ProviderReportedUsage.RequestID)
	}
	if turn.ProviderReportedUsage.InputTokens != 1200 || turn.ProviderReportedUsage.CacheReadInputTokens != 320 || turn.ProviderReportedUsage.CacheWriteInputTokens != 0 {
		t.Fatalf("reported input tokens = %#v", turn.ProviderReportedUsage)
	}
	if !turn.ProviderReportedUsage.CachePricingApplied || turn.ProviderReportedUsage.CachePricingMissing {
		t.Fatalf("cache pricing flags = %#v", turn.ProviderReportedUsage)
	}
	if math.Abs(turn.ProviderReportedUsage.EstimatedCacheSavingsCost-0.00038) > 1e-9 {
		t.Fatalf("EstimatedCacheSavingsCost = %f, want 0.000380", turn.ProviderReportedUsage.EstimatedCacheSavingsCost)
	}
	if math.Abs(turn.ProviderUsage.EstimatedInputCost-0.00162) > 1e-9 {
		t.Fatalf("EstimatedInputCost = %f, want 0.001620", turn.ProviderUsage.EstimatedInputCost)
	}
	if math.Abs(turn.ProviderUsage.EstimatedOutputCost-0.0018) > 1e-9 {
		t.Fatalf("EstimatedOutputCost = %f, want 0.001800", turn.ProviderUsage.EstimatedOutputCost)
	}
	if len(turn.ProviderAttempts) != 1 {
		t.Fatalf("ProviderAttempts = %d, want 1", len(turn.ProviderAttempts))
	}
	attempt := turn.ProviderAttempts[0]
	if attempt.ReportedInputTokens != 1200 || attempt.ReportedCacheReadInputTokens != 320 || attempt.ReportedCacheWriteInputTokens != 0 {
		t.Fatalf("reported attempt = %#v", attempt)
	}
	if attempt.ReportedOutputTokens != 180 || attempt.ReportedReasoningTokens != 60 || attempt.ReportedTotalTokens != 1380 {
		t.Fatalf("reported attempt totals = %#v", attempt)
	}
	if math.Abs(attempt.EstimatedCacheSavingsCost-0.00038) > 1e-9 {
		t.Fatalf("reported attempt cache savings = %#v", attempt)
	}
	if math.Abs(attempt.EstimatedInputCost-0.00162) > 1e-9 || math.Abs(attempt.EstimatedOutputCost-0.0018) > 1e-9 {
		t.Fatalf("reported attempt cost = %#v", attempt)
	}
}

func TestProjectorKeepsHybridTokenTotalsWhenOnlySomeAttemptsReportUsage(t *testing.T) {
	projector := NewProjector("session-1")

	eventsToApply := []Event{
		{
			SessionID: "session-1",
			TurnID:    "turn-1",
			Sequence:  0,
			Time:      time.Now().UTC(),
			Type:      TypeTurnProviderUsageRecorded,
			Payload: TurnProviderUsageRecordedPayload{
				Model:                     "openai/gpt-5",
				Step:                      1,
				Attempt:                   1,
				EstimatedRequestTokens:    900,
				EstimatedCompletionTokens: 100,
				EstimatedInputCost:        0.0045,
				EstimatedOutputCost:       0.0005,
			},
		},
		{
			SessionID: "session-1",
			TurnID:    "turn-1",
			Sequence:  1,
			Time:      time.Now().UTC(),
			Type:      TypeTurnProviderUsageRecorded,
			Payload: TurnProviderUsageRecordedPayload{
				Model:                     "openai/gpt-5",
				Step:                      2,
				Attempt:                   1,
				EstimatedRequestTokens:    700,
				EstimatedCompletionTokens: 80,
				EstimatedInputCost:        0.0035,
				EstimatedOutputCost:       0.0004,
			},
		},
		{
			SessionID: "session-1",
			TurnID:    "turn-1",
			Sequence:  2,
			Time:      time.Now().UTC(),
			Type:      TypeTurnProviderUsageReported,
			Payload: TurnProviderUsageReportedPayload{
				Model:               "openai/gpt-5",
				RequestID:           "resp_123",
				Step:                2,
				Attempt:             1,
				InputTokens:         650,
				OutputTokens:        90,
				ReasoningTokens:     30,
				TotalTokens:         740,
				EstimatedInputCost:  0.00325,
				EstimatedOutputCost: 0.00045,
			},
		},
	}
	for _, event := range eventsToApply {
		if err := projector.Apply(event); err != nil {
			t.Fatalf("Apply(%s) error = %v", event.Type, err)
		}
	}

	state := projector.Snapshot()
	turn := state.Turns["turn-1"]
	if turn == nil || turn.ProviderUsage == nil {
		t.Fatal("turn provider usage missing")
	}
	if turn.ProviderUsage.RequestTokens != 1550 {
		t.Fatalf("RequestTokens = %d, want 1550", turn.ProviderUsage.RequestTokens)
	}
	if turn.ProviderUsage.CompletionTokens != 190 {
		t.Fatalf("CompletionTokens = %d, want 190", turn.ProviderUsage.CompletionTokens)
	}
	if math.Abs(turn.ProviderUsage.EstimatedInputCost-0.00775) > 1e-9 {
		t.Fatalf("EstimatedInputCost = %f, want 0.007750", turn.ProviderUsage.EstimatedInputCost)
	}
	if math.Abs(turn.ProviderUsage.EstimatedOutputCost-0.00095) > 1e-9 {
		t.Fatalf("EstimatedOutputCost = %f, want 0.000950", turn.ProviderUsage.EstimatedOutputCost)
	}
}

func TestProjectorMatchesReportedProviderUsageByKind(t *testing.T) {
	projector := NewProjector("session-1")

	eventsToApply := []Event{
		{
			SessionID: "session-1",
			TurnID:    "turn-1",
			Sequence:  0,
			Time:      time.Now().UTC(),
			Type:      TypeTurnProviderUsageRecorded,
			Payload: TurnProviderUsageRecordedPayload{
				Model:                  "openai/gpt-5-mini",
				Kind:                   string(TurnProviderUsageKindUtilityCompaction),
				Step:                   1,
				Attempt:                1,
				EstimatedRequestTokens: 300,
				EstimatedInputCost:     0.0003,
			},
		},
		{
			SessionID: "session-1",
			TurnID:    "turn-1",
			Sequence:  1,
			Time:      time.Now().UTC(),
			Type:      TypeTurnProviderUsageRecorded,
			Payload: TurnProviderUsageRecordedPayload{
				Model:                  "openai/gpt-5",
				Kind:                   string(TurnProviderUsageKindAgent),
				Step:                   1,
				Attempt:                1,
				EstimatedRequestTokens: 900,
				EstimatedInputCost:     0.0045,
			},
		},
		{
			SessionID: "session-1",
			TurnID:    "turn-1",
			Sequence:  2,
			Time:      time.Now().UTC(),
			Type:      TypeTurnProviderUsageReported,
			Payload: TurnProviderUsageReportedPayload{
				Model:               "openai/gpt-5-mini",
				Kind:                string(TurnProviderUsageKindUtilityCompaction),
				RequestID:           "utility_resp_1",
				Step:                1,
				Attempt:             1,
				InputTokens:         250,
				OutputTokens:        40,
				TotalTokens:         290,
				EstimatedInputCost:  0.00025,
				EstimatedOutputCost: 0.00008,
			},
		},
	}
	for _, event := range eventsToApply {
		if err := projector.Apply(event); err != nil {
			t.Fatalf("Apply(%s) error = %v", event.Type, err)
		}
	}

	turn := projector.Snapshot().Turns["turn-1"]
	if turn == nil || len(turn.ProviderAttempts) != 2 {
		t.Fatalf("ProviderAttempts = %#v", turn)
	}
	if turn.ProviderAttempts[0].ReportedRequestID != "utility_resp_1" {
		t.Fatalf("utility attempt did not receive reported usage: %#v", turn.ProviderAttempts[0])
	}
	if turn.ProviderAttempts[1].ReportedRequestID != "" {
		t.Fatalf("agent attempt received utility reported usage: %#v", turn.ProviderAttempts[1])
	}
	if turn.ProviderUsage == nil || turn.ProviderUsage.RequestTokens != 1150 {
		t.Fatalf("ProviderUsage = %#v, want utility reported plus agent estimated tokens", turn.ProviderUsage)
	}
}
