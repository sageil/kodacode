package service

import (
	"encoding/json"
	"log"
	"strings"

	"github.com/sageil/kodacode/v1/internal/pipeline"
)

func loadWorkflowState(raw string) *pipeline.WorkflowState {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var ws pipeline.WorkflowState
	if err := json.Unmarshal([]byte(raw), &ws); err != nil {
		log.Printf("workflow: invalid stored state: %v", err)
		return nil
	}
	return &ws
}

func saveWorkflowState(ws *pipeline.WorkflowState) string {
	if ws == nil {
		return ""
	}
	data, err := json.Marshal(ws)
	if err != nil {
		log.Printf("workflow: marshal stored state: %v", err)
		return ""
	}
	return string(data)
}
