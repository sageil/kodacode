package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/sageil/kodacode/v1/internal/config"
	"github.com/sageil/kodacode/v1/internal/message"
	"github.com/sageil/kodacode/v1/internal/repository"
	"github.com/sageil/kodacode/v1/internal/snapshot"
)

type sessionTaskStore interface{ CleanupSession(string) }

type sessionTaskCloner interface {
	CloneSession(context.Context, string, string) error
}

type sessionStoreService struct {
	sessions repository.SessionRepo
	messages repository.MessageRepo

	settings    repository.SettingsRepo
	snapshotSvc *snapshot.Service
	attachments *sessionAttachmentService
	taskStore   sessionTaskStore
}

func newSessionStoreService(
	sessions repository.SessionRepo,
	messages repository.MessageRepo,
	projectDir string,
	cfg *config.Config,
) *sessionStoreService {
	return &sessionStoreService{
		sessions:    sessions,
		messages:    messages,
		attachments: newSessionAttachmentService(projectDir, cfg),
	}
}

func (s *sessionStoreService) SetSettings(settings repository.SettingsRepo) {
	if s == nil {
		return
	}
	s.settings = settings
}

func (s *sessionStoreService) SetAttachmentRepo(ar repository.AttachmentRepo) {
	if s == nil || s.attachments == nil {
		return
	}
	s.attachments.SetRepo(ar)
}

func (s *sessionStoreService) SetSnapshotService(svc *snapshot.Service) {
	if s == nil {
		return
	}
	s.snapshotSvc = svc
}

func (s *sessionStoreService) SnapshotService() *snapshot.Service {
	if s == nil {
		return nil
	}
	return s.snapshotSvc
}

func (s *sessionStoreService) SetTaskStore(ts sessionTaskStore) {
	if s == nil {
		return
	}
	s.taskStore = ts
}

func (s *sessionStoreService) CleanupSession(id string) {
	if s == nil || id == "" {
		return
	}
	if s.attachments != nil {
		s.attachments.CleanupSession(id)
	}
	if s.taskStore != nil {
		s.taskStore.CleanupSession(id)
	}
}

func (s *sessionStoreService) Create(ctx context.Context, defaultAgent, agentID, modelID string, opts ...CreateOption) (repository.Session, error) {
	if agentID == "" {
		agentID = defaultAgent
	}
	if modelID == "" {
		return repository.Session{}, fmt.Errorf("no model selected")
	}
	sess := repository.Session{
		AgentID: agentID,
		ModelID: modelID,
	}
	for _, opt := range opts {
		opt(&sess)
	}
	created, err := s.sessions.Create(ctx, sess)
	if err != nil {
		return repository.Session{}, fmt.Errorf("session service create: %w", err)
	}
	return created, nil
}

func (s *sessionStoreService) Get(ctx context.Context, id string) (repository.Session, error) {
	sess, err := s.sessions.Get(ctx, id)
	if err != nil {
		return repository.Session{}, fmt.Errorf("session service get: %w", err)
	}
	return sess, nil
}

func (s *sessionStoreService) UpdateSession(ctx context.Context, sess repository.Session) error {
	return s.sessions.Update(ctx, sess)
}

func (s *sessionStoreService) List(ctx context.Context) ([]repository.Session, error) {
	sessions, err := s.sessions.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("session service list: %w", err)
	}
	return sessions, nil
}

func (s *sessionStoreService) Delete(ctx context.Context, id string) error {
	var (
		counts     map[string]attachmentRefCount
		err        error
		releaseErr error
	)
	if s.attachments != nil && s.attachments.refs != nil {
		err = s.attachments.withMutation(id, func() error {
			counts, err = s.attachments.sessionRefCounts(ctx, s.messages, id)
			if err != nil {
				return err
			}
			if err := s.sessions.Delete(ctx, id); err != nil {
				return err
			}
			releaseErr = s.attachments.releaseRefs(ctx, counts)
			return nil
		})
	} else {
		err = s.sessions.Delete(ctx, id)
	}
	if err != nil {
		return fmt.Errorf("session service delete: %w", err)
	}
	if releaseErr != nil {
		log.Printf("session delete: release attachments for %s failed: %v (refs=%v)", id, releaseErr, counts)
		if reconcileErr := s.ReconcileAttachmentBlobs(ctx); reconcileErr != nil {
			log.Printf("session delete: reconcile attachments after release failure for %s: %v", id, reconcileErr)
		}
	}

	s.CleanupSession(id)
	if s.snapshotSvc != nil {
		if err := s.snapshotSvc.Cleanup(id); err != nil {
			log.Printf("session delete: snapshot cleanup: %v", err)
		}
	}
	return nil
}

