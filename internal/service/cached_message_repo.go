package service

import (
	"container/list"
	"context"
	"sync"

	"github.com/sageil/kodacode/v1/internal/repository"
)

const defaultCachedMessageRepoLimit = 128

type cachedMessageRepo struct {
	base  repository.MessageRepo
	limit int

	mu    sync.RWMutex
	cache map[string]*cachedMessageEntry
	lru   *list.List
}

type cachedMessageEntry struct {
	sessionID      string
	snapshot       []repository.Message
	snapshotLoaded bool
	tail           []repository.Message
	appendedParts  map[string][]repository.MessagePart
	updatedParts   map[string]repository.MessagePart
	deletedParts   map[string]struct{}
	clearAllParts  bool
	elem           *list.Element
}

type turnMessageSnapshotRepo interface {
	SnapshotMessagesWithParts(context.Context, string) ([]repository.Message, error)
}

func NewCachedMessageRepo(base repository.MessageRepo) repository.MessageRepo {
	return newCachedMessageRepoWithLimit(base, defaultCachedMessageRepoLimit)
}

func newCachedMessageRepoWithLimit(base repository.MessageRepo, limit int) repository.MessageRepo {
	if base == nil {
		return nil
	}
	if limit <= 0 {
		limit = defaultCachedMessageRepoLimit
	}
	return &cachedMessageRepo{
		base:  base,
		limit: limit,
		cache: make(map[string]*cachedMessageEntry),
		lru:   list.New(),
	}
}

func (r *cachedMessageRepo) Create(ctx context.Context, m repository.Message) (repository.Message, error) {
	created, err := r.base.Create(ctx, m)
	if err != nil {
		return repository.Message{}, err
	}
	r.mu.Lock()
	if entry, ok := r.cache[created.SessionID]; ok {
		entry.tail = append(entry.tail, cloneMessage(created))
		r.touchLocked(entry)
	}
	r.mu.Unlock()
	return created, nil
}

func (r *cachedMessageRepo) CreateWithParts(ctx context.Context, m repository.Message, parts []repository.MessagePart) (repository.Message, error) {
	created, err := r.base.CreateWithParts(ctx, m, parts)
	if err != nil {
		return repository.Message{}, err
	}
	r.mu.Lock()
	if entry, ok := r.cache[created.SessionID]; ok {
		entry.tail = append(entry.tail, cloneMessage(created))
		r.touchLocked(entry)
	}
	r.mu.Unlock()
	return created, nil
}

func (r *cachedMessageRepo) Get(ctx context.Context, id string) (repository.Message, error) {
	return r.base.Get(ctx, id)
}

