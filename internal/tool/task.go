package tool

import (
	"context"
	"fmt"
	"log"
	"regexp"
	"slices"
	"strings"
	"sync"

	"github.com/sageil/kodacode/v1/internal/repository"
)

// Task represents a single tracked work item.
type Task struct {
	ID                string `json:"id"`
	Title             string `json:"title"`
	Kind              string `json:"kind,omitempty"`
	Status            string `json:"status"`
	Notes             string `json:"notes,omitempty"`
	Progress          string `json:"progress,omitempty"`
	ReviewStatus      string `json:"reviewStatus,omitempty"`
	BlockReason       string `json:"blockReason,omitempty"`
	LastReviewSummary string `json:"lastReviewSummary,omitempty"`
	SortOrder         int    `json:"-"`
}

const (
	TaskKindImplementation = "implementation"
	TaskKindAnalysis       = "analysis"
	TaskKindReport         = "report"

	TaskReviewPass     = "pass"
	TaskReviewConcern  = "concern"
	TaskReviewFail     = "fail"
	TaskReviewAccepted = "accepted"

	TaskBlockReasonReviewCap      = "review_cap"
	TaskBlockReasonExecutionStall = "execution_stall"
)

// TaskStore holds in-memory task state with optional database persistence.
type TaskStore struct {
	mu    sync.RWMutex
	tasks map[string][]*Task // sessionID -> tasks
	seq   map[string]int     // sessionID -> next sequence number
	repo  repository.TaskRepo
}

// NewTaskStore creates a TaskStore. Pass nil for repo to disable persistence.
func NewTaskStore(repo repository.TaskRepo) *TaskStore {
	return &TaskStore{
		tasks: make(map[string][]*Task),
		seq:   make(map[string]int),
		repo:  repo,
	}
}

// LoadTasks loads tasks from the database into the in-memory store for a session.
// Call when resuming a session so the task panel shows persisted tasks.
func (s *TaskStore) LoadTasks(ctx context.Context, sessionID string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.repo == nil {
		return
	}
	if _, loaded := s.tasks[sessionID]; loaded {
		return
	}

	dbTasks, err := s.repo.ListBySession(ctx, sessionID)
	if err != nil {
		log.Printf("task: load from db failed for session %s: %v", sessionID, err)
		return
	}
	if len(dbTasks) == 0 {
		return
	}

	tasks := make([]*Task, len(dbTasks))
	maxSeq := 0
	for i, dt := range dbTasks {
		var seq int
		fmt.Sscanf(dt.ID, "task %d", &seq) //nolint:errcheck
		sortOrder := dt.SortOrder
		if sortOrder == 0 && seq > 0 {
			sortOrder = seq
		}
		tasks[i] = &Task{
			ID:                dt.ID,
			Title:             dt.Title,
			Kind:              normalizeTaskKind(dt.Kind),
			Status:            dt.Status,
			Notes:             dt.Notes,
			Progress:          dt.Progress,
			ReviewStatus:      normalizeTaskReviewStatus(dt.ReviewStatus),
			BlockReason:       normalizeTaskBlockReason(dt.BlockReason),
			LastReviewSummary: strings.TrimSpace(dt.LastReviewSummary),
			SortOrder:         sortOrder,
		}
		if seq > maxSeq {
			maxSeq = seq
		}
	}
	slices.SortStableFunc(tasks, func(a, b *Task) int { return a.SortOrder - b.SortOrder })
	s.tasks[sessionID] = tasks
	s.seq[sessionID] = maxSeq
	log.Printf("task: loaded %d tasks for session %s", len(tasks), sessionID)
}

// GetTasks returns the current tasks for a session.
func (s *TaskStore) GetTasks(sessionID string) []*Task {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return cloneTasks(s.tasks[sessionID])
}

// CleanupSession removes all in-memory task state for a session.
func (s *TaskStore) CleanupSession(sessionID string) {
	s.mu.Lock()
	delete(s.tasks, sessionID)
	delete(s.seq, sessionID)
	s.mu.Unlock()
}

