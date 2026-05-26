package events

import "strings"

func renderUserTranscriptText(payload UserMessagePayload) string {
	body := strings.TrimSpace(payload.Content)
	if len(payload.Attachments) == 0 {
		return body
	}
	lines := make([]string, 0, len(payload.Attachments)+1)
	if body != "" {
		lines = append(lines, body)
	}
	for _, attachment := range payload.Attachments {
		name := strings.TrimSpace(attachment.Name)
		if name == "" {
			name = "attachment"
		}
		lines = append(lines, "[Attached image: "+name+"]")
	}
	return strings.Join(lines, "\n")
}
