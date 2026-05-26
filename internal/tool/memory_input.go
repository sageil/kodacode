package tool

import (
	"encoding/json"
	"errors"
	"strings"
)

var (
	ErrMemoryActionInvalid   = errors.New("action must be save, list, or delete")
	ErrMemoryContentRequired = errors.New("content is required for save")
	ErrMemoryIDRequired      = errors.New("id is required for delete")
)

type memoryAction string

const (
	memoryActionSave   memoryAction = "save"
	memoryActionList   memoryAction = "list"
	memoryActionDelete memoryAction = "delete"
)

type memoryInput struct {
	Action  memoryAction
	Content string
	ID      string
}

func parseMemoryInput(args json.RawMessage) (_ memoryInput, err error) {
	defer func() {
		err = normalizeToolInputError(MemoryToolName, err)
	}()
	var raw struct {
		Action  string  `json:"action"`
		Content *string `json:"content"`
		ID      *string `json:"id"`
	}
	if err := DecodeArgs(MemoryToolName, args, &raw); err != nil {
		return memoryInput{}, err
	}

	input := memoryInput{
		Action:  memoryAction(strings.TrimSpace(raw.Action)),
		Content: strings.TrimSpace(memoryString(raw.Content)),
		ID:      strings.TrimSpace(memoryString(raw.ID)),
	}
	switch input.Action {
	case memoryActionSave:
		if input.Content == "" {
			return memoryInput{}, ErrMemoryContentRequired
		}
	case memoryActionList:
	case memoryActionDelete:
		if input.ID == "" {
			return memoryInput{}, ErrMemoryIDRequired
		}
	default:
		return memoryInput{}, ErrMemoryActionInvalid
	}
	return input, nil
}

func memoryString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
