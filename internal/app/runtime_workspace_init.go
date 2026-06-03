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
	anthropicPromptInstructionsFilename = "CLAUDE.md"

	workspaceInstructionsSourceTemplate = "template"
	workspaceInstructionsSourceUtility  = "utility"

	workspaceInitUtilityAgentID             = "_workspace_init"
	workspaceInitUtilityTurnID              = "turn-1"
	workspaceInitUtilityTimeout             = 20 * time.Second
	workspaceInitUtilitySummaryMaxBytes     = 2000
	workspaceInitUtilityFileMaxBytes        = 3000
	workspaceInitUtilityContextBudgetBytes  = 12000
	workspaceInitUtilityTopLevelEntryLimit  = 24
	workspaceInitUtilityContextFileMaxCount = 10
	workspaceInitUtilityPreviewMaxBytes     = 240
)

const defaultWorkspaceInstructionsTemplate = `# AGENTS.md

Use this file for repo-specific agent instructions.

## Project Context
- Describe what this repository does and what good changes should optimize for.
- Note important domain terms or product constraints.

## Workflow
- List the build, test, and lint commands agents should run.
- State what must be verified before a change is considered done.

## Architecture Notes
- Document important ownership boundaries and source-of-truth files.
- Record saved conventions that are easy to miss but should be followed.

Keep this file concise and repo-specific. Put saved policy here, not temporary task notes.
`

const defaultAnthropicPromptInstructionsTemplate = `# CLAUDE.md

This repository keeps its canonical agent instructions in ` + "`AGENTS.md`" + `.

Before making changes, read and follow ` + "`AGENTS.md`" + `.
If this file and ` + "`AGENTS.md`" + ` ever differ, treat ` + "`AGENTS.md`" + ` as the source of truth.
`

const workspaceInitUtilityPrompt = `Generate the initial AGENTS.md for this software repository.

Return JSON only with this shape:
{"agents_md":"<full markdown file content>"}

Requirements:
- Write saved repository-specific instructions for coding agents.
- Use only facts or conservative inferences supported by the provided repository context.
- Keep it concise and directly usable.
- Prefer concrete workflow and architecture guidance when the repo context supports it.
- If the context is thin, keep the file short instead of inventing specifics.
- Do not include TODO placeholders, questionnaires, or requests for the user to fill anything in.
- Do not output markdown fences.
- Do not mention this prompt, JSON, or utility models.`

var workspaceInitContextFiles = []string{
	"README.md",
	"README",
	"go.mod",
	"go.work",
	"package.json",
	"pnpm-workspace.yaml",
	"tsconfig.json",
	"Cargo.toml",
	"pyproject.toml",
	"requirements.txt",
	"Makefile",
	"Taskfile.yml",
	"Taskfile.yaml",
	"justfile",
	"pom.xml",
	"build.gradle",
	"build.gradle.kts",
	"Gemfile",
	"docker-compose.yml",
	"docker-compose.yaml",
	"compose.yml",
	"compose.yaml",
}

type InitializeWorkspaceInstructionsInput struct {
	WorkspaceRoot string
	IncludeClaude bool
	ModelRoute    provider.ModelRoute
}

type InitializeWorkspaceInstructionsResult struct {
	WorkspaceRoot string
	AgentsPath    string
	AgentsCreated bool
	AgentsSource  string
	AgentsModel   provider.ModelRef
	ClaudePath    string
	ClaudeCreated bool
}

type workspaceInitUtilityPayload struct {
	AgentsMD string `json:"agents_md"`
}

