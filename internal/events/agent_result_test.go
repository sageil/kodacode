package events

import "testing"

func TestAgentResultPayloadValidateAllowsPendingQuestionWithoutToolName(t *testing.T) {
	payload := AgentResultPayload{
		HandoffID:         "handoff-1",
		ChildSessionID:    "session-child",
		ChildTurnID:       "turn-child",
		Status:            AgentResultStatusPendingQuestion,
		QuestionRequestID: "question-1",
		QuestionText:      "Continue or stop this turn?",
		QuestionOptions:   []string{"Continue", "Stop turn"},
	}
	if err := payload.validate(); err != nil {
		t.Fatalf("validate() error = %v", err)
	}
}
