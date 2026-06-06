package app

import (
	"strings"

	"github.com/sageil/kodacode/internal/prompt"
)

type ResponseStyle string

const (
	ResponseStyleDefault ResponseStyle = "default"
	ResponseStyleTerse   ResponseStyle = "terse"
)

func normalizeResponseStyle(style ResponseStyle) ResponseStyle {
	switch strings.ToLower(strings.TrimSpace(string(style))) {
	case "", string(ResponseStyleDefault):
		return ResponseStyleDefault
	case string(ResponseStyleTerse):
		return ResponseStyleTerse
	default:
		return ""
	}
}

func validResponseStyle(style ResponseStyle) bool {
	return normalizeResponseStyle(style) != ""
}

func responseStylePromptFragment(style ResponseStyle) (prompt.Fragment, bool) {
	if normalizeResponseStyle(style) != ResponseStyleTerse {
		return prompt.Fragment{}, false
	}
	return prompt.Fragment{
		Kind:      prompt.KindRuntime,
		Source:    prompt.SourceRuntime,
		Stability: prompt.StabilityStable,
		Layer:     "response-style",
		Key:       "response-style",
		Label:     "response-style",
		Content: strings.Join([]string{
			"Response style: terse.",
			"- Keep ordinary model replies brief and direct.",
			"- Do not shorten safety, permission, destructive-action, or ambiguity clarifications.",
		}, "\n"),
	}, true
}
