package app

import (
	"errors"

	"github.com/sageil/kodacode/internal/events"
)

func turnFailureCodeForError(err error) events.TurnFailureCode {
	switch {
	case err == nil:
		return events.TurnFailureCodeUnknown
	case errors.Is(err, ErrTurnExceededProviderRequestLimit):
		return events.TurnFailureCodeProviderRequestLimit
	case errors.Is(err, ErrTurnStalledNoProgress):
		return events.TurnFailureCodeNoProgress
	default:
		var budgetErr BudgetExceededError
		if errors.As(err, &budgetErr) {
			return events.TurnFailureCodeBudgetExceeded
		}
		return events.TurnFailureCodeUnknown
	}
}