// EnsureActiveTask returns the current in-progress task for a session, or
// promotes the first pending task to in_progress when no task is active.
func (s *TaskStore) EnsureActiveTask(ctx context.Context, sessionID string) (*Task, bool, error) {
	s.mu.Lock()
	tasks := s.tasks[sessionID]
	if active := activeTask(tasks); active != nil {
		cp := *active
		s.mu.Unlock()
		return &cp, false, nil
	}
	var target *Task
	for _, t := range tasks {
		if t != nil && t.Status == "pending" {
			t.Status = "in_progress"
			target = t
			break
		}
	}
	if target == nil {
		s.mu.Unlock()
		return nil, false, nil
	}
	updated := *target
	s.mu.Unlock()

	if err := s.persistUpdate(ctx, sessionID, &updated); err != nil {
		s.mu.Lock()
		if current := findTaskByID(s.tasks[sessionID], updated.ID); current != nil {
			current.Status = "pending"
		}
		s.mu.Unlock()
		return nil, false, err
	}
	return &updated, true, nil
}

// UpdateTask updates an existing task directly from controller code.
func (s *TaskStore) UpdateTask(ctx context.Context, sessionID, taskID, status, progress, notes string, replaceNotes bool) (*Task, error) {
	s.mu.Lock()
	target := resolveTaskReference(s.tasks[sessionID], taskID)
	if target == nil {
		s.mu.Unlock()
		return nil, fmt.Errorf("task: task %q not found", taskID)
	}
	before := *target
	if status != "" {
		target.Status = status
		if status != "blocked" {
			target.BlockReason = ""
		}
	}
	if progress != "" {
		target.Progress = progress
	}
	if notes != "" {
		if replaceNotes || strings.TrimSpace(target.Notes) == "" {
			target.Notes = notes
		} else if progress == "" {
			target.Progress = notes
		}
	}
	if replaceNotes && notes == "" {
		target.Notes = ""
	}
	if err := validateCompletedTaskState(target); err != nil {
		s.mu.Unlock()
		return nil, err
	}
	after := *target
	s.mu.Unlock()

	if err := s.persistUpdate(ctx, sessionID, &after); err != nil {
		s.mu.Lock()
		if current := findTaskByID(s.tasks[sessionID], before.ID); current != nil {
			*current = before
		}
		s.mu.Unlock()
		return nil, err
	}
	return &after, nil
}

// UpdateTaskWorkflowState updates controller-owned task workflow metadata.
func (s *TaskStore) UpdateTaskWorkflowState(ctx context.Context, sessionID, taskID, status, reviewStatus, blockReason, lastReviewSummary string) (*Task, error) {
	s.mu.Lock()
	target := resolveTaskReference(s.tasks[sessionID], taskID)
	if target == nil {
		s.mu.Unlock()
		return nil, fmt.Errorf("task: task %q not found", taskID)
	}
	before := *target
	if status != "" {
		target.Status = status
	}
	target.ReviewStatus = normalizeTaskReviewStatus(reviewStatus)
	target.BlockReason = normalizeTaskBlockReason(blockReason)
	if target.Status != "blocked" {
		target.BlockReason = ""
	}
	target.LastReviewSummary = strings.TrimSpace(lastReviewSummary)
	if err := validateCompletedTaskState(target); err != nil {
		s.mu.Unlock()
		return nil, err
	}
	after := *target
	s.mu.Unlock()

	if err := s.persistUpdate(ctx, sessionID, &after); err != nil {
		s.mu.Lock()
		if current := findTaskByID(s.tasks[sessionID], before.ID); current != nil {
			*current = before
		}
		s.mu.Unlock()
		return nil, err
	}
	return &after, nil
}

