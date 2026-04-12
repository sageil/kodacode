package tool

import (
	"context"
	"fmt"
	"regexp"
	"strings"
)

type MemoryStore interface {
	Save(content string) error
	List() ([]MemoryEntry, error)
	Delete(id string) error
}

type MemoryEntry struct {
	ID      string
	Content string
}

var memoryIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

var memoryParams = []byte(`{
	"type": "object",
	"properties": {
		"action": {"type": "string", "enum": ["save", "list", "delete"], "description": "save to store a memory, list to view existing memories, delete to remove a memory by ID"},
		"content": {"type": "string", "description": "The memory content to save (required for save)"},
		"id": {"type": "string", "description": "The memory ID to delete (required for delete)"}
	},
	"required": ["action"]
}`)

func NewMemoryTool(store MemoryStore) *Tool {
	return &Tool{
		Name:        "memory",
		Description: "Save, list, or delete project memories that persist across sessions. Use this to remember important discoveries, architectural decisions, and non-obvious context.",
		Parameters:  memoryParams,
		Execute: func(ctx context.Context, ectx ExecutionContext, args []byte) (*Result, error) {
			return executeMemory(store, args)
		},
	}
}

func executeMemory(store MemoryStore, args []byte) (*Result, error) {
	var params struct {
		Action  string `json:"action"`
		Content string `json:"content"`
		ID      string `json:"id"`
	}
	if err := flexUnmarshal(args, &params); err != nil {
		return nil, err
	}

	switch params.Action {
	case "save":
		if strings.TrimSpace(params.Content) == "" {
			return ErrorResult(ErrCodeInvalidArgs, "content is required for save action", true), nil
		}
		if err := store.Save(params.Content); err != nil {
			return nil, fmt.Errorf("save memory: %w", err)
		}
		return &Result{Title: "memory save", Output: "Memory saved."}, nil

	case "list":
		entries, err := store.List()
		if err != nil {
			return nil, fmt.Errorf("list memories: %w", err)
		}
		if len(entries) == 0 {
			return &Result{Title: "memory list", Output: "No memories saved."}, nil
		}
		var sb strings.Builder
		for _, e := range entries {
			fmt.Fprintf(&sb, "## %s\n%s\n\n", e.ID, e.Content)
		}
		return &Result{Title: "memory list", Output: strings.TrimSpace(sb.String())}, nil

	case "delete":
		id := strings.TrimSpace(params.ID)
		if id == "" {
			return ErrorResult(ErrCodeInvalidArgs, "id is required for delete action", true), nil
		}
		if !memoryIDPattern.MatchString(id) || strings.Contains(id, "..") || strings.ContainsAny(id, `/\`) {
			return ErrorResult(ErrCodeInvalidArgs, "id must be a bare memory id, not a path", false), nil
		}
		if err := store.Delete(id); err != nil {
			if strings.Contains(err.Error(), "invalid memory id") || strings.Contains(err.Error(), "memory id is required") {
				return ErrorResult(ErrCodeInvalidArgs, err.Error(), false), nil
			}
			return ErrorResult(ErrCodeNotFound, err.Error(), false), nil
		}
		return &Result{Title: "memory delete", Output: fmt.Sprintf("Memory %s deleted.", id)}, nil

	default:
		return ErrorResult(ErrCodeInvalidArgs, fmt.Sprintf("unknown action %q, use save, list, or delete", params.Action), true), nil
	}
}