func (s *sessionStoreService) ListMessages(ctx context.Context, sessionID string) ([]repository.Message, error) {
	msgs, err := s.messages.ListMessagesWithParts(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("session service list messages: %w", err)
	}
	return msgs, nil
}

func (s *sessionStoreService) Branch(ctx context.Context, sessionID, messageID string) (repository.Session, error) {
	parent, err := s.sessions.Get(ctx, sessionID)
	if err != nil {
		return repository.Session{}, fmt.Errorf("session service branch: %w", err)
	}

	msgs, err := s.messages.ListMessagesWithParts(ctx, sessionID)
	if err != nil {
		return repository.Session{}, fmt.Errorf("session service branch list messages: %w", err)
	}

	foundBranchPoint := false
	for _, m := range msgs {
		if m.ID == messageID {
			foundBranchPoint = true
			break
		}
	}
	if !foundBranchPoint {
		return repository.Session{}, fmt.Errorf("session service branch: %w", repository.ErrNotFound)
	}

	branch, err := s.sessions.Create(ctx, repository.Session{
		AgentID:              parent.AgentID,
		ModelID:              parent.ModelID,
		ParentID:             sessionID,
		BranchPointMessageID: messageID,
		WorkflowState:        parent.WorkflowState,
	})
	if err != nil {
		return repository.Session{}, fmt.Errorf("session service branch create: %w", err)
	}
	rollbackBranch := func(branchErr error) (repository.Session, error) {
		if cleanupErr := s.rollbackFailedBranch(ctx, branch.ID); cleanupErr != nil {
			return repository.Session{}, fmt.Errorf("%w (branch rollback failed: %v)", branchErr, cleanupErr)
		}
		return repository.Session{}, branchErr
	}

	clonedMessageIDs := make(map[string]string, len(msgs))
	for _, m := range msgs {
		clonedCompactionParentID := ""
		if m.CompactionParentID != "" {
			clonedCompactionParentID = clonedMessageIDs[m.CompactionParentID]
		}
		cloned, err := s.messages.Create(ctx, repository.Message{
			SessionID:          branch.ID,
			Role:               m.Role,
			CompactionParentID: clonedCompactionParentID,
			Summary:            m.Summary,
		})
		if err != nil {
			return rollbackBranch(fmt.Errorf("session service branch copy message: %w", err))
		}
		clonedMessageIDs[m.ID] = cloned.ID
		for _, p := range m.Parts {
			if p.Type == "file" {
				rollbackFailed := false
				if err := s.WithAttachmentMutation(branch.ID, func() error {
					var trackedFile *message.FileContent
					var fc message.FileContent
					if err := json.Unmarshal([]byte(p.Content), &fc); err == nil && fc.StorageKey != "" {
						if err := s.TrackAttachmentRef(ctx, fc, 1); err != nil {
							return fmt.Errorf("track attachment: %w", err)
						}
						trackedFile = &fc
					}
					_, err := s.messages.CreatePart(ctx, repository.MessagePart{
						MessageID: cloned.ID,
						SessionID: branch.ID,
						Type:      p.Type,
						Content:   p.Content,
						Synthetic: p.Synthetic,
					})
					if err != nil {
						if trackedFile != nil {
							if rollbackErr := s.TrackAttachmentRef(ctx, *trackedFile, -1); rollbackErr != nil {
								rollbackFailed = true
								return fmt.Errorf("copy part: %w (attachment rollback failed: %v)", err, rollbackErr)
							}
						}
						return fmt.Errorf("copy part: %w", err)
					}
					return nil
				}); err != nil {
					if rollbackFailed {
						if reconcileErr := s.ReconcileAttachmentBlobs(ctx); reconcileErr != nil {
							log.Printf("session branch: reconcile attachments after rollback failure for %s: %v", branch.ID, reconcileErr)
						}
					}
					return rollbackBranch(fmt.Errorf("session service branch file part: %w", err))
				}
				continue
			}
			_, err := s.messages.CreatePart(ctx, repository.MessagePart{
				MessageID: cloned.ID,
				SessionID: branch.ID,
				Type:      p.Type,
				Content:   p.Content,
				Synthetic: p.Synthetic,
			})
			if err != nil {
				return rollbackBranch(fmt.Errorf("session service branch copy part: %w", err))
			}
		}
		if m.ID == messageID {
			break
		}
	}

	if cloner, ok := s.taskStore.(sessionTaskCloner); ok {
		if err := cloner.CloneSession(ctx, sessionID, branch.ID); err != nil {
			return rollbackBranch(fmt.Errorf("session service branch copy tasks: %w", err))
		}
	}
	if s.settings != nil {
		if raw, err := s.settings.Get(ctx, "pins:"+sessionID); err == nil && raw != "" {
			if err := s.settings.Set(ctx, "pins:"+branch.ID, raw); err != nil {
				return rollbackBranch(fmt.Errorf("session service branch copy pins: %w", err))
			}
		} else if err != nil && err != repository.ErrNotFound {
			return rollbackBranch(fmt.Errorf("session service branch load pins: %w", err))
		}
	}

	return branch, nil
}