func (r *cachedMessageRepo) ListBySession(ctx context.Context, sessionID string) ([]repository.Message, error) {
	msgs, err := r.SnapshotMessagesWithParts(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	out := make([]repository.Message, len(msgs))
	for i := range out {
		out[i] = msgs[i]
		out[i].Parts = nil
	}
	return out, nil
}

func (r *cachedMessageRepo) DeleteBySession(ctx context.Context, sessionID string) error {
	if err := r.base.DeleteBySession(ctx, sessionID); err != nil {
		return err
	}
	r.mu.Lock()
	r.deleteEntryLocked(sessionID)
	r.mu.Unlock()
	return nil
}

func (r *cachedMessageRepo) ListMessagesWithParts(ctx context.Context, sessionID string) ([]repository.Message, error) {
	msgs, err := r.SnapshotMessagesWithParts(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	return cloneMessages(msgs), nil
}

func (r *cachedMessageRepo) SnapshotMessagesWithParts(ctx context.Context, sessionID string) ([]repository.Message, error) {
	r.mu.Lock()
	if entry, ok := r.cache[sessionID]; ok {
		r.touchLocked(entry)
		if entry.hasPendingChanges() {
			entry.materialize()
		}
		msgs := entry.snapshot
		r.mu.Unlock()
		return msgs, nil
	}
	r.mu.Unlock()

	msgs, err := r.base.ListMessagesWithParts(ctx, sessionID)
	if err != nil {
		return nil, err
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if entry, ok := r.cache[sessionID]; ok {
		r.touchLocked(entry)
		if !entry.snapshotLoaded {
			entry.snapshot = cloneMessages(msgs)
			entry.snapshotLoaded = true
		}
		if entry.hasPendingChanges() {
			entry.materialize()
		}
		return entry.snapshot, nil
	}
	if len(r.cache) >= r.limit {
		back := r.lru.Back()
		if back != nil {
			evictSessionID, _ := back.Value.(string)
			r.deleteEntryLocked(evictSessionID)
		}
	}
	entry := &cachedMessageEntry{
		sessionID:      sessionID,
		snapshot:       cloneMessages(msgs),
		snapshotLoaded: true,
		elem:           r.lru.PushFront(sessionID),
	}
	r.cache[sessionID] = entry
	r.evictLocked()
	return entry.snapshot, nil
}

func (r *cachedMessageRepo) CreatePart(ctx context.Context, p repository.MessagePart) (repository.MessagePart, error) {
	created, err := r.base.CreatePart(ctx, p)
	if err != nil {
		return repository.MessagePart{}, err
	}
	r.mu.Lock()
	if entry, ok := r.cache[created.SessionID]; ok {
		if entry.hasMessage(created.MessageID) {
			if entry.appendedParts == nil {
				entry.appendedParts = make(map[string][]repository.MessagePart)
			}
			entry.appendedParts[created.MessageID] = append(entry.appendedParts[created.MessageID], clonePart(created))
			r.touchLocked(entry)
		} else {
			r.deleteEntryLocked(created.SessionID)
		}
	}
	r.mu.Unlock()
	return created, nil
}

func (r *cachedMessageRepo) ListPartsByMessage(ctx context.Context, messageID string) ([]repository.MessagePart, error) {
	return r.base.ListPartsByMessage(ctx, messageID)
}

func (r *cachedMessageRepo) ListPartsBySession(ctx context.Context, sessionID string) ([]repository.MessagePart, error) {
	msgs, err := r.SnapshotMessagesWithParts(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	var parts []repository.MessagePart
	for _, msg := range msgs {
		parts = append(parts, cloneParts(msg.Parts)...)
	}
	return parts, nil
}

func (r *cachedMessageRepo) UpdatePart(ctx context.Context, p repository.MessagePart) error {
	if err := r.base.UpdatePart(ctx, p); err != nil {
		return err
	}
	r.updateCachedPart(p)
	return nil
}

func (r *cachedMessageRepo) BatchUpdateParts(ctx context.Context, parts []repository.MessagePart) error {
	if err := r.base.BatchUpdateParts(ctx, parts); err != nil {
		return err
	}
	for _, part := range parts {
		r.updateCachedPart(part)
	}
	return nil
}

func (r *cachedMessageRepo) DeletePart(ctx context.Context, partID string) error {
	if err := r.base.DeletePart(ctx, partID); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, entry := range r.cache {
		if entry.removePendingPart(partID) {
			r.touchLocked(entry)
			return nil
		}
		if !entry.hasPart(partID) {
			continue
		}
		if entry.deletedParts == nil {
			entry.deletedParts = make(map[string]struct{})
		}
		delete(entry.updatedParts, partID)
		entry.deletedParts[partID] = struct{}{}
		r.touchLocked(entry)
		return nil
	}
	return nil
}

func (r *cachedMessageRepo) DeletePartsBySession(ctx context.Context, sessionID string) error {
	if err := r.base.DeletePartsBySession(ctx, sessionID); err != nil {
		return err
	}
	r.mu.Lock()
	if entry, ok := r.cache[sessionID]; ok {
		entry.clearAllParts = true
		entry.appendedParts = nil
		entry.updatedParts = nil
		entry.deletedParts = nil
		r.touchLocked(entry)
	}
	r.mu.Unlock()
	return nil
}

func (r *cachedMessageRepo) updateCachedPart(p repository.MessagePart) {
	r.mu.Lock()
	defer r.mu.Unlock()
	entry, ok := r.cache[p.SessionID]
	if !ok {
		return
	}
	if entry.updatePendingPart(p) {
		r.touchLocked(entry)
		return
	}
	if !entry.hasPart(p.ID) {
		r.deleteEntryLocked(p.SessionID)
		return
	}
	if entry.updatedParts == nil {
		entry.updatedParts = make(map[string]repository.MessagePart)
	}
	entry.updatedParts[p.ID] = clonePart(p)
	r.touchLocked(entry)
}

func (r *cachedMessageRepo) touchLocked(entry *cachedMessageEntry) {
	if entry == nil || entry.elem == nil {
		return
	}
	r.lru.MoveToFront(entry.elem)
}

func (r *cachedMessageRepo) evictLocked() {
	for len(r.cache) > r.limit {
		back := r.lru.Back()
		if back == nil {
			return
		}
		sessionID, _ := back.Value.(string)
		r.deleteEntryLocked(sessionID)
	}
}

func (r *cachedMessageRepo) deleteEntryLocked(sessionID string) {
	entry, ok := r.cache[sessionID]
	if !ok {
		return
	}
	if entry.elem != nil {
		r.lru.Remove(entry.elem)
	}
	delete(r.cache, sessionID)
}

func (e *cachedMessageEntry) hasPendingChanges() bool {
	return len(e.tail) > 0 || e.clearAllParts || len(e.appendedParts) > 0 || len(e.updatedParts) > 0 || len(e.deletedParts) > 0
}

func (e *cachedMessageEntry) hasMessage(messageID string) bool {
	for _, msg := range e.snapshot {
		if msg.ID == messageID {
			return true
		}
	}
	for _, msg := range e.tail {
		if msg.ID == messageID {
			return true
		}
	}
	return false
}

func (e *cachedMessageEntry) hasPart(partID string) bool {
	if _, deleted := e.deletedParts[partID]; deleted {
		return false
	}
	if _, ok := e.updatedParts[partID]; ok {
		return true
	}
	for _, parts := range e.appendedParts {
		for _, part := range parts {
			if part.ID == partID {
				return true
			}
		}
	}
	if e.clearAllParts {
		return false
	}
	for _, msg := range e.snapshot {
		for _, part := range msg.Parts {
			if part.ID == partID {
				return true
			}
		}
	}
	for _, msg := range e.tail {
		for _, part := range msg.Parts {
			if part.ID == partID {
				return true
			}
		}
	}
	return false
}

func (e *cachedMessageEntry) updatePendingPart(p repository.MessagePart) bool {
	for msgID, parts := range e.appendedParts {
		for i := range parts {
			if parts[i].ID == p.ID {
				parts[i] = clonePart(p)
				e.appendedParts[msgID] = parts
				return true
			}
		}
	}
	if _, ok := e.updatedParts[p.ID]; ok {
		e.updatedParts[p.ID] = clonePart(p)
		return true
	}
	return false
}

func (e *cachedMessageEntry) removePendingPart(partID string) bool {
	for msgID, parts := range e.appendedParts {
		for i := range parts {
			if parts[i].ID != partID {
				continue
			}
			parts = append(parts[:i], parts[i+1:]...)
			if len(parts) == 0 {
				delete(e.appendedParts, msgID)
			} else {
				e.appendedParts[msgID] = parts
			}
			delete(e.updatedParts, partID)
			delete(e.deletedParts, partID)
			return true
		}
	}
	delete(e.updatedParts, partID)
	return false
}

func (e *cachedMessageEntry) materialize() {
	out := cloneMessages(e.snapshot)
	if len(e.tail) > 0 {
		out = append(out, cloneMessages(e.tail)...)
	}
	if e.clearAllParts || len(e.appendedParts) > 0 || len(e.updatedParts) > 0 || len(e.deletedParts) > 0 {
		for i := range out {
			msg := out[i]
			parts := msg.Parts
			if e.clearAllParts {
				parts = nil
			} else if len(parts) > 0 && (len(e.updatedParts) > 0 || len(e.deletedParts) > 0) {
				filtered := make([]repository.MessagePart, 0, len(parts))
				for _, part := range parts {
					if _, deleted := e.deletedParts[part.ID]; deleted {
						continue
					}
					if updated, ok := e.updatedParts[part.ID]; ok {
						filtered = append(filtered, clonePart(updated))
						continue
					}
					filtered = append(filtered, part)
				}
				parts = filtered
			}
			if appended := e.appendedParts[msg.ID]; len(appended) > 0 {
				parts = append(parts, cloneParts(appended)...)
			}
			msg.Parts = parts
			out[i] = msg
		}
	}
	e.snapshot = out
	e.snapshotLoaded = true
	e.tail = nil
	e.appendedParts = nil
	e.updatedParts = nil
	e.deletedParts = nil
	e.clearAllParts = false
}

func cloneMessages(in []repository.Message) []repository.Message {
	out := make([]repository.Message, len(in))
	for i := range in {
		out[i] = cloneMessage(in[i])
	}
	return out
}

func cloneMessage(in repository.Message) repository.Message {
	out := in
	out.Parts = cloneParts(in.Parts)
	return out
}

func cloneParts(in []repository.MessagePart) []repository.MessagePart {
	out := make([]repository.MessagePart, len(in))
	for i := range in {
		out[i] = clonePart(in[i])
	}
	return out
}

func clonePart(in repository.MessagePart) repository.MessagePart {
	out := in
	if in.CompactedAt != nil {
		t := *in.CompactedAt
		out.CompactedAt = &t
	}
	return out
}

func listMessagesForTurn(ctx context.Context, repo repository.MessageRepo, sessionID string) ([]repository.Message, error) {
	if repo == nil {
		return nil, repository.ErrNotFound
	}
	if snapshots, ok := repo.(turnMessageSnapshotRepo); ok {
		return snapshots.SnapshotMessagesWithParts(ctx, sessionID)
	}
	return repo.ListMessagesWithParts(ctx, sessionID)
}
