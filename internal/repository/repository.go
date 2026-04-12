// Package repository defines the data access interfaces for kodacode.
// All storage implementations must satisfy these interfaces.
package repository

import (
	"context"
	"errors"
	"time"
)

// ErrNotFound is returned by repository methods when the requested record
// does not exist.
var ErrNotFound = errors.New("not found")

// Session represents a conversation session.
type Session struct {
	ID                   string
	Title                string
	AgentID              string
	ModelID              string
	ParentID             string // non-empty for branched sessions
	BranchPointMessageID string // message ID at which the branch was created
	Ephemeral            bool
	TotalCost            float64
	TotalInputTokens     int
	TotalOutputTokens    int
	LastInputTokens      int
	WorkflowState        string
	CreatedAt            time.Time
	UpdatedAt            time.Time
}

// Message represents a single message within a session.
// Content is stored in MessagePart rows — Message holds identity and metadata only.
type Message struct {
	ID                 string
	SessionID          string
	Role               string // "user" | "assistant"
	CompactionParentID string // non-empty only on compaction summary messages
	Summary            bool   // true when this message is a compaction summary
	CreatedAt          time.Time
	UpdatedAt          time.Time
	Parts              []MessagePart // cache of parts for this message (in-memory only, not persisted)
}

// MessagePart is a typed content unit within a message.
type MessagePart struct {
	ID          string
	MessageID   string
	SessionID   string
	Type        string     // "text" | "tool_call" | "tool_result" | "reasoning" | "file"
	Content     string     // JSON payload — unmarshal with message.UnmarshalContent
	Synthetic   bool       // true for system-injected parts (not user-visible in UI)
	CompactedAt *time.Time // non-nil when this tool_result output has been pruned
	CreatedAt   time.Time
}

// SessionRepo defines CRUD operations for sessions.
type SessionRepo interface {
	// Create inserts a new session and returns the created Session with
	// server-assigned ID and timestamps.
	Create(ctx context.Context, s Session) (Session, error)

	// Get retrieves a single session by ID.
	// Returns ErrNotFound if the session does not exist.
	Get(ctx context.Context, id string) (Session, error)

	// List returns all sessions ordered by updated_at descending.
	List(ctx context.Context) ([]Session, error)

	// Update persists changes to Title, AgentID, ModelID, and UpdatedAt.
	Update(ctx context.Context, s Session) error

	UpdateWorkflow(ctx context.Context, id, workflowState string) error

	// Delete removes a session and all its messages.
	Delete(ctx context.Context, id string) error

	DeleteEphemeral(ctx context.Context) (int, error)

	// UpdateCost persists accumulated token usage and cost for a session.
	UpdateCost(ctx context.Context, id string, inputTokens, outputTokens, lastInputTokens int, totalCost float64) error
}

type TurnOperation struct {
	SessionID       string
	OperationID     string
	State           string
	Active          bool
	CancelRequested bool
	Error           string
	StartedAt       time.Time
	UpdatedAt       time.Time
	FinishedAt      time.Time
}

type TurnOperationRepo interface {
	SaveTurnOperation(ctx context.Context, op TurnOperation) error
	GetTurnOperation(ctx context.Context, sessionID, operationID string) (TurnOperation, error)
	LatestTurnOperation(ctx context.Context, sessionID string) (TurnOperation, error)
}

// MessageRepo defines CRUD operations for messages and their parts.
type MessageRepo interface {
	Create(ctx context.Context, m Message) (Message, error)
	CreateWithParts(ctx context.Context, m Message, parts []MessagePart) (Message, error)
	Get(ctx context.Context, id string) (Message, error)
	ListBySession(ctx context.Context, sessionID string) ([]Message, error)
	DeleteBySession(ctx context.Context, sessionID string) error

	// ListMessagesWithParts bulk fetches messages with their parts in a single operation.
	// The Parts field in each returned Message will be populated, avoiding N+1 queries.
	ListMessagesWithParts(ctx context.Context, sessionID string) ([]Message, error)

	CreatePart(ctx context.Context, p MessagePart) (MessagePart, error)
	ListPartsByMessage(ctx context.Context, messageID string) ([]MessagePart, error)
	ListPartsBySession(ctx context.Context, sessionID string) ([]MessagePart, error)
	UpdatePart(ctx context.Context, p MessagePart) error
	BatchUpdateParts(ctx context.Context, parts []MessagePart) error
	DeletePart(ctx context.Context, partID string) error
	DeletePartsBySession(ctx context.Context, sessionID string) error
}

// Task represents a tracked work item within a session.
type Task struct {
	ID                string
	SessionID         string
	Title             string
	Kind              string
	Status            string
	Notes             string
	Progress          string
	ReviewStatus      string
	BlockReason       string
	LastReviewSummary string
	SortOrder         int
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

// TaskRepo defines CRUD operations for session-scoped tasks.
type TaskRepo interface {
	Create(ctx context.Context, t Task) (Task, error)
	Update(ctx context.Context, t Task) error
	Delete(ctx context.Context, sessionID, taskID string) error
	ListBySession(ctx context.Context, sessionID string) ([]Task, error)
	DeleteBySession(ctx context.Context, sessionID string) error
}

type TraceEntry struct {
	SessionID string
	TurnIndex int
	Data      string // JSON-encoded []StepTrace
	CreatedAt time.Time
}

type TraceRepo interface {
	Save(ctx context.Context, sessionID string, turnIndex int, data string) error
	ListBySession(ctx context.Context, sessionID string) ([]TraceEntry, error)
	DeleteBySession(ctx context.Context, sessionID string) error
}

type AttachmentRefDelta struct {
	StorageKey string
	MimeType   string
	Size       int64
	Delta      int
}

type AttachmentBlob struct {
	StorageKey string
	MimeType   string
	Size       int64
	RefCount   int
	UpdatedAt  time.Time
}

type AttachmentFileRef struct {
	StorageKey string
	MimeType   string
	Size       int64
}

type AttachmentRepo interface {
	ApplyDeltas(ctx context.Context, deltas []AttachmentRefDelta) ([]AttachmentBlob, error)
	List(ctx context.Context) ([]AttachmentBlob, error)
	Delete(ctx context.Context, storageKeys []string) error
	Reconcile(ctx context.Context) error
}

// SettingsRepo defines key/value storage for application settings.
type SettingsRepo interface {
	// Get returns the value for key.
	// Returns ErrNotFound if the key does not exist.
	Get(ctx context.Context, key string) (string, error)

	// Set upserts a key/value pair.
	Set(ctx context.Context, key, value string) error
}
