package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/sageil/kodacode/v1/internal/config"
	"github.com/sageil/kodacode/v1/internal/message"
	"github.com/sageil/kodacode/v1/internal/repository"
)

const defaultServiceMaxAttachmentSize int64 = 20 * 1024 * 1024

type sessionAttachmentLock struct {
	mu   sync.Mutex
	refs int
}

type sessionAttachmentService struct {
	store *attachmentStore
	refs  repository.AttachmentRepo

	maxSize int64

	mu    sync.Mutex
	gate  sync.RWMutex
	locks map[string]*sessionAttachmentLock
}

func newSessionAttachmentService(projectDir string, cfg *config.Config) *sessionAttachmentService {
	maxSize := defaultServiceMaxAttachmentSize
	if cfg != nil && cfg.TUI.MaxAttachmentSize > 0 {
		maxSize = cfg.TUI.MaxAttachmentSize
	}
	return &sessionAttachmentService{
		store:   newAttachmentStore(projectDir),
		maxSize: maxSize,
		locks:   make(map[string]*sessionAttachmentLock),
	}
}

func (s *sessionAttachmentService) SetRepo(ar repository.AttachmentRepo) {
	if s == nil {
		return
	}
	s.refs = ar
}

func (s *sessionAttachmentService) CleanupSession(sessionID string) {
	if s == nil || sessionID == "" {
		return
	}
	s.mu.Lock()
	delete(s.locks, sessionID)
	s.mu.Unlock()
}

func (s *sessionAttachmentService) ValidateFiles(attachments []FileAttachment) ([]FileAttachment, error) {
	if len(attachments) == 0 {
		return nil, nil
	}
	normalized := make([]FileAttachment, len(attachments))
	for i, att := range attachments {
		valid, err := s.validateFile(att)
		if err != nil {
			return nil, err
		}
		normalized[i] = valid
	}
	return normalized, nil
}

func (s *sessionAttachmentService) validateFile(att FileAttachment) (FileAttachment, error) {
	name := filepath.Base(att.Path)
	if name == "." || name == "/" || name == "" {
		name = "attachment"
	}

	if len(att.Data) == 0 {
		return FileAttachment{}, fmt.Errorf("send: attachment %s is empty", name)
	}
	if s != nil && s.maxSize > 0 && int64(len(att.Data)) > s.maxSize {
		return FileAttachment{}, fmt.Errorf("send: attachment %s exceeds %s", name, formatAttachmentSize(s.maxSize))
	}

	if att.Path != "" {
		if info, err := os.Lstat(att.Path); err == nil {
			switch {
			case info.Mode()&os.ModeSymlink != 0:
				return FileAttachment{}, fmt.Errorf("send: attachment %s cannot be a symlink", name)
			case info.IsDir():
				return FileAttachment{}, fmt.Errorf("send: attachment %s cannot be a directory", name)
			case s != nil && s.maxSize > 0 && info.Size() > s.maxSize:
				return FileAttachment{}, fmt.Errorf("send: attachment %s exceeds %s", name, formatAttachmentSize(s.maxSize))
			case info.Size() == 0:
				return FileAttachment{}, fmt.Errorf("send: attachment %s is empty", name)
			}
		}
	}

	detected := detectAttachmentMIME(att)
	switch {
	case att.MimeType == "":
		att.MimeType = detected
	case attachmentMIMEIsUnknown(att.MimeType) && attachmentMIMEHasStrongType(detected):
		att.MimeType = detected
	case !attachmentMIMECompatible(att.MimeType, detected):
		return FileAttachment{}, fmt.Errorf("send: attachment %s content does not match declared MIME type %s", name, att.MimeType)
	}
	return att, nil
}

func detectAttachmentMIME(att FileAttachment) string {
	if len(att.Data) > 0 {
		detected := canonicalAttachmentMIME(http.DetectContentType(att.Data))
		if !attachmentMIMEIsUnknown(detected) {
			return detected
		}
	}
	if att.Path != "" {
		if byExt := mime.TypeByExtension(strings.ToLower(filepath.Ext(att.Path))); byExt != "" {
			return canonicalAttachmentMIME(byExt)
		}
	}
	if att.MimeType != "" {
		return canonicalAttachmentMIME(att.MimeType)
	}
	return "application/octet-stream"
}

