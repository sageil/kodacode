package events

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
)

const maxEventRecordSize = 16 * 1024 * 1024
const snapshotPayloadEncodingGzipBase64 = "gzip+base64"

type eventRecord struct {
	ID        string          `json:"id"`
	SessionID string          `json:"session_id"`
	TurnID    string          `json:"turn_id"`
	Sequence  int64           `json:"sequence"`
	Time      string          `json:"time"`
	Type      Type            `json:"type"`
	Payload   json.RawMessage `json:"payload"`
}

type snapshotPayloadRecord struct {
	Encoding string `json:"encoding,omitempty"`
	Data     string `json:"data,omitempty"`
}

func encodeEvent(event Event) ([]byte, error) {
	payload, err := encodePayload(event.Type, event.Payload)
	if err != nil {
		return nil, err
	}
	record := eventRecord{
		ID:        event.ID,
		SessionID: event.SessionID,
		TurnID:    event.TurnID,
		Sequence:  event.Sequence,
		Time:      event.Time.UTC().Format(jsonTimeLayout),
		Type:      event.Type,
		Payload:   payload,
	}
	return json.Marshal(record)
}

func encodePayload(kind Type, payload Payload) ([]byte, error) {
	switch kind {
	case TypeSessionStateSnapshot:
		return encodeSnapshotPayload(payload)
	default:
		return json.Marshal(payload)
	}
}

func encodeSnapshotPayload(payload Payload) ([]byte, error) {
	snapshot, ok := payload.(SessionStateSnapshotPayload)
	if !ok {
		return nil, fmt.Errorf("session snapshot payload = %T", payload)
	}
	plain, err := json.Marshal(snapshot)
	if err != nil {
		return nil, err
	}
	compressed, err := gzipSnapshotPayload(plain)
	if err != nil {
		return nil, err
	}
	if len(compressed) >= len(plain) {
		return plain, nil
	}
	return json.Marshal(snapshotPayloadRecord{
		Encoding: snapshotPayloadEncodingGzipBase64,
		Data:     base64.StdEncoding.EncodeToString(compressed),
	})
}

