package app

import (
	"context"
	"strings"
)

const derivedTurnSourceMaxChars = 12000

func (r *Runtime) runDerivedSessionTurn(ctx context.Context, input runExistingTurnInput) (RunSessionResult, error) {
	input.HistoryMode = turnHistoryModeCurrentTurnOnly
	return r.runExistingSessionTurn(ctx, input)
}

func boundedDerivedTurnSource(sourceText string) string {
	return boundedDerivedTurnText(sourceText, derivedTurnSourceMaxChars)
}

func boundedDerivedTurnText(text string, maxChars int) string {
	text = strings.TrimSpace(text)
	if maxChars <= 0 {
		return ""
	}
	runes := []rune(text)
	if len(runes) <= maxChars {
		return text
	}
	headLen := maxChars / 2
	tailLen := maxChars - headLen
	return strings.TrimSpace(string(runes[:headLen])) +
		"\n\n[...derived turn context truncated...]\n\n" +
		strings.TrimSpace(string(runes[len(runes)-tailLen:]))
}