// CloneSession copies the tasks from one session onto another so branching
// preserves task state even before the new session is opened in the TUI.
func (s *TaskStore) CloneSession(ctx context.Context, fromSessionID, toSessionID string) error {
	if fromSessionID == "" || toSessionID == "" || fromSessionID == toSessionID {
		return nil
	}

	var cloned []*Task
	var seq int

	s.mu.RLock()
	if tasks, ok := s.tasks[fromSessionID]; ok {
		cloned = cloneTasks(tasks)
		seq = s.seq[fromSessionID]
	}
	s.mu.RUnlock()

	if cloned == nil && s.repo != nil {
		dbTasks, err := s.repo.ListBySession(ctx, fromSessionID)
		if err != nil {
			return err
		}
		if len(dbTasks) > 0 {
			cloned = make([]*Task, len(dbTasks))
			for i, dt := range dbTasks {
				cloned[i] = &Task{
					ID:                dt.ID,
					Title:             dt.Title,
					Kind:              normalizeTaskKind(dt.Kind),
					Status:            dt.Status,
					Notes:             dt.Notes,
					Progress:          dt.Progress,
					ReviewStatus:      normalizeTaskReviewStatus(dt.ReviewStatus),
					BlockReason:       normalizeTaskBlockReason(dt.BlockReason),
					LastReviewSummary: strings.TrimSpace(dt.LastReviewSummary),
					SortOrder:         dt.SortOrder,
				}
				if n := taskSeq(dt.ID, dt.SortOrder); n > seq {
					seq = n
				}
			}
		}
	}

	if len(cloned) == 0 {
		return nil
	}

	if s.repo != nil {
		for _, t := range cloned {
			if _, err := s.repo.Create(ctx, repository.Task{
				ID:                t.ID,
				SessionID:         toSessionID,
				Title:             t.Title,
				Kind:              normalizeTaskKind(t.Kind),
				Status:            t.Status,
				Notes:             t.Notes,
				Progress:          t.Progress,
				ReviewStatus:      normalizeTaskReviewStatus(t.ReviewStatus),
				BlockReason:       normalizeTaskBlockReason(t.BlockReason),
				LastReviewSummary: strings.TrimSpace(t.LastReviewSummary),
				SortOrder:         t.SortOrder,
			}); err != nil {
				return err
			}
		}
	}

	s.mu.Lock()
	s.tasks[toSessionID] = cloneTasks(cloned)
	if seq > s.seq[toSessionID] {
		s.seq[toSessionID] = seq
	}
	s.mu.Unlock()
	return nil
}

var taskParams = []byte(`{
	"type": "object",
	"properties": {
		"action": {"type": "string", "enum": ["create", "update", "list", "delete"], "description": "Task operation"},
		"id": {"type": "string", "description": "Task ID (for update/delete)"},
		"title": {"type": "string", "description": "Task title (for create)"},
		"kind": {"type": "string", "enum": ["implementation", "analysis", "report"], "description": "Optional task kind. Use implementation for code changes, analysis for audit/research tasks, and report for summary/report deliverables."},
		"status": {"type": "string", "enum": ["pending", "in_progress", "completed", "blocked"], "description": "Task status"},
		"notes": {"type": "string", "description": "Durable task definition details and acceptance criteria. On update, this only replaces the stored notes when replaceNotes is true or the task has no existing notes."},
		"progress": {"type": "string", "description": "Short transient progress or status summary for the task."},
		"replaceNotes": {"type": "boolean", "description": "Explicitly replace the stored task notes during update. Do not use for routine progress updates."}
	},
	"required": ["action"]
}`)

func NewTaskTool(store *TaskStore) *Tool {
	return &Tool{
		Name:        "task",
		ReadOnly:    false,
		Description: prompt("task"),
		Parameters:  taskParams,
		Execute:     store.executeTask,
	}
}

type taskArgs struct {
	Action       string `json:"action"`
	ID           string `json:"id"`
	Title        string `json:"title"`
	Kind         string `json:"kind"`
	Status       string `json:"status"`
	Notes        string `json:"notes"`
	Progress     string `json:"progress"`
	ReplaceNotes bool   `json:"replaceNotes"`
}

var taskIDMentionRE = regexp.MustCompile(`(?i)\btask\s+(\d+)\b`)

func (s *TaskStore) executeTask(ctx context.Context, ectx ExecutionContext, args []byte) (*Result, error) {
	var a taskArgs
	if err := flexUnmarshal(args, &a); err != nil {
		return ErrorResult(ErrCodeInvalidArgs, fmt.Sprintf("task: invalid arguments: %v", err), false), nil
	}

	sid := ectx.TaskSessionID
	if sid == "" {
		sid = ectx.SessionID
	}
	if sid == "" {
		sid = "_default"
	}

	switch a.Action {
	case "create":
		return s.taskCreate(ctx, sid, a)
	case "list":
		return s.taskList(sid)
	case "update":
		return s.taskUpdate(ctx, sid, a)
	case "delete":
		return s.taskDelete(ctx, sid, a)
	default:
		return ErrorResult(ErrCodeInvalidArgs,
			fmt.Sprintf("task: unknown action %q — valid actions are: create, update, list, delete. "+
				"Call this tool with {\"action\": \"create\", \"title\": \"...\"} to create a task.", a.Action),
			false), nil
	}
}

