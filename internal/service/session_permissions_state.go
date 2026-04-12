package service

import (
	"fmt"
	"strings"
	"sync"

	"github.com/sageil/kodacode/v1/internal/tool"
)

type sessionPermissionBroker struct {
	pendingMu sync.Mutex
	pending   map[string]pendingQuestion
	user      map[string]pendingUserQuestion

	approvedMu  sync.Mutex
	approved    map[string]map[string]bool
	deniedPaths map[string]map[string]bool
}

func newSessionPermissionBroker() *sessionPermissionBroker {
	return &sessionPermissionBroker{
		pending:     make(map[string]pendingQuestion),
		user:        make(map[string]pendingUserQuestion),
		approved:    make(map[string]map[string]bool),
		deniedPaths: make(map[string]map[string]bool),
	}
}

func (b *sessionPermissionBroker) CleanupSession(sessionID string) {
	b.pendingMu.Lock()
	for k, pq := range b.pending {
		if pq.sessionID == sessionID {
			delete(b.pending, k)
		}
	}
	for k, uq := range b.user {
		if uq.sessionID == sessionID {
			delete(b.user, k)
		}
	}
	b.pendingMu.Unlock()

	b.approvedMu.Lock()
	delete(b.approved, sessionID)
	delete(b.deniedPaths, sessionID)
	b.approvedMu.Unlock()
}

func (b *sessionPermissionBroker) SetPendingUserQuestion(questionID string, pq pendingUserQuestion) {
	b.pendingMu.Lock()
	b.user[questionID] = pq
	b.pendingMu.Unlock()
}

func (b *sessionPermissionBroker) DeletePendingUserQuestion(questionID string) {
	b.pendingMu.Lock()
	delete(b.user, questionID)
	b.pendingMu.Unlock()
}

func (b *sessionPermissionBroker) SetPendingPermission(questionID string, pq pendingQuestion) {
	b.pendingMu.Lock()
	b.pending[questionID] = pq
	b.pendingMu.Unlock()
}

func (b *sessionPermissionBroker) DeletePendingPermission(questionID string) {
	b.pendingMu.Lock()
	delete(b.pending, questionID)
	b.pendingMu.Unlock()
}

func (b *sessionPermissionBroker) Lookup(questionID string) (pendingQuestion, bool, pendingUserQuestion, bool, int, int) {
	b.pendingMu.Lock()
	defer b.pendingMu.Unlock()
	pq, pqOK := b.pending[questionID]
	uq, uqOK := b.user[questionID]
	return pq, pqOK, uq, uqOK, len(b.pending), len(b.user)
}

func (b *sessionPermissionBroker) DeliverUserAnswer(questionID string, response AnswerResponse) error {
	b.pendingMu.Lock()
	uq, ok := b.user[questionID]
	b.pendingMu.Unlock()
	if !ok {
		return fmt.Errorf("answer: question %q not found", questionID)
	}
	select {
	case uq.ch <- string(response):
		return nil
	default:
		return fmt.Errorf("answer: question %q already answered or expired", questionID)
	}
}

func (b *sessionPermissionBroker) DeliverPermissionAnswer(sessionID string, questionID string, response AnswerResponse) error {
	b.pendingMu.Lock()
	pq, ok := b.pending[questionID]
	b.pendingMu.Unlock()
	if !ok {
		return fmt.Errorf("answer: question %q not found", questionID)
	}

	var val error
	switch response {
	case AnswerReject:
		val = tool.ErrDenied
	case AnswerAlways:
		for _, pattern := range pq.patterns {
			if pattern != "" {
				b.StoreApproval(sessionID, pq.tool+":"+pattern)
			}
		}
	}

	select {
	case pq.ch <- val:
		return nil
	default:
		return fmt.Errorf("answer: question %q already answered or expired", questionID)
	}
}

func (b *sessionPermissionBroker) StoreApproval(sessionID, key string) {
	b.approvedMu.Lock()
	if b.approved[sessionID] == nil {
		b.approved[sessionID] = make(map[string]bool)
	}
	b.approved[sessionID][key] = true
	b.approvedMu.Unlock()
}

func (b *sessionPermissionBroker) DenyPath(sessionID, path string) {
	b.approvedMu.Lock()
	if b.deniedPaths[sessionID] == nil {
		b.deniedPaths[sessionID] = make(map[string]bool)
	}
	b.deniedPaths[sessionID][path] = true
	b.approvedMu.Unlock()
}

func (b *sessionPermissionBroker) IsPathDenied(sessionID, parentSessionID, path string) bool {
	b.approvedMu.Lock()
	defer b.approvedMu.Unlock()
	if denied := b.deniedPaths[sessionID]; denied != nil {
		for deniedPath := range denied {
			if pathSubjectContains(deniedPath, path) {
				return true
			}
		}
	}
	if parentSessionID != "" {
		if denied := b.deniedPaths[parentSessionID]; denied != nil {
			for deniedPath := range denied {
				if pathSubjectContains(deniedPath, path) {
					return true
				}
			}
		}
	}
	return false
}

func (b *sessionPermissionBroker) IsApproved(sessionID, parentSessionID, toolName string, patterns []string) bool {
	if len(patterns) == 0 {
		return false
	}
	b.approvedMu.Lock()
	defer b.approvedMu.Unlock()
	for _, pattern := range patterns {
		if pattern == "" {
			return false
		}
		key := toolName + ":" + pattern
		if approved := b.approved[sessionID]; approved != nil && approved[key] {
			continue
		}
		if parentSessionID != "" {
			if approved := b.approved[parentSessionID]; approved != nil && approved[key] {
				continue
			}
		}
		return false
	}
	return true
}

func pathSubjectContains(parent, child string) bool {
	if parent == "" || child == "" {
		return false
	}
	if parent == child {
		return true
	}
	return strings.HasPrefix(child, parent+"/")
}
