package events

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/sageil/kodacode/internal/textdiff"
)

type ToolCallDeltaPayload struct {
	CallID     string
	ToolName   string
	ToolKind   string
	InputDelta string
}

func (ToolCallDeltaPayload) eventType() Type { return TypeToolCallDelta }

func (p ToolCallDeltaPayload) validate() error {
	if strings.TrimSpace(p.CallID) == "" {
		return errors.New("call_id is required")
	}
	if strings.TrimSpace(p.ToolName) == "" {
		return errors.New("tool_name is required")
	}
	if p.InputDelta == "" {
		return errors.New("input_delta is required")
	}
	return nil
}

type ToolCallDeclaredPayload struct {
	CallID                 string
	ToolName               string
	ToolKind               string
	Input                  string
	GoogleThoughtSignature []byte
	OpenAIReasoningContent string
}

func (ToolCallDeclaredPayload) eventType() Type { return TypeToolCallDeclared }

func (p ToolCallDeclaredPayload) validate() error {
	if strings.TrimSpace(p.CallID) == "" {
		return errors.New("call_id is required")
	}
	if strings.TrimSpace(p.ToolName) == "" {
		return errors.New("tool_name is required")
	}
	if strings.TrimSpace(p.Input) == "" {
		return errors.New("input is required")
	}
	return nil
}

type ToolCallBatchPayload struct {
	CallIDs []string
}

func (ToolCallBatchPayload) eventType() Type { return TypeToolCallBatch }

func (p ToolCallBatchPayload) validate() error {
	if len(p.CallIDs) == 0 {
		return errors.New("call_ids are required")
	}
	seen := make(map[string]struct{}, len(p.CallIDs))
	for _, callID := range p.CallIDs {
		callID = strings.TrimSpace(callID)
		if callID == "" {
			return errors.New("call_ids must not contain empty values")
		}
		if _, ok := seen[callID]; ok {
			return fmt.Errorf("duplicate call_id %q", callID)
		}
		seen[callID] = struct{}{}
	}
	return nil
}

type ToolExecStartPayload struct {
	CallID   string
	ToolName string
	ToolKind string
	Input    string
}

func (ToolExecStartPayload) eventType() Type { return TypeToolExecStart }

func (p ToolExecStartPayload) validate() error {
	if strings.TrimSpace(p.CallID) == "" {
		return errors.New("call_id is required")
	}
	if strings.TrimSpace(p.ToolName) == "" {
		return errors.New("tool_name is required")
	}
	if strings.TrimSpace(p.Input) == "" {
		return errors.New("input is required")
	}
	return nil
}

type ToolExecOutputPayload struct {
	CallID string
	Chunk  string
	Stream string
}

func (ToolExecOutputPayload) eventType() Type { return TypeToolExecOutput }

func (p ToolExecOutputPayload) validate() error {
	if strings.TrimSpace(p.CallID) == "" {
		return errors.New("call_id is required")
	}
	if p.Chunk == "" {
		return errors.New("chunk is required")
	}
	return nil
}

type ToolExecEndPayload struct {
	CallID              string
	ToolName            string
	ToolKind            string
	ReusedFromCallID    string
	ReusedFromSessionID string
	ReusedFromTurnID    string
	RetryOfCallID       string
	HandoffID           string
	ExecutionID         string
	ExecutionStatus     string
	FailureClass        string
	Succeeded           bool
	Output              string
	Error               string
	ErrorDetail         *ToolErrorDetail
	StructuredResult    json.RawMessage
	MutationRanges      []MutationRange
	WriteMutation       *WriteMutation
	WriteMutations      []WriteMutation
	ObservedResources   []ObservedResource
	OutputBlob          *ToolResultBlobRef
	ErrorBlob           *ToolResultBlobRef
	OutputTruncated     bool
	ErrorTruncated      bool
	ExitCode            *int
	DurationMS          int64
	CommandActions      []string
	Backend             string
}

type ToolErrorDetail struct {
	Code      string
	Message   string
	Retryable bool
	Recovery  string
	Fields    map[string]string
}

func (ToolExecEndPayload) eventType() Type { return TypeToolExecEnd }

