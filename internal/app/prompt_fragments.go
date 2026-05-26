package app

import (
	"strings"

	"github.com/sageil/kodacode/internal/prompt"
)

func dedupePromptFragments(fragments []prompt.Fragment) []prompt.Fragment {
	if len(fragments) == 0 {
		return nil
	}

	out := make([]prompt.Fragment, 0, len(fragments))
	seenStableKeys := make(map[string]struct{}, len(fragments))

	for i := len(fragments) - 1; i >= 0; i-- {
		fragment := fragments[i]
		if fragment.Stability == prompt.StabilityStable {
			key := strings.TrimSpace(fragment.Key)
			if key != "" {
				if _, ok := seenStableKeys[key]; ok {
					continue
				}
				seenStableKeys[key] = struct{}{}
			}
		}
		out = append(out, fragment)
	}

	for left, right := 0, len(out)-1; left < right; left, right = left+1, right-1 {
		out[left], out[right] = out[right], out[left]
	}
	return out
}
