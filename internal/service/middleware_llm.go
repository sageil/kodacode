package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"github.com/sageil/kodacode/v1/internal/config"
	"github.com/sageil/kodacode/v1/internal/pipeline"
	"github.com/sageil/kodacode/v1/internal/provider"
)

const defaultContextSize = 8192
const defaultMaxRetries = 5

type AskPermissionFunc func(ctx context.Context, sessionID, toolName, input string) func(toolName, input string) error

type SpawnSubagentFunc func(ctx context.Context, parentSessionID, agentID, task string, onProgress ProgressFunc) (string, error)

func NewLLMMiddleware(cc *ChainConfig) pipeline.TurnMiddleware {
	return func(ctx context.Context, req *pipeline.TurnRequest, next pipeline.TurnHandler) error {
		prov, modelID := resolveRequestProvider(cc.Registry, req)
		if prov == nil {
			return fmt.Errorf("llm: provider %q not registered", req.ProviderID)
		}

		var sc *SessionCost
		if cc.GetCost != nil {
			sc = cc.GetCost(ctx, req.SessionID)
		}
		var traces *SessionTraces
		if cc.GetTraces != nil {
			traces = cc.GetTraces(req.SessionID)
		}

		tl := &turnLoop{
			ctx: ctx, req: req, prov: prov, modelID: modelID,
			publish: cc.Publish, sndbx: cc.Sandbox, msgs: cc.Messages,
			askPerm: cc.AskPerm, askUser: cc.AskUser, spawnSubagent: cc.SpawnSubagent,
			globalCfg:     cc.Config,
			cfg:           &cc.Config.Session,
			utility:       resolveUtility(cc.Registry, cc.Config, req, cc.UtilityHealth),
			utilityHealth: cc.UtilityHealth,
			snapshotSvc:   cc.SnapshotSvc, sc: sc,
			budgetStatus: func(ctx context.Context, sessionID string, cfg *config.SessionConfig) BudgetStatus {
				if cc.GetBudget == nil {
					return BudgetStatus{}
				}
				return cc.GetBudget(ctx, sessionID)
			},
			toolCache: newToolResultCache(), lspDiagnoser: cc.LSPDiag,
			taskStore:        cc.TaskStore,
			sessionTraces:    traces,
			primaryModelID:   req.Model.ID,
			requestCostModel: req.Model,
		}
		tl.init()
		err := tl.run()
		if err != nil {
			err = tryFallbacks(ctx, req, err, cc, sc, traces)
		}
		if err != nil {
			return err
		}
		return next(ctx, req)
	}
}

func tryFallbacks(
	ctx context.Context,
	req *pipeline.TurnRequest,
	primaryErr error,
	cc *ChainConfig,
	sc *SessionCost,
	sessionTraces *SessionTraces,
) error {
	if len(req.FallbackModels) == 0 {
		return primaryErr
	}
	utility := resolveUtility(cc.Registry, cc.Config, req, cc.UtilityHealth)
	for _, fb := range req.FallbackModels {
		idx := strings.IndexByte(fb, '/')
		if idx <= 0 {
			continue
		}
		fbProv, ok := cc.Registry.Get(fb[:idx])
		if !ok {
			continue
		}
		log.Printf("llm: primary model failed, falling back to %s", fb)
		cc.Publish(req.SessionID, SSEEvent{
			Type: "retry",
			Data: SSEErrorData{Message: fmt.Sprintf("Primary model unavailable, trying %s…", fb)},
		})
		tl := &turnLoop{
			ctx: ctx, req: req, prov: fbProv, modelID: fb[idx+1:],
			publish: cc.Publish, sndbx: cc.Sandbox, msgs: cc.Messages,
			askPerm: cc.AskPerm, askUser: cc.AskUser, spawnSubagent: cc.SpawnSubagent,
			globalCfg:     cc.Config,
			cfg:           &cc.Config.Session,
			utility:       utility,
			utilityHealth: cc.UtilityHealth,
			snapshotSvc:   cc.SnapshotSvc, sc: sc,
			budgetStatus: func(ctx context.Context, sessionID string, cfg *config.SessionConfig) BudgetStatus {
				if cc.GetBudget == nil {
					return BudgetStatus{}
				}
				return cc.GetBudget(ctx, sessionID)
			},
			toolCache: newToolResultCache(), lspDiagnoser: cc.LSPDiag,
			taskStore:        cc.TaskStore,
			sessionTraces:    sessionTraces,
			primaryModelID:   req.Model.ID,
			requestCostModel: resolveCostModelForTurn(ctx, cc.Registry, fb[:idx], fb[idx+1:], req.Model),
		}
		tl.init()
		err := tl.run()
		if err == nil {
			return nil
		}
		log.Printf("llm: fallback %s also failed: %v", fb, err)
	}
	return primaryErr
}

func resolveCostModelForTurn(ctx context.Context, registry *provider.Registry, providerID, modelID string, fallback provider.Model) provider.Model {
	if registry == nil {
		return fallback
	}
	model, err := registry.ResolveModel(ctx, providerID, modelID)
	if err != nil {
		return fallback
	}
	return model
}

func providerSupportsCaching(id string) bool {
	return id == "anthropic" || id == "google"
}

// compactToolSchemas strips parameter-level "description" fields from tool
// schemas to save tokens when under context pressure. Tool-level Description
// is preserved for tool selection.
func compactToolSchemas(tools []provider.Tool) []provider.Tool {
	compact := make([]provider.Tool, len(tools))
	for i, t := range tools {
		meta := toolPromptMetaForTool(t)
		compact[i] = t
		compact[i].Description = compactToolDescriptionForTool(t)
		if len(t.Parameters) == 0 || meta.PreserveParameterDocs {
			continue
		}
		var schema map[string]any
		if json.Unmarshal(t.Parameters, &schema) != nil {
			continue
		}
		if props, ok := schema["properties"].(map[string]any); ok {
			for _, v := range props {
				if pm, ok := v.(map[string]any); ok {
					delete(pm, "description")
				}
			}
		}
		if out, err := json.Marshal(schema); err == nil {
			compact[i].Parameters = out
		}
	}
	return compact
}
