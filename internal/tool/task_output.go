package tool

import (
	"encoding/json"
	"strings"
)

const taskWorkflowReminder = "finish child tasks before finishing the parent"

func taskListOutput(tasks []TaskRecord) string {
	if tasks == nil {
		tasks = []TaskRecord{}
	}
	currentTaskID := activeTaskID(tasks)
	payload, err := json.Marshal(struct {
		ActiveTaskID string       `json:"active_task_id,omitempty"`
		Tasks        []TaskRecord `json:"tasks"`
	}{
		ActiveTaskID: currentTaskID,
		Tasks:        tasks,
	})
	if err != nil {
		return `{"tasks":[]}`
	}
	return string(payload)
}

func taskRecordOutput(task TaskRecord) string {
	return taskRecordMessageOutput(task, "")
}

func taskRecordMessageOutput(task TaskRecord, message string) string {
	payload, err := json.Marshal(struct {
		Task    TaskRecord `json:"task"`
		Message string     `json:"message,omitempty"`
	}{
		Task:    task,
		Message: strings.TrimSpace(message),
	})
	if err != nil {
		return `{"task":{}}`
	}
	return string(payload)
}

func taskCreateOutput(result TaskCreateResult) string {
	if strings.TrimSpace(result.Task.ParentTaskID) != "" || result.ParentTask != nil {
		result.Message = taskWorkflowMessage(result.Message, taskWorkflowReminder)
	}
	payload, err := json.Marshal(result)
	if err != nil {
		return `{"task":{}}`
	}
	return string(payload)
}

func taskWorkflowMessage(parts ...string) string {
	msgs := make([]string, 0, len(parts))
	seen := make(map[string]struct{}, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if _, ok := seen[part]; ok {
			continue
		}
		seen[part] = struct{}{}
		msgs = append(msgs, part)
	}
	return strings.Join(msgs, "; ")
}

func activeTaskID(tasks []TaskRecord) string {
	if len(tasks) == 0 {
		return ""
	}
	parents := make(map[string]string, len(tasks))
	activeIDs := make([]string, 0, len(tasks))
	for _, task := range tasks {
		parents[task.TaskID] = task.ParentTaskID
		if task.Status == "in_progress" {
			activeIDs = append(activeIDs, task.TaskID)
		}
	}
	for _, candidateID := range activeIDs {
		deepest := true
		for _, otherID := range activeIDs {
			if otherID == candidateID {
				continue
			}
			if !taskRecordIsAncestor(parents, otherID, candidateID) {
				deepest = false
				break
			}
		}
		if deepest {
			return candidateID
		}
	}
	return ""
}

func taskRecordIsAncestor(parents map[string]string, ancestorID, taskID string) bool {
	ancestorID = strings.TrimSpace(ancestorID)
	taskID = strings.TrimSpace(taskID)
	if ancestorID == "" || taskID == "" {
		return false
	}
	visited := map[string]struct{}{}
	for currentID := taskID; currentID != ""; currentID = strings.TrimSpace(parents[currentID]) {
		if currentID == ancestorID {
			return true
		}
		if _, seen := visited[currentID]; seen {
			return false
		}
		visited[currentID] = struct{}{}
	}
	return false
}