func gzipSnapshotPayload(plain []byte) ([]byte, error) {
	var out bytes.Buffer
	zw, err := gzip.NewWriterLevel(&out, gzip.BestSpeed)
	if err != nil {
		return nil, err
	}
	if _, err := zw.Write(plain); err != nil {
		_ = zw.Close()
		return nil, err
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

func gunzipSnapshotPayload(compressed []byte) ([]byte, error) {
	zr, err := gzip.NewReader(bytes.NewReader(compressed))
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = zr.Close()
	}()
	return io.ReadAll(zr)
}

func decodeEvent(line []byte) (Event, error) {
	var record eventRecord
	if err := json.Unmarshal(line, &record); err != nil {
		return Event{}, err
	}
	payload, err := decodePayload(record.Type, record.Payload)
	if err != nil {
		return Event{}, err
	}
	timestamp, err := parseJSONTime(record.Time)
	if err != nil {
		return Event{}, err
	}
	event := Event{
		ID:        record.ID,
		SessionID: record.SessionID,
		TurnID:    record.TurnID,
		Sequence:  record.Sequence,
		Time:      timestamp,
		Type:      record.Type,
		Payload:   payload,
	}
	if err := event.Validate(); err != nil {
		return Event{}, err
	}
	return event, nil
}

func decodePayload(kind Type, raw json.RawMessage) (Payload, error) {
	switch kind {
	case TypeSessionConfigured:
		return decodePayloadAs[SessionConfiguredPayload](raw)
	case TypeSessionModelRouteUpdated:
		return decodePayloadAs[SessionModelRouteUpdatedPayload](raw)
	case TypeSessionWorkspaceRootsAdded:
		return decodePayloadAs[SessionWorkspaceRootsAddedPayload](raw)
	case TypeSessionPermissionModeUpdated:
		return decodePayloadAs[SessionPermissionModeUpdatedPayload](raw)
	case TypeSessionProviderLimitUpdated:
		return decodePayloadAs[SessionProviderLimitUpdatedPayload](raw)
	case TypeSessionMCPCatalogUpdated:
		return decodePayloadAs[SessionMCPCatalogUpdatedPayload](raw)
	case TypeSessionStateSnapshot:
		return decodeSnapshotPayload(raw)
	case TypeWorkspaceWriteRestored:
		return decodePayloadAs[WorkspaceWriteRestoredPayload](raw)
	case TypeSessionTitleUpdated:
		return decodePayloadAs[SessionTitleUpdatedPayload](raw)
	case TypeTaskCreated:
		return decodePayloadAs[TaskCreatedPayload](raw)
	case TypeTaskProgressUpdated:
		return decodePayloadAs[TaskProgressUpdatedPayload](raw)
	case TypeTaskBlocked:
		return decodePayloadAs[TaskBlockedPayload](raw)
	case TypeTaskCompleted:
		return decodePayloadAs[TaskCompletedPayload](raw)
	case TypeTaskReviewed:
		return decodePayloadAs[TaskReviewedPayload](raw)
	case TypeAssistantPreviewDelta:
		return decodePayloadAs[AssistantPreviewDeltaPayload](raw)
	case TypeAssistantPreviewReset:
		return decodePayloadAs[AssistantPreviewResetPayload](raw)
	case TypeAssistantWorklogCommit:
		return decodePayloadAs[AssistantWorklogCommitPayload](raw)
	case TypeAssistantCommit:
		return decodePayloadAs[AssistantCommitPayload](raw)
	case TypeReasoningDelta:
		return decodePayloadAs[ReasoningDeltaPayload](raw)
	case TypeAnthropicThinkingCommitted:
		return decodePayloadAs[AnthropicThinkingCommittedPayload](raw)
	case TypeOpenAIReasoningCommitted:
		return decodePayloadAs[OpenAIReasoningCommittedPayload](raw)
	case TypeToolCallDelta:
		return decodePayloadAs[ToolCallDeltaPayload](raw)
	case TypeToolCallDeclared:
		return decodePayloadAs[ToolCallDeclaredPayload](raw)
	case TypeToolCallBatch:
		return decodePayloadAs[ToolCallBatchPayload](raw)
	case TypeToolExecStart:
		return decodePayloadAs[ToolExecStartPayload](raw)
	case TypeToolExecOutput:
		return decodePayloadAs[ToolExecOutputPayload](raw)
	case TypeToolExecEnd:
		return decodePayloadAs[ToolExecEndPayload](raw)
	case TypeExecutionDeclared:
		return decodePayloadAs[ExecutionDeclaredPayload](raw)
	case TypeExecutionApprovalRequested:
		return decodePayloadAs[ExecutionApprovalRequestedPayload](raw)
	case TypeExecutionApprovalResolved:
		return decodePayloadAs[ExecutionApprovalResolvedPayload](raw)
	case TypeExecutionStarted:
		return decodePayloadAs[ExecutionStartedPayload](raw)
	case TypeExecutionOutput:
		return decodePayloadAs[ExecutionOutputPayload](raw)
	case TypeExecutionBackgroundStarted:
		return decodePayloadAs[ExecutionBackgroundStartedPayload](raw)
	case TypeExecutionBackgroundObserved:
		return decodePayloadAs[ExecutionBackgroundObservedPayload](raw)
	case TypeExecutionBackgroundReady:
		return decodePayloadAs[ExecutionBackgroundReadyPayload](raw)
	case TypeExecutionBackgroundExited:
		return decodePayloadAs[ExecutionBackgroundExitedPayload](raw)
	case TypeExecutionBackgroundLost:
		return decodePayloadAs[ExecutionBackgroundLostPayload](raw)
	case TypePermissionRequested:
		return decodePayloadAs[PermissionRequestedPayload](raw)
	case TypePermissionResolved:
		return decodePayloadAs[PermissionResolvedPayload](raw)
	case TypeQuestionRequested:
		return decodePayloadAs[QuestionRequestedPayload](raw)
	case TypeQuestionAnswered:
		return decodePayloadAs[QuestionAnsweredPayload](raw)
	case TypeTurnConfigured:
		return decodePayloadAs[TurnConfiguredPayload](raw)
	case TypeTurnContinuationStarted:
		return decodePayloadAs[TurnContinuationStartedPayload](raw)
	case TypeTurnWorkStateUpdated:
		return decodePayloadAs[TurnWorkStateUpdatedPayload](raw)
	case TypeSessionHistoryCheckpoint:
		return decodePayloadAs[SessionHistoryCheckpointPayload](raw)
	case TypeContextCompactionStarted:
		return decodePayloadAs[ContextCompactionStartedPayload](raw)
	case TypeContextCompactionFailed:
		return decodePayloadAs[ContextCompactionFailedPayload](raw)
	case TypeTurnProviderUsageRecorded:
		return decodePayloadAs[TurnProviderUsageRecordedPayload](raw)
	case TypeTurnProviderUsageReported:
		return decodePayloadAs[TurnProviderUsageReportedPayload](raw)
	case TypeTurnRetryScheduled:
		return decodePayloadAs[TurnRetryScheduledPayload](raw)
	case TypeReviewRecorded:
		return decodePayloadAs[ReviewRecordedPayload](raw)
	case TypePlanRecorded:
		return decodePayloadAs[PlanRecordedPayload](raw)
	case TypeTurnDone:
		return decodePayloadAs[TurnDonePayload](raw)
	case TypeTurnCanceled:
		return decodePayloadAs[TurnCanceledPayload](raw)
	case TypeTurnError:
		return decodePayloadAs[TurnErrorPayload](raw)
	case TypePromptCompiled:
		return decodePayloadAs[PromptCompiledPayload](raw)
	case TypeUserMessage:
		return decodePayloadAs[UserMessagePayload](raw)
	case TypeContextPruned:
		return decodePayloadAs[ContextPrunedPayload](raw)
	case TypeSessionHistoryContinuationUpdated:
		return decodePayloadAs[SessionHistoryContinuationUpdatedPayload](raw)
	case TypeAgentHandoff:
		return decodePayloadAs[AgentHandoffPayload](raw)
	case TypeAgentHandoffPreview:
		return decodePayloadAs[AgentHandoffPreviewPayload](raw)
	case TypeAgentResult:
		return decodePayloadAs[AgentResultPayload](raw)
	case TypeAgentResultReused:
		return decodePayloadAs[AgentResultReusedPayload](raw)
	default:
		return nil, fmt.Errorf("unsupported event type %q", kind)
	}
}

func decodeSnapshotPayload(raw json.RawMessage) (Payload, error) {
	var encoded snapshotPayloadRecord
	if err := json.Unmarshal(raw, &encoded); err == nil && encoded.Encoding != "" {
		if encoded.Encoding != snapshotPayloadEncodingGzipBase64 {
			return nil, fmt.Errorf("unsupported session snapshot payload encoding %q", encoded.Encoding)
		}
		compressed, err := base64.StdEncoding.DecodeString(encoded.Data)
		if err != nil {
			return nil, err
		}
		plain, err := gunzipSnapshotPayload(compressed)
		if err != nil {
			return nil, err
		}
		return decodePayloadAs[SessionStateSnapshotPayload](plain)
	}
	return decodePayloadAs[SessionStateSnapshotPayload](raw)
}

func decodePayloadAs[T Payload](raw json.RawMessage) (Payload, error) {
	var payload T
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, err
	}
	return payload, nil
}

func readEventsFromFile(path string) ([]Event, error) {
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer func() {
		_ = file.Close()
	}()

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), maxEventRecordSize)

	var events []Event
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		event, err := decodeEvent(append([]byte(nil), line...))
		if err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return events, nil
}

func ReadSessionConfigured(path string) (Event, error) {
	events, err := readEventsFromFile(path)
	if err != nil {
		return Event{}, err
	}
	if len(events) == 0 {
		return Event{}, fmt.Errorf("session file %q is empty", path)
	}
	event := events[0]
	if event.Type != TypeSessionConfigured {
		return Event{}, fmt.Errorf("session file %q does not start with session_configured", path)
	}
	return event, nil
}
