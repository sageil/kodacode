package sqlite_test

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/sageil/kodacode/v1/internal/repository"
	"github.com/sageil/kodacode/v1/internal/repository/sqlite"
)

// openTestDB opens a temp-file SQLite database, runs migrations, and
// registers t.Cleanup to close it. Returns a *sql.DB so callers can construct
// individual repos.
func openTestDB(t *testing.T) *sql.DB {
	t.Helper()

	// Write to a temp file so the driver can use WAL mode (in-memory does not
	// support WAL with some versions of the driver).
	f, err := os.CreateTemp("", "kodacode-test-*.db")
	if err != nil {
		t.Fatalf("create temp db file: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("close temp db file: %v", err)
	}
	t.Cleanup(func() { _ = os.Remove(f.Name()) })

	db, err := sqlite.Open(f.Name())
	if err != nil {
		t.Fatalf("sqlite.Open(%q): %v", f.Name(), err)
	}
	t.Cleanup(func() { _ = db.Close() })

	return db
}

// ---- Session tests ---------------------------------------------------------

func TestSessionRepo_Create(t *testing.T) {
	db := openTestDB(t)
	sessions := sqlite.NewSessionRepo(db)
	ctx := context.Background()

	got, err := sessions.Create(ctx, repository.Session{
		Title:   "first session",
		AgentID: "builder",
		ModelID: "openai/gpt-4o",
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if got.ID == "" {
		t.Errorf("Create() ID is empty")
	}
	if got.Title != "first session" {
		t.Errorf("Create() Title = %q, want %q", got.Title, "first session")
	}
	if got.CreatedAt.IsZero() {
		t.Errorf("Create() CreatedAt is zero")
	}
	if got.UpdatedAt.IsZero() {
		t.Errorf("Create() UpdatedAt is zero")
	}
}

func TestSessionRepo_Get(t *testing.T) {
	db := openTestDB(t)
	sessions := sqlite.NewSessionRepo(db)
	ctx := context.Background()

	created, err := sessions.Create(ctx, repository.Session{Title: "my session"})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	got, err := sessions.Get(ctx, created.ID)
	if err != nil {
		t.Fatalf("Get(%q) error = %v", created.ID, err)
	}
	if got.ID != created.ID {
		t.Errorf("Get() ID = %q, want %q", got.ID, created.ID)
	}
	if got.Title != created.Title {
		t.Errorf("Get() Title = %q, want %q", got.Title, created.Title)
	}
}

func TestSessionRepo_Get_NotFound(t *testing.T) {
	db := openTestDB(t)
	sessions := sqlite.NewSessionRepo(db)
	ctx := context.Background()

	_, err := sessions.Get(ctx, "non-existent-id")
	if !errors.Is(err, repository.ErrNotFound) {
		t.Errorf("Get(non-existent) error = %v, want ErrNotFound", err)
	}
}

func TestSessionRepo_List(t *testing.T) {
	db := openTestDB(t)
	sessions := sqlite.NewSessionRepo(db)
	ctx := context.Background()

	_, err := sessions.Create(ctx, repository.Session{Title: "alpha"})
	if err != nil {
		t.Fatalf("Create alpha: %v", err)
	}
	_, err = sessions.Create(ctx, repository.Session{Title: "beta"})
	if err != nil {
		t.Fatalf("Create beta: %v", err)
	}

	list, err := sessions.List(ctx)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(list) != 2 {
		t.Errorf("List() returned %d sessions, want 2", len(list))
	}
}

func TestSessionRepo_List_Empty(t *testing.T) {
	db := openTestDB(t)
	sessions := sqlite.NewSessionRepo(db)
	ctx := context.Background()

	list, err := sessions.List(ctx)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if list != nil {
		t.Errorf("List() on empty DB = %v, want nil slice", list)
	}
}

func TestSessionRepo_Update(t *testing.T) {
	db := openTestDB(t)
	sessions := sqlite.NewSessionRepo(db)
	ctx := context.Background()

	s, err := sessions.Create(ctx, repository.Session{Title: "original"})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	s.Title = "updated"
	if err := sessions.Update(ctx, s); err != nil {
		t.Fatalf("Update() error = %v", err)
	}

	got, err := sessions.Get(ctx, s.ID)
	if err != nil {
		t.Fatalf("Get() after update error = %v", err)
	}
	if got.Title != "updated" {
		t.Errorf("Get() after Update Title = %q, want %q", got.Title, "updated")
	}
}

func TestSessionRepo_Update_NotFound(t *testing.T) {
	db := openTestDB(t)
	sessions := sqlite.NewSessionRepo(db)
	ctx := context.Background()

	err := sessions.Update(ctx, repository.Session{ID: "ghost"})
	if !errors.Is(err, repository.ErrNotFound) {
		t.Errorf("Update(non-existent) error = %v, want ErrNotFound", err)
	}
}

func TestSessionRepo_Delete(t *testing.T) {
	db := openTestDB(t)
	sessions := sqlite.NewSessionRepo(db)
	ctx := context.Background()

	s, err := sessions.Create(ctx, repository.Session{Title: "to delete"})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	if err := sessions.Delete(ctx, s.ID); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	_, err = sessions.Get(ctx, s.ID)
	if !errors.Is(err, repository.ErrNotFound) {
		t.Errorf("Get() after Delete error = %v, want ErrNotFound", err)
	}
}

func TestSessionRepo_Delete_NotFound(t *testing.T) {
	db := openTestDB(t)
	sessions := sqlite.NewSessionRepo(db)
	ctx := context.Background()

	err := sessions.Delete(ctx, "ghost")
	if !errors.Is(err, repository.ErrNotFound) {
		t.Errorf("Delete(non-existent) error = %v, want ErrNotFound", err)
	}
}

func TestSessionRepo_DeleteEphemeral(t *testing.T) {
	db := openTestDB(t)
	sessions := sqlite.NewSessionRepo(db)
	ctx := context.Background()

	_, err := sessions.Create(ctx, repository.Session{Title: "keeper", AgentID: "a", ModelID: "m"})
	if err != nil {
		t.Fatalf("Create keeper: %v", err)
	}
	_, err = sessions.Create(ctx, repository.Session{AgentID: "a", ModelID: "m", Ephemeral: true})
	if err != nil {
		t.Fatalf("Create ephemeral: %v", err)
	}

	n, err := sessions.DeleteEphemeral(ctx)
	if err != nil {
		t.Fatalf("DeleteEphemeral() error = %v", err)
	}
	if n != 1 {
		t.Errorf("DeleteEphemeral() deleted %d, want 1", n)
	}

	list, err := sessions.List(ctx)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("List() returned %d sessions, want 1", len(list))
	}
	if list[0].Title != "keeper" {
		t.Errorf("surviving session Title = %q, want %q", list[0].Title, "keeper")
	}
}

func TestSessionRepo_TurnOperationsPersistAndCascade(t *testing.T) {
	db := openTestDB(t)
	sessions := sqlite.NewSessionRepo(db)
	turns, ok := sessions.(repository.TurnOperationRepo)
	if !ok {
		t.Fatal("session repo does not implement TurnOperationRepo")
	}
	ctx := context.Background()

	sess, err := sessions.Create(ctx, repository.Session{Title: "ops", AgentID: "a", ModelID: "m"})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	started := time.Now().UTC().Add(-2 * time.Minute).Truncate(time.Microsecond)
	running := repository.TurnOperation{
		SessionID:   sess.ID,
		OperationID: "op-1",
		State:       "running",
		Active:      true,
		StartedAt:   started,
		UpdatedAt:   started,
	}
	if err := turns.SaveTurnOperation(ctx, running); err != nil {
		t.Fatalf("SaveTurnOperation(running) error = %v", err)
	}

	done := running
	done.State = "succeeded"
	done.Active = false
	done.UpdatedAt = started.Add(time.Minute)
	done.FinishedAt = started.Add(time.Minute)
	if err := turns.SaveTurnOperation(ctx, done); err != nil {
		t.Fatalf("SaveTurnOperation(done) error = %v", err)
	}

	got, err := turns.GetTurnOperation(ctx, sess.ID, "op-1")
	if err != nil {
		t.Fatalf("GetTurnOperation() error = %v", err)
	}
	if got.State != "succeeded" || got.Active {
		t.Fatalf("GetTurnOperation() = %+v, want terminal persisted state", got)
	}

	latest, err := turns.LatestTurnOperation(ctx, sess.ID)
	if err != nil {
		t.Fatalf("LatestTurnOperation() error = %v", err)
	}
	if latest.OperationID != "op-1" || latest.State != "succeeded" {
		t.Fatalf("LatestTurnOperation() = %+v, want op-1 succeeded", latest)
	}

	if err := sessions.Delete(ctx, sess.ID); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if _, err := turns.GetTurnOperation(ctx, sess.ID, "op-1"); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("GetTurnOperation() after session delete error = %v, want ErrNotFound", err)
	}
}

// ---- Message tests ---------------------------------------------------------

func TestMessageRepo_Create(t *testing.T) {
	db := openTestDB(t)
	sessions := sqlite.NewSessionRepo(db)
	messages := sqlite.NewMessageRepo(db)
	ctx := context.Background()

	sess, err := sessions.Create(ctx, repository.Session{Title: "sess"})
	if err != nil {
		t.Fatalf("Create session: %v", err)
	}

	got, err := messages.Create(ctx, repository.Message{
		SessionID: sess.ID,
		Role:      "user",
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if got.ID == "" {
		t.Errorf("Create() ID is empty")
	}
	if got.CreatedAt.IsZero() {
		t.Errorf("Create() CreatedAt is zero")
	}
}

func TestMessageRepo_Get(t *testing.T) {
	db := openTestDB(t)
	sessions := sqlite.NewSessionRepo(db)
	messages := sqlite.NewMessageRepo(db)
	ctx := context.Background()

	sess, _ := sessions.Create(ctx, repository.Session{Title: "sess"})
	created, err := messages.Create(ctx, repository.Message{
		SessionID: sess.ID,
		Role:      "user",
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	got, err := messages.Get(ctx, created.ID)
	if err != nil {
		t.Fatalf("Get(%q) error = %v", created.ID, err)
	}
	if got.ID != created.ID {
		t.Errorf("Get() ID = %q, want %q", got.ID, created.ID)
	}
	if got.Role != created.Role {
		t.Errorf("Get() Role = %q, want %q", got.Role, created.Role)
	}
}

func TestMessageRepo_Get_NotFound(t *testing.T) {
	db := openTestDB(t)
	messages := sqlite.NewMessageRepo(db)
	ctx := context.Background()

	_, err := messages.Get(ctx, "ghost")
	if !errors.Is(err, repository.ErrNotFound) {
		t.Errorf("Get(non-existent) error = %v, want ErrNotFound", err)
	}
}

func TestMessageRepo_ListBySession(t *testing.T) {
	db := openTestDB(t)
	sessions := sqlite.NewSessionRepo(db)
	messages := sqlite.NewMessageRepo(db)
	ctx := context.Background()

	sess, _ := sessions.Create(ctx, repository.Session{Title: "chat"})

	roles := []string{"user", "assistant", "user"}
	for i, role := range roles {
		_, err := messages.Create(ctx, repository.Message{
			SessionID: sess.ID,
			Role:      role,
		})
		if err != nil {
			t.Fatalf("Create message %d: %v", i, err)
		}
	}

	list, err := messages.ListBySession(ctx, sess.ID)
	if err != nil {
		t.Fatalf("ListBySession() error = %v", err)
	}
	if len(list) != 3 {
		t.Errorf("ListBySession() = %d messages, want 3", len(list))
	}
	// Should be ordered ascending by created_at.
	if list[0].Role != "user" {
		t.Errorf("ListBySession()[0].Role = %q, want %q", list[0].Role, "user")
	}
}

func TestMessageRepo_ListBySession_Empty(t *testing.T) {
	db := openTestDB(t)
	sessions := sqlite.NewSessionRepo(db)
	messages := sqlite.NewMessageRepo(db)
	ctx := context.Background()

	sess, _ := sessions.Create(ctx, repository.Session{Title: "empty"})

	list, err := messages.ListBySession(ctx, sess.ID)
	if err != nil {
		t.Fatalf("ListBySession() error = %v", err)
	}
	if list != nil {
		t.Errorf("ListBySession() on empty session = %v, want nil", list)
	}
}

func TestMessageRepo_DeleteBySession(t *testing.T) {
	db := openTestDB(t)
	sessions := sqlite.NewSessionRepo(db)
	messages := sqlite.NewMessageRepo(db)
	ctx := context.Background()

	sess, _ := sessions.Create(ctx, repository.Session{Title: "doomed"})
	_, _ = messages.Create(ctx, repository.Message{SessionID: sess.ID, Role: "user"})

	if err := messages.DeleteBySession(ctx, sess.ID); err != nil {
		t.Fatalf("DeleteBySession() error = %v", err)
	}

	list, err := messages.ListBySession(ctx, sess.ID)
	if err != nil {
		t.Fatalf("ListBySession() after delete error = %v", err)
	}
	if len(list) != 0 {
		t.Errorf("ListBySession() after delete = %d messages, want 0", len(list))
	}
}

func TestSessionRepo_Delete_CascadesMessages(t *testing.T) {
	db := openTestDB(t)
	sessions := sqlite.NewSessionRepo(db)
	messages := sqlite.NewMessageRepo(db)
	ctx := context.Background()

	sess, _ := sessions.Create(ctx, repository.Session{Title: "cascade"})
	_, _ = messages.Create(ctx, repository.Message{SessionID: sess.ID, Role: "user"})

	if err := sessions.Delete(ctx, sess.ID); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	// The foreign-key ON DELETE CASCADE must have removed the messages too.
	list, err := messages.ListBySession(ctx, sess.ID)
	if err != nil {
		t.Fatalf("ListBySession() after cascade error = %v", err)
	}
	if len(list) != 0 {
		t.Errorf("ListBySession() after cascade delete = %d messages, want 0", len(list))
	}
}

func TestMessageRepo_UpdatedAt(t *testing.T) {
	db := openTestDB(t)
	sessions := sqlite.NewSessionRepo(db)
	messages := sqlite.NewMessageRepo(db)
	ctx := context.Background()

	sess, _ := sessions.Create(ctx, repository.Session{Title: "sess"})
	created, err := messages.Create(ctx, repository.Message{
		SessionID: sess.ID,
		Role:      "user",
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	got, err := messages.Get(ctx, created.ID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got.UpdatedAt.IsZero() {
		t.Error("Get() UpdatedAt is zero, want non-zero")
	}
	if got.UpdatedAt.Before(got.CreatedAt.Add(-time.Second)) {
		t.Errorf("Get() UpdatedAt (%v) is before CreatedAt (%v)", got.UpdatedAt, got.CreatedAt)
	}
}

func TestMessageRepo_ListMessagesWithParts(t *testing.T) {
	db := openTestDB(t)
	sessions := sqlite.NewSessionRepo(db)
	messages := sqlite.NewMessageRepo(db)
	ctx := context.Background()

	sess, err := sessions.Create(ctx, repository.Session{Title: "chat"})
	if err != nil {
		t.Fatalf("Create session: %v", err)
	}

	msg1, err := messages.Create(ctx, repository.Message{SessionID: sess.ID, Role: "user"})
	if err != nil {
		t.Fatalf("Create msg1: %v", err)
	}
	if _, err := messages.CreatePart(ctx, repository.MessagePart{
		MessageID: msg1.ID,
		SessionID: sess.ID,
		Type:      "text",
		Content:   `{"text":"hello"}`,
	}); err != nil {
		t.Fatalf("CreatePart msg1 text: %v", err)
	}
	if _, err := messages.CreatePart(ctx, repository.MessagePart{
		MessageID: msg1.ID,
		SessionID: sess.ID,
		Type:      "tool_call",
		Content:   `{"id":"c1","name":"bash","arguments":"{}"}`,
	}); err != nil {
		t.Fatalf("CreatePart msg1 tool_call: %v", err)
	}

	msg2, err := messages.Create(ctx, repository.Message{SessionID: sess.ID, Role: "assistant"})
	if err != nil {
		t.Fatalf("Create msg2: %v", err)
	}

	msg3, err := messages.Create(ctx, repository.Message{SessionID: sess.ID, Role: "assistant"})
	if err != nil {
		t.Fatalf("Create msg3: %v", err)
	}
	if _, err := messages.CreatePart(ctx, repository.MessagePart{
		MessageID: msg3.ID,
		SessionID: sess.ID,
		Type:      "text",
		Content:   `{"text":"done"}`,
	}); err != nil {
		t.Fatalf("CreatePart msg3 text: %v", err)
	}

	list, err := messages.ListMessagesWithParts(ctx, sess.ID)
	if err != nil {
		t.Fatalf("ListMessagesWithParts() error = %v", err)
	}
	if len(list) != 3 {
		t.Fatalf("ListMessagesWithParts() returned %d messages, want 3", len(list))
	}
	if list[0].ID != msg1.ID || list[1].ID != msg2.ID || list[2].ID != msg3.ID {
		t.Fatalf("message order = [%s %s %s], want [%s %s %s]", list[0].ID, list[1].ID, list[2].ID, msg1.ID, msg2.ID, msg3.ID)
	}
	if len(list[0].Parts) != 2 {
		t.Fatalf("msg1 parts = %d, want 2", len(list[0].Parts))
	}
	if list[0].Parts[0].Type != "text" || list[0].Parts[1].Type != "tool_call" {
		t.Fatalf("msg1 part order = [%s %s], want [text tool_call]", list[0].Parts[0].Type, list[0].Parts[1].Type)
	}
	if len(list[1].Parts) != 0 {
		t.Fatalf("msg2 parts = %d, want 0", len(list[1].Parts))
	}
	if len(list[2].Parts) != 1 || list[2].Parts[0].Content != `{"text":"done"}` {
		t.Fatalf("msg3 parts = %+v, want one text part", list[2].Parts)
	}
}

// ---- Attachment tests -----------------------------------------------------

func TestAttachmentRepo_ReconcileRebuildsCountsAndDeletesStaleRows(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	sessions := sqlite.NewSessionRepo(db)
	messages := sqlite.NewMessageRepo(db)
	attachments := sqlite.NewAttachmentRepo(db)

	sess, err := sessions.Create(ctx, repository.Session{AgentID: "default", ModelID: "fake/model"})
	if err != nil {
		t.Fatalf("Create session: %v", err)
	}
	msg, err := messages.Create(ctx, repository.Message{SessionID: sess.ID, Role: "user"})
	if err != nil {
		t.Fatalf("Create message: %v", err)
	}
	for _, raw := range []string{
		`{"path":"a.png","mime_type":"image/png","storage_key":"live.png","size":4}`,
		`{"path":"b.png","mime_type":"image/png","storage_key":"live.png","size":4}`,
	} {
		if _, err := messages.CreatePart(ctx, repository.MessagePart{
			MessageID: msg.ID,
			SessionID: sess.ID,
			Type:      "file",
			Content:   raw,
		}); err != nil {
			t.Fatalf("CreatePart(%s): %v", raw, err)
		}
	}

	if _, err := attachments.ApplyDeltas(ctx, []repository.AttachmentRefDelta{
		{StorageKey: "live.png", MimeType: "image/png", Size: 4, Delta: 9},
		{StorageKey: "stale.png", MimeType: "image/png", Size: 5, Delta: 1},
	}); err != nil {
		t.Fatalf("ApplyDeltas(seed): %v", err)
	}

	if err := attachments.Reconcile(ctx); err != nil {
		t.Fatalf("Reconcile(): %v", err)
	}

	blobs, err := attachments.List(ctx)
	if err != nil {
		t.Fatalf("List(): %v", err)
	}
	if len(blobs) != 1 {
		t.Fatalf("List() returned %d blobs, want 1", len(blobs))
	}
	if blobs[0].StorageKey != "live.png" {
		t.Fatalf("storage key = %q, want %q", blobs[0].StorageKey, "live.png")
	}
	if blobs[0].RefCount != 2 {
		t.Fatalf("ref_count = %d, want 2", blobs[0].RefCount)
	}
}

// ---- MessagePart tests -----------------------------------------------------

func TestMessageParts_CreateAndList(t *testing.T) {
	db := openTestDB(t)
	sessions := sqlite.NewSessionRepo(db)
	messages := sqlite.NewMessageRepo(db)
	sess, err := sessions.Create(context.Background(), repository.Session{
		AgentID: "test", ModelID: "openai/gpt-4o",
	})
	if err != nil {
		t.Fatal(err)
	}
	msg, err := messages.Create(context.Background(), repository.Message{
		SessionID: sess.ID,
		Role:      "user",
	})
	if err != nil {
		t.Fatal(err)
	}
	part, err := messages.CreatePart(context.Background(), repository.MessagePart{
		MessageID: msg.ID,
		SessionID: sess.ID,
		Type:      "text",
		Content:   `{"text":"hello"}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if part.ID == "" {
		t.Fatal("expected non-empty part ID")
	}
	parts, err := messages.ListPartsByMessage(context.Background(), msg.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(parts) != 1 || parts[0].Content != `{"text":"hello"}` {
		t.Errorf("unexpected parts: %+v", parts)
	}
}

func TestMessageParts_ListBySession(t *testing.T) {
	db := openTestDB(t)
	sessions := sqlite.NewSessionRepo(db)
	messages := sqlite.NewMessageRepo(db)
	sess, _ := sessions.Create(context.Background(), repository.Session{AgentID: "a", ModelID: "m"})
	msg, _ := messages.Create(context.Background(), repository.Message{SessionID: sess.ID, Role: "user"})
	_, _ = messages.CreatePart(context.Background(), repository.MessagePart{
		MessageID: msg.ID, SessionID: sess.ID, Type: "text", Content: `{"text":"x"}`,
	})
	_, _ = messages.CreatePart(context.Background(), repository.MessagePart{
		MessageID: msg.ID, SessionID: sess.ID, Type: "tool_call",
		Content: `{"id":"c1","name":"bash","arguments":"{}"}`,
	})
	parts, err := messages.ListPartsBySession(context.Background(), sess.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(parts) != 2 {
		t.Errorf("ListPartsBySession() want 2 parts, got %d", len(parts))
	}
}

func TestMessageParts_UpdatePart(t *testing.T) {
	db := openTestDB(t)
	sessions := sqlite.NewSessionRepo(db)
	messages := sqlite.NewMessageRepo(db)
	sess, _ := sessions.Create(context.Background(), repository.Session{AgentID: "a", ModelID: "m"})
	msg, _ := messages.Create(context.Background(), repository.Message{SessionID: sess.ID, Role: "assistant"})
	part, _ := messages.CreatePart(context.Background(), repository.MessagePart{
		MessageID: msg.ID, SessionID: sess.ID, Type: "tool_result",
		Content: `{"tool_call_id":"c1","output":"original"}`,
	})
	now := time.Now().UTC()
	part.Content = `{"tool_call_id":"c1","output":"[output pruned]"}`
	part.CompactedAt = &now
	if err := messages.UpdatePart(context.Background(), part); err != nil {
		t.Fatal(err)
	}
	updated, err := messages.ListPartsByMessage(context.Background(), msg.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(updated) != 1 || updated[0].CompactedAt == nil {
		t.Errorf("UpdatePart() expected compacted_at to be set: %+v", updated)
	}
}

func TestMessageParts_DeleteBySession(t *testing.T) {
	db := openTestDB(t)
	sessions := sqlite.NewSessionRepo(db)
	messages := sqlite.NewMessageRepo(db)
	sess, _ := sessions.Create(context.Background(), repository.Session{AgentID: "a", ModelID: "m"})
	msg, _ := messages.Create(context.Background(), repository.Message{SessionID: sess.ID, Role: "user"})
	_, _ = messages.CreatePart(context.Background(), repository.MessagePart{
		MessageID: msg.ID, SessionID: sess.ID, Type: "text", Content: `{"text":"x"}`,
	})
	if err := messages.DeletePartsBySession(context.Background(), sess.ID); err != nil {
		t.Fatal(err)
	}
	parts, err := messages.ListPartsBySession(context.Background(), sess.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(parts) != 0 {
		t.Errorf("DeletePartsBySession() want 0 parts after delete, got %d", len(parts))
	}
}

func TestMessage_Summary(t *testing.T) {
	db := openTestDB(t)
	sessions := sqlite.NewSessionRepo(db)
	messages := sqlite.NewMessageRepo(db)
	sess, _ := sessions.Create(context.Background(), repository.Session{AgentID: "a", ModelID: "m"})
	parentMsg, _ := messages.Create(context.Background(), repository.Message{
		SessionID: sess.ID, Role: "user",
	})
	summaryMsg, err := messages.Create(context.Background(), repository.Message{
		SessionID:          sess.ID,
		Role:               "assistant",
		Summary:            true,
		CompactionParentID: parentMsg.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := messages.Get(context.Background(), summaryMsg.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Summary {
		t.Error("Message.Get() Summary = false, want true")
	}
	if got.CompactionParentID != parentMsg.ID {
		t.Errorf("Message.Get() CompactionParentID = %q, want %q", got.CompactionParentID, parentMsg.ID)
	}
}

// ---- Settings tests --------------------------------------------------------

func openTestDBWithSettings(t *testing.T) (repository.SessionRepo, repository.MessageRepo, repository.SettingsRepo) {
	t.Helper()
	db := openTestDB(t)
	return sqlite.NewSessionRepo(db), sqlite.NewMessageRepo(db), sqlite.NewSettingsRepo(db)
}

func TestSettingsRepo_SetAndGet(t *testing.T) {
	_, _, settings := openTestDBWithSettings(t)
	ctx := context.Background()

	if err := settings.Set(ctx, "tui.theme", "rose-pine"); err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	got, err := settings.Get(ctx, "tui.theme")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got != "rose-pine" {
		t.Errorf("Get() = %q, want %q", got, "rose-pine")
	}
}

func TestSettingsRepo_Get_NotFound(t *testing.T) {
	_, _, settings := openTestDBWithSettings(t)
	ctx := context.Background()

	_, err := settings.Get(ctx, "nonexistent")
	if !errors.Is(err, repository.ErrNotFound) {
		t.Errorf("Get(missing) error = %v, want ErrNotFound", err)
	}
}

func TestSettingsRepo_Set_Upsert(t *testing.T) {
	_, _, settings := openTestDBWithSettings(t)
	ctx := context.Background()

	if err := settings.Set(ctx, "tui.theme", "first"); err != nil {
		t.Fatalf("first Set() error = %v", err)
	}
	if err := settings.Set(ctx, "tui.theme", "second"); err != nil {
		t.Fatalf("second Set() error = %v", err)
	}

	got, err := settings.Get(ctx, "tui.theme")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got != "second" {
		t.Errorf("Get() after upsert = %q, want %q", got, "second")
	}
}

// TestRunMigrations_AddsColumnsToExistingDB verifies that RunMigrations adds
// the summary and compaction_parent_id columns to an existing messages table
// that was created without them. This is the regression test for the
// "no such column: summary" startup error.
func TestRunMigrations_AddsColumnsToExistingDB(t *testing.T) {
	f, err := os.CreateTemp("", "kodacode-migration-test-*.db")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	name := f.Name()
	_ = f.Close()
	t.Cleanup(func() { _ = os.Remove(name) })

	// Open a raw connection using the same DSN as sqlite.Open.
	dsn := "file:" + name + "?_journal_mode=WAL"
	rawDB, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	rawDB.SetMaxOpenConns(1)
	defer rawDB.Close() //nolint:errcheck

	// Create the old messages table without summary or compaction_parent_id.
	oldSchema := `
CREATE TABLE IF NOT EXISTS sessions (
    id TEXT PRIMARY KEY,
    title TEXT NOT NULL DEFAULT '',
    agent_id TEXT NOT NULL DEFAULT '',
    model_id TEXT NOT NULL DEFAULT '',
    parent_id TEXT NOT NULL DEFAULT '',
    branch_point_message_id TEXT NOT NULL DEFAULT '',
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS messages (
    id                    TEXT    PRIMARY KEY,
    session_id            TEXT    NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    role                  TEXT    NOT NULL,
    created_at            INTEGER NOT NULL,
    updated_at            INTEGER NOT NULL DEFAULT 0
);
CREATE TABLE IF NOT EXISTS message_parts (
    id           TEXT    PRIMARY KEY,
    message_id   TEXT    NOT NULL REFERENCES messages(id) ON DELETE CASCADE,
    session_id   TEXT    NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    type         TEXT    NOT NULL,
    content      TEXT    NOT NULL DEFAULT '',
    synthetic    INTEGER NOT NULL DEFAULT 0,
    compacted_at INTEGER,
    created_at   INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS settings (key TEXT PRIMARY KEY, value TEXT NOT NULL DEFAULT '');
`
	if _, err := rawDB.Exec(oldSchema); err != nil {
		t.Fatalf("create old schema: %v", err)
	}

	// Verify the columns are absent before migration.
	if _, err := rawDB.Exec(`SELECT summary FROM messages LIMIT 1`); err == nil {
		t.Fatal("expected summary column to be absent before migration, but query succeeded")
	}

	// Run the migration.
	if err := sqlite.RunMigrations(rawDB); err != nil {
		t.Fatalf("RunMigrations: %v", err)
	}

	// Verify both columns are now present and queryable.
	if _, err := rawDB.Exec(`SELECT summary, compaction_parent_id FROM messages LIMIT 1`); err != nil {
		t.Errorf("columns missing after migration: %v", err)
	}
}
