// Package pipeline provides the middleware chain for processing one LLM turn.
package pipeline

import (
	"context"

	"github.com/sageil/kodacode/v1/internal/config"
	"github.com/sageil/kodacode/v1/internal/provider"
)

// TurnRequest is the shared state threaded through the pipeline for one turn.
// Agent is typed config.AgentConfig (not service.AgentConfig) to avoid import cycles.
type TurnRequest struct {
	SessionID         string
	AgentID           string
	ProviderID        string             // provider to use for this turn (e.g. "lmstudio")
	Messages          []provider.Message // built from parts; may be replaced by CompactionMiddleware
	CurrentInput      *provider.Message  // hydrated current-turn user message; reapplied after transcript reloads
	SummaryText       string             // formatted conversation summary loaded from storage into SystemParts[2]
	SystemParts       []string           // [stable, semiStable, volatile] — three elements
	ContextUsage      float64            // fraction [0.0, 1.0] of context window consumed
	Tools             []provider.Tool    // resolved by ToolResolverMiddleware
	Model             provider.Model
	Agent             config.AgentConfig
	Usage             *provider.Usage // populated by LLMMiddleware after the LLM call
	Step              int             // incremented by LLMMiddleware each tool-call loop iteration
	Origin            int             // 0 = TUI, 1 = API (matches sandbox.OriginTUI/OriginAPI)
	Variant           string          // thinking effort: "low", "high", "max", or "" (default)
	FallbackModels    []string        // ordered "providerID/modelID" fallbacks when primary fails
	Pins              []string        // sticky instructions that survive compaction (injected into system prompt)
	Ephemeral         bool            // true for subagent sessions — skips title gen, persistence, compaction
	HasTitle          bool            // true when the session already has a title — skips title generation
	FullTools         []provider.Tool // complete tool set before phase filtering; set when PhaseFilterActive
	PhaseFilterActive bool            // true when phase rules are gating tools
	Workflow          *WorkflowState  // structured workflow/approval/design state for this turn
	StreamInterrupted bool            // set when stream recovered from error with partial content — triggers orphan cleanup
}

// TurnMiddleware wraps a TurnHandler, adding behaviour before and/or after it.
type TurnMiddleware func(ctx context.Context, req *TurnRequest, next TurnHandler) error

// TurnHandler is the final handler or the next middleware in the chain.
type TurnHandler func(ctx context.Context, req *TurnRequest) error

// Chain executes an ordered sequence of middleware.
type Chain struct {
	middlewares []TurnMiddleware
}

// BuildChain constructs a Chain from an ordered list of middleware.
// Middleware executes in registration order (first registered = outermost wrapper).
func BuildChain(middlewares ...TurnMiddleware) *Chain {
	return &Chain{middlewares: middlewares}
}

// Execute runs the chain, invoking middleware in registration order.
func (c *Chain) Execute(ctx context.Context, req *TurnRequest) error {
	return c.build(0)(ctx, req)
}

func (c *Chain) build(i int) TurnHandler {
	if i >= len(c.middlewares) {
		return func(ctx context.Context, req *TurnRequest) error { return nil }
	}
	return func(ctx context.Context, req *TurnRequest) error {
		return c.middlewares[i](ctx, req, c.build(i+1))
	}
}
