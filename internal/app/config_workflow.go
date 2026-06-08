package app

import (
	"errors"
	"strings"

	"github.com/sageil/kodacode/internal/provider"
)

type WorkflowReviewMode string

const (
	WorkflowReviewOff    WorkflowReviewMode = "off"
	WorkflowReviewManual WorkflowReviewMode = "manual"
	WorkflowReviewAuto   WorkflowReviewMode = "auto"
)

type WorkflowConfig struct {
	ReviewMode       WorkflowReviewMode
	ReviewModelRoute provider.ModelRoute
	PlannerApproval  bool
}

func defaultWorkflowConfig() WorkflowConfig {
	return WorkflowConfig{
		ReviewMode: WorkflowReviewManual,
	}
}

func (c WorkflowConfig) Validate(parent Config) error {
	switch strings.TrimSpace(string(c.ReviewMode)) {
	case "", string(WorkflowReviewOff), string(WorkflowReviewManual), string(WorkflowReviewAuto):
	default:
		return errors.New("workflow review mode must be off, manual, or auto")
	}
	if hasConfiguredModelRoute(c.ReviewModelRoute) {
		if err := c.ReviewModelRoute.Validate(); err != nil {
			return err
		}
		if err := parent.validateModelRoute(c.ReviewModelRoute); err != nil {
			return err
		}
	}
	return nil
}
