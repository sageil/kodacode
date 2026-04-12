package service

import (
	"context"
	"testing"

	"github.com/sageil/kodacode/v1/internal/repository"
)

type countingMessageRepo struct {
	*turnLoopMessageRepo
	listWithPartsCalls int
}

func newCountingMessageRepo() *countingMessageRepo {
	return &countingMessageRepo{turnLoopMessageRepo: &turnLoopMessageRepo{}}
}

func (r *countingMessageRepo) ListMessagesWithParts(ctx context.Context, sessionID string) ([]repository.Message, error) {
	r.listWithPartsCalls++
	return r.turnLoopMessageRepo.ListMessagesWithParts(ctx, sessionID)
}

func TestCachedMessageRepoCachesAndUpdatesConversation(t *testing.T) {
	ctx := context.Background()
	base := newCountingMessageRepo()

	msg, err := base.Create(ctx, repository.Message{SessionID: "s1", Role: "user"})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if _, err := base.CreatePart(ctx, repository.MessagePart{
		MessageID: msg.ID,
		SessionID: "s1",
		Type:      "text",
		Content:   `{"text":"hello"}`,
	}); err != nil {
		t.Fatalf("CreatePart() error = %v", err)
	}

	repo := NewCachedMessageRepo(base)
	first, err := repo.ListMessagesWithParts(ctx, "s1")
	if err != nil {
		t.Fatalf("ListMessagesWithParts() error = %v", err)
	}
	second, err := repo.ListMessagesWithParts(ctx, "s1")
	if err != nil {
		t.Fatalf("ListMessagesWithParts() second error = %v", err)
	}
	if base.listWithPartsCalls != 1 {
		t.Fatalf("base ListMessagesWithParts calls = %d, want 1", base.listWithPartsCalls)
	}
	if len(first) != 1 || len(second) != 1 || len(second[0].Parts) != 1 {
		t.Fatalf("unexpected cached conversation shape: first=%d second=%d secondParts=%d", len(first), len(second), len(second[0].Parts))
	}

	if _, err := repo.CreatePart(ctx, repository.MessagePart{
		MessageID: msg.ID,
		SessionID: "s1",
		Type:      "file",
		Content:   `{"path":"note.txt","mime_type":"text/plain","storage_key":"abc","size":4}`,
	}); err != nil {
		t.Fatalf("CreatePart() via cache error = %v", err)
	}
	updated, err := repo.ListMessagesWithParts(ctx, "s1")
	if err != nil {
		t.Fatalf("ListMessagesWithParts() updated error = %v", err)
	}
	if base.listWithPartsCalls != 1 {
		t.Fatalf("base ListMessagesWithParts calls after cache update = %d, want 1", base.listWithPartsCalls)
	}
	if got := len(updated[0].Parts); got != 2 {
		t.Fatalf("updated parts = %d, want 2", got)
	}
}

func TestCachedMessageRepoEvictsLeastRecentlyUsedSession(t *testing.T) {
	ctx := context.Background()
	base := newCountingMessageRepo()
	for _, sessionID := range []string{"s1", "s2", "s3"} {
		msg, err := base.Create(ctx, repository.Message{SessionID: sessionID, Role: "user"})
		if err != nil {
			t.Fatalf("Create(%s) error = %v", sessionID, err)
		}
		if _, err := base.CreatePart(ctx, repository.MessagePart{
			MessageID: msg.ID,
			SessionID: sessionID,
			Type:      "text",
			Content:   `{"text":"hello"}`,
		}); err != nil {
			t.Fatalf("CreatePart(%s) error = %v", sessionID, err)
		}
	}

	repo := newCachedMessageRepoWithLimit(base, 2)
	for _, sessionID := range []string{"s1", "s2"} {
		if _, err := repo.ListMessagesWithParts(ctx, sessionID); err != nil {
			t.Fatalf("prime cache %s: %v", sessionID, err)
		}
	}
	if _, err := repo.ListMessagesWithParts(ctx, "s1"); err != nil {
		t.Fatalf("refresh s1: %v", err)
	}
	if _, err := repo.ListMessagesWithParts(ctx, "s3"); err != nil {
		t.Fatalf("load s3: %v", err)
	}

	before := base.listWithPartsCalls
	if _, err := repo.ListMessagesWithParts(ctx, "s2"); err != nil {
		t.Fatalf("reload evicted s2: %v", err)
	}
	if got := base.listWithPartsCalls - before; got != 1 {
		t.Fatalf("base reload count = %d, want 1", got)
	}
}

