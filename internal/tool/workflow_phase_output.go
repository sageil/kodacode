package tool

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

const WorkflowPhaseOutputToolName = "workflow_phase_output"

var ErrWorkflowPhaseOutputFieldsRequired = errors.New("fields must include at least one non-empty value")

type WorkflowPhaseOutputRequest struct {
	Fields map[string]string
}

type WorkflowPhaseOutputRecord struct {
	RecordedKeys []string `json:"recorded_keys"`
	Message      string   `json:"message"`
}

type WorkflowPhaseOutputManager interface {
	RecordWorkflowPhaseOutput(WorkflowPhaseOutputRequest) (WorkflowPhaseOutputRecord, error)
}

type WorkflowPhaseOutputTool struct{}

func NewWorkflowPhaseOutputTool() WorkflowPhaseOutputTool {
	return WorkflowPhaseOutputTool{}
}

func (WorkflowPhaseOutputTool) Definition() Definition {
	return Definition{
		Name:                WorkflowPhaseOutputToolName,
		Description:         "Record required structured output for the current workflow phase. Use this before the final assistant response when the active workflow phase lists required phase outputs.",
		ProviderDescription: "Record required structured output for the active workflow phase before your final response. Include every required output key in fields.",
		InputSchema:         json.RawMessage(`{"type":"object","properties":{"fields":{"type":"object","description":"Required workflow phase output fields keyed by the names requested in the workflow phase prompt.","additionalProperties":{"type":"string"}}},"required":["fields"],"additionalProperties":false}`),
		ProviderInputSchema: json.RawMessage(`{"type":"object","properties":{"fields":{"type":"object","additionalProperties":{"type":"string"}}},"required":["fields"],"additionalProperties":false}`),
		ArgumentExamples: []string{
			`{"fields":{"plan":"Implement OAuth callback routes, session integration, and login/register buttons.","affected_files":"src/routes/auth.ts, src/lib/oauth.ts, client/src/views/Login.vue","risks":"OAuth redirect validation and account-linking edge cases.","implementation_tasks":"[\"Add OAuth provider config\",\"Add callback route\",\"Add login UI button\"]","acceptance_criteria":"[\"Users can start OAuth login\",\"Callback validates state and creates a session\"]","verification_plan":"Run backend auth route tests and client typecheck."}}`,
		},
		ProviderRichGuidance: true,
	}
}

func (WorkflowPhaseOutputTool) Execute(_ context.Context, ectx ExecutionContext, args json.RawMessage) (Result, error) {
	manager, err := ectx.WorkflowPhaseOutput()
	if err != nil {
		return Result{}, err
	}
	input, err := parseWorkflowPhaseOutputInput(args)
	if err != nil {
		return Result{}, err
	}
	record, err := manager.RecordWorkflowPhaseOutput(WorkflowPhaseOutputRequest(input))
	if err != nil {
		return Result{}, err
	}
	encoded, err := json.Marshal(record)
	if err != nil {
		return Result{}, err
	}
	return Result{Output: string(encoded)}, nil
}

type workflowPhaseOutputInput struct {
	Fields map[string]string
}

func parseWorkflowPhaseOutputInput(args json.RawMessage) (workflowPhaseOutputInput, error) {
	var raw struct {
		Fields map[string]any `json:"fields"`
	}
	if err := DecodeArgs(WorkflowPhaseOutputToolName, args, &raw); err != nil {
		return workflowPhaseOutputInput{}, err
	}
	fields := make(map[string]string, len(raw.Fields))
	for key, value := range raw.Fields {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		formatted := workflowPhaseOutputInputValue(value)
		if formatted == "" {
			continue
		}
		fields[key] = formatted
	}
	if len(fields) == 0 {
		return workflowPhaseOutputInput{}, InvalidArguments(WorkflowPhaseOutputToolName, ErrWorkflowPhaseOutputFieldsRequired)
	}
	return workflowPhaseOutputInput{Fields: fields}, nil
}

func workflowPhaseOutputInputValue(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(typed)
	case []any, map[string]any:
		encoded, err := json.Marshal(typed)
		if err != nil {
			return ""
		}
		return strings.TrimSpace(string(encoded))
	default:
		return strings.TrimSpace(fmt.Sprint(typed))
	}
}