func canonicalAttachmentMIME(mimeType string) string {
	mimeType = strings.TrimSpace(strings.ToLower(mimeType))
	if mimeType == "" {
		return ""
	}
	if parsed, _, err := mime.ParseMediaType(mimeType); err == nil && parsed != "" {
		return parsed
	}
	if idx := strings.IndexByte(mimeType, ';'); idx >= 0 {
		return strings.TrimSpace(mimeType[:idx])
	}
	return mimeType
}

func attachmentMIMEIsUnknown(mimeType string) bool {
	mimeType = canonicalAttachmentMIME(mimeType)
	return mimeType == "" || mimeType == "application/octet-stream"
}

func attachmentMIMEHasStrongType(mimeType string) bool {
	mimeType = canonicalAttachmentMIME(mimeType)
	return strings.HasPrefix(mimeType, "image/") || mimeType == "application/pdf"
}

func attachmentMIMECompatible(declared, detected string) bool {
	declared = canonicalAttachmentMIME(declared)
	detected = canonicalAttachmentMIME(detected)
	switch {
	case declared == "" || detected == "":
		return true
	case declared == detected:
		return true
	case attachmentMIMEIsUnknown(declared) || attachmentMIMEIsUnknown(detected):
		return true
	case isTextLikeMIME(declared) && isTextLikeMIME(detected):
		return true
	default:
		return false
	}
}