func TestCachedMessageRepoCreatePartInvalidatesSessionWhenMessageMissingFromCache(t *testing.T) {
	ctx := context.Background()
	base := newCountingMessageRepo()

	msg, err := base.Create(ctx, repository.Message{SessionID: "s1", Role: "user"})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if _, err := base.CreatePart(ctx, repository.MessagePart{
		MessageID: msg.ID,
		SessionID: "s1",
		Type:      "text",
		Content:   `{"text":"hello"}`,
	}); err != nil {
		t.Fatalf("CreatePart() error = %v", err)
	}

	repo := NewCachedMessageRepo(base)
	if _, err := repo.ListMessagesWithParts(ctx, "s1"); err != nil {
		t.Fatalf("prime cache: %v", err)
	}
	if base.listWithPartsCalls != 1 {
		t.Fatalf("base ListMessagesWithParts calls = %d, want 1", base.listWithPartsCalls)
	}

	missedMsg, err := base.Create(ctx, repository.Message{SessionID: "s1", Role: "assistant"})
	if err != nil {
		t.Fatalf("Create() missing cached message error = %v", err)
	}
	if _, err := repo.CreatePart(ctx, repository.MessagePart{
		MessageID: missedMsg.ID,
		SessionID: "s1",
		Type:      "text",
		Content:   `{"text":"world"}`,
	}); err != nil {
		t.Fatalf("CreatePart() error = %v", err)
	}

	msgs, err := repo.ListMessagesWithParts(ctx, "s1")
	if err != nil {
		t.Fatalf("ListMessagesWithParts() reload error = %v", err)
	}
	if base.listWithPartsCalls != 2 {
		t.Fatalf("base ListMessagesWithParts calls after invalidation = %d, want 2", base.listWithPartsCalls)
	}
	if len(msgs) != 2 {
		t.Fatalf("messages len = %d, want 2", len(msgs))
	}
	if got := len(msgs[1].Parts); got != 1 {
		t.Fatalf("new message parts len = %d, want 1", got)
	}
}

func TestCachedMessageRepoSnapshotMessagesWithPartsUsesStableSnapshots(t *testing.T) {
	ctx := context.Background()
	base := newCountingMessageRepo()

	msg, err := base.Create(ctx, repository.Message{SessionID: "s1", Role: "user"})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if _, err := base.CreatePart(ctx, repository.MessagePart{
		MessageID: msg.ID,
		SessionID: "s1",
		Type:      "text",
		Content:   `{"text":"hello"}`,
	}); err != nil {
		t.Fatalf("CreatePart() error = %v", err)
	}

	repo, ok := NewCachedMessageRepo(base).(*cachedMessageRepo)
	if !ok {
		t.Fatal("NewCachedMessageRepo() did not return *cachedMessageRepo")
	}

	first, err := repo.SnapshotMessagesWithParts(ctx, "s1")
	if err != nil {
		t.Fatalf("SnapshotMessagesWithParts() error = %v", err)
	}
	if got := len(first[0].Parts); got != 1 {
		t.Fatalf("snapshot part count = %d, want 1", got)
	}

	if _, err := repo.CreatePart(ctx, repository.MessagePart{
		MessageID: msg.ID,
		SessionID: "s1",
		Type:      "file",
		Content:   `{"path":"note.txt","mime_type":"text/plain","storage_key":"abc","size":4}`,
	}); err != nil {
		t.Fatalf("CreatePart() via cache error = %v", err)
	}

	if got := len(first[0].Parts); got != 1 {
		t.Fatalf("stale snapshot part count = %d, want 1", got)
	}

	second, err := repo.SnapshotMessagesWithParts(ctx, "s1")
	if err != nil {
		t.Fatalf("SnapshotMessagesWithParts() second error = %v", err)
	}
	if got := len(second[0].Parts); got != 2 {
		t.Fatalf("updated snapshot part count = %d, want 2", got)
	}
}

func TestCachedMessageRepoCreateWithPartsAppendsWithoutReload(t *testing.T) {
	ctx := context.Background()
	base := newCountingMessageRepo()

	msg, err := base.Create(ctx, repository.Message{SessionID: "s1", Role: "user"})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if _, err := base.CreatePart(ctx, repository.MessagePart{
		MessageID: msg.ID,
		SessionID: "s1",
		Type:      "text",
		Content:   `{"text":"hello"}`,
	}); err != nil {
		t.Fatalf("CreatePart() error = %v", err)
	}

	repo, ok := NewCachedMessageRepo(base).(*cachedMessageRepo)
	if !ok {
		t.Fatal("NewCachedMessageRepo() did not return *cachedMessageRepo")
	}

	first, err := repo.SnapshotMessagesWithParts(ctx, "s1")
	if err != nil {
		t.Fatalf("SnapshotMessagesWithParts() error = %v", err)
	}
	if _, err := repo.CreateWithParts(ctx, repository.Message{
		SessionID: "s1",
		Role:      "assistant",
	}, []repository.MessagePart{{
		SessionID: "s1",
		Type:      "text",
		Content:   `{"text":"world"}`,
	}}); err != nil {
		t.Fatalf("CreateWithParts() error = %v", err)
	}
	if got := len(first); got != 1 {
		t.Fatalf("stale snapshot len = %d, want 1", got)
	}

	second, err := repo.SnapshotMessagesWithParts(ctx, "s1")
	if err != nil {
		t.Fatalf("SnapshotMessagesWithParts() second error = %v", err)
	}
	if base.listWithPartsCalls != 1 {
		t.Fatalf("base ListMessagesWithParts calls = %d, want 1", base.listWithPartsCalls)
	}
	if got := len(second); got != 2 {
		t.Fatalf("updated snapshot len = %d, want 2", got)
	}
	if got := len(second[1].Parts); got != 1 {
		t.Fatalf("appended message parts len = %d, want 1", got)
	}
}

