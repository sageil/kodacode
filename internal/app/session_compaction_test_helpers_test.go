package app

import (
	"encoding/json"

	"github.com/sageil/kodacode/internal/events"
)

func testHistoryContinuationArtifactJSON(artifact events.HistoryContinuationArtifact) string {
	data, err := json.Marshal(artifact)
	if err != nil {
		panic(err)
	}
	return string(data)
}

func testSimpleHistoryContinuationArtifact(objective string, completed []string, openThreads []string, paths ...string) events.HistoryContinuationArtifact {
	artifact := events.HistoryContinuationArtifact{
		SessionObjective: objective,
	}
	sourceTurnID := "turn-1"
	for index, summary := range completed {
		turnID := sourceTurnID
		if index > 0 {
			turnID = "turn-" + string(rune('1'+index))
		}
		artifact.CompletedEpisodes = append(artifact.CompletedEpisodes, events.HistoryEpisodePayload{
			EpisodeID:     historyContinuationEpisodeID([]string{turnID}),
			Summary:       summary,
			TouchedPaths:  append([]string(nil), paths...),
			SourceTurnIDs: []string{turnID},
		})
	}
	for _, item := range openThreads {
		artifact.OpenThreads = append(artifact.OpenThreads, events.HistoryOpenThreadPayload{
			Item:         item,
			Status:       events.HistoryOpenThreadStatusPending,
			Owner:        events.HistoryOpenThreadOwnerAgent,
			SourceTurnID: sourceTurnID,
		})
	}
	for _, path := range paths {
		artifact.WorkspaceFacts = append(artifact.WorkspaceFacts, events.HistoryWorkspaceFactPayload{
			Path:         path,
			Fact:         "Touched during consolidated work",
			SourceTurnID: sourceTurnID,
		})
	}
	return artifact
}
