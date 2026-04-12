package service

import (
	"context"
	"log"
	"strings"
	"time"

	"github.com/sageil/kodacode/v1/internal/config"
	"github.com/sageil/kodacode/v1/internal/pipeline"
	"github.com/sageil/kodacode/v1/internal/provider"
)

const titlePrompt = "Generate a concise title (max 6 words) for a conversation that starts with this message. Reply with only the title, no punctuation."

const (
	titleTimeout    = 30 * time.Second
	titleMaxRetries = 3
	titleRetryDelay = 2 * time.Second
)

func NewTitleMiddleware(
	registry *provider.Registry,
	cfg *config.Config,
	updateTitle func(ctx context.Context, sessionID, title string),
	getCost func(ctx context.Context, sessionID string) *SessionCost,
	utilityHealth *utilityHealthTracker,
) pipeline.TurnMiddleware {
	return func(ctx context.Context, req *pipeline.TurnRequest, next pipeline.TurnHandler) error {
		if req.Step == 0 && !req.Ephemeral && !req.HasTitle {
			utility := resolveUtility(registry, cfg, req, utilityHealth)
			if utility.prov != nil {
				sessionID := req.SessionID
				firstUserMsg := extractFirstUserMessage(req.Messages)
				var sc *SessionCost
				if getCost != nil {
					sc = getCost(ctx, sessionID)
				}
				go generateTitle(context.WithoutCancel(ctx), utility, sessionID, firstUserMsg, updateTitle, sc, utilityHealth)
			}
		}
		return next(ctx, req)
	}
}

func extractFirstUserMessage(msgs []provider.Message) string {
	for _, m := range msgs {
		if m.Role == "user" {
			for _, p := range m.Parts {
				if tp, ok := p.(provider.TextPart); ok && tp.Text != "" {
					return tp.Text
				}
			}
		}
	}
	return ""
}

func generateTitle(parent context.Context, utility utilityProvider, sessionID string, userMsg string, updateTitle func(context.Context, string, string), sc *SessionCost, utilityHealth *utilityHealthTracker) {
	if userMsg == "" {
		return
	}

	msgs := []provider.Message{
		{
			Role:  "user",
			Parts: []provider.MessagePart{provider.TextPart{Text: userMsg}},
		},
	}
	opts := provider.ChatOptions{
		SystemParts: []string{titlePrompt, ""},
	}

	for _, candidate := range utilityCandidates(utility) {
		log.Printf("title: using provider=%s model=%s for session %s", candidate.prov.ID(), candidate.modelID, sessionID)
		for attempt := range titleMaxRetries {
			if attempt > 0 {
				select {
				case <-parent.Done():
					log.Printf("title: cancelled for session %s", sessionID)
					return
				case <-time.After(titleRetryDelay):
				}
			}

			title, usage, err := tryGenerateTitle(parent, candidate.prov, candidate.modelID, msgs, opts)
			if err != nil {
				if parent.Err() != nil {
					log.Printf("title: cancelled for session %s", sessionID)
					return
				}
				log.Printf("title: attempt %d/%d for session %s with %s/%s: %v", attempt+1, titleMaxRetries, sessionID, candidate.prov.ID(), candidate.modelID, err)
				if isUtilityPermanentUnavailable(err) {
					utilityHealth.markUnavailable(candidate)
					break
				}
				continue
			}
			if title == "" {
				log.Printf("title: attempt %d/%d for session %s with %s/%s: empty response", attempt+1, titleMaxRetries, sessionID, candidate.prov.ID(), candidate.modelID)
				continue
			}

			utilityHealth.markAvailable(candidate)
			if sc != nil && usage != nil {
				sc.Add(usage, provider.Model{CostInput: candidate.costIn, CostOutput: candidate.costOut})
			}

			log.Printf("title: generated %q for session %s via %s/%s", title, sessionID, candidate.prov.ID(), candidate.modelID)
			ctx, cancel := context.WithTimeout(parent, 5*time.Second)
			updateTitle(ctx, sessionID, title)
			cancel()
			return
		}
	}
	log.Printf("title: giving up after %d attempts for session %s", titleMaxRetries, sessionID)
}

func tryGenerateTitle(parent context.Context, prov provider.Provider, modelID string, msgs []provider.Message, opts provider.ChatOptions) (string, *provider.Usage, error) {
	ctx, cancel := context.WithTimeout(parent, titleTimeout)
	defer cancel()

	stream, err := prov.Chat(ctx, modelID, msgs, opts)
	if err != nil {
		return "", nil, err
	}
	var sb strings.Builder
	var usage *provider.Usage
	for chunk := range stream {
		if chunk.Err != nil {
			return "", nil, chunk.Err
		}
		sb.WriteString(chunk.Delta)
		if chunk.Usage != nil {
			usage = chunk.Usage
		}
	}
	return strings.TrimSpace(sb.String()), usage, nil
}