func (r *Runtime) InitializeWorkspaceInstructions(ctx context.Context, input InitializeWorkspaceInstructionsInput) (InitializeWorkspaceInstructionsResult, error) {
	if strings.TrimSpace(input.WorkspaceRoot) == "" {
		return InitializeWorkspaceInstructionsResult{}, ErrWorkspaceRootRequired
	}
	if ctx == nil {
		ctx = context.Background()
	}
	logger := r.log("workspace_init")

	scope, err := workspace.New(input.WorkspaceRoot)
	if err != nil {
		return InitializeWorkspaceInstructionsResult{}, fmt.Errorf("resolve workspace root: %w", err)
	}
	if logger != nil {
		logger.Debug("workspace instructions initialization started",
			"workspace_root", scope.Root(),
			"include_claude", input.IncludeClaude,
			"selected_model", input.ModelRoute.Primary.String(),
		)
	}

	result := InitializeWorkspaceInstructionsResult{
		WorkspaceRoot: scope.Root(),
		AgentsPath:    filepath.Join(scope.Root(), promptInstructionsFilename),
	}

	agentsContent, agentsSource, agentsModel := defaultWorkspaceInstructionsTemplate, workspaceInstructionsSourceTemplate, provider.ModelRef{}
	if !fileExists(result.AgentsPath) {
		if generated, model, ok := r.generateWorkspaceInstructionsWithUtilityModel(ctx, scope.Root(), input.ModelRoute); ok {
			agentsContent, agentsSource, agentsModel = generated, workspaceInstructionsSourceUtility, model
		}
	} else if logger != nil {
		logger.Debug("workspace instructions initialization skipped existing agents file",
			"workspace_root", scope.Root(),
			"path", result.AgentsPath,
		)
	}
	result.AgentsCreated, err = writeFileIfMissing(result.AgentsPath, agentsContent)
	if err != nil {
		return InitializeWorkspaceInstructionsResult{}, err
	}
	if result.AgentsCreated {
		result.AgentsSource = agentsSource
		result.AgentsModel = agentsModel
	}

	if input.IncludeClaude {
		result.ClaudePath = filepath.Join(scope.Root(), anthropicPromptInstructionsFilename)
		result.ClaudeCreated, err = writeFileIfMissing(result.ClaudePath, defaultAnthropicPromptInstructionsTemplate)
		if err != nil {
			return InitializeWorkspaceInstructionsResult{}, err
		}
	}

	if logger != nil {
		logger.Debug("workspace instructions initialization completed",
			"workspace_root", scope.Root(),
			"agents_created", result.AgentsCreated,
			"agents_source", result.AgentsSource,
			"agents_model", result.AgentsModel.String(),
			"claude_created", result.ClaudeCreated,
		)
	}

	return result, nil
}

