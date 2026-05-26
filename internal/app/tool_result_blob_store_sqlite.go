package app

import (
	"context"
	"os"
	"path/filepath"

	"github.com/sageil/kodacode/internal/events"
)

type SQLiteToolResultBlobStore struct {
	store *events.SQLiteStore
}

func NewSQLiteToolResultBlobStore(store *events.SQLiteStore) *SQLiteToolResultBlobStore {
	return &SQLiteToolResultBlobStore{store: store}
}

func (s *SQLiteToolResultBlobStore) Save(ctx context.Context, key ToolResultBlobKey, text string) (*events.ToolResultBlobRef, error) {
	if s == nil || s.store == nil || text == "" {
		return nil, nil
	}
	return s.store.SaveToolResultBlob(
		ctx,
		toolResultBlobRefForKey(key),
		key.SessionID,
		key.TurnID,
		key.CallID,
		key.Stream,
		text,
	)
}

func (s *SQLiteToolResultBlobStore) Load(ctx context.Context, ref string) (string, error) {
	if s == nil || s.store == nil {
		return "", os.ErrNotExist
	}
	return s.store.LoadToolResultBlob(ctx, ref)
}

func toolResultBlobRefForKey(key ToolResultBlobKey) string {
	return filepath.ToSlash(filepath.Join(
		sanitizeBlobPathPart(key.SessionID),
		sanitizeBlobPathPart(key.TurnID),
		sanitizeBlobPathPart(key.CallID)+"-"+sanitizeBlobPathPart(key.Stream)+".txt",
	))
}
