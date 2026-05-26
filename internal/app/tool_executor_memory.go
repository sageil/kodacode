package app

import (
	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/tool"
)

func (e *ToolExecutor) SetMemoryService(service *MemoryService) {
	e.memory = service
}

func (e *ToolExecutor) toolMemoryManager(state events.SessionState) tool.MemoryManager {
	if e.memory == nil {
		return nil
	}
	return e.memory.Store(state.WorkspaceRoot)
}
