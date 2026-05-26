package events

import (
	"errors"
	"strings"
)

const TypeSessionTitleUpdated Type = "session_title_updated"

type SessionTitleUpdatedPayload struct {
	Title string
}

func (SessionTitleUpdatedPayload) eventType() Type { return TypeSessionTitleUpdated }

func (p SessionTitleUpdatedPayload) validate() error {
	if strings.TrimSpace(p.Title) == "" {
		return errors.New("title is required")
	}
	return nil
}