func (s *TaskStore) taskCreate(ctx context.Context, sid string, a taskArgs) (*Result, error) {
	if strings.TrimSpace(a.Title) == "" {
		return ErrorResult(ErrCodeInvalidArgs, "task: title is required for create", false), nil
	}
	t, created, err := s.CreateTaskWithKind(ctx, sid, a.Title, normalizeTaskKind(a.Kind), a.Status, a.Notes)
	if err != nil {
		return ErrorResult(ErrCodeInternal, fmt.Sprintf("task: failed to persist create: %v", err), true), nil
	}

	return &Result{
		Title:  "task: create",
		Output: createdTaskOutput(t, created),
		Metadata: map[string]any{
			"id":   t.ID,
			"kind": t.Kind,
		},
	}, nil
}

func (s *TaskStore) taskList(sid string) (*Result, error) {
	tasks := s.GetTasks(sid)

	if len(tasks) == 0 {
		return &Result{
			Title:  "task: list",
			Output: "No tasks.",
			Metadata: map[string]any{
				"count": 0,
			},
		}, nil
	}

	var sb strings.Builder
	for _, t := range tasks {
		icon := statusIcon(t.Status)
		fmt.Fprintf(&sb, "%s %s  %s\n", icon, t.ID, t.Title)
		if t.Notes != "" {
			for line := range strings.SplitSeq(t.Notes, "\n") {
				fmt.Fprintf(&sb, "     %s\n", line)
			}
		}
		if t.Progress != "" {
			fmt.Fprintf(&sb, "     Progress: %s\n", t.Progress)
		}
		if meta := TaskWorkflowStateSummary(t); meta != "" {
			fmt.Fprintf(&sb, "     State: %s\n", meta)
		}
		if detail := strings.TrimSpace(t.LastReviewSummary); detail != "" && ShouldShowReviewSummary(t) {
			fmt.Fprintf(&sb, "     Review: %s\n", detail)
		}
	}

	return &Result{
		Title:  "task: list",
		Output: strings.TrimRight(sb.String(), "\n"),
		Metadata: map[string]any{
			"count": len(tasks),
		},
	}, nil
}

func (s *TaskStore) taskUpdate(ctx context.Context, sid string, a taskArgs) (*Result, error) {
	if a.ID == "" {
		return ErrorResult(ErrCodeInvalidArgs, "task: id is required for update", false), nil
	}

	s.mu.Lock()
	tasks := s.tasks[sid]
	target, matchedByRef := resolveTaskForUpdate(tasks, a)
	if target == nil {
		s.mu.Unlock()
		return ErrorResult(ErrCodeNotFound, fmt.Sprintf("task: task %q not found. Available tasks: %s", a.ID, availableTaskRefs(tasks)), false), nil
	}
	before := *target
	if a.Status == "in_progress" {
		if active := activeTask(tasks); active != nil && active.ID != target.ID {
			s.mu.Unlock()
			return ErrorResult(
				ErrCodeInvalidArgs,
				fmt.Sprintf("task: %s (%s) is already in_progress. Mark it completed or blocked before starting %s (%s).", active.ID, active.Title, target.ID, target.Title),
				false,
			), nil
		}
	}
	if a.Status == "completed" {
		if active := activeTask(tasks); active != nil && active.ID != target.ID {
			s.mu.Unlock()
			return ErrorResult(
				ErrCodeInvalidArgs,
				fmt.Sprintf("task: %s (%s) is already in_progress. Mark it completed or blocked before completing %s (%s).", active.ID, active.Title, target.ID, target.Title),
				false,
			), nil
		}
		if target.Status != "in_progress" && target.Status != "completed" {
			s.mu.Unlock()
			return ErrorResult(
				ErrCodeInvalidArgs,
				fmt.Sprintf("task: %s (%s) must be in_progress before it can be marked completed.", target.ID, target.Title),
				false,
			), nil
		}
	}

	if a.Title != "" && !matchedByRef && !isTaskReference(tasks, a.Title, target.ID) {
		target.Title = a.Title
	}
	if a.Status != "" {
		target.Status = a.Status
		if a.Status != "blocked" {
			target.BlockReason = ""
		}
	}
	if a.Progress != "" {
		target.Progress = a.Progress
	}
	if a.Notes != "" {
		if a.ReplaceNotes || strings.TrimSpace(target.Notes) == "" {
			target.Notes = a.Notes
		} else if a.Progress == "" {
			target.Progress = a.Notes
		}
	}
	if a.ReplaceNotes && a.Notes == "" {
		target.Notes = ""
	}
	if err := validateCompletedTaskState(target); err != nil {
		s.mu.Unlock()
		return ErrorResult(ErrCodeInvalidArgs, err.Error(), false), nil
	}
	after := *target
	s.mu.Unlock()
	if err := s.persistUpdate(ctx, sid, &after); err != nil {
		s.mu.Lock()
		if current := findTaskByID(s.tasks[sid], before.ID); current != nil {
			*current = before
		}
		s.mu.Unlock()
		return ErrorResult(ErrCodeInternal, fmt.Sprintf("task: failed to persist update for %s: %v", before.ID, err), true), nil
	}

	return &Result{
		Title:  "task: update",
		Output: fmt.Sprintf("Updated task %s: %s [%s]", after.ID, after.Title, after.Status),
		Metadata: map[string]any{
			"id":       after.ID,
			"kind":     after.Kind,
			"title":    after.Title,
			"status":   after.Status,
			"progress": after.Progress,
		},
	}, nil
}

