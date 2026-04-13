package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/sageil/kodacode/v1/internal/provider"
)

type workflowIntentKind string

const (
	workflowIntentCasual             workflowIntentKind = "casual"
	workflowIntentExplain            workflowIntentKind = "explain"
	workflowIntentDirectExecute      workflowIntentKind = "direct_execute"
	workflowIntentPlanOnly           workflowIntentKind = "plan_only"
	workflowIntentBroadReviewExecute workflowIntentKind = "broad_review_execute"
)

const (
	workflowIntentTimeout   = 12 * time.Second
	workflowIntentMaxTokens = 96
)

const workflowIntentClassifierPrompt = `You classify engineer requests for workflow routing.

Return ONLY JSON with this exact schema:
{"kind":"casual|explain|direct_execute|plan_only|broad_review_execute","confidence":0.0,"reason":"short reason"}

Classification rules:
- casual: greeting, thanks, chit-chat, or no real work request
- explain: explanation, analysis, or answer-only request with no execution
- direct_execute: narrow implementation request or targeted change
- plan_only: request for a plan, breakdown, or task list without execution
- broad_review_execute: review, audit, or improve a subsystem/project and then execute changes

Do not answer the user request. Do not use markdown fences. Do not call tools.`

type workflowIntentResult struct {
	Kind       workflowIntentKind `json:"kind"`
	Confidence float64            `json:"confidence"`
	Reason     string             `json:"reason"`
}

type workflowIntentAttempt struct {
	prov      provider.Provider
	modelID   string
	costModel provider.Model
}

func (tl *turnLoop) workflowIntentForTurn() *workflowIntentResult {
	if tl.intentChecked {
		return tl.workflowIntent
	}
	tl.intentChecked = true

	if tl.req == nil || tl.prov == nil || tl.modelID == "" {
		return nil
	}

	userText := initialUserText(tl.req.Messages)
	if strings.TrimSpace(userText) == "" {
		return nil
	}

	callID := "workflow-controller-intent"
	if tl.publish != nil {
		tl.publish(tl.req.SessionID, SSEEvent{
			Type: "tool_start",
			Data: SSEToolStartData{
				Tool:   "intent",
				Input:  userText,
				CallID: callID,
			},
		})
	}

	attempts := tl.workflowIntentAttempts()
	var errs []string
	for idx, attempt := range attempts {
		intent, usage, err := classifyWorkflowIntent(tl.ctx, attempt.prov, attempt.modelID, userText)
		if err == nil {
			if tl.sc != nil && usage != nil {
				tl.sc.Add(usage, attempt.costModel)
			}
			log.Printf(
				"workflow-intent: session=%s provider=%s model=%s kind=%s confidence=%.2f reason=%q",
				tl.req.SessionID,
				attempt.prov.ID(),
				attempt.modelID,
				intent.Kind,
				intent.Confidence,
				truncateForLog(intent.Reason, 160),
			)
			if tl.publish != nil {
				tl.publish(tl.req.SessionID, SSEEvent{
					Type: "tool_end",
					Data: SSEToolEndData{
						Tool:   "intent",
						Output: formatWorkflowIntentOutput(attempt.prov.ID(), attempt.modelID, intent),
						CallID: callID,
					},
				})
			}
			tl.workflowIntent = &intent
			return tl.workflowIntent
		}

		errMsg := fmt.Sprintf("classifier %s/%s failed: %s", attempt.prov.ID(), attempt.modelID, cleanErrorMessage(err.Error()))
		errs = append(errs, errMsg)
		log.Printf("workflow-intent: %s", errMsg)
		if idx < len(attempts)-1 {
			next := attempts[idx+1]
			log.Printf("workflow-intent: falling back to provider=%s model=%s for session %s", next.prov.ID(), next.modelID, tl.req.SessionID)
		}
	}

	if tl.publish != nil {
		errText := strings.Join(errs, "\n")
		errTextCopy := errText
		tl.publish(tl.req.SessionID, SSEEvent{
			Type: "tool_end",
			Data: SSEToolEndData{
				Tool:   "intent",
				Output: errText,
				Error:  &errTextCopy,
				CallID: callID,
			},
		})
	}
	return nil
}

func (tl *turnLoop) workflowIntentAttempts() []workflowIntentAttempt {
	attempts := []workflowIntentAttempt{{
		prov:      tl.prov,
		modelID:   tl.modelID,
		costModel: tl.requestCostModel,
	}}
	if tl.utility.prov == nil || tl.utility.modelID == "" {
		return attempts
	}
	if tl.utility.prov.ID() == tl.prov.ID() && tl.utility.modelID == tl.modelID {
		return attempts
	}
	utilityAttempt := workflowIntentAttempt{
		prov:    tl.utility.prov,
		modelID: tl.utility.modelID,
		costModel: provider.Model{
			CostInput:      tl.utility.costIn,
			CostOutput:     tl.utility.costOut,
			CostReasoning:  0,
			CostCacheRead:  0,
			CostCacheWrite: 0,
		},
	}
	return append([]workflowIntentAttempt{utilityAttempt}, attempts...)
}

