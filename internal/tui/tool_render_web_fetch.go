package tui

import "encoding/json"

type webFetchToolViewInput struct {
	URL    string `json:"url"`
	Format string `json:"format"`
}

func parseWebFetchToolViewInput(raw string) (webFetchToolViewInput, bool) {
	var input webFetchToolViewInput
	if err := json.Unmarshal([]byte(raw), &input); err != nil {
		return webFetchToolViewInput{}, false
	}
	return input, true
}