func formatAttachmentSize(bytes int64) string {
	const (
		kb = 1024
		mb = kb * 1024
	)
	switch {
	case bytes >= mb:
		return fmt.Sprintf("%.1f MB", float64(bytes)/float64(mb))
	case bytes >= kb:
		return fmt.Sprintf("%.1f KB", float64(bytes)/float64(kb))
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}

func (s *sessionAttachmentService) withMutation(sessionID string, fn func() error) error {
	if s == nil || s.refs == nil {
		return fn()
	}
	unlock := s.lockSession(sessionID)
	defer unlock()
	return fn()
}

func (s *sessionAttachmentService) withGlobalMutation(fn func() error) error {
	if s == nil || s.refs == nil {
		return fn()
	}
	s.gate.Lock()
	defer s.gate.Unlock()
	return fn()
}

func (s *sessionAttachmentService) lockSession(sessionID string) func() {
	if s == nil || sessionID == "" {
		if s != nil {
			s.gate.RLock()
			return s.gate.RUnlock
		}
		return func() {}
	}

	s.gate.RLock()
	s.mu.Lock()
	lock := s.locks[sessionID]
	if lock == nil {
		lock = &sessionAttachmentLock{}
		s.locks[sessionID] = lock
	}
	lock.refs++
	s.mu.Unlock()

	lock.mu.Lock()
	return func() {
		lock.mu.Unlock()
		s.mu.Lock()
		lock.refs--
		if lock.refs == 0 {
			delete(s.locks, sessionID)
		}
		s.mu.Unlock()
		s.gate.RUnlock()
	}
}

func (s *sessionAttachmentService) trackRef(ctx context.Context, fc message.FileContent, delta int) error {
	if s == nil || s.refs == nil || fc.StorageKey == "" || delta == 0 {
		return nil
	}
	zero, err := s.refs.ApplyDeltas(ctx, []repository.AttachmentRefDelta{{
		StorageKey: fc.StorageKey,
		MimeType:   fc.MimeType,
		Size:       fc.Size,
		Delta:      delta,
	}})
	if err != nil {
		return err
	}
	if delta < 0 {
		return s.pruneZeroRefBlobs(ctx, zero)
	}
	return nil
}

func (s *sessionAttachmentService) releaseSessionRefs(ctx context.Context, messages repository.MessageRepo, sessionID string) {
	counts, err := s.sessionRefCounts(ctx, messages, sessionID)
	if err != nil {
		log.Printf("attachments: list parts for release session=%s: %v", sessionID, err)
		return
	}
	if err := s.releaseRefs(ctx, counts); err != nil {
		log.Printf("attachments: release refs for session=%s: %v", sessionID, err)
	}
}

func (s *sessionAttachmentService) sessionRefCounts(ctx context.Context, messages repository.MessageRepo, sessionID string) (map[string]attachmentRefCount, error) {
	if s == nil || s.refs == nil || messages == nil {
		return nil, nil
	}
	parts, err := messages.ListPartsBySession(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	return attachmentRefCountsFromParts(parts), nil
}

func (s *sessionAttachmentService) releaseRefs(ctx context.Context, counts map[string]attachmentRefCount) error {
	if s == nil || s.refs == nil || len(counts) == 0 {
		return nil
	}
	deltas := make([]repository.AttachmentRefDelta, 0, len(counts))
	for _, ref := range counts {
		deltas = append(deltas, repository.AttachmentRefDelta{
			StorageKey: ref.StorageKey,
			MimeType:   ref.MimeType,
			Size:       ref.Size,
			Delta:      -ref.Count,
		})
	}
	zero, err := s.refs.ApplyDeltas(ctx, deltas)
	if err != nil {
		return err
	}
	return s.pruneZeroRefBlobs(ctx, zero)
}

func (s *sessionAttachmentService) pruneZeroRefBlobs(ctx context.Context, zero []repository.AttachmentBlob) error {
	if s == nil || s.refs == nil || len(zero) == 0 {
		return nil
	}
	keys := make([]string, 0, len(zero))
	for _, blob := range zero {
		if blob.StorageKey != "" {
			keys = append(keys, blob.StorageKey)
		}
	}
	if len(keys) == 0 {
		return nil
	}
	if err := s.store.Delete(keys); err != nil {
		return err
	}
	return s.refs.Delete(ctx, keys)
}

func (s *sessionAttachmentService) Reconcile(ctx context.Context) error {
	if s == nil || s.refs == nil {
		return nil
	}
	return s.withGlobalMutation(func() error {
		if err := s.refs.Reconcile(ctx); err != nil {
			return err
		}
		return s.pruneOrphanedFiles(ctx)
	})
}

func (s *sessionAttachmentService) pruneOrphanedFiles(ctx context.Context) error {
	if s == nil || s.refs == nil {
		return nil
	}
	blobs, err := s.refs.List(ctx)
	if err != nil {
		return err
	}
	live := make(map[string]bool, len(blobs))
	for _, blob := range blobs {
		if blob.RefCount > 0 {
			live[blob.StorageKey] = true
		}
	}
	keys, err := s.store.ListStorageKeys()
	if err != nil {
		return err
	}
	var stale []string
	for _, key := range keys {
		if !live[key] {
			stale = append(stale, key)
		}
	}
	return s.store.Delete(stale)
}

type attachmentRefCount struct {
	StorageKey string
	MimeType   string
	Size       int64
	Count      int
}

func attachmentRefCountsFromParts(parts []repository.MessagePart) map[string]attachmentRefCount {
	counts := make(map[string]attachmentRefCount)
	for _, part := range parts {
		if part.Type != "file" {
			continue
		}
		var fc message.FileContent
		if err := json.Unmarshal([]byte(part.Content), &fc); err != nil || fc.StorageKey == "" {
			continue
		}
		ref := counts[fc.StorageKey]
		ref.StorageKey = fc.StorageKey
		ref.MimeType = fc.MimeType
		ref.Count++
		if fc.Size > 0 {
			ref.Size = fc.Size
		}
		counts[fc.StorageKey] = ref
	}
	return counts
}

func (s *SessionService) validateAttachments(attachments []FileAttachment) ([]FileAttachment, error) {
	if s == nil || s.store == nil {
		return attachments, nil
	}
	return s.store.ValidateAttachments(attachments)
}

func (s *SessionService) withAttachmentMutation(sessionID string, fn func() error) error {
	if s == nil || s.store == nil {
		return fn()
	}
	return s.store.WithAttachmentMutation(sessionID, fn)
}

func (s *SessionService) trackAttachmentRef(ctx context.Context, fc message.FileContent, delta int) error {
	if s == nil || s.store == nil {
		return nil
	}
	return s.store.TrackAttachmentRef(ctx, fc, delta)
}

func (s *SessionService) ReconcileAttachmentBlobs(ctx context.Context) error {
	if s == nil || s.store == nil {
		return nil
	}
	return s.store.ReconcileAttachmentBlobs(ctx)
}
