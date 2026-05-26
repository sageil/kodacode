package events

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

const TypeSessionHistoryContinuationUpdated Type = "session_history_continuation_updated"

type CompactionScope string

const (
	CompactionScopeHistory CompactionScope = "history"
)

const (
	historyContinuationEventVersion    = 1
	historyContinuationArtifactVersion = 1
	historyContinuationRendererVersion = 1
)

const (
	HistoryDecisionStatusActive                      = "active"
	HistoryDecisionStatusSuperseded                  = "superseded"
	HistoryOpenThreadStatusPending                   = "pending"
	HistoryOpenThreadStatusBlocked                   = "blocked"
	HistoryOpenThreadStatusDeferred                  = "deferred"
	HistoryOpenThreadOwnerAgent                      = "agent"
	HistoryOpenThreadOwnerUser                       = "user"
	HistoryOpenThreadOwnerShared                     = "shared"
	HistoryPageInMatchKindAudit                      = "audit"
	HistoryPageInMatchKindUserWording                = "user_wording"
	HistoryPageInMatchKindToolOutput                 = "tool_output"
	HistoryPageInMatchKindToolError                  = "tool_error"
	HistoryPageInMatchKindToolCommand                = "tool_command"
	HistoryPageInMatchKindPathContext                = "path_context"
	HistoryVerificationKindToolResult                = "tool_result"
	HistoryVerificationKindRuntimeNote               = "runtime_note"
	HistoryVerificationKindTurnStatus                = "turn_status"
	HistoryContinuationUpdateReasonSemanticClosure   = "semantic_closure"
	HistoryContinuationUpdateReasonTokenPressure     = "token_pressure"
	HistoryContinuationUpdateReasonRebuild           = "rebuild"
	HistoryContinuationUpdateReasonCheckpointRestore = "checkpoint_restore"
	HistoryContinuationUpdateReasonManualRequest     = "manual_request"
)

type HistoryInputBudgetPayload struct {
	InputLimitTokens          int `json:"input_limit_tokens"`
	TriggerTokens             int `json:"trigger_tokens"`
	TargetTokens              int `json:"target_tokens"`
	EstimatedRequestTokens    int `json:"estimated_request_tokens"`
	ConsolidatedRequestTokens int `json:"consolidated_request_tokens"`
}

func (p HistoryInputBudgetPayload) validate() error {
	if p.InputLimitTokens <= 0 {
		return errors.New("input_limit_tokens must be > 0")
	}
	if p.TriggerTokens <= 0 {
		return errors.New("trigger_tokens must be > 0")
	}
	if p.TargetTokens <= 0 {
		return errors.New("target_tokens must be > 0")
	}
	if p.EstimatedRequestTokens < 0 {
		return errors.New("estimated_request_tokens must be >= 0")
	}
	if p.ConsolidatedRequestTokens < 0 {
		return errors.New("consolidated_request_tokens must be >= 0")
	}
	return nil
}

type HistoryContinuationAttribution struct {
	Model             string `json:"model,omitempty"`
	InputLimitSource  string `json:"input_limit_source,omitempty"`
	MeasurementSource string `json:"measurement_source,omitempty"`
	SummarySource     string `json:"summary_source,omitempty"`
	Algorithm         string `json:"algorithm,omitempty"`
	DurationMS        int64  `json:"duration_ms,omitempty"`
}

func (a HistoryContinuationAttribution) validate() error {
	if a.DurationMS < 0 {
		return errors.New("duration_ms must be >= 0")
	}
	return nil
}

type HistoryContinuationArtifact struct {
	SessionObjective  string                        `json:"session_objective,omitempty"`
	Constraints       []string                      `json:"constraints,omitempty"`
	SettledDecisions  []HistoryDecisionPayload      `json:"settled_decisions,omitempty"`
	CompletedEpisodes []HistoryEpisodePayload       `json:"completed_episodes,omitempty"`
	OpenThreads       []HistoryOpenThreadPayload    `json:"open_threads,omitempty"`
	WorkspaceFacts    []HistoryWorkspaceFactPayload `json:"workspace_facts,omitempty"`
	PageInHints       []HistoryPageInHintPayload    `json:"page_in_hints,omitempty"`
}