func (s *TaskStore) taskDelete(ctx context.Context, sid string, a taskArgs) (*Result, error) {
	if a.ID == "" {
		return ErrorResult(ErrCodeInvalidArgs, "task: id is required for delete", false), nil
	}

	s.mu.Lock()
	tasks := s.tasks[sid]
	target := resolveTaskReference(tasks, a.ID)
	if target == nil {
		s.mu.Unlock()
		return ErrorResult(ErrCodeNotFound, fmt.Sprintf("task: task %q not found. Available tasks: %s", a.ID, availableTaskRefs(tasks)), false), nil
	}
	for i, t := range tasks {
		if t.ID == target.ID {
			title := t.Title
			s.tasks[sid] = append(tasks[:i], tasks[i+1:]...)
			s.mu.Unlock()
			if err := s.persistDelete(ctx, sid, t.ID); err != nil {
				s.mu.Lock()
				current := s.tasks[sid]
				restored := append([]*Task(nil), current[:i]...)
				restored = append(restored, t)
				restored = append(restored, current[i:]...)
				s.tasks[sid] = restored
				s.mu.Unlock()
				return ErrorResult(ErrCodeInternal, fmt.Sprintf("task: failed to persist delete for %s: %v", t.ID, err), true), nil
			}

			return &Result{
				Title:  "task: delete",
				Output: fmt.Sprintf("Deleted task %s: %s", t.ID, title),
				Metadata: map[string]any{
					"id": t.ID,
				},
			}, nil
		}
	}
	s.mu.Unlock()

	return ErrorResult(ErrCodeNotFound, fmt.Sprintf("task: task %q not found. Available tasks: %s", a.ID, availableTaskRefs(tasks)), false), nil
}

func (s *TaskStore) persistCreate(ctx context.Context, sessionID string, t *Task) error {
	if s.repo == nil {
		return nil
	}
	if _, err := s.repo.Create(ctx, repository.Task{
		ID:                t.ID,
		SessionID:         sessionID,
		Title:             t.Title,
		Kind:              normalizeTaskKind(t.Kind),
		Status:            t.Status,
		Notes:             t.Notes,
		Progress:          t.Progress,
		ReviewStatus:      normalizeTaskReviewStatus(t.ReviewStatus),
		BlockReason:       normalizeTaskBlockReason(t.BlockReason),
		LastReviewSummary: strings.TrimSpace(t.LastReviewSummary),
		SortOrder:         t.SortOrder,
	}); err != nil {
		log.Printf("task: persist create failed: %v", err)
		return err
	}
	return nil
}

func (s *TaskStore) persistUpdate(ctx context.Context, sessionID string, t *Task) error {
	if s.repo == nil {
		return nil
	}
	if err := s.repo.Update(ctx, repository.Task{
		ID:                t.ID,
		SessionID:         sessionID,
		Title:             t.Title,
		Kind:              normalizeTaskKind(t.Kind),
		Status:            t.Status,
		Notes:             t.Notes,
		Progress:          t.Progress,
		ReviewStatus:      normalizeTaskReviewStatus(t.ReviewStatus),
		BlockReason:       normalizeTaskBlockReason(t.BlockReason),
		LastReviewSummary: strings.TrimSpace(t.LastReviewSummary),
	}); err != nil {
		log.Printf("task: persist update failed: %v", err)
		return err
	}
	return nil
}