func (p ToolExecEndPayload) validate() error {
	if strings.TrimSpace(p.CallID) == "" {
		return errors.New("call_id is required")
	}
	if strings.TrimSpace(p.ToolName) == "" {
		return errors.New("tool_name is required")
	}
	if strings.TrimSpace(p.ExecutionID) != "" && strings.TrimSpace(p.ExecutionStatus) == "" {
		return errors.New("execution_status is required when execution_id is set")
	}
	if strings.TrimSpace(p.ReusedFromSessionID) != "" && strings.TrimSpace(p.ReusedFromCallID) == "" {
		return errors.New("reused_from_call_id is required when reused_from_session_id is set")
	}
	if strings.TrimSpace(p.ReusedFromTurnID) != "" && strings.TrimSpace(p.ReusedFromSessionID) == "" {
		return errors.New("reused_from_session_id is required when reused_from_turn_id is set")
	}
	if !p.Succeeded && !toolResultPayloadHasOutputOrError(p.Output, p.Error, p.OutputBlob, p.ErrorBlob) {
		return errors.New("output or error is required")
	}
	if p.ErrorDetail != nil && !p.ErrorDetail.valid() {
		return errors.New("error_detail is invalid")
	}
	if p.DurationMS < 0 {
		return errors.New("duration_ms must not be negative")
	}
	for _, r := range p.MutationRanges {
		if !r.valid() {
			return errors.New("mutation_ranges contains invalid line anchors")
		}
	}
	if !validStructuredResult(p.StructuredResult) {
		return errors.New("structured_result must be valid JSON")
	}
	if p.WriteMutation != nil && !p.WriteMutation.valid() {
		return errors.New("write_mutation is invalid")
	}
	for _, mutation := range p.WriteMutations {
		if !mutation.valid() {
			return errors.New("write_mutations contains invalid entries")
		}
	}
	for _, resource := range p.ObservedResources {
		if !resource.valid() {
			return errors.New("observed_resources contains invalid entries")
		}
	}
	return nil
}

func (p ToolExecEndPayload) Successful() bool {
	if p.Succeeded {
		return true
	}
	return strings.TrimSpace(p.Error) == "" && toolResultPayloadHasOutput(p.Output, p.OutputBlob)
}

func toolResultPayloadHasOutputOrError(output, errorText string, outputBlob, errorBlob *ToolResultBlobRef) bool {
	return toolResultPayloadHasOutput(output, outputBlob) || toolResultPayloadHasError(errorText, errorBlob)
}

func toolResultPayloadHasOutput(output string, outputBlob *ToolResultBlobRef) bool {
	return strings.TrimSpace(output) != "" || (outputBlob != nil && outputBlob.valid())
}

func toolResultPayloadHasError(errorText string, errorBlob *ToolResultBlobRef) bool {
	return strings.TrimSpace(errorText) != "" || (errorBlob != nil && errorBlob.valid())
}

func (d *ToolErrorDetail) valid() bool {
	if d == nil {
		return true
	}
	code := strings.TrimSpace(d.Code)
	if code == "" || strings.ContainsAny(code, " \t\r\n") {
		return false
	}
	if strings.TrimSpace(d.Message) == "" {
		return false
	}
	if strings.ContainsAny(strings.TrimSpace(d.Recovery), " \t\r\n") {
		return false
	}
	for key := range d.Fields {
		if strings.TrimSpace(key) == "" || strings.ContainsAny(key, " \t\r\n") {
			return false
		}
	}
	return true
}

func validStructuredResult(raw json.RawMessage) bool {
	if len(raw) == 0 {
		return true
	}
	return json.Valid(raw)
}

type MutationRange struct {
	OldStartLine int `json:"old_start_line,omitempty"`
	NewStartLine int `json:"new_start_line,omitempty"`
}

func (r MutationRange) valid() bool {
	if r.OldStartLine < 0 || r.NewStartLine < 0 {
		return false
	}
	return r.OldStartLine > 0 || r.NewStartLine > 0
}

type WriteMutation struct {
	Path            string             `json:"path,omitempty"`
	Existed         bool               `json:"existed,omitempty"`
	Before          string             `json:"before,omitempty"`
	BeforeBlob      *ToolResultBlobRef `json:"before_blob,omitempty"`
	BeforeTruncated bool               `json:"before_truncated,omitempty"`
	DiffPreview     *textdiff.Preview  `json:"diff_preview,omitempty"`
	Mode            uint32             `json:"mode,omitempty"`
}

func (m WriteMutation) valid() bool {
	if strings.TrimSpace(m.Path) == "" {
		return false
	}
	if m.BeforeBlob != nil && !m.BeforeBlob.valid() {
		return false
	}
	if m.DiffPreview != nil && !m.DiffPreview.Valid() {
		return false
	}
	return true
}

type ObservedResource struct {
	Kind       string `json:"kind,omitempty"`
	Path       string `json:"path,omitempty"`
	Version    string `json:"version,omitempty"`
	State      string `json:"state,omitempty"`
	Complete   bool   `json:"complete,omitempty"`
	StartLine  int    `json:"start_line,omitempty"`
	EndLine    int    `json:"end_line,omitempty"`
	TotalLines int    `json:"total_lines,omitempty"`
}

func (r ObservedResource) valid() bool {
	switch strings.TrimSpace(r.Kind) {
	case "file_content", "dir_entries":
	default:
		return false
	}
	if strings.TrimSpace(r.Path) == "" || strings.TrimSpace(r.Version) == "" {
		return false
	}
	if r.StartLine < 0 || r.EndLine < 0 || r.TotalLines < 0 {
		return false
	}
	if r.EndLine > 0 {
		if r.StartLine <= 0 || r.EndLine < r.StartLine {
			return false
		}
	}
	if r.TotalLines > 0 {
		if r.StartLine > r.TotalLines || r.EndLine > r.TotalLines {
			return false
		}
	}
	return true
}