func (a HistoryContinuationArtifact) validate() error {
	if err := validateNonEmptyStrings("constraints", a.Constraints); err != nil {
		return err
	}
	for i, decision := range a.SettledDecisions {
		if err := decision.validate(); err != nil {
			return fmt.Errorf("settled_decisions[%d]: %w", i, err)
		}
	}
	for i, episode := range a.CompletedEpisodes {
		if err := episode.validate(); err != nil {
			return fmt.Errorf("completed_episodes[%d]: %w", i, err)
		}
	}
	for i, thread := range a.OpenThreads {
		if err := thread.validate(); err != nil {
			return fmt.Errorf("open_threads[%d]: %w", i, err)
		}
	}
	for i, fact := range a.WorkspaceFacts {
		if err := fact.validate(); err != nil {
			return fmt.Errorf("workspace_facts[%d]: %w", i, err)
		}
	}
	for i, hint := range a.PageInHints {
		if err := hint.validate(); err != nil {
			return fmt.Errorf("page_in_hints[%d]: %w", i, err)
		}
	}
	return nil
}

type HistoryDecisionPayload struct {
	Decision     string `json:"decision"`
	Rationale    string `json:"rationale,omitempty"`
	Status       string `json:"status"`
	SourceTurnID string `json:"source_turn_id"`
}

func (p HistoryDecisionPayload) validate() error {
	if strings.TrimSpace(p.Decision) == "" {
		return errors.New("decision is required")
	}
	if err := validateAllowedValue("status", p.Status, HistoryDecisionStatusActive, HistoryDecisionStatusSuperseded); err != nil {
		return err
	}
	if strings.TrimSpace(p.SourceTurnID) == "" {
		return errors.New("source_turn_id is required")
	}
	return nil
}

type HistoryEpisodePayload struct {
	EpisodeID     string                       `json:"episode_id"`
	Summary       string                       `json:"summary"`
	TouchedPaths  []string                     `json:"touched_paths,omitempty"`
	Verification  []HistoryVerificationPayload `json:"verification,omitempty"`
	SourceTurnIDs []string                     `json:"source_turn_ids"`
}

func (p HistoryEpisodePayload) validate() error {
	if strings.TrimSpace(p.EpisodeID) == "" {
		return errors.New("episode_id is required")
	}
	if strings.TrimSpace(p.Summary) == "" {
		return errors.New("summary is required")
	}
	if err := validateNonEmptyStrings("touched_paths", p.TouchedPaths); err != nil {
		return err
	}
	for i, verification := range p.Verification {
		if err := verification.validate(); err != nil {
			return fmt.Errorf("verification[%d]: %w", i, err)
		}
	}
	if err := validateSourceTurnIDs("source_turn_ids", p.SourceTurnIDs); err != nil {
		return err
	}
	return nil
}

type HistoryVerificationPayload struct {
	Kind      string `json:"kind"`
	Value     string `json:"value"`
	Succeeded bool   `json:"succeeded"`
}

func (p HistoryVerificationPayload) validate() error {
	if err := validateAllowedValue(
		"kind",
		p.Kind,
		HistoryVerificationKindToolResult,
		HistoryVerificationKindRuntimeNote,
		HistoryVerificationKindTurnStatus,
	); err != nil {
		return err
	}
	if strings.TrimSpace(p.Value) == "" {
		return errors.New("value is required")
	}
	return nil
}

type HistoryOpenThreadPayload struct {
	Item         string `json:"item"`
	Status       string `json:"status"`
	Blocking     bool   `json:"blocking,omitempty"`
	Owner        string `json:"owner,omitempty"`
	SourceTurnID string `json:"source_turn_id"`
}