func TestCachedMessageRepoUpdatePartUsesOverlayWithoutReload(t *testing.T) {
	ctx := context.Background()
	base := newCountingMessageRepo()

	msg, err := base.Create(ctx, repository.Message{SessionID: "s1", Role: "user"})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	part, err := base.CreatePart(ctx, repository.MessagePart{
		MessageID: msg.ID,
		SessionID: "s1",
		Type:      "text",
		Content:   `{"text":"hello"}`,
	})
	if err != nil {
		t.Fatalf("CreatePart() error = %v", err)
	}

	repo, ok := NewCachedMessageRepo(base).(*cachedMessageRepo)
	if !ok {
		t.Fatal("NewCachedMessageRepo() did not return *cachedMessageRepo")
	}

	first, err := repo.SnapshotMessagesWithParts(ctx, "s1")
	if err != nil {
		t.Fatalf("SnapshotMessagesWithParts() error = %v", err)
	}

	part.Content = `{"text":"updated"}`
	if err := repo.UpdatePart(ctx, part); err != nil {
		t.Fatalf("UpdatePart() error = %v", err)
	}
	if got := first[0].Parts[0].Content; got != `{"text":"hello"}` {
		t.Fatalf("stale snapshot content = %q, want original", got)
	}

	second, err := repo.SnapshotMessagesWithParts(ctx, "s1")
	if err != nil {
		t.Fatalf("SnapshotMessagesWithParts() second error = %v", err)
	}
	if base.listWithPartsCalls != 1 {
		t.Fatalf("base ListMessagesWithParts calls = %d, want 1", base.listWithPartsCalls)
	}
	if got := second[0].Parts[0].Content; got != `{"text":"updated"}` {
		t.Fatalf("updated content = %q, want updated value", got)
	}
}

func TestCachedMessageRepoDeletePartUsesOverlayWithoutReload(t *testing.T) {
	ctx := context.Background()
	base := newCountingMessageRepo()

	msg, err := base.Create(ctx, repository.Message{SessionID: "s1", Role: "user"})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if _, err := base.CreatePart(ctx, repository.MessagePart{
		MessageID: msg.ID,
		SessionID: "s1",
		Type:      "text",
		Content:   `{"text":"hello"}`,
	}); err != nil {
		t.Fatalf("CreatePart() error = %v", err)
	}
	filePart, err := base.CreatePart(ctx, repository.MessagePart{
		MessageID: msg.ID,
		SessionID: "s1",
		Type:      "file",
		Content:   `{"path":"note.txt","mime_type":"text/plain","storage_key":"abc","size":4}`,
	})
	if err != nil {
		t.Fatalf("CreatePart() file error = %v", err)
	}

	repo, ok := NewCachedMessageRepo(base).(*cachedMessageRepo)
	if !ok {
		t.Fatal("NewCachedMessageRepo() did not return *cachedMessageRepo")
	}

	first, err := repo.SnapshotMessagesWithParts(ctx, "s1")
	if err != nil {
		t.Fatalf("SnapshotMessagesWithParts() error = %v", err)
	}
	if err := repo.DeletePart(ctx, filePart.ID); err != nil {
		t.Fatalf("DeletePart() error = %v", err)
	}
	if got := len(first[0].Parts); got != 2 {
		t.Fatalf("stale snapshot part count = %d, want 2", got)
	}

	second, err := repo.SnapshotMessagesWithParts(ctx, "s1")
	if err != nil {
		t.Fatalf("SnapshotMessagesWithParts() second error = %v", err)
	}
	if base.listWithPartsCalls != 1 {
		t.Fatalf("base ListMessagesWithParts calls = %d, want 1", base.listWithPartsCalls)
	}
	if got := len(second[0].Parts); got != 1 {
		t.Fatalf("updated part count = %d, want 1", got)
	}
}
