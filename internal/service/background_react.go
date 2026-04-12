package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/sageil/kodacode/v1/internal/config"
	"github.com/sageil/kodacode/v1/internal/repository"
	"github.com/sageil/kodacode/v1/internal/sandbox"
)

const (
	BackgroundAutoReactOutputChars = 1600
	backgroundAutoReactRetryDelay  = 250 * time.Millisecond
	backgroundEventPartType        = "background_event"
)

type backgroundEventContent struct {
	TaskID         string `json:"task_id"`
	Command        string `json:"command"`
	Description    string `json:"description,omitempty"`
	ExitCode       int    `json:"exit_code"`
	Output         string `json:"output"`
	ElapsedMS      int64  `json:"elapsed_ms"`
	Status         string `json:"status"`
	AutoReactState string `json:"auto_react_state,omitempty"`
	LastError      string `json:"last_error,omitempty"`
}

type backgroundEventRecord struct {
	part  repository.MessagePart
	event backgroundEventContent
}

func (s *SessionService) BackgroundDoneHandler() func(sessionID, taskID, command, description string, exitCode int, output string, elapsedMS int64) {
	if s == nil {
		return nil
	}
	return s.handleBackgroundDone
}

func (s *SessionService) handleBackgroundDone(sessionID, taskID, command, description string, exitCode int, output string, elapsedMS int64) {
	if s == nil || s.runtime == nil || s.runtime.cfg == nil || s.store == nil || s.store.sessions == nil {
		return
	}

	sess, err := s.store.sessions.Get(context.Background(), sessionID)
	if err != nil || sess.Ephemeral {
		return
	}

	autoReact := config.Bool(s.runtime.cfg.Session.BackgroundAutoReact)
	event := backgroundEventContent{
		TaskID:         taskID,
		Command:        command,
		Description:    description,
		ExitCode:       exitCode,
		Output:         output,
		ElapsedMS:      elapsedMS,
		Status:         backgroundEventStatus(exitCode),
		AutoReactState: backgroundEventInitialState(autoReact),
	}

	if err := s.persistBackgroundEvent(context.Background(), sessionID, event); err != nil {
		log.Printf("background event: session=%s task=%s persist failed: %v", sessionID, taskID, err)
	}
	s.publishBackgroundEvent(sessionID, event)

	if autoReact {
		s.startBackgroundAutoReactDrain(sessionID)
	}
}

func backgroundEventStatus(exitCode int) string {
	if exitCode == 0 {
		return "completed"
	}
	return "failed"
}

func backgroundEventInitialState(autoReact bool) string {
	if autoReact {
		return "queued"
	}
	return "recorded"
}

func (s *SessionService) publishBackgroundEvent(sessionID string, event backgroundEventContent) {
	if s == nil {
		return
	}
	s.publish(sessionID, SSEEvent{
		Type: "background_event",
		Data: SSEBackgroundEventData(event),
	})
}

func (s *SessionService) persistBackgroundEvent(ctx context.Context, sessionID string, event backgroundEventContent) error {
	if s == nil || s.store == nil || s.store.messages == nil {
		return nil
	}
	content, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal background event: %w", err)
	}
	_, err = s.store.messages.CreateWithParts(ctx, repository.Message{
		SessionID: sessionID,
		Role:      "system",
	}, []repository.MessagePart{{
		SessionID: sessionID,
		Type:      backgroundEventPartType,
		Content:   string(content),
	}})
	if err != nil {
		return fmt.Errorf("store background event: %w", err)
	}
	return nil
}

func (s *SessionService) startBackgroundAutoReactDrain(sessionID string) {
	if s == nil || s.state == nil || s.state.background == nil {
		return
	}
	if !s.state.background.StartDrain(sessionID) {
		return
	}
	go func() {
		defer s.state.background.FinishDrain(sessionID)
		s.drainBackgroundAutoReact(sessionID)
	}()
}