func formatWorkflowIntentOutput(providerID, modelID string, intent workflowIntentResult) string {
	output := fmt.Sprintf("Selected workflow: %s", workflowIntentLabel(intent.Kind))
	if intent.Reason != "" {
		output += "\nReason: " + intent.Reason
	}
	output += fmt.Sprintf("\nModel: %s/%s", providerID, modelID)
	if intent.Confidence > 0 {
		output += fmt.Sprintf("\nConfidence: %.2f", intent.Confidence)
	}
	return output
}

func workflowIntentLabel(kind workflowIntentKind) string {
	switch kind {
	case workflowIntentCasual:
		return "No Workflow"
	case workflowIntentExplain:
		return "Explain Only"
	case workflowIntentDirectExecute:
		return "Direct Execute"
	case workflowIntentPlanOnly:
		return "Plan Only"
	case workflowIntentBroadReviewExecute:
		return "Review + Plan + Execute"
	default:
		return "Unknown"
	}
}

func classifyWorkflowIntent(ctx context.Context, prov provider.Provider, modelID, userText string) (workflowIntentResult, *provider.Usage, error) {
	ctx, cancel := context.WithTimeout(ctx, workflowIntentTimeout)
	defer cancel()

	temp := 0.0
	stream, err := prov.Chat(ctx, modelID, []provider.Message{{
		Role: "user",
		Parts: []provider.MessagePart{
			provider.TextPart{Text: strings.TrimSpace(userText)},
		},
	}}, provider.ChatOptions{
		SystemParts: []string{workflowIntentClassifierPrompt, "", ""},
		Temperature: &temp,
		MaxTokens:   workflowIntentMaxTokens,
	})
	if err != nil {
		return workflowIntentResult{}, nil, err
	}

	var (
		sb    strings.Builder
		usage *provider.Usage
	)
	for chunk := range stream {
		if chunk.Err != nil {
			return workflowIntentResult{}, usage, chunk.Err
		}
		sb.WriteString(chunk.Delta)
		if chunk.Usage != nil {
			usage = chunk.Usage
		}
	}

	intent, err := parseWorkflowIntentResult(sb.String())
	if err != nil {
		return workflowIntentResult{}, usage, err
	}
	return intent, usage, nil
}

func parseWorkflowIntentResult(raw string) (workflowIntentResult, error) {
	payload := strings.TrimSpace(raw)
	if strings.HasPrefix(payload, "```") {
		payload = strings.TrimPrefix(payload, "```json")
		payload = strings.TrimPrefix(payload, "```")
		payload = strings.TrimSpace(strings.TrimSuffix(payload, "```"))
	}

	intent, err := parseWorkflowIntentJSON(payload)
	if err == nil {
		return intent, nil
	}
	fallback, ferr := parseWorkflowIntentJSONLike(payload)
	if ferr == nil {
		return fallback, nil
	}
	prose, perr := parseWorkflowIntentText(payload)
	if perr == nil {
		return prose, nil
	}
	return workflowIntentResult{}, fmt.Errorf("%v; %v; %v", err, ferr, perr)
}

func parseWorkflowIntentJSON(payload string) (workflowIntentResult, error) {
	start := strings.Index(payload, "{")
	end := strings.LastIndex(payload, "}")
	if start < 0 || end < start {
		return workflowIntentResult{}, fmt.Errorf("no json object found in classifier output")
	}

	var intent workflowIntentResult
	if err := json.Unmarshal([]byte(payload[start:end+1]), &intent); err != nil {
		return workflowIntentResult{}, fmt.Errorf("invalid classifier json: %w", err)
	}
	return cleanWorkflowIntentResult(intent)
}

func parseWorkflowIntentJSONLike(payload string) (workflowIntentResult, error) {
	intent := workflowIntentResult{}
	if kinds := extractStructuredPlanKeyValues(payload, "kind"); len(kinds) > 0 {
		intent.Kind = workflowIntentKind(strings.TrimSpace(kinds[0]))
	}
	if reasons := extractStructuredPlanKeyValues(payload, "reason"); len(reasons) > 0 {
		intent.Reason = strings.TrimSpace(reasons[0])
	}
	if confidences := extractStructuredPlanKeyValues(payload, "confidence"); len(confidences) > 0 {
		value, err := strconv.ParseFloat(strings.TrimSpace(confidences[0]), 64)
		if err != nil {
			return workflowIntentResult{}, fmt.Errorf("invalid classifier confidence %q", confidences[0])
		}
		intent.Confidence = value
	}
	if intent.Kind == "" {
		return workflowIntentResult{}, fmt.Errorf("no classifier kind found in json-like output")
	}
	return cleanWorkflowIntentResult(intent)
}