func (s *TaskStore) persistDelete(ctx context.Context, sessionID, taskID string) error {
	if s.repo == nil {
		return nil
	}
	if err := s.repo.Delete(ctx, sessionID, taskID); err != nil {
		log.Printf("task: persist delete failed: %v", err)
		return err
	}
	return nil
}

func normalizeTaskID(id string) string {
	if !strings.HasPrefix(id, "task ") {
		return "task " + id
	}
	return id
}

func resolveTaskForUpdate(tasks []*Task, a taskArgs) (*Task, bool) {
	if t := resolveTaskReference(tasks, a.ID); t != nil {
		return t, false
	}
	if t := resolveTaskReference(tasks, a.Title); t != nil {
		return t, true
	}
	if t := resolveTaskReference(tasks, a.Notes); t != nil {
		return t, true
	}
	return nil, false
}

func resolveTaskReference(tasks []*Task, ref string) *Task {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return nil
	}

	if task := findTaskByID(tasks, normalizeTaskID(ref)); task != nil {
		return task
	}
	if task := findTaskByID(tasks, extractTaskIDMention(ref)); task != nil {
		return task
	}
	for _, t := range tasks {
		if strings.EqualFold(strings.TrimSpace(t.Title), ref) {
			return t
		}
	}
	trimmed := stripTaskRefPrefix(ref)
	if trimmed != ref {
		for _, t := range tasks {
			if strings.EqualFold(strings.TrimSpace(t.Title), trimmed) {
				return t
			}
		}
	}
	return nil
}

func findTaskByID(tasks []*Task, id string) *Task {
	if id == "" {
		return nil
	}
	for _, t := range tasks {
		if t.ID == id {
			return t
		}
	}
	return nil
}

func activeTask(tasks []*Task) *Task {
	for _, t := range tasks {
		if t != nil && t.Status == "in_progress" {
			return t
		}
	}
	return nil
}

func extractTaskIDMention(ref string) string {
	m := taskIDMentionRE.FindStringSubmatch(ref)
	if len(m) != 2 {
		return ""
	}
	return normalizeTaskID(m[1])
}

func stripTaskRefPrefix(ref string) string {
	if m := taskIDMentionRE.FindStringIndex(ref); m != nil && m[0] == 0 {
		ref = strings.TrimSpace(ref[m[1]:])
		ref = strings.TrimLeft(ref, ":- ")
	}
	return strings.TrimSpace(ref)
}

func isTaskReference(tasks []*Task, ref, targetID string) bool {
	t := resolveTaskReference(tasks, ref)
	return t != nil && t.ID == targetID
}

func availableTaskRefs(tasks []*Task) string {
	if len(tasks) == 0 {
		return "none"
	}
	refs := make([]string, 0, len(tasks))
	for _, t := range tasks {
		refs = append(refs, fmt.Sprintf("%s (%s)", t.ID, t.Title))
	}
	return strings.Join(refs, ", ")
}

func cloneTasks(tasks []*Task) []*Task {
	if len(tasks) == 0 {
		return nil
	}
	cloned := make([]*Task, len(tasks))
	for i, t := range tasks {
		if t == nil {
			continue
		}
		cp := *t
		cloned[i] = &cp
	}
	return cloned
}

// CreateTask inserts a task for sessionID or returns the existing task with the
// same title. It is used by both the model-facing task tool and internal
// controller code that needs deterministic task persistence.
func (s *TaskStore) CreateTask(ctx context.Context, sessionID, title, status, notes string) (*Task, bool, error) {
	return s.CreateTaskWithKind(ctx, sessionID, title, TaskKindImplementation, status, notes)
}

func (s *TaskStore) CreateTaskWithKind(ctx context.Context, sessionID, title, kind, status, notes string) (*Task, bool, error) {
	if strings.TrimSpace(title) == "" {
		return nil, false, fmt.Errorf("task: title is required for create")
	}
	if status == "" {
		status = "pending"
	}
	kind = normalizeTaskKind(kind)

	s.mu.Lock()
	for _, existing := range s.tasks[sessionID] {
		if strings.EqualFold(existing.Title, title) {
			cp := *existing
			s.mu.Unlock()
			return &cp, false, nil
		}
	}
	s.seq[sessionID]++
	seq := s.seq[sessionID]
	t := &Task{
		ID:        fmt.Sprintf("task %d", seq),
		Title:     title,
		Kind:      kind,
		Status:    status,
		Notes:     notes,
		Progress:  "",
		SortOrder: seq,
	}
	s.tasks[sessionID] = append(s.tasks[sessionID], t)
	cp := *t
	s.mu.Unlock()

	if err := s.persistCreate(ctx, sessionID, &cp); err != nil {
		s.mu.Lock()
		tasks := s.tasks[sessionID]
		for i, existing := range tasks {
			if existing != nil && existing.ID == t.ID {
				s.tasks[sessionID] = append(tasks[:i], tasks[i+1:]...)
				break
			}
		}
		s.mu.Unlock()
		return nil, false, err
	}
	return &cp, true, nil
}

