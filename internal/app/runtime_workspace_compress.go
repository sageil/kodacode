package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/sageil/kodacode/internal/provider"
	"github.com/sageil/kodacode/internal/workspace"
)

const (
	workspacePromptCompressionAgentID        = "_workspace_prompt_compress"
	workspacePromptCompressionTimeout        = 20 * time.Second
	workspacePromptCompressionSourceMaxBytes = 6000
)

const workspaceAgentsCompressionUtilityPrompt = `Compress this repository's AGENTS.md for coding agents.

Return JSON only with this shape:
{"content":"<full markdown file content>"}

Requirements:
- Preserve durable repository-specific rules, constraints, workflow commands, verification requirements, architecture boundaries, and source-of-truth notes supported by the provided content.
- Remove redundancy, repeated section content, filler, and verbose phrasing.
- Keep the result concise, directly usable, and in Markdown.
- Do not add placeholders, TODOs, or requests for missing information.
- Do not output markdown fences.`

const workspaceMemoryCompressionUtilityPrompt = `Compress this durable project memory entry.

Return JSON only with this shape:
{"content":"<full memory entry content>"}

Requirements:
- Preserve durable facts, decisions, constraints, identifiers, and non-obvious project context supported by the provided content.
- Keep the result shorter and easier to inject into future prompts.
- Remove repetition and filler, but do not remove substantive facts.
- Keep the entry standalone and directly usable.
- Do not add placeholders, TODOs, or requests for missing information.
- Do not output markdown fences.`

type CompressWorkspacePromptSourcesInput struct {
	WorkspaceRoot string
	ModelRoute    provider.ModelRoute
}

type CompressWorkspacePromptSourcesResult struct {
	WorkspaceRoot      string
	AgentsPath         string
	AgentsPresent      bool
	AgentsUpdated      bool
	AgentsSkippedLarge bool
	AgentsBytesBefore  int
	AgentsBytesAfter   int
	MemoryCount        int
	MemoryUpdatedCount int
	MemorySkippedLarge int
	MemoryBytesBefore  int
	MemoryBytesAfter   int
}

type workspacePromptCompressionPayload struct {
	Content string `json:"content"`
}

type workspacePromptCompressionTarget struct {
	Kind          string
	Path          string
	DisplayName   string
	Prompt        string
	InputLabel    string
	Content       string
	TurnID        string
	MarkdownStyle bool
}

func (r *Runtime) CompressWorkspacePromptSources(ctx context.Context, input CompressWorkspacePromptSourcesInput) (CompressWorkspacePromptSourcesResult, error) {
	if strings.TrimSpace(input.WorkspaceRoot) == "" {
		return CompressWorkspacePromptSourcesResult{}, ErrWorkspaceRootRequired
	}
	if ctx == nil {
		ctx = context.Background()
	}
	logger := r.log("workspace_compress")

	scope, err := workspace.New(input.WorkspaceRoot)
	if err != nil {
		return CompressWorkspacePromptSourcesResult{}, fmt.Errorf("resolve workspace root: %w", err)
	}
	result := CompressWorkspacePromptSourcesResult{
		WorkspaceRoot: scope.Root(),
		AgentsPath:    filepath.Join(scope.Root(), promptInstructionsFilename),
	}
	if logger != nil {
		logger.Debug("workspace prompt-source compression started",
			"workspace_root", scope.Root(),
			"selected_model", input.ModelRoute.Primary.String(),
		)
	}

	targets, err := collectWorkspacePromptCompressionTargets(scope.Root(), r.Memory)
	if err != nil {
		return CompressWorkspacePromptSourcesResult{}, err
	}
	if len(targets) == 0 {
		if logger != nil {
			logger.Debug("workspace prompt-source compression skipped; no prompt sources",
				"workspace_root", scope.Root(),
			)
		}
		return result, nil
	}

	replacements := make(map[string]string, len(targets))
	for _, target := range targets {
		beforeBytes := len(strings.TrimSpace(target.Content))
		switch target.Kind {
		case "agents":
			result.AgentsPresent = true
			result.AgentsBytesBefore = beforeBytes
			result.AgentsBytesAfter = beforeBytes
		case "memory":
			result.MemoryCount++
			result.MemoryBytesBefore += beforeBytes
			result.MemoryBytesAfter += beforeBytes
		}
		if beforeBytes > workspacePromptCompressionSourceMaxBytes {
			switch target.Kind {
			case "agents":
				result.AgentsSkippedLarge = true
			case "memory":
				result.MemorySkippedLarge++
			}
			if logger != nil {
				logger.Debug("workspace prompt-source compression skipped; source exceeds safe utility input window",
					"workspace_root", scope.Root(),
					"target", target.DisplayName,
					"content_bytes", beforeBytes,
					"max_input_bytes", workspacePromptCompressionSourceMaxBytes,
				)
			}
			continue
		}

		compressed, _, err := r.compressWorkspacePromptSourceWithUtilityModel(ctx, scope.Root(), input.ModelRoute, target)
		if err != nil {
			return CompressWorkspacePromptSourcesResult{}, err
		}
		afterBytes := len(strings.TrimSpace(compressed))
		if afterBytes <= 0 || afterBytes >= beforeBytes {
			continue
		}
		replacements[target.Path] = compressed
		switch target.Kind {
		case "agents":
			result.AgentsUpdated = true
			result.AgentsBytesAfter = afterBytes
		case "memory":
			result.MemoryUpdatedCount++
			result.MemoryBytesAfter += afterBytes - beforeBytes
		}
	}

	for _, target := range targets {
		content, ok := replacements[target.Path]
		if !ok {
			continue
		}
		if err := os.WriteFile(target.Path, []byte(content), 0o644); err != nil {
			return CompressWorkspacePromptSourcesResult{}, fmt.Errorf("write %s: %w", target.DisplayName, err)
		}
	}

	if logger != nil {
		logger.Debug("workspace prompt-source compression completed",
			"workspace_root", scope.Root(),
			"agents_present", result.AgentsPresent,
			"agents_updated", result.AgentsUpdated,
			"agents_skipped_large", result.AgentsSkippedLarge,
			"agents_bytes_before", result.AgentsBytesBefore,
			"agents_bytes_after", result.AgentsBytesAfter,
			"memory_count", result.MemoryCount,
			"memory_updated_count", result.MemoryUpdatedCount,
			"memory_skipped_large", result.MemorySkippedLarge,
			"memory_bytes_before", result.MemoryBytesBefore,
			"memory_bytes_after", result.MemoryBytesAfter,
		)
	}
	return result, nil
}