func (p HistoryOpenThreadPayload) validate() error {
	if strings.TrimSpace(p.Item) == "" {
		return errors.New("item is required")
	}
	if err := validateAllowedValue(
		"status",
		p.Status,
		HistoryOpenThreadStatusPending,
		HistoryOpenThreadStatusBlocked,
		HistoryOpenThreadStatusDeferred,
	); err != nil {
		return err
	}
	if strings.TrimSpace(p.Owner) != "" {
		if err := validateAllowedValue(
			"owner",
			p.Owner,
			HistoryOpenThreadOwnerAgent,
			HistoryOpenThreadOwnerUser,
			HistoryOpenThreadOwnerShared,
		); err != nil {
			return err
		}
	}
	if strings.TrimSpace(p.SourceTurnID) == "" {
		return errors.New("source_turn_id is required")
	}
	return nil
}

type HistoryWorkspaceFactPayload struct {
	Path         string `json:"path"`
	Fact         string `json:"fact"`
	SourceTurnID string `json:"source_turn_id"`
}

func (p HistoryWorkspaceFactPayload) validate() error {
	if strings.TrimSpace(p.Path) == "" {
		return errors.New("path is required")
	}
	if strings.TrimSpace(p.Fact) == "" {
		return errors.New("fact is required")
	}
	if strings.TrimSpace(p.SourceTurnID) == "" {
		return errors.New("source_turn_id is required")
	}
	return nil
}

type HistoryPageInHintPayload struct {
	When          string   `json:"when"`
	MatchKinds    []string `json:"match_kinds,omitempty"`
	ToolNames     []string `json:"tool_names,omitempty"`
	Paths         []string `json:"paths,omitempty"`
	CallIDs       []string `json:"call_ids,omitempty"`
	SourceTurnIDs []string `json:"source_turn_ids"`
}

func (p HistoryPageInHintPayload) validate() error {
	if strings.TrimSpace(p.When) == "" {
		return errors.New("when is required")
	}
	for _, kind := range p.MatchKinds {
		if err := validateAllowedValue(
			"match_kinds",
			kind,
			HistoryPageInMatchKindAudit,
			HistoryPageInMatchKindUserWording,
			HistoryPageInMatchKindToolOutput,
			HistoryPageInMatchKindToolError,
			HistoryPageInMatchKindToolCommand,
			HistoryPageInMatchKindPathContext,
		); err != nil {
			return err
		}
	}
	if err := validateNonEmptyStrings("tool_names", p.ToolNames); err != nil {
		return err
	}
	if err := validateNonEmptyStrings("paths", p.Paths); err != nil {
		return err
	}
	if err := validateNonEmptyStrings("call_ids", p.CallIDs); err != nil {
		return err
	}
	if err := validateSourceTurnIDs("source_turn_ids", p.SourceTurnIDs); err != nil {
		return err
	}
	return nil
}

type ContextCompactionStartedPayload struct {
	Scope                  CompactionScope `json:"scope"`
	InputLimitTokens       int             `json:"input_limit_tokens"`
	TriggerTokens          int             `json:"trigger_tokens"`
	TargetTokens           int             `json:"target_tokens"`
	EstimatedRequestTokens int             `json:"estimated_request_tokens"`
}

func (ContextCompactionStartedPayload) eventType() Type { return TypeContextCompactionStarted }

func (p ContextCompactionStartedPayload) validate() error {
	if err := validateCompactionScope(p.Scope); err != nil {
		return err
	}
	if p.InputLimitTokens <= 0 {
		return errors.New("input_limit_tokens must be > 0")
	}
	if p.TriggerTokens <= 0 {
		return errors.New("trigger_tokens must be > 0")
	}
	if p.TargetTokens <= 0 {
		return errors.New("target_tokens must be > 0")
	}
	if p.EstimatedRequestTokens < 0 {
		return errors.New("estimated_request_tokens must be >= 0")
	}
	return nil
}

type ContextCompactionFailedPayload struct {
	Scope                  CompactionScope `json:"scope"`
	Reason                 string          `json:"reason"`
	Detail                 string          `json:"detail"`
	InputLimitTokens       int             `json:"input_limit_tokens"`
	TriggerTokens          int             `json:"trigger_tokens"`
	TargetTokens           int             `json:"target_tokens"`
	EstimatedRequestTokens int             `json:"estimated_request_tokens"`
}

func (ContextCompactionFailedPayload) eventType() Type { return TypeContextCompactionFailed }