func normalizeTaskKind(kind string) string {
	switch strings.TrimSpace(strings.ToLower(kind)) {
	case TaskKindAnalysis:
		return TaskKindAnalysis
	case TaskKindReport:
		return TaskKindReport
	default:
		return TaskKindImplementation
	}
}

func normalizeTaskReviewStatus(status string) string {
	switch strings.TrimSpace(strings.ToLower(status)) {
	case TaskReviewPass:
		return TaskReviewPass
	case TaskReviewConcern:
		return TaskReviewConcern
	case TaskReviewFail:
		return TaskReviewFail
	case TaskReviewAccepted:
		return TaskReviewAccepted
	default:
		return ""
	}
}

func normalizeTaskBlockReason(reason string) string {
	switch strings.TrimSpace(strings.ToLower(reason)) {
	case TaskBlockReasonReviewCap:
		return TaskBlockReasonReviewCap
	case TaskBlockReasonExecutionStall:
		return TaskBlockReasonExecutionStall
	default:
		return ""
	}
}

func validateCompletedTaskState(task *Task) error {
	if task == nil || task.Status != "completed" {
		return nil
	}
	switch normalizeTaskKind(task.Kind) {
	case TaskKindAnalysis, TaskKindReport:
		if strings.TrimSpace(task.Progress) == "" {
			return fmt.Errorf("task: %s tasks require a short progress summary before completion", normalizeTaskKind(task.Kind))
		}
	}
	return nil
}

func createdTaskOutput(t *Task, created bool) string {
	if created {
		return fmt.Sprintf("Created task %s: %s", t.ID, t.Title)
	}
	return fmt.Sprintf("Task already exists: %s (%s) [%s]", t.ID, t.Title, t.Status)
}

func taskSeq(id string, sortOrder int) int {
	var seq int
	fmt.Sscanf(id, "task %d", &seq) //nolint:errcheck
	if seq > 0 {
		return seq
	}
	if sortOrder > 0 {
		return sortOrder
	}
	return 0
}

func statusIcon(status string) string {
	switch status {
	case "in_progress":
		return "[>]"
	case "completed":
		return "[✓]"
	case "blocked":
		return "[!]"
	default:
		return "[ ]"
	}
}

// TaskWorkflowStateSummary returns a concise user-facing summary of the
// workflow-owned review/block state for a task.
func TaskWorkflowStateSummary(task *Task) string {
	if task == nil {
		return ""
	}
	parts := make([]string, 0, 2)
	if task.Status == "blocked" {
		switch normalizeTaskBlockReason(task.BlockReason) {
		case TaskBlockReasonReviewCap:
			parts = append(parts, "blocked: reviewer retry limit")
		case TaskBlockReasonExecutionStall:
			parts = append(parts, "blocked: execution stalled")
		default:
			parts = append(parts, "blocked")
		}
	}
	switch normalizeTaskReviewStatus(task.ReviewStatus) {
	case TaskReviewFail:
		parts = append(parts, "review: fail")
	case TaskReviewConcern:
		parts = append(parts, "review: concern")
	case TaskReviewAccepted:
		parts = append(parts, "review: accepted with unresolved findings")
	case TaskReviewPass:
		if task.Status != "completed" {
			parts = append(parts, "review: pass")
		}
	}
	return strings.Join(parts, "; ")
}

// ShouldShowReviewSummary reports whether the last review summary should be
// surfaced to the user for the current task state.
func ShouldShowReviewSummary(task *Task) bool {
	if task == nil {
		return false
	}
	if task.Status == "blocked" {
		return true
	}
	switch normalizeTaskReviewStatus(task.ReviewStatus) {
	case TaskReviewFail, TaskReviewConcern, TaskReviewAccepted:
		return true
	default:
		return false
	}
}