func collectWorkspacePromptCompressionTargets(workspaceRoot string, memories *MemoryService) ([]workspacePromptCompressionTarget, error) {
	root := strings.TrimSpace(workspaceRoot)
	if root == "" {
		return nil, nil
	}
	targets := make([]workspacePromptCompressionTarget, 0, 8)
	agentsPath := filepath.Join(root, promptInstructionsFilename)
	if content, ok, err := readPromptSource(agentsPath); err != nil {
		return nil, err
	} else if ok {
		targets = append(targets, workspacePromptCompressionTarget{
			Kind:          "agents",
			Path:          agentsPath,
			DisplayName:   promptInstructionsFilename,
			Prompt:        workspaceAgentsCompressionUtilityPrompt,
			InputLabel:    "Current AGENTS.md",
			Content:       content,
			TurnID:        "turn-agents",
			MarkdownStyle: true,
		})
	}
	if memories == nil {
		return targets, nil
	}
	store := memories.Store(root)
	if store == nil {
		return targets, nil
	}
	records, err := store.ListMemories()
	if err != nil {
		return nil, err
	}
	for _, record := range records {
		content := strings.TrimSpace(record.Content)
		if content == "" || strings.TrimSpace(record.Path) == "" {
			continue
		}
		targets = append(targets, workspacePromptCompressionTarget{
			Kind:          "memory",
			Path:          filepath.Join(root, filepath.FromSlash(record.Path)),
			DisplayName:   "project memory " + record.ID,
			Prompt:        workspaceMemoryCompressionUtilityPrompt,
			InputLabel:    "Current project memory entry (" + record.ID + ")",
			Content:       content,
			TurnID:        "turn-memory-" + record.ID,
			MarkdownStyle: false,
		})
	}
	return targets, nil
}

