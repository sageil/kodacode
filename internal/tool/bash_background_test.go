package tool

import (
	"testing"
	"time"
)

func TestShouldRemoveBackgroundTaskUsesCompletionTime(t *testing.T) {
	now := time.Now()
	task := &BashTask{
		StartTime: now.Add(-10 * time.Minute),
		done:      true,
	}
	task.completedAt = now.Add(-1 * time.Minute)

	if shouldRemoveBackgroundTask(now, task) {
		t.Fatal("completed task was removed based on start time instead of completion time")
	}
}

func TestShouldRemoveBackgroundTaskExpiresAfterCompletionTTL(t *testing.T) {
	now := time.Now()
	task := &BashTask{
		StartTime: now.Add(-10 * time.Minute),
		done:      true,
	}
	task.completedAt = now.Add(-(taskTTLAfterDone + time.Minute))

	if !shouldRemoveBackgroundTask(now, task) {
		t.Fatal("completed task was retained past the post-completion TTL")
	}
}

func TestShouldRemoveBackgroundTaskFallsBackToStartTime(t *testing.T) {
	now := time.Now()
	task := &BashTask{
		StartTime: now.Add(-(taskTTLAfterDone + time.Minute)),
		done:      true,
	}

	if !shouldRemoveBackgroundTask(now, task) {
		t.Fatal("legacy completed task without completedAt should fall back to start time")
	}
}
