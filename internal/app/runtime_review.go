package app

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"

	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/prompt"
	toolpkg "github.com/sageil/kodacode/internal/tool"
)

const (
	reviewerAgentID               = "reviewer"
	manualReviewDefaultUserText   = "[review] Review the current workspace changes and report concrete, actionable issues."
	manualReviewDefaultTitle      = "Full Project Review"
	manualReviewPromptFragmentKey = "review-mode"
	manualReviewInstructionsKey   = "review-scope"
)

type StartSessionReviewInput struct {
	SessionID       string
	TurnID          string
	Instructions    string
	SkillIDs        []string
	ThinkingEnabled bool
	ThinkingMode    string
}

func (r *Runtime) StartSessionReview(ctx context.Context, input StartSessionReviewInput) (RunSessionResult, error) {
	if strings.TrimSpace(input.SessionID) == "" {
		return RunSessionResult{}, ErrSessionIDRequired
	}
	if strings.TrimSpace(input.TurnID) == "" {
		return RunSessionResult{}, ErrTurnIDRequired
	}

	view, err := r.loadTurnStartSessionView(ctx, input.SessionID)
	if err != nil {
		return RunSessionResult{}, err
	}
	mcpState, err := r.syncSessionMCPState(ctx, input.SessionID, view.workspaceRoot, view.mcp)
	if err != nil {
		return RunSessionResult{}, err
	}

	availableSkills, err := r.availableTurnSkills(view.workspaceRoot)
	if err != nil {
		return r.recordTurnFailure(ctx, input.SessionID, input.TurnID, manualReviewUserText(input.Instructions), nil, err)
	}
	effectiveSkillIDs := skillIDsForTurn(manualReviewUserText(input.Instructions), input.SkillIDs, availableSkills)

	resolved, err := r.resolveTurnCapabilitiesFromState(view.capabilitiesState(), resolveTurnCapabilitiesOptions{
		AgentID:          reviewerAgentID,
		SkillIDs:         append([]string(nil), effectiveSkillIDs...),
		StrictModelRoute: true,
	})
	if err != nil {
		return r.recordTurnFailure(ctx, input.SessionID, input.TurnID, manualReviewUserText(input.Instructions), nil, err)
	}

	allowedTools := manualReviewAllowedTools(resolved.AllowedTools)
	fragments, err := defaultTurnFragments(
		resolved.definition,
		view.workspaceRoot,
		view.additionalWorkspaceRoots,
		availableSkills,
		resolved.selectedSkills,
		allowedTools,
		mcpState,
		r.Memory,
		r.Config.Sessions.EffectiveResponseStyle(),
		r.Config.Execution,
		view.inspectionProgress,
	)
	if err != nil {
		return r.recordTurnFailure(ctx, input.SessionID, input.TurnID, manualReviewUserText(input.Instructions), nil, err)
	}
	fragments = append(fragments, manualReviewPromptFragments(input.Instructions)...)

	result, err := r.runExistingSessionTurn(ctx, runExistingTurnInput{
		SessionID:            input.SessionID,
		TurnID:               input.TurnID,
		UserText:             manualReviewUserText(input.Instructions),
		AgentID:              resolved.AgentID,
		SkillIDs:             append([]string(nil), effectiveSkillIDs...),
		ThinkingEnabled:      input.ThinkingEnabled,
		ThinkingMode:         input.ThinkingMode,
		Fragments:            fragments,
		AllowedToolsOverride: allowedTools,
		ModelRouteOverride:   resolved.ModelRoute,
		PreserveSessionModel: true,
		HideAssistantPreview: true,
	})
	if err != nil {
		return RunSessionResult{}, err
	}
	if result.Status == TurnRunStatusCompleted {
		if err := r.recordStructuredManualReview(ctx, input.SessionID, input.TurnID, manualReviewTitle(input.Instructions), result.AssistantText); err != nil {
			return RunSessionResult{}, err
		}
		return r.loadSessionTurnResult(ctx, input.SessionID, input.TurnID, RunTurnResult{Status: result.Status})
	}
	return result, nil
}