func (p ContextCompactionFailedPayload) validate() error {
	if err := validateCompactionScope(p.Scope); err != nil {
		return err
	}
	if strings.TrimSpace(p.Reason) == "" {
		return errors.New("reason is required")
	}
	if strings.TrimSpace(p.Detail) == "" {
		return errors.New("detail is required")
	}
	if p.InputLimitTokens <= 0 {
		return errors.New("input_limit_tokens must be > 0")
	}
	if p.TriggerTokens <= 0 {
		return errors.New("trigger_tokens must be > 0")
	}
	if p.TargetTokens <= 0 {
		return errors.New("target_tokens must be > 0")
	}
	if p.EstimatedRequestTokens < 0 {
		return errors.New("estimated_request_tokens must be >= 0")
	}
	return nil
}

type SessionHistoryContinuationUpdatedPayload struct {
	EventVersion               int                            `json:"event_version"`
	ArtifactVersion            int                            `json:"artifact_version"`
	RendererVersion            int                            `json:"renderer_version"`
	FrontierTurnID             string                         `json:"frontier_turn_id,omitempty"`
	ConsolidatedTurnCount      int                            `json:"consolidated_turn_count"`
	NewlyConsolidatedTurnCount int                            `json:"newly_consolidated_turn_count"`
	UpdateReason               string                         `json:"update_reason"`
	ActivityText               string                         `json:"activity_text,omitempty"`
	Artifact                   HistoryContinuationArtifact    `json:"artifact"`
	RenderedSummary            string                         `json:"rendered_summary"`
	InputBudget                *HistoryInputBudgetPayload     `json:"input_budget,omitempty"`
	Attribution                HistoryContinuationAttribution `json:"attribution"`
}

func (SessionHistoryContinuationUpdatedPayload) eventType() Type {
	return TypeSessionHistoryContinuationUpdated
}

func (p SessionHistoryContinuationUpdatedPayload) validate() error {
	normalized := p.normalized()
	if normalized.EventVersion <= 0 {
		return errors.New("event_version must be > 0")
	}
	if normalized.ArtifactVersion <= 0 {
		return errors.New("artifact_version must be > 0")
	}
	if normalized.RendererVersion <= 0 {
		return errors.New("renderer_version must be > 0")
	}
	if normalized.ConsolidatedTurnCount <= 0 {
		return errors.New("consolidated_turn_count must be > 0")
	}
	if normalized.NewlyConsolidatedTurnCount < 0 {
		return errors.New("newly_consolidated_turn_count must be >= 0")
	}
	if normalized.NewlyConsolidatedTurnCount > normalized.ConsolidatedTurnCount {
		return errors.New("newly_consolidated_turn_count must be <= consolidated_turn_count")
	}
	if normalized.ConsolidatedTurnCount > 0 && strings.TrimSpace(normalized.FrontierTurnID) == "" {
		return errors.New("frontier_turn_id is required")
	}
	if err := validateAllowedValue(
		"update_reason",
		normalized.UpdateReason,
		HistoryContinuationUpdateReasonSemanticClosure,
		HistoryContinuationUpdateReasonTokenPressure,
		HistoryContinuationUpdateReasonRebuild,
		HistoryContinuationUpdateReasonCheckpointRestore,
		HistoryContinuationUpdateReasonManualRequest,
	); err != nil {
		return err
	}
	if strings.TrimSpace(normalized.RenderedSummary) == "" {
		return errors.New("rendered_summary is required")
	}
	if err := normalized.Artifact.validate(); err != nil {
		return fmt.Errorf("artifact: %w", err)
	}
	if normalized.InputBudget != nil {
		if err := normalized.InputBudget.validate(); err != nil {
			return fmt.Errorf("input_budget: %w", err)
		}
	}
	if err := normalized.Attribution.validate(); err != nil {
		return fmt.Errorf("attribution: %w", err)
	}
	return nil
}

