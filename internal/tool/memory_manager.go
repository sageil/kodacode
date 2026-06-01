package tool

import "errors"

var ErrMemoryManagerRequired = errors.New("memory manager is required")

const MemoryContentMaxChars = 2000

type MemoryRecord struct {
	ID        string `json:"id,omitempty"`
	Path      string `json:"path,omitempty"`
	Content   string `json:"content,omitempty"`
	UpdatedAt string `json:"updated_at,omitempty"`
}

type MemorySaveRequest struct {
	Content string
}

type MemoryDeleteRequest struct {
	ID string
}

type MemoryManager interface {
	SaveMemory(MemorySaveRequest) (MemoryRecord, error)
	ListMemories() ([]MemoryRecord, error)
	DeleteMemory(MemoryDeleteRequest) error
}
