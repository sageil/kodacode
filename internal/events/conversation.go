package events

import (
	"errors"
	"strings"
)

const (
	TypeUserMessage   Type = "user_message"
	TypeContextPruned Type = "context_pruned"
)

type UserMessagePayload struct {
	Content     string
	Attachments []UserAttachmentPayload `json:"attachments,omitempty"`
}

type UserAttachmentPayload struct {
	Name     string `json:"name"`
	MIMEType string `json:"mime_type"`
	DataURL  string `json:"data_url"`
}

func (UserMessagePayload) eventType() Type { return TypeUserMessage }

func (p UserMessagePayload) validate() error {
	if strings.TrimSpace(p.Content) == "" && len(p.Attachments) == 0 {
		return errors.New("content or attachments is required")
	}
	for _, attachment := range p.Attachments {
		if err := attachment.validate(); err != nil {
			return err
		}
	}
	return nil
}

func (p UserAttachmentPayload) validate() error {
	if strings.TrimSpace(p.Name) == "" {
		return errors.New("name is required")
	}
	if strings.TrimSpace(p.MIMEType) == "" {
		return errors.New("mime_type is required")
	}
	prefix := "data:" + strings.TrimSpace(p.MIMEType) + ";base64,"
	if !strings.HasPrefix(strings.TrimSpace(p.DataURL), prefix) {
		return errors.New("data_url must be a base64 data URL matching mime_type")
	}
	return nil
}

type ContextPrunedPayload struct {
	PriorTurns          int
	PriorInputBytes     int
	RawPriorTurns       int
	RawInputBytes       int
	CompactedPriorTurns int
	CompactedInputBytes int
	OmittedPriorTurns   int
	OmittedInputBytes   int
}

func (ContextPrunedPayload) eventType() Type { return TypeContextPruned }

func (p ContextPrunedPayload) validate() error {
	if p.PriorTurns < 0 {
		return errors.New("prior_turns must be >= 0")
	}
	if p.PriorInputBytes < 0 {
		return errors.New("prior_input_bytes must be >= 0")
	}
	if p.RawPriorTurns < 0 {
		return errors.New("raw_prior_turns must be >= 0")
	}
	if p.RawInputBytes < 0 {
		return errors.New("raw_input_bytes must be >= 0")
	}
	if p.CompactedPriorTurns < 0 {
		return errors.New("compacted_prior_turns must be >= 0")
	}
	if p.CompactedInputBytes < 0 {
		return errors.New("compacted_input_bytes must be >= 0")
	}
	if p.OmittedPriorTurns < 0 {
		return errors.New("omitted_prior_turns must be >= 0")
	}
	if p.OmittedInputBytes < 0 {
		return errors.New("omitted_input_bytes must be >= 0")
	}
	return nil
}