func (p SessionHistoryContinuationUpdatedPayload) MarshalJSON() ([]byte, error) {
	type durableSessionHistoryContinuationUpdatedPayload struct {
		EventVersion               int                            `json:"event_version"`
		ArtifactVersion            int                            `json:"artifact_version"`
		RendererVersion            int                            `json:"renderer_version"`
		FrontierTurnID             string                         `json:"frontier_turn_id,omitempty"`
		ConsolidatedTurnCount      int                            `json:"consolidated_turn_count"`
		NewlyConsolidatedTurnCount int                            `json:"newly_consolidated_turn_count"`
		UpdateReason               string                         `json:"update_reason"`
		ActivityText               string                         `json:"activity_text,omitempty"`
		Artifact                   HistoryContinuationArtifact    `json:"artifact"`
		RenderedSummary            string                         `json:"rendered_summary"`
		InputBudget                *HistoryInputBudgetPayload     `json:"input_budget,omitempty"`
		Attribution                HistoryContinuationAttribution `json:"attribution"`
	}
	normalized := p.normalized()
	durable := durableSessionHistoryContinuationUpdatedPayload(normalized)
	return json.Marshal(durable)
}

func (p *SessionHistoryContinuationUpdatedPayload) UnmarshalJSON(data []byte) error {
	type durableSessionHistoryContinuationUpdatedPayload struct {
		EventVersion               int                            `json:"event_version"`
		ArtifactVersion            int                            `json:"artifact_version"`
		RendererVersion            int                            `json:"renderer_version"`
		FrontierTurnID             string                         `json:"frontier_turn_id,omitempty"`
		ConsolidatedTurnCount      int                            `json:"consolidated_turn_count"`
		NewlyConsolidatedTurnCount int                            `json:"newly_consolidated_turn_count"`
		UpdateReason               string                         `json:"update_reason"`
		ActivityText               string                         `json:"activity_text,omitempty"`
		Artifact                   HistoryContinuationArtifact    `json:"artifact"`
		RenderedSummary            string                         `json:"rendered_summary"`
		InputBudget                *HistoryInputBudgetPayload     `json:"input_budget,omitempty"`
		Attribution                HistoryContinuationAttribution `json:"attribution"`
	}
	var decoded durableSessionHistoryContinuationUpdatedPayload
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	payload := SessionHistoryContinuationUpdatedPayload(decoded)
	*p = payload.normalized()
	return nil
}

func (p SessionHistoryContinuationUpdatedPayload) normalized() SessionHistoryContinuationUpdatedPayload {
	out := p
	if out.EventVersion <= 0 {
		out.EventVersion = historyContinuationEventVersion
	}
	if out.ArtifactVersion <= 0 {
		out.ArtifactVersion = historyContinuationArtifactVersion
	}
	if out.RendererVersion <= 0 {
		out.RendererVersion = historyContinuationRendererVersion
	}
	out.FrontierTurnID = strings.TrimSpace(out.FrontierTurnID)
	out.UpdateReason = strings.TrimSpace(out.UpdateReason)
	if out.UpdateReason == "" {
		out.UpdateReason = HistoryContinuationUpdateReasonTokenPressure
	}
	out.ActivityText = strings.TrimSpace(out.ActivityText)
	out.RenderedSummary = strings.TrimSpace(out.RenderedSummary)
	if strings.TrimSpace(out.Attribution.Algorithm) == "" {
		out.Attribution.Algorithm = "session_history_continuation_v1"
	}
	return out
}

func validateCompactionScope(scope CompactionScope) error {
	switch scope {
	case CompactionScopeHistory:
		return nil
	default:
		return errors.New("scope must be history")
	}
}

func validateNonEmptyStrings(name string, values []string) error {
	for _, value := range values {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("%s must not contain empty values", name)
		}
	}
	return nil
}

func validateSourceTurnIDs(name string, values []string) error {
	if len(values) == 0 {
		return fmt.Errorf("%s must contain at least one value", name)
	}
	return validateNonEmptyStrings(name, values)
}

func validateAllowedValue(name string, value string, allowed ...string) error {
	value = strings.TrimSpace(value)
	for _, candidate := range allowed {
		if value == candidate {
			return nil
		}
	}
	return fmt.Errorf("%s has unknown value %q", name, value)
}
