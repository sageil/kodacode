package app

import (
	"context"
	"strings"
	"time"

	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/provider"
	"github.com/sageil/kodacode/internal/sessiontitle"
)

const (
	sessionTitleAgentID       = "_session_title"
	sessionTitlePrompt        = "Generate a concise session title (max 6 words) for a coding conversation that starts with this user request. Reply with only the title text."
	sessionTitleTimeout       = 15 * time.Second
	maxSessionTitleRuneLength = 72
)

func (r *Runtime) maybeStartSessionTitleGeneration(ctx context.Context, sessionID, turnID, userText string, fallbackRoute provider.ModelRoute, eligible bool) {
	if r == nil || !r.enableSessionTitles || r.Sessions == nil || strings.TrimSpace(userText) == "" || ctx == nil || !eligible {
		return
	}
	go r.generateSessionTitle(ctx, sessionID, turnID, userText, fallbackRoute)
}

func (r *Runtime) generateSessionTitle(ctx context.Context, sessionID, turnID, userText string, fallbackRoute provider.ModelRoute) {
	logger := r.log("session_title")
	for _, candidate := range availableUtilityTextCandidates(r.Config.UtilityModel, fallbackRoute, r.utilityProviderAvailable()) {
		client, err := r.rawProviderClient(candidate.Ref.ProviderID)
		if err != nil {
			if logger != nil {
				logger.Debug("session title provider unavailable", "session_id", sessionID, "model", candidate.Ref.String(), "error", err.Error())
			}
			continue
		}

		title, err := r.requestSessionTitle(ctx, client, candidate.Ref, sessionID, turnID, userText)
		if err != nil {
			if logger != nil {
				logger.Debug("session title generation failed", "session_id", sessionID, "model", candidate.Ref.String(), "error", err.Error())
			}
			continue
		}
		if title == "" {
			continue
		}
		if !r.sessionCanPersistGeneratedTitle(ctx, sessionID, turnID) {
			return
		}
		if _, err := r.Sessions.SetTitle(ctx, sessionID, title); err != nil {
			if logger != nil {
				logger.Debug("session title persist failed", "session_id", sessionID, "model", candidate.Ref.String(), "error", err.Error())
			}
			return
		}
		if logger != nil {
			logger.Debug("session title generated", "session_id", sessionID, "model", candidate.Ref.String(), "title", title)
		}
		return
	}
}

func (r *Runtime) sessionCanPersistGeneratedTitle(ctx context.Context, sessionID, turnID string) bool {
	if r == nil || r.Sessions == nil {
		return false
	}
	canPersist := false
	_ = r.Sessions.Inspect(ctx, sessionID, func(state events.SessionState) error {
		if strings.TrimSpace(state.Title) != "" {
			return nil
		}
		if len(state.TurnOrder) == 0 || state.TurnOrder[0] != turnID {
			return nil
		}
		canPersist = state.Turns[turnID] != nil
		return nil
	})
	return canPersist
}

func (r *Runtime) rawProviderClient(providerID string) (provider.Client, error) {
	if r == nil {
		return nil, provider.ErrProviderNotConfigured
	}
	var client provider.Client
	var err error
	if r.rawProviderFactory != nil {
		client, err = r.rawProviderFactory(r.Config, providerID)
	} else {
		client, err = buildProviderClientForID(r.Config, providerID)
	}
	if err != nil {
		return nil, err
	}
	return provider.WrapClient(client, r.extensionProviderMiddleware...)
}

func (r *Runtime) requestSessionTitle(ctx context.Context, client provider.Client, model provider.ModelRef, sessionID, turnID, userText string) (string, error) {
	request := provider.Request{
		SessionID:       sessionID,
		TurnID:          turnID,
		AgentID:         sessionTitleAgentID,
		Model:           model,
		MaxOutputTokens: requestMaxOutputTokensForModel(r.ModelCatalog, r.Config.ModelOverrides, r.Config.OutputBudgets, model, outputBudgetSessionTitle, false),
		Instructions:    sessionTitlePrompt,
		Inputs: []provider.Input{{
			Kind:    provider.InputKindUserMessage,
			Content: strings.TrimSpace(userText),
		}},
	}
	request.RawSSEObserver = r.sessionTitleRawSSEObserver(request)
	title, err := requestUtilityText(ctx, client, request, effectiveUtilityTimeout(utilityTimeoutDuration(r.Config.UtilityModelTimeoutSeconds), sessionTitleTimeout), utilityRetryPolicyFromConfig(r.Config))
	if err != nil {
		return "", err
	}
	return sanitizeSessionTitle(title), nil
}

func (r *Runtime) sessionTitleRawSSEObserver(request provider.Request) provider.RawSSEObserver {
	logger := r.log("session_title")
	if logger == nil || !logger.DebugEnabled() || provider.CanonicalProviderID(request.Model.ProviderID) != "github-copilot" {
		return nil
	}
	mode := rawSSEFrameLogMode()
	if mode == rawSSELogOff {
		return nil
	}
	return func(frame provider.RawSSEFrame) {
		args := []any{
			"session_id", request.SessionID,
			"turn_id", request.TurnID,
			"agent_id", request.AgentID,
			"requested_model", request.Model.String(),
			"model", request.Model.String(),
			"api_mode", frame.APIMode,
			"sse_sequence", frame.Sequence,
			"sse_event", frame.Event,
			"sse_data_bytes", len(frame.Data),
		}
		if mode == rawSSELogData {
			args = append(args, "sse_data", string(frame.Data))
		}
		logger.Debug("session title raw sse frame", args...)
	}
}

func sanitizeSessionTitle(raw string) string {
	title := sessiontitle.Normalize(raw)
	if title == "" {
		return ""
	}

	words := strings.Fields(title)
	if len(words) > 6 {
		title = strings.Join(words[:6], " ")
	}
	runes := []rune(title)
	if len(runes) > maxSessionTitleRuneLength {
		title = strings.TrimSpace(string(runes[:maxSessionTitleRuneLength]))
	}
	return strings.TrimSpace(title)
}