func manualReviewUserText(instructions string) string {
	if trimmed := strings.TrimSpace(instructions); trimmed != "" {
		return trimmed
	}
	return manualReviewDefaultUserText
}

func manualReviewTitle(instructions string) string {
	trimmed := strings.Join(strings.Fields(strings.TrimSpace(instructions)), " ")
	if trimmed == "" {
		return manualReviewDefaultTitle
	}
	trimmed = uppercaseLeadingLetter(trimmed)
	if strings.HasSuffix(strings.ToLower(trimmed), "review") {
		return trimmed
	}
	return trimmed + " Review"
}

func uppercaseLeadingLetter(text string) string {
	if text == "" {
		return ""
	}
	runes := []rune(text)
	for i, r := range runes {
		if r >= 'a' && r <= 'z' {
			runes[i] = r - ('a' - 'A')
			return string(runes)
		}
		if (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			return text
		}
	}
	return text
}

func manualReviewAllowedTools(allowedTools []string) []string {
	if len(allowedTools) == 0 {
		return nil
	}
	filtered := make([]string, 0, len(allowedTools))
	for _, toolName := range allowedTools {
		switch strings.TrimSpace(toolName) {
		case toolpkg.TaskWorkflowToolName, toolpkg.WebSearchToolName, toolpkg.WebFetchToolName:
			continue
		default:
			filtered = append(filtered, toolName)
		}
	}
	return slices.Compact(filtered)
}

func manualReviewPromptFragments(instructions string) []prompt.Fragment {
	fragments := []prompt.Fragment{
		{
			Kind:      prompt.KindRuntime,
			Source:    prompt.SourceRuntime,
			Stability: prompt.StabilityStable,
			Key:       manualReviewPromptFragmentKey,
			Label:     manualReviewPromptFragmentKey,
			Content: strings.Join([]string{
				"Review mode is active.",
				"- Use the configured `reviewer` agent guidance to decide how to conduct the review.",
				"- Primary target: the current uncommitted workspace changes, including newly added files when relevant.",
				"Output contract:",
				"- Return exactly one JSON object and nothing else. Do not wrap it in markdown or code fences.",
				"- The object must contain `findings`, `overall_correctness`, and `overall_summary`.",
				"- `findings` must be an array of objects with `severity`, `path`, `line`, `title`, and `explanation`.",
				"- `severity` must be one of `P0`, `P1`, `P2`, or `P3`.",
				"- `path` must be workspace-relative and `line` must be a 1-based integer.",
				"- `overall_correctness` must be `correct` or `incorrect`.",
				"- `overall_summary` must be 1-3 sentences.",
				"- If there are no qualifying findings, return `\"findings\": []`.",
			}, "\n"),
		},
	}
	scopeLines := []string{
		"Review scope:",
		"- Default scope is the current uncommitted workspace changes in this repository.",
	}
	if trimmed := strings.TrimSpace(instructions); trimmed != "" {
		scopeLines = append(scopeLines, "- Additional user review instructions: "+trimmed)
		scopeLines = append(scopeLines, "- Treat the user instructions as additional focus or narrowing criteria while keeping the review evidence grounded in the repository.")
	}
	fragments = append(fragments, prompt.Fragment{
		Kind:      prompt.KindRuntime,
		Source:    prompt.SourceRuntime,
		Stability: prompt.StabilityDynamic,
		Key:       manualReviewInstructionsKey,
		Label:     manualReviewInstructionsKey,
		Content:   strings.Join(scopeLines, "\n"),
	})
	return fragments
}

