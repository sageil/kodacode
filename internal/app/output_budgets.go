package app

import "fmt"

const (
	defaultOutputBudgetSessionTitle      = 128
	defaultOutputBudgetUtilityText       = 2048
	defaultOutputBudgetReview            = 4096
	defaultOutputBudgetAgentTurn         = 8192
	defaultOutputBudgetAgentTurnThinking = 16000
	defaultOutputBudgetWorkspaceCompress = 1200
	defaultOutputBudgetSessionCompaction = 2000
)

type OutputBudgetsConfig struct {
	SessionTitle      int
	UtilityText       int
	Review            int
	AgentTurn         int
	AgentTurnThinking int
	WorkspaceCompress int
	SessionCompaction int
}

func defaultOutputBudgets() OutputBudgetsConfig {
	return OutputBudgetsConfig{
		SessionTitle:      defaultOutputBudgetSessionTitle,
		UtilityText:       defaultOutputBudgetUtilityText,
		Review:            defaultOutputBudgetReview,
		AgentTurn:         defaultOutputBudgetAgentTurn,
		AgentTurnThinking: defaultOutputBudgetAgentTurnThinking,
		WorkspaceCompress: defaultOutputBudgetWorkspaceCompress,
		SessionCompaction: defaultOutputBudgetSessionCompaction,
	}
}

func (c OutputBudgetsConfig) Effective() OutputBudgetsConfig {
	defaults := defaultOutputBudgets()
	if c.SessionTitle <= 0 {
		c.SessionTitle = defaults.SessionTitle
	}
	if c.UtilityText <= 0 {
		c.UtilityText = defaults.UtilityText
	}
	if c.Review <= 0 {
		c.Review = defaults.Review
	}
	if c.AgentTurn <= 0 {
		c.AgentTurn = defaults.AgentTurn
	}
	if c.AgentTurnThinking <= 0 {
		c.AgentTurnThinking = defaults.AgentTurnThinking
	}
	if c.WorkspaceCompress <= 0 {
		c.WorkspaceCompress = defaults.WorkspaceCompress
	}
	if c.SessionCompaction <= 0 {
		c.SessionCompaction = defaults.SessionCompaction
	}
	return c
}

func (c OutputBudgetsConfig) Validate() error {
	if c.SessionTitle < 0 {
		return fmt.Errorf("output_budgets.session_title must be non-negative, got %d", c.SessionTitle)
	}
	if c.UtilityText < 0 {
		return fmt.Errorf("output_budgets.utility_text must be non-negative, got %d", c.UtilityText)
	}
	if c.Review < 0 {
		return fmt.Errorf("output_budgets.review must be non-negative, got %d", c.Review)
	}
	if c.AgentTurn < 0 {
		return fmt.Errorf("output_budgets.agent_turn must be non-negative, got %d", c.AgentTurn)
	}
	if c.AgentTurnThinking < 0 {
		return fmt.Errorf("output_budgets.agent_turn_thinking must be non-negative, got %d", c.AgentTurnThinking)
	}
	if c.WorkspaceCompress < 0 {
		return fmt.Errorf("output_budgets.workspace_compress must be non-negative, got %d", c.WorkspaceCompress)
	}
	if c.SessionCompaction < 0 {
		return fmt.Errorf("output_budgets.session_compaction must be non-negative, got %d", c.SessionCompaction)
	}
	return nil
}
