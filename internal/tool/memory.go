package tool

import (
	"context"
	"encoding/json"
	"unicode/utf8"
)

const MemoryToolName = "memory"
const memoryListPreviewMaxChars = 240

type MemoryTool struct{}

func NewMemoryTool() MemoryTool {
	return MemoryTool{}
}

func (MemoryTool) Definition() Definition {
	return Definition{
		Name:                MemoryToolName,
		Description:         "Save, list, or delete explicit project memory entries that persist across sessions. Use this for saved architectural decisions, project facts, and non-obvious context worth carrying forward. Saved content must stay at or below 2000 characters.",
		ProviderDescription: "Save, list, or delete saved project memory entries. Saved content must stay at or below 2000 characters.",
		InputSchema:         json.RawMessage(`{"type":"object","properties":{"action":{"type":"string","enum":["save","list","delete"],"description":"Memory operation to perform."},"content":{"type":["string","null"],"description":"Memory content to save. Keep saved content at or below 2000 characters. Use null or omit this field for list or delete."},"id":{"type":["string","null"],"description":"Existing memory id to delete. Use null or omit this field for save or list."}},"required":["action"],"additionalProperties":false}`),
		ArgumentExamples:    []string{`{"action":"list","content":null,"id":null}`},
	}
}

func (MemoryTool) Execute(_ context.Context, ectx ExecutionContext, args json.RawMessage) (Result, error) {
	manager, err := ectx.Memories()
	if err != nil {
		return Result{}, err
	}
	input, err := parseMemoryInput(args)
	if err != nil {
		return Result{}, err
	}

	switch input.Action {
	case memoryActionSave:
		record, err := manager.SaveMemory(MemorySaveRequest{Content: input.Content})
		if err != nil {
			return Result{}, err
		}
		return Result{Output: memoryRecordOutput(record)}, nil
	case memoryActionList:
		records, err := manager.ListMemories()
		if err != nil {
			return Result{}, err
		}
		return Result{Output: memoryListOutput(records)}, nil
	case memoryActionDelete:
		if err := manager.DeleteMemory(MemoryDeleteRequest{ID: input.ID}); err != nil {
			return Result{}, err
		}
		return Result{Output: memoryDeleteOutput(input.ID)}, nil
	default:
		return Result{}, ErrMemoryActionInvalid
	}
}

func memoryRecordOutput(record MemoryRecord) string {
	payload, err := json.Marshal(struct {
		Memory MemoryRecord `json:"memory"`
	}{Memory: record})
	if err != nil {
		return `{"memory":{}}`
	}
	return string(payload)
}

func memoryListOutput(records []MemoryRecord) string {
	if records == nil {
		records = []MemoryRecord{}
	}
	type memoryListItem struct {
		ID        string `json:"id"`
		Path      string `json:"path,omitempty"`
		Content   string `json:"content"`
		UpdatedAt string `json:"updated_at,omitempty"`
		Truncated bool   `json:"truncated,omitempty"`
	}
	items := make([]memoryListItem, 0, len(records))
	for _, record := range records {
		content, truncated := truncateMemoryPreview(record.Content, memoryListPreviewMaxChars)
		items = append(items, memoryListItem{
			ID:        record.ID,
			Path:      record.Path,
			Content:   content,
			UpdatedAt: record.UpdatedAt,
			Truncated: truncated,
		})
	}
	payload, err := json.Marshal(struct {
		Memories []memoryListItem `json:"memories"`
	}{Memories: items})
	if err != nil {
		return `{"memories":[]}`
	}
	return string(payload)
}

func memoryDeleteOutput(id string) string {
	payload, err := json.Marshal(struct {
		Deleted string `json:"deleted"`
	}{Deleted: id})
	if err != nil {
		return `{"deleted":""}`
	}
	return string(payload)
}

func truncateMemoryPreview(content string, maxChars int) (string, bool) {
	if maxChars <= 0 || content == "" {
		return "", content != ""
	}
	if utf8.RuneCountInString(content) <= maxChars {
		return content, false
	}
	runes := []rune(content)
	if maxChars == 1 {
		return string(runes[:1]), true
	}
	return string(runes[:maxChars-1]) + "…", true
}