func (s *SessionService) drainBackgroundAutoReact(sessionID string) {
	for {
		if s == nil || s.runtime == nil || s.runtime.cfg == nil {
			return
		}
		if !config.Bool(s.runtime.cfg.Session.BackgroundAutoReact) {
			return
		}

		record, ok, err := s.nextQueuedBackgroundEvent(context.Background(), sessionID)
		if err != nil {
			log.Printf("background auto-react: session=%s load queued events failed: %v", sessionID, err)
			return
		}
		if !ok {
			return
		}

		ctx := context.Background()
		reservation, err := s.PrepareSend(ctx, sessionID, nil)
		switch {
		case err == nil:
			record.event.AutoReactState = "reviewing"
			record.event.LastError = ""
			if updateErr := s.updateBackgroundEvent(ctx, record); updateErr != nil {
				log.Printf("background auto-react: session=%s task=%s mark reviewing failed: %v", sessionID, record.event.TaskID, updateErr)
			}

			sendCtx := ctx
			if reservation != nil {
				sendCtx = reservation.Context(ctx)
			}
			behavior := sendBehavior{
				currentRole:  "user",
				persistInput: false,
			}
			sendErr := s.sendWithBehavior(sendCtx, sessionID, BuildBackgroundAutoReactPrompt(record.event.TaskID, record.event.Command, record.event.ExitCode, record.event.Output), nil, sandbox.OriginAPI, behavior)
			if reservation != nil {
				reservation.Release()
			}
			if sendErr != nil {
				if errors.Is(sendErr, repository.ErrNotFound) {
					return
				}
				record.event.AutoReactState = "failed"
				record.event.LastError = sendErr.Error()
				if updateErr := s.updateBackgroundEvent(ctx, record); updateErr != nil {
					log.Printf("background auto-react: session=%s task=%s mark failed failed: %v", sessionID, record.event.TaskID, updateErr)
				}
				log.Printf("background auto-react: session=%s task=%s send failed: %v", sessionID, record.event.TaskID, sendErr)
				return
			}

			record.event.AutoReactState = "reviewed"
			record.event.LastError = ""
			if updateErr := s.updateBackgroundEvent(ctx, record); updateErr != nil {
				log.Printf("background auto-react: session=%s task=%s mark reviewed failed: %v", sessionID, record.event.TaskID, updateErr)
			}
		case errors.Is(err, ErrSessionBusy):
			time.Sleep(backgroundAutoReactRetryDelay)
		case errors.Is(err, repository.ErrNotFound):
			return
		default:
			log.Printf("background auto-react: session=%s prepare failed: %v", sessionID, err)
			return
		}
	}
}

func (s *SessionService) nextQueuedBackgroundEvent(ctx context.Context, sessionID string) (backgroundEventRecord, bool, error) {
	if s == nil || s.store == nil || s.store.messages == nil {
		return backgroundEventRecord{}, false, nil
	}
	msgs, err := s.store.messages.ListMessagesWithParts(ctx, sessionID)
	if err != nil {
		return backgroundEventRecord{}, false, err
	}
	for _, msg := range msgs {
		for _, part := range msg.Parts {
			event, ok := backgroundEventFromPart(part)
			if !ok || event.AutoReactState != "queued" {
				continue
			}
			return backgroundEventRecord{part: part, event: event}, true, nil
		}
	}
	return backgroundEventRecord{}, false, nil
}

func (s *SessionService) updateBackgroundEvent(ctx context.Context, record backgroundEventRecord) error {
	if s == nil || s.store == nil || s.store.messages == nil {
		return nil
	}
	content, err := json.Marshal(record.event)
	if err != nil {
		return fmt.Errorf("marshal background event: %w", err)
	}
	part := record.part
	part.Content = string(content)
	return s.store.messages.UpdatePart(ctx, part)
}

func backgroundEventFromPart(part repository.MessagePart) (backgroundEventContent, bool) {
	if part.Type != backgroundEventPartType {
		return backgroundEventContent{}, false
	}
	var event backgroundEventContent
	if err := json.Unmarshal([]byte(part.Content), &event); err != nil {
		return backgroundEventContent{}, false
	}
	return event, true
}

func BuildBackgroundAutoReactPrompt(taskID, command string, exitCode int, output string) string {
	status := "completed"
	if exitCode != 0 {
		status = fmt.Sprintf("failed (exit %d)", exitCode)
	}
	excerpt := truncateBackgroundOutputForPrompt(output)

	var sb strings.Builder
	fmt.Fprintf(&sb, "Background task %s (`%s`) has %s.\n", taskID, command, status)
	sb.WriteString("A tail excerpt is included below and may be truncated. If you need the full retained output, call `task_output` with this task ID before taking action.\n\n")
	if excerpt == "" {
		sb.WriteString("No output was captured.\n\n")
	} else {
		sb.WriteString("Output excerpt:\n```\n")
		sb.WriteString(excerpt)
		if !strings.HasSuffix(excerpt, "\n") {
			sb.WriteString("\n")
		}
		sb.WriteString("```\n\n")
	}
	sb.WriteString("Please review and take any necessary action.")
	return sb.String()
}

func truncateBackgroundOutputForPrompt(output string) string {
	output = strings.TrimSpace(output)
	if output == "" {
		return ""
	}
	if len(output) <= BackgroundAutoReactOutputChars {
		return output
	}
	tail := output[len(output)-BackgroundAutoReactOutputChars:]
	if idx := strings.IndexByte(tail, '\n'); idx >= 0 && idx < len(tail)-1 {
		tail = tail[idx+1:]
	}
	return "[...truncated; showing tail...]\n" + tail
}
