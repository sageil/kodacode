package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"

	"github.com/sageil/kodacode/v1/internal/message"
	"github.com/sageil/kodacode/v1/internal/pipeline"
	"github.com/sageil/kodacode/v1/internal/provider"
	"github.com/sageil/kodacode/v1/internal/repository"
	"github.com/sageil/kodacode/v1/internal/sandbox"
)

// FileAttachment represents a file to attach to a user message.
type FileAttachment struct {
	Path     string
	MimeType string
	Data     []byte
}

type resolvedSend struct {
	sess        repository.Session
	providerID  string
	modelID     string
	prov        provider.Provider
	model       provider.Model
	attachments []FileAttachment
	ephemeral   bool
}

type sendBehavior struct {
	persistRole  string
	currentRole  string
	synthetic    bool
	persistInput bool
}

var defaultSendBehavior = sendBehavior{
	persistRole:  "user",
	currentRole:  "user",
	persistInput: true,
}

// PrepareSend reserves the session turn slot and validates the turn before an
// async caller dispatches work in the background.
func (s *SessionService) PrepareSend(ctx context.Context, sessionID string, attachments []FileAttachment) (*SendReservation, error) {
	reservation, ownedReservation, err := s.reserveSend(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	if !ownedReservation {
		return reservation, nil
	}
	if _, err := s.resolveSend(ctx, sessionID, attachments); err != nil {
		completeTurnReservation(reservation, err)
		reservation.Release()
		return nil, err
	}
	return reservation, nil
}

func (s *SessionService) resolveSend(ctx context.Context, sessionID string, attachments []FileAttachment) (resolvedSend, error) {
	sess, err := s.store.sessions.Get(ctx, sessionID)
	if err != nil {
		return resolvedSend{}, fmt.Errorf("send: get session: %w", err)
	}

	ephemeral := ctx.Value(ephemeralKey{}) != nil
	if ephemeral && len(attachments) > 0 {
		return resolvedSend{}, fmt.Errorf("send: ephemeral sessions do not support attachments")
	}
	attachments, err = s.validateAttachments(attachments)
	if err != nil {
		return resolvedSend{}, err
	}

	providerID, modelID, err := splitModelID(sess.ModelID)
	if err != nil {
		return resolvedSend{}, fmt.Errorf("send: %w", err)
	}
	prov, model, err := s.resolveSendModel(ctx, providerID, modelID)
	if err != nil {
		return resolvedSend{}, err
	}
	if err := validateAttachmentsForProvider(prov, model, attachments); err != nil {
		return resolvedSend{}, err
	}
	return resolvedSend{
		sess:        sess,
		providerID:  providerID,
		modelID:     modelID,
		prov:        prov,
		model:       model,
		attachments: attachments,
		ephemeral:   ephemeral,
	}, nil
}

func (s *SessionService) resolveSendModel(ctx context.Context, providerID, modelID string) (provider.Provider, provider.Model, error) {
	prov, ok := s.runtime.providers.Get(providerID)
	if !ok {
		var registered []string
		for _, p := range s.runtime.providers.List() {
			registered = append(registered, p.ID())
		}
		log.Printf("send: provider %q not registered (model=%q, registered=%v)", providerID, providerID+"/"+modelID, registered)
		return nil, provider.Model{}, fmt.Errorf("send: provider %q not registered", providerID)
	}

	model, err := s.runtime.providers.ResolveModel(ctx, providerID, modelID)
	if err != nil {
		return nil, provider.Model{}, fmt.Errorf("send: resolve model: %w", err)
	}
	if s.runtime.cfg != nil {
		if mc, ok := s.runtime.cfg.SessionModelConfig(providerID, modelID); ok && mc.MaxInputTokens > 0 {
			model.MaxInputTokens = mc.MaxInputTokens
		}
		if budget := s.runtime.cfg.ModelThinkingBudget(providerID, modelID); budget != nil {
			model.ThinkingBudget = budget
		}
	}
	return prov, model, nil
}

// Send persists the user prompt, builds a TurnRequest, and executes the pipeline.
func (s *SessionService) Send(ctx context.Context, sessionID, prompt string, attachments []FileAttachment, origin sandbox.Origin, variant ...string) (err error) {
	return s.sendWithBehavior(ctx, sessionID, prompt, attachments, origin, defaultSendBehavior, variant...)
}

func (s *SessionService) sendWithBehavior(ctx context.Context, sessionID, prompt string, attachments []FileAttachment, origin sandbox.Origin, behavior sendBehavior, variant ...string) (err error) {
	if behavior.persistRole == "" {
		behavior.persistRole = "user"
	}
	if behavior.currentRole == "" {
		behavior.currentRole = behavior.persistRole
	}
	if !behavior.persistInput && behavior.persistRole == "" {
		behavior.persistRole = "user"
	}
	fail := func(format string, args ...any) error {
		err := fmt.Errorf(format, args...)
		msg := err.Error()
		if errors.Is(err, context.Canceled) {
			msg = "turn cancelled"
		}
		s.publish(sessionID, SSEEvent{Type: "error", Data: SSEErrorData{Message: msg}})
		return markPublishedError(err)
	}

	reservation, ownedReservation, err := s.reserveSend(ctx, sessionID)
	if err != nil {
		return fail("%w", err)
	}
	if ownedReservation {
		defer reservation.Release()
	}
	defer completeTurnReservation(reservation, err)

	resolved, err := s.resolveSend(ctx, sessionID, attachments)
	if err != nil {
		return fail("%w", err)
	}
	sess := resolved.sess
	providerID := resolved.providerID
	model := resolved.model
	attachments = resolved.attachments
	ephemeral := resolved.ephemeral

	var (
		providerMsgs []provider.Message
		summaryText  string
	)
	if ephemeral {
		// Ephemeral sessions (subagents): build messages in-memory, skip DB entirely.
		providerMsgs = []provider.Message{
			{Role: behavior.currentRole, Parts: buildCurrentTurnParts(prompt, attachments)},
		}
	} else {
		if behavior.persistInput {
			if err := s.persistUserMessage(ctx, sessionID, prompt, attachments, behavior); err != nil {
				return fail("send: %w", err)
			}
		}

		msgs, err := listMessagesForTurn(ctx, s.store.messages, sessionID)
		if err != nil {
			return fail("send: list messages with parts: %w", err)
		}
		providerMsgs, summaryText = buildTurnMessagesFromMessages(msgs)
		if len(providerMsgs) > 0 {
			providerMsgs[len(providerMsgs)-1] = provider.Message{
				Role:  behavior.currentRole,
				Parts: buildCurrentTurnParts(prompt, attachments),
			}
		} else {
			providerMsgs = append(providerMsgs, provider.Message{
				Role:  behavior.currentRole,
				Parts: buildCurrentTurnParts(prompt, attachments),
			})
		}
	}

	agentCfg := s.resolveAgentConfig(sess.AgentID)

	var v string
	if len(variant) > 0 {
		v = variant[0]
	}
	// Load pinned instructions from settings so they're injected into the
	// system prompt (survives compaction) rather than the message text.
	var pins []string
	if s.store.settings != nil {
		if raw, err := s.store.settings.Get(ctx, "pins:"+sessionID); err == nil && raw != "" {
			_ = json.Unmarshal([]byte(raw), &pins)
		}
	}
	var fallbackModels []string
	if s.runtime.cfg != nil {
		fallbackModels = s.runtime.cfg.FallbackModels
	}

	req := &pipeline.TurnRequest{
		SessionID:      sessionID,
		AgentID:        sess.AgentID,
		ProviderID:     providerID,
		Messages:       providerMsgs,
		CurrentInput:   currentInputMessage(behavior.currentRole, prompt, attachments),
		SummaryText:    summaryText,
		SystemParts:    []string{"", ""}, // filled by SystemPromptMiddleware (Task 20)
		Model:          model,
		Agent:          agentCfg,
		Origin:         int(origin),
		Variant:        v,
		FallbackModels: fallbackModels,
		Pins:           pins,
		Ephemeral:      ctx.Value(ephemeralKey{}) != nil,
		HasTitle:       sess.Title != "",
		Workflow:       loadWorkflowState(sess.WorkflowState),
	}

	chain := s.runtime.Chain()
	if chain != nil {
		if err := chain.Execute(ctx, req); err != nil {
			return fail("send: pipeline: %w", err)
		}
		if !ephemeral {
			if workflowState := saveWorkflowState(req.Workflow); workflowState != sess.WorkflowState {
				if err := s.store.sessions.UpdateWorkflow(ctx, sessionID, workflowState); err != nil {
					return fail("send: save workflow state: %w", err)
				}
			}
			if err := s.state.accounting.SaveSessionCost(ctx, sessionID); err != nil {
				log.Printf("send: save session cost: %v", err)
			}
		}
	}
	return nil
}

func (s *SessionService) persistUserMessage(ctx context.Context, sessionID, prompt string, attachments []FileAttachment, behavior sendBehavior) error {
	textContent, _ := message.MarshalContent(message.TextContent{Text: prompt})
	parts := []repository.MessagePart{{
		SessionID: sessionID,
		Type:      "text",
		Content:   textContent,
		Synthetic: behavior.synthetic,
	}}
	if len(attachments) == 0 {
		_, err := s.store.messages.CreateWithParts(ctx, repository.Message{
			SessionID: sessionID,
			Role:      behavior.persistRole,
		}, parts)
		return err
	}

	return s.withAttachmentMutation(sessionID, func() error {
		tracked := make([]message.FileContent, 0, len(attachments))
		for _, att := range attachments {
			fc, err := s.store.attachments.store.Store(att)
			if err != nil {
				return fmt.Errorf("store attachment: %w", err)
			}
			if err := s.trackAttachmentRef(ctx, fc, 1); err != nil {
				return fmt.Errorf("track attachment: %w", err)
			}
			tracked = append(tracked, fc)
			fileContent, _ := message.MarshalContent(fc)
			parts = append(parts, repository.MessagePart{
				SessionID: sessionID,
				Type:      "file",
				Content:   fileContent,
			})
		}

		if _, err := s.store.messages.CreateWithParts(ctx, repository.Message{
			SessionID: sessionID,
			Role:      behavior.persistRole,
		}, parts); err != nil {
			var rollbackErr error
			for i := len(tracked) - 1; i >= 0; i-- {
				if releaseErr := s.trackAttachmentRef(ctx, tracked[i], -1); releaseErr != nil && rollbackErr == nil {
					rollbackErr = releaseErr
				}
			}
			if rollbackErr != nil {
				if reconcileErr := s.ReconcileAttachmentBlobs(ctx); reconcileErr != nil {
					log.Printf("send: reconcile attachments after rollback failure for %s: %v", sessionID, reconcileErr)
				}
				return fmt.Errorf("save user message: %w (attachment rollback failed: %v)", err, rollbackErr)
			}
			return fmt.Errorf("save user message: %w", err)
		}
		return nil
	})
}

// groupPartsByMessage builds a messageID → parts map from a flat slice.
func groupPartsByMessage(parts []repository.MessagePart) map[string][]repository.MessagePart {
	m := make(map[string][]repository.MessagePart, len(parts))
	for _, p := range parts {
		m[p.MessageID] = append(m[p.MessageID], p)
	}
	return m
}

func buildCurrentTurnParts(prompt string, attachments []FileAttachment) []provider.MessagePart {
	parts := make([]provider.MessagePart, 0, 1+len(attachments))
	if prompt != "" {
		parts = append(parts, provider.TextPart{Text: prompt})
	}
	for _, att := range attachments {
		parts = append(parts, buildAttachmentProviderPart(att))
	}
	if len(parts) == 0 {
		parts = append(parts, provider.TextPart{Text: ""})
	}
	return parts
}

func currentInputMessage(role, prompt string, attachments []FileAttachment) *provider.Message {
	if role == "" {
		role = "user"
	}
	return &provider.Message{
		Role:  role,
		Parts: buildCurrentTurnParts(prompt, attachments),
	}
}