func (r *Runtime) compressWorkspacePromptSourceWithUtilityModel(
	ctx context.Context,
	workspaceRoot string,
	route provider.ModelRoute,
	target workspacePromptCompressionTarget,
) (string, provider.ModelRef, error) {
	if r == nil {
		return "", provider.ModelRef{}, provider.ErrProviderNotConfigured
	}
	logger := r.log("workspace_compress")
	inputs := buildWorkspacePromptCompressionInputs(workspaceRoot, target)
	if len(inputs) == 0 {
		return "", provider.ModelRef{}, fmt.Errorf("compress %s: no prompt-source input", target.DisplayName)
	}
	candidates := availableUtilityTextCandidates(r.Config.UtilityModel, route, r.utilityProviderAvailable())
	if len(candidates) == 0 {
		return "", provider.ModelRef{}, fmt.Errorf("compress %s: no available utility model candidate", target.DisplayName)
	}

	var (
		lastErr           error
		skippedForContext bool
	)
	for idx, candidate := range candidates {
		request := provider.Request{
			SessionID:       workspaceInitUtilitySessionID(workspaceRoot),
			TurnID:          target.TurnID,
			AgentID:         workspacePromptCompressionAgentID,
			Model:           candidate.Ref,
			MaxOutputTokens: workspacePromptCompressionMaxOutputTokensForModel(r.ModelCatalog, r.Config.ModelOverrides, r.Config.OutputBudgets, candidate.Ref),
			Instructions:    target.Prompt,
			Inputs:          append([]provider.Input(nil), inputs...),
		}
		model := utilityCatalogModelForRef(r.ModelCatalog, candidate.Ref)
		requiredContext := provider.EstimateRequestTokens(request)
		if !utilityCandidateMeetsContext(model, requiredContext) {
			skippedForContext = true
			if logger != nil {
				logger.Debug("workspace prompt-source compression candidate skipped; insufficient context",
					"workspace_root", workspaceRoot,
					"target", target.DisplayName,
					"model", candidate.Ref.String(),
					"candidate_index", idx+1,
					"candidate_count", len(candidates),
					"required_context_tokens", requiredContext,
					"context_limit_tokens", effectiveUtilityContextSize(model),
				)
			}
			continue
		}
		client, err := r.rawProviderClient(candidate.Ref.ProviderID)
		if err != nil {
			lastErr = err
			if logger != nil {
				logger.Debug("workspace prompt-source compression provider unavailable",
					"workspace_root", workspaceRoot,
					"target", target.DisplayName,
					"model", candidate.Ref.String(),
					"candidate_index", idx+1,
					"candidate_count", len(candidates),
					"error", err.Error(),
				)
			}
			continue
		}
		raw, err := requestUtilityText(
			ctx,
			client,
			request,
			effectiveUtilityTimeout(utilityTimeoutDuration(r.Config.UtilityModelTimeoutSeconds), workspacePromptCompressionTimeout),
			utilityRetryPolicyFromConfig(r.Config),
		)
		if err != nil {
			lastErr = err
			if logger != nil {
				args := []any{
					"workspace_root", workspaceRoot,
					"target", target.DisplayName,
					"model", candidate.Ref.String(),
					"candidate_index", idx + 1,
					"candidate_count", len(candidates),
				}
				args = append(args, providerErrorLogFields(err)...)
				args = append(args, "error", err.Error())
				logger.Debug("workspace prompt-source compression request failed", args...)
			}
			continue
		}
		content, err := parseWorkspacePromptCompressionContent(raw, target.MarkdownStyle)
		if err != nil {
			lastErr = err
			if logger != nil {
				logger.Debug("workspace prompt-source compression response parse failed",
					"workspace_root", workspaceRoot,
					"target", target.DisplayName,
					"model", candidate.Ref.String(),
					"candidate_index", idx+1,
					"candidate_count", len(candidates),
					"error", err.Error(),
					"response_bytes", len(raw),
					"response_preview", workspaceInitResponsePreview(raw),
				)
			}
			continue
		}
		if logger != nil {
			logger.Debug("workspace prompt-source compression succeeded",
				"workspace_root", workspaceRoot,
				"target", target.DisplayName,
				"model", candidate.Ref.String(),
				"candidate_index", idx+1,
				"candidate_count", len(candidates),
				"content_bytes_before", len(strings.TrimSpace(target.Content)),
				"content_bytes_after", len(strings.TrimSpace(content)),
			)
		}
		return content, candidate.Ref, nil
	}

	switch {
	case lastErr != nil:
		return "", provider.ModelRef{}, fmt.Errorf("compress %s: %w", target.DisplayName, lastErr)
	case skippedForContext:
		return "", provider.ModelRef{}, fmt.Errorf("compress %s: no utility model had enough context", target.DisplayName)
	default:
		return "", provider.ModelRef{}, fmt.Errorf("compress %s: no usable utility model response", target.DisplayName)
	}
}

func buildWorkspacePromptCompressionInputs(workspaceRoot string, target workspacePromptCompressionTarget) []provider.Input {
	inputs := make([]provider.Input, 0, 2)
	if summary := renderWorkspaceInitSummary(workspaceRoot); summary != "" {
		inputs = append(inputs, provider.Input{
			Kind:    provider.InputKindUserMessage,
			Content: truncateUTF8Bytes(summary, workspaceInitUtilitySummaryMaxBytes),
		})
	}
	content := strings.TrimSpace(target.Content)
	if content == "" {
		return inputs
	}
	inputs = append(inputs, provider.Input{
		Kind: provider.InputKindUserMessage,
		Content: target.InputLabel + ":\n" +
			truncateUTF8Bytes(content, workspacePromptCompressionSourceMaxBytes),
	})
	return inputs
}

func workspacePromptCompressionMaxOutputTokensForModel(models modelCatalog, overrides []ModelOverrideConfig, budgets OutputBudgetsConfig, ref provider.ModelRef) int {
	return requestMaxOutputTokensForModel(models, overrides, budgets, ref, outputBudgetWorkspaceCompress, false)
}

func parseWorkspacePromptCompressionContent(raw string, markdown bool) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", errors.New("empty utility response")
	}
	start := strings.Index(trimmed, "{")
	end := strings.LastIndex(trimmed, "}")
	if start < 0 || end <= start {
		return "", errors.New("utility response missing json object")
	}
	trimmed = trimmed[start : end+1]

	var payload workspacePromptCompressionPayload
	if err := json.Unmarshal([]byte(trimmed), &payload); err != nil {
		return "", err
	}
	content := strings.TrimSpace(payload.Content)
	if markdown {
		content = sanitizeWorkspaceInitMarkdown(content)
	} else {
		content = strings.TrimSpace(content)
	}
	if strings.TrimSpace(content) == "" {
		return "", errors.New("utility response content is empty")
	}
	return content, nil
}