func (r *Runtime) generateWorkspaceInstructionsWithUtilityModel(ctx context.Context, workspaceRoot string, route provider.ModelRoute) (string, provider.ModelRef, bool) {
	if r == nil {
		return "", provider.ModelRef{}, false
	}
	logger := r.log("workspace_init")
	inputs := buildWorkspaceInitUtilityInputs(workspaceRoot)
	if len(inputs) == 0 {
		if logger != nil {
			logger.Debug("workspace instructions utility generation skipped; no repository context",
				"workspace_root", workspaceRoot,
			)
		}
		return "", provider.ModelRef{}, false
	}
	candidates := availableUtilityTextCandidates(r.Config.UtilityModel, route, r.utilityProviderAvailable())
	if logger != nil {
		logger.Debug("workspace instructions utility generation started",
			"workspace_root", workspaceRoot,
			"candidate_count", len(candidates),
			"input_count", len(inputs),
			"input_bytes", workspaceInitInputBytes(inputs),
			"configured_utility_model", r.Config.UtilityModel.String(),
			"selected_model", route.Primary.String(),
		)
	}
	if len(candidates) == 0 {
		if logger != nil {
			logger.Debug("workspace instructions utility generation skipped; no available candidates",
				"workspace_root", workspaceRoot,
				"configured_utility_model", r.Config.UtilityModel.String(),
				"selected_model", route.Primary.String(),
			)
		}
		return "", provider.ModelRef{}, false
	}

	for idx, candidate := range candidates {
		request := provider.Request{
			SessionID:       workspaceInitUtilitySessionID(workspaceRoot),
			TurnID:          workspaceInitUtilityTurnID,
			AgentID:         workspaceInitUtilityAgentID,
			Model:           candidate.Ref,
			MaxOutputTokens: workspaceInitMaxOutputTokensForModel(r.ModelCatalog, r.Config.ModelOverrides, r.Config.OutputBudgets, candidate.Ref),
			Instructions:    workspaceInitUtilityPrompt,
			Inputs:          append([]provider.Input(nil), inputs...),
		}
		model := utilityCatalogModelForRef(r.ModelCatalog, candidate.Ref)
		requiredContext := provider.EstimateRequestTokens(request)
		contextLimit := effectiveUtilityContextSize(model)
		if !utilityCandidateMeetsContext(model, requiredContext) {
			if logger != nil {
				logger.Debug("workspace instructions utility candidate skipped; insufficient context",
					"workspace_root", workspaceRoot,
					"model", candidate.Ref.String(),
					"candidate_index", idx+1,
					"candidate_count", len(candidates),
					"required_context_tokens", requiredContext,
					"context_limit_tokens", contextLimit,
				)
			}
			continue
		}
		client, err := r.rawProviderClient(candidate.Ref.ProviderID)
		if err != nil {
			if logger != nil {
				logger.Debug("workspace instructions utility provider unavailable",
					"workspace_root", workspaceRoot,
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
			effectiveUtilityTimeout(utilityTimeoutDuration(r.Config.UtilityModelTimeoutSeconds), workspaceInitUtilityTimeout),
			utilityRetryPolicyFromConfig(r.Config),
		)
		if err != nil {
			if logger != nil {
				args := []any{
					"workspace_root", workspaceRoot,
					"model", candidate.Ref.String(),
					"candidate_index", idx + 1,
					"candidate_count", len(candidates),
				}
				args = append(args, providerErrorLogFields(err)...)
				args = append(args, "error", err.Error())
				logger.Debug("workspace instructions utility request failed", args...)
			}
			continue
		}
		content, err := parseWorkspaceInitUtilityContent(raw)
		if err != nil {
			if logger != nil {
				logger.Debug("workspace instructions utility response parse failed",
					"workspace_root", workspaceRoot,
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
			logger.Debug("workspace instructions utility generation succeeded",
				"workspace_root", workspaceRoot,
				"model", candidate.Ref.String(),
				"candidate_index", idx+1,
				"candidate_count", len(candidates),
				"content_bytes", len(content),
			)
		}
		return content, candidate.Ref, true
	}
	if logger != nil {
		logger.Debug("workspace instructions utility generation exhausted; using template fallback",
			"workspace_root", workspaceRoot,
			"candidate_count", len(candidates),
		)
	}
	return "", provider.ModelRef{}, false
}

func buildWorkspaceInitUtilityInputs(workspaceRoot string) []provider.Input {
	inputs := make([]provider.Input, 0, workspaceInitUtilityContextFileMaxCount+1)
	budget := workspaceInitUtilityContextBudgetBytes

	if summary := renderWorkspaceInitSummary(workspaceRoot); summary != "" {
		summary = truncateUTF8Bytes(summary, workspaceInitUtilitySummaryMaxBytes)
		inputs = append(inputs, provider.Input{
			Kind:    provider.InputKindUserMessage,
			Content: summary,
		})
		budget -= len(summary)
	}
	if budget <= 0 {
		return inputs
	}

	count := 0
	for _, relativePath := range workspaceInitContextFiles {
		if count >= workspaceInitUtilityContextFileMaxCount || budget <= 0 {
			break
		}
		fullPath := filepath.Join(workspaceRoot, relativePath)
		info, err := os.Stat(fullPath)
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		content, err := os.ReadFile(fullPath)
		if err != nil {
			continue
		}
		body := strings.TrimSpace(string(content))
		if body == "" {
			continue
		}
		rendered := "File: " + relativePath + "\n" + truncateUTF8Bytes(body, min(workspaceInitUtilityFileMaxBytes, budget))
		if strings.TrimSpace(rendered) == "" {
			continue
		}
		inputs = append(inputs, provider.Input{
			Kind:    provider.InputKindUserMessage,
			Content: rendered,
		})
		budget -= len(rendered)
		count++
	}

	return inputs
}

func renderWorkspaceInitSummary(workspaceRoot string) string {
	name := filepath.Base(strings.TrimSpace(workspaceRoot))
	if name == "" {
		name = "repo"
	}
	lines := []string{
		"Repository summary:",
		"- name: " + name,
	}
	entries, err := os.ReadDir(workspaceRoot)
	if err != nil {
		return strings.Join(lines, "\n")
	}
	topLevel := make([]string, 0, min(len(entries), workspaceInitUtilityTopLevelEntryLimit))
	for _, entry := range entries {
		if len(topLevel) >= workspaceInitUtilityTopLevelEntryLimit {
			break
		}
		name := entry.Name()
		if entry.IsDir() {
			name += "/"
		}
		topLevel = append(topLevel, name)
	}
	if len(topLevel) > 0 {
		lines = append(lines, "- top-level entries: "+strings.Join(topLevel, ", "))
	}
	return strings.Join(lines, "\n")
}

func workspaceInitMaxOutputTokensForModel(models modelCatalog, overrides []ModelOverrideConfig, budgets OutputBudgetsConfig, ref provider.ModelRef) int {
	return requestMaxOutputTokensForModel(models, overrides, budgets, ref, outputBudgetUtilityText, false)
}

func parseWorkspaceInitUtilityContent(raw string) (string, error) {
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

	var payload workspaceInitUtilityPayload
	if err := json.Unmarshal([]byte(trimmed), &payload); err != nil {
		return "", err
	}
	content := sanitizeWorkspaceInitMarkdown(payload.AgentsMD)
	if strings.TrimSpace(content) == "" {
		return "", errors.New("utility response agents_md is empty")
	}
	return content, nil
}

func sanitizeWorkspaceInitMarkdown(raw string) string {
	text := strings.TrimSpace(strings.ReplaceAll(raw, "\r\n", "\n"))
	if strings.HasPrefix(text, "```") {
		lines := strings.Split(text, "\n")
		if len(lines) >= 3 {
			last := strings.TrimSpace(lines[len(lines)-1])
			if strings.HasPrefix(last, "```") {
				text = strings.Join(lines[1:len(lines)-1], "\n")
			}
		}
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	if !strings.HasSuffix(text, "\n") {
		text += "\n"
	}
	return text
}

func writeFileIfMissing(path string, content string) (bool, error) {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return false, nil
		}
		return false, fmt.Errorf("create %s: %w", filepath.Base(path), err)
	}
	defer func() {
		_ = file.Close()
	}()

	if _, err := file.WriteString(content); err != nil {
		return false, fmt.Errorf("write %s: %w", filepath.Base(path), err)
	}
	return true, nil
}

func workspaceInitInputBytes(inputs []provider.Input) int {
	total := 0
	for _, input := range inputs {
		total += len(input.Content)
	}
	return total
}

func workspaceInitResponsePreview(raw string) string {
	preview := strings.TrimSpace(raw)
	if preview == "" {
		return ""
	}
	return truncateUTF8Bytes(preview, workspaceInitUtilityPreviewMaxBytes)
}

func workspaceInitUtilitySessionID(workspaceRoot string) string {
	name := filepath.Base(strings.TrimSpace(workspaceRoot))
	if name == "" {
		name = "repo"
	}
	name = strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z':
			return r
		case r >= 'A' && r <= 'Z':
			return r + ('a' - 'A')
		case r >= '0' && r <= '9':
			return r
		case r == '-' || r == '_':
			return r
		default:
			return '-'
		}
	}, name)
	name = strings.Trim(name, "-")
	if name == "" {
		name = "repo"
	}
	return "workspace-init-" + name
}

func fileExists(path string) bool {
	if strings.TrimSpace(path) == "" {
		return false
	}
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