func (s *sessionStoreService) rollbackFailedBranch(ctx context.Context, branchID string) error {
	if branchID == "" {
		return nil
	}
	var (
		counts       map[string]attachmentRefCount
		countsErr    error
		releaseErr   error
		partErr      error
		messageErr   error
		sessionErr   error
		reconcileErr error
	)
	if s.attachments != nil && s.attachments.refs != nil {
		counts, countsErr = s.attachments.sessionRefCounts(ctx, s.messages, branchID)
		if countsErr == nil {
			releaseErr = s.attachments.releaseRefs(ctx, counts)
			if releaseErr != nil {
				reconcileErr = s.ReconcileAttachmentBlobs(ctx)
			}
		}
	}
	if s.messages != nil {
		partErr = s.messages.DeletePartsBySession(ctx, branchID)
		messageErr = s.messages.DeleteBySession(ctx, branchID)
	}
	sessionErr = s.sessions.Delete(ctx, branchID)
	s.CleanupSession(branchID)
	switch {
	case countsErr != nil:
		return fmt.Errorf("count attachment refs: %w", countsErr)
	case releaseErr != nil && reconcileErr != nil:
		return fmt.Errorf("release attachment refs: %w (reconcile failed: %v)", releaseErr, reconcileErr)
	case releaseErr != nil:
		return fmt.Errorf("release attachment refs: %w", releaseErr)
	case partErr != nil:
		return fmt.Errorf("delete branch parts: %w", partErr)
	case messageErr != nil:
		return fmt.Errorf("delete branch messages: %w", messageErr)
	case sessionErr != nil && sessionErr != repository.ErrNotFound:
		return fmt.Errorf("delete branch session: %w", sessionErr)
	default:
		return nil
	}
}

func (s *sessionStoreService) ValidateAttachments(attachments []FileAttachment) ([]FileAttachment, error) {
	if s == nil || s.attachments == nil {
		return attachments, nil
	}
	return s.attachments.ValidateFiles(attachments)
}

func (s *sessionStoreService) WithAttachmentMutation(sessionID string, fn func() error) error {
	if s == nil || s.attachments == nil {
		return fn()
	}
	return s.attachments.withMutation(sessionID, fn)
}

func (s *sessionStoreService) WithGlobalAttachmentMutation(fn func() error) error {
	if s == nil || s.attachments == nil {
		return fn()
	}
	return s.attachments.withGlobalMutation(fn)
}

func (s *sessionStoreService) TrackAttachmentRef(ctx context.Context, fc message.FileContent, delta int) error {
	if s == nil || s.attachments == nil {
		return nil
	}
	return s.attachments.trackRef(ctx, fc, delta)
}

func (s *sessionStoreService) ReleaseSessionAttachmentRefs(ctx context.Context, sessionID string) {
	if s == nil || s.attachments == nil {
		return
	}
	s.attachments.releaseSessionRefs(ctx, s.messages, sessionID)
}

func (s *sessionStoreService) SessionAttachmentRefCounts(ctx context.Context, sessionID string) (map[string]attachmentRefCount, error) {
	if s == nil || s.attachments == nil {
		return nil, nil
	}
	return s.attachments.sessionRefCounts(ctx, s.messages, sessionID)
}

func (s *sessionStoreService) ReleaseAttachmentRefs(ctx context.Context, counts map[string]attachmentRefCount) error {
	if s == nil || s.attachments == nil {
		return nil
	}
	return s.attachments.releaseRefs(ctx, counts)
}

func (s *sessionStoreService) ReconcileAttachmentBlobs(ctx context.Context) error {
	if s == nil || s.attachments == nil {
		return nil
	}
	return s.attachments.Reconcile(ctx)
}
