package config

import (
	"fmt"
	"os"
	"strings"
)

// Validate checks the config for invalid values and returns all errors found.
func (c *Config) Validate() error {
	var errs []string

	if c.Version > CurrentConfigVersion {
		errs = append(errs, fmt.Sprintf("config version %d is newer than supported version %d — please upgrade kodacode", c.Version, CurrentConfigVersion))
	}

	if v := c.Session.CompactionThreshold; v != nil && (*v < 0 || *v > 1) {
		errs = append(errs, fmt.Sprintf("session.compaction_threshold must be between 0 and 1, got %v", *v))
	}
	if v := c.Session.ContextLimit; v != nil && (*v < 0 || *v > 1) {
		errs = append(errs, fmt.Sprintf("session.context_limit must be between 0 and 1, got %v", *v))
	}
	if c.Session.CompactionThreshold != nil && c.Session.ContextLimit != nil && *c.Session.ContextLimit <= *c.Session.CompactionThreshold {
		errs = append(errs, fmt.Sprintf("session.context_limit (%v) must be greater than session.compaction_threshold (%v)", *c.Session.ContextLimit, *c.Session.CompactionThreshold))
	}
	if v := c.Session.BudgetWarn; v != 0 && (v < 0 || v > 1) {
		errs = append(errs, fmt.Sprintf("session.budget_warn must be between 0 and 1, got %v", v))
	}
	if v := c.Session.TotalBudgetWarn; v != 0 && (v < 0 || v > 1) {
		errs = append(errs, fmt.Sprintf("session.total_budget_warn must be between 0 and 1, got %v", v))
	}
	if c.Session.MaxRetries < 0 {
		errs = append(errs, fmt.Sprintf("session.max_retries must be non-negative, got %d", c.Session.MaxRetries))
	}
	if c.Session.ToolCallArgumentTimeout < 0 {
		errs = append(errs, fmt.Sprintf("session.tool_call_argument_timeout must be non-negative, got %d", c.Session.ToolCallArgumentTimeout))
	}
	if c.Session.EngineerReviewLimit < 0 {
		errs = append(errs, fmt.Sprintf("session.engineer_review_limit must be non-negative, got %d", c.Session.EngineerReviewLimit))
	}
	if c.Session.MaxSubagents < 0 {
		errs = append(errs, fmt.Sprintf("session.max_subagents must be non-negative, got %d", c.Session.MaxSubagents))
	}
	if c.Session.Budget < 0 {
		errs = append(errs, fmt.Sprintf("session.budget must be non-negative, got %v", c.Session.Budget))
	}
	if c.Session.TotalBudget < 0 {
		errs = append(errs, fmt.Sprintf("session.total_budget must be non-negative, got %v", c.Session.TotalBudget))
	}

	if v := c.Session.CompactionKeepTurns; v != nil && *v < 1 {
		errs = append(errs, fmt.Sprintf("session.compaction_keep_turns must be at least 1, got %d", *v))
	}

	for _, mc := range c.Session.Models {
		if v := mc.CompactionThreshold; v != nil && (*v < 0 || *v > 1) {
			errs = append(errs, fmt.Sprintf("session.models: compaction_threshold must be between 0 and 1, got %v", *v))
		}
		if v := mc.ContextLimit; v != nil && (*v < 0 || *v > 1) {
			errs = append(errs, fmt.Sprintf("session.models: context_limit must be between 0 and 1, got %v", *v))
		}
		if mc.ToolCallArgumentTimeout < 0 {
			errs = append(errs, fmt.Sprintf("session.models: tool_call_argument_timeout must be non-negative, got %d", mc.ToolCallArgumentTimeout))
		}
		if mc.CompactionThreshold != nil && mc.ContextLimit != nil && *mc.ContextLimit <= *mc.CompactionThreshold {
			errs = append(errs, fmt.Sprintf("session.models: context_limit (%v) must be greater than compaction_threshold (%v)", *mc.ContextLimit, *mc.CompactionThreshold))
		}
	}

	validThinkingTypes := map[string]bool{"": true, "adaptive": true, "enabled": true}
	for _, p := range c.Providers {
		if p.ID == "" {
			errs = append(errs, "provider id is required")
		}
		if !validThinkingTypes[p.ThinkingType] {
			errs = append(errs, fmt.Sprintf("provider %q: thinking_type must be \"adaptive\" or \"enabled\", got %q", p.ID, p.ThinkingType))
		}
	}

	if len(errs) == 0 {
		return nil
	}
	return fmt.Errorf("config validation:\n  - %s", strings.Join(errs, "\n  - "))
}

func hasEnvVarRef(s string) bool {
	return strings.Contains(s, "${")
}

// expandBracedEnvVars expands only ${VAR} references, leaving bare $VAR and
// other dollar signs untouched. This prevents accidental corruption of literal
// values that happen to contain '$'.
func expandBracedEnvVars(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); {
		if i+1 < len(s) && s[i] == '$' && s[i+1] == '{' {
			end := strings.IndexByte(s[i+2:], '}')
			if end >= 0 {
				name := s[i+2 : i+2+end]
				b.WriteString(os.Getenv(name))
				i = i + 2 + end + 1
				continue
			}
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
}
