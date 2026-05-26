package events

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

type TurnDonePayload struct{}

func (TurnDonePayload) eventType() Type { return TypeTurnDone }

func (TurnDonePayload) validate() error { return nil }

type TurnRetryScheduledPayload struct {
	Message     string
	Attempt     int
	MaxAttempts int
	RetryAt     time.Time
}

func (TurnRetryScheduledPayload) eventType() Type { return TypeTurnRetryScheduled }

func (p TurnRetryScheduledPayload) validate() error {
	if strings.TrimSpace(p.Message) == "" {
		return errors.New("message is required")
	}
	if p.Attempt <= 0 {
		return errors.New("attempt must be > 0")
	}
	if p.MaxAttempts < p.Attempt {
		return errors.New("max_attempts must be >= attempt")
	}
	if p.RetryAt.IsZero() {
		return errors.New("retry_at is required")
	}
	return nil
}

type TurnCanceledPayload struct {
	Message string
}

func (TurnCanceledPayload) eventType() Type { return TypeTurnCanceled }

func (p TurnCanceledPayload) validate() error {
	if strings.TrimSpace(p.Message) == "" {
		return errors.New("message is required")
	}
	return nil
}

type TurnErrorPayload struct {
	Message   string
	Retryable bool
	Code      TurnFailureCode
}

func (TurnErrorPayload) eventType() Type { return TypeTurnError }

func (p TurnErrorPayload) validate() error {
	if strings.TrimSpace(p.Message) == "" {
		return errors.New("message is required")
	}
	if err := p.Code.validate(); err != nil {
		return err
	}
	return nil
}

type TurnFailureCode string

const (
	TurnFailureCodeUnknown              TurnFailureCode = ""
	TurnFailureCodeProviderRequestLimit TurnFailureCode = "provider_request_limit"
	TurnFailureCodeNoProgress           TurnFailureCode = "no_progress"
	TurnFailureCodeBudgetExceeded       TurnFailureCode = "budget_exceeded"
)

func (c TurnFailureCode) validate() error {
	switch c {
	case TurnFailureCodeUnknown, TurnFailureCodeProviderRequestLimit, TurnFailureCodeNoProgress, TurnFailureCodeBudgetExceeded:
		return nil
	default:
		return fmt.Errorf("turn failure code %q is invalid", c)
	}
}