func (r *Runtime) recordStructuredManualReview(ctx context.Context, sessionID, turnID, title, raw string) error {
	payload, err := parseStructuredManualReview(raw, title)
	if err != nil {
		if logger := r.log("runtime_review"); logger != nil {
			logger.Debug("manual review output did not validate as structured json",
				"session_id", sessionID,
				"turn_id", turnID,
				"error", err.Error(),
			)
		}
		return nil
	}
	_, err = r.Sessions.append(ctx, events.Draft{
		SessionID: sessionID,
		TurnID:    turnID,
		Type:      events.TypeReviewRecorded,
		Payload:   payload,
	})
	return err
}

func parseStructuredManualReview(raw, title string) (events.ReviewRecordedPayload, error) {
	var payload events.ReviewRecordedPayload
	body, err := extractStructuredReviewJSON(raw)
	if err != nil {
		return events.ReviewRecordedPayload{}, err
	}
	if err := json.Unmarshal([]byte(body), &payload); err != nil {
		return events.ReviewRecordedPayload{}, err
	}
	payload.Title = strings.TrimSpace(title)
	if payload.Title == "" {
		payload.Title = manualReviewDefaultTitle
	}
	if err := events.ValidateReviewRecordedPayload(payload); err != nil {
		return events.ReviewRecordedPayload{}, err
	}
	return payload, nil
}

func extractStructuredReviewJSON(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", errors.New("structured review output is empty")
	}
	if body, ok, err := unwrapStructuredReviewFence(trimmed); err != nil {
		return "", err
	} else if ok {
		return body, nil
	}
	body, err := extractSingleBalancedJSONObject(trimmed)
	if err != nil {
		return "", err
	}
	return body, nil
}

func unwrapStructuredReviewFence(raw string) (string, bool, error) {
	lines := strings.Split(strings.TrimSpace(raw), "\n")
	if len(lines) < 3 {
		return "", false, nil
	}
	type fence struct {
		start int
		end   int
	}
	var fences []fence
	for i := 0; i < len(lines); i++ {
		opener := strings.TrimSpace(lines[i])
		if opener != "```" && !strings.EqualFold(opener, "```json") {
			continue
		}
		for j := i + 1; j < len(lines); j++ {
			if strings.TrimSpace(lines[j]) == "```" {
				fences = append(fences, fence{start: i, end: j})
				i = j
				break
			}
		}
	}
	if len(fences) == 0 {
		return "", false, nil
	}
	if len(fences) > 1 {
		return "", false, errors.New("structured review output contains multiple fenced blocks")
	}
	block := strings.TrimSpace(strings.Join(lines[fences[0].start+1:fences[0].end], "\n"))
	if block == "" {
		return "", false, errors.New("structured review fenced block is empty")
	}
	return block, true, nil
}

func extractSingleBalancedJSONObject(raw string) (string, error) {
	type span struct {
		start int
		end   int
	}
	var spans []span
	inString := false
	escaped := false
	depth := 0
	start := -1
	for idx, r := range raw {
		if inString {
			if escaped {
				escaped = false
				continue
			}
			switch r {
			case '\\':
				escaped = true
			case '"':
				inString = false
			}
			continue
		}
		switch r {
		case '"':
			if depth > 0 {
				inString = true
			}
		case '{':
			if depth == 0 {
				start = idx
			}
			depth++
		case '}':
			if depth == 0 {
				return "", errors.New("structured review output has an unmatched closing brace")
			}
			depth--
			if depth == 0 && start >= 0 {
				spans = append(spans, span{start: start, end: idx + len(string(r))})
				start = -1
			}
		}
	}
	if depth != 0 {
		return "", errors.New("structured review output has an unterminated JSON object")
	}
	if len(spans) == 0 {
		return "", errors.New("structured review output did not contain a JSON object")
	}
	if len(spans) > 1 {
		return "", errors.New("structured review output contains multiple JSON objects")
	}
	body := strings.TrimSpace(raw[spans[0].start:spans[0].end])
	if body == "" {
		return "", errors.New("structured review output did not contain a JSON object")
	}
	return body, nil
}