func parseWorkflowIntentText(payload string) (workflowIntentResult, error) {
	lines := strings.Split(strings.ReplaceAll(payload, "\r\n", "\n"), "\n")
	intent := workflowIntentResult{}
	for _, line := range lines {
		trimmed := normalizeWorkflowIntentTextToken(line)
		if trimmed == "" {
			continue
		}
		if kind := parseWorkflowIntentLabeledKind(trimmed); kind != "" {
			intent.Kind = kind
			continue
		}
		if value, ok := parseWorkflowIntentLabeledFloat(trimmed, "confidence"); ok {
			intent.Confidence = value
			continue
		}
		if reason, ok := parseWorkflowIntentLabeledText(trimmed, "reason"); ok {
			intent.Reason = reason
			continue
		}
		if kind := normalizeWorkflowIntentKind(trimmed); kind != "" {
			intent.Kind = kind
		}
	}
	if intent.Kind == "" {
		if kind := findWorkflowIntentKindInText(payload); kind != "" {
			intent.Kind = kind
		}
	}
	if intent.Kind == "" {
		return workflowIntentResult{}, fmt.Errorf("no classifier kind found in text output")
	}
	if intent.Reason == "" {
		intent.Reason = strings.TrimSpace(payload)
		if normalizeWorkflowIntentTextToken(intent.Reason) == strings.TrimSpace(string(intent.Kind)) {
			intent.Reason = ""
		}
	}
	return cleanWorkflowIntentResult(intent)
}

func cleanWorkflowIntentResult(intent workflowIntentResult) (workflowIntentResult, error) {
	intent.Kind = workflowIntentKind(strings.TrimSpace(string(intent.Kind)))
	intent.Reason = strings.TrimSpace(intent.Reason)
	if intent.Confidence < 0 {
		intent.Confidence = 0
	} else if intent.Confidence > 1 && intent.Confidence <= 100 {
		intent.Confidence = intent.Confidence / 100
	} else if intent.Confidence > 1 {
		intent.Confidence = 1
	}
	switch intent.Kind {
	case workflowIntentCasual, workflowIntentExplain, workflowIntentDirectExecute, workflowIntentPlanOnly, workflowIntentBroadReviewExecute:
		return intent, nil
	default:
		return workflowIntentResult{}, fmt.Errorf("unknown classifier kind %q", intent.Kind)
	}
}

func normalizeWorkflowIntentKind(raw string) workflowIntentKind {
	switch strings.TrimSpace(strings.ToLower(raw)) {
	case string(workflowIntentCasual):
		return workflowIntentCasual
	case string(workflowIntentExplain):
		return workflowIntentExplain
	case string(workflowIntentDirectExecute):
		return workflowIntentDirectExecute
	case string(workflowIntentPlanOnly):
		return workflowIntentPlanOnly
	case string(workflowIntentBroadReviewExecute):
		return workflowIntentBroadReviewExecute
	default:
		return ""
	}
}

func normalizeWorkflowIntentTextToken(raw string) string {
	trimmed := strings.TrimSpace(raw)
	trimmed = strings.Trim(trimmed, "`\"'")
	trimmed = strings.TrimLeft(trimmed, "-*• \t")
	return strings.TrimSpace(trimmed)
}

func parseWorkflowIntentLabeledKind(line string) workflowIntentKind {
	for _, prefix := range []string{"kind:", "kind=", "intent:", "intent=", "workflow:", "workflow=", "classification:", "classification="} {
		if strings.HasPrefix(strings.ToLower(line), prefix) {
			return normalizeWorkflowIntentKind(strings.TrimSpace(line[len(prefix):]))
		}
	}
	return ""
}

func parseWorkflowIntentLabeledFloat(line, key string) (float64, bool) {
	for _, prefix := range []string{key + ":", key + "="} {
		if strings.HasPrefix(strings.ToLower(line), prefix) {
			value, err := strconv.ParseFloat(strings.TrimSpace(line[len(prefix):]), 64)
			if err != nil {
				return 0, false
			}
			return value, true
		}
	}
	return 0, false
}

func parseWorkflowIntentLabeledText(line, key string) (string, bool) {
	for _, prefix := range []string{key + ":", key + "="} {
		if strings.HasPrefix(strings.ToLower(line), prefix) {
			return strings.TrimSpace(line[len(prefix):]), true
		}
	}
	return "", false
}

func findWorkflowIntentKindInText(payload string) workflowIntentKind {
	lower := strings.ToLower(payload)
	var matched workflowIntentKind
	for _, kind := range []workflowIntentKind{
		workflowIntentCasual,
		workflowIntentExplain,
		workflowIntentDirectExecute,
		workflowIntentPlanOnly,
		workflowIntentBroadReviewExecute,
	} {
		if !strings.Contains(lower, string(kind)) {
			continue
		}
		if matched != "" {
			return ""
		}
		matched = kind
	}
	return matched
}
