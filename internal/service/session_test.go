package service_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"github.com/sageil/kodacode/v1/internal/config"
	"github.com/sageil/kodacode/v1/internal/message"
	"github.com/sageil/kodacode/v1/internal/pipeline"
	"github.com/sageil/kodacode/v1/internal/provider"
	"github.com/sageil/kodacode/v1/internal/repository"
	"github.com/sageil/kodacode/v1/internal/sandbox"
	"github.com/sageil/kodacode/v1/internal/service"
	"github.com/sageil/kodacode/v1/internal/tool"
)

type fakeSessionRepo struct {
	mu       sync.Mutex
	sessions map[string]repository.Session
	created  []repository.Session
	nextID   int
}

func newFakeSessionRepo() *fakeSessionRepo {
	return &fakeSessionRepo{sessions: make(map[string]repository.Session)}
}

func (f *fakeSessionRepo) Create(_ context.Context, s repository.Session) (repository.Session, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.nextID++
	s.ID = fmt.Sprintf("s%d", f.nextID)
	s.CreatedAt = time.Now()
	s.UpdatedAt = s.CreatedAt
	f.sessions[s.ID] = s
	f.created = append(f.created, s)
	return s, nil
}

func (f *fakeSessionRepo) Get(_ context.Context, id string) (repository.Session, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	s, ok := f.sessions[id]
	if !ok {
		return repository.Session{}, repository.ErrNotFound
	}
	return s, nil
}

func (f *fakeSessionRepo) List(_ context.Context) ([]repository.Session, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]repository.Session, 0, len(f.sessions))
	for _, s := range f.sessions {
		out = append(out, s)
	}
	return out, nil
}

func (f *fakeSessionRepo) Update(_ context.Context, s repository.Session) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.sessions[s.ID]; !ok {
		return repository.ErrNotFound
	}
	s.UpdatedAt = time.Now()
	f.sessions[s.ID] = s
	return nil
}

func (f *fakeSessionRepo) Delete(_ context.Context, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.sessions[id]; !ok {
		return repository.ErrNotFound
	}
	delete(f.sessions, id)
	return nil
}

func (f *fakeSessionRepo) createdSessions() []repository.Session {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]repository.Session, len(f.created))
	copy(out, f.created)
	return out
}

func (f *fakeSessionRepo) DeleteEphemeral(_ context.Context) (int, error) { return 0, nil }

func (f *fakeSessionRepo) UpdateCost(_ context.Context, id string, inputTokens, outputTokens, lastInputTokens int, totalCost float64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	s, ok := f.sessions[id]
	if !ok {
		return repository.ErrNotFound
	}
	s.TotalInputTokens = inputTokens
	s.TotalOutputTokens = outputTokens
	s.TotalCost = totalCost
	f.sessions[id] = s
	return nil
}

func (f *fakeSessionRepo) UpdateWorkflow(_ context.Context, id, workflowState string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	s, ok := f.sessions[id]
	if !ok {
		return repository.ErrNotFound
	}
	s.WorkflowState = workflowState
	s.UpdatedAt = time.Now()
	f.sessions[id] = s
	return nil
}

func validPNGData() []byte {
	return []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}
}

type fakeMessageRepo struct {
	mu       sync.Mutex
	messages map[string]repository.Message
	parts    map[string]repository.MessagePart
	nextID   int

	failCreatePart func(repository.MessagePart) error
}

func newFakeMessageRepo() *fakeMessageRepo {
	return &fakeMessageRepo{
		messages: make(map[string]repository.Message),
		parts:    make(map[string]repository.MessagePart),
	}
}

func (f *fakeMessageRepo) Create(_ context.Context, m repository.Message) (repository.Message, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.nextID++
	m.ID = fmt.Sprintf("m%d", f.nextID)
	m.CreatedAt = time.Now()
	f.messages[m.ID] = m
	return m, nil
}

func (f *fakeMessageRepo) CreateWithParts(ctx context.Context, m repository.Message, parts []repository.MessagePart) (repository.Message, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.nextID++
	created := m
	created.ID = fmt.Sprintf("m%d", f.nextID)
	created.CreatedAt = time.Now()
	f.messages[created.ID] = created
	created.Parts = make([]repository.MessagePart, 0, len(parts))
	for _, p := range parts {
		p.MessageID = created.ID
		if p.SessionID == "" {
			p.SessionID = created.SessionID
		}
		if f.failCreatePart != nil {
			if err := f.failCreatePart(p); err != nil {
				delete(f.messages, created.ID)
				for id, stored := range f.parts {
					if stored.MessageID == created.ID {
						delete(f.parts, id)
					}
				}
				return repository.Message{}, err
			}
		}
		f.nextID++
		part := p
		part.ID = fmt.Sprintf("p%d", f.nextID)
		part.CreatedAt = time.Now()
		f.parts[part.ID] = part
		created.Parts = append(created.Parts, part)
	}
	f.messages[created.ID] = created
	return created, nil
}

func (f *fakeMessageRepo) Get(_ context.Context, id string) (repository.Message, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	m, ok := f.messages[id]
	if !ok {
		return repository.Message{}, repository.ErrNotFound
	}
	return m, nil
}

func (f *fakeMessageRepo) ListBySession(_ context.Context, sessionID string) ([]repository.Message, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []repository.Message
	for _, m := range f.messages {
		if m.SessionID == sessionID {
			out = append(out, m)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (f *fakeMessageRepo) DeleteBySession(_ context.Context, sessionID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for id, m := range f.messages {
		if m.SessionID == sessionID {
			delete(f.messages, id)
		}
	}
	return nil
}

func (f *fakeMessageRepo) ListMessagesWithParts(_ context.Context, sessionID string) ([]repository.Message, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var messages []repository.Message
	for _, m := range f.messages {
		if m.SessionID == sessionID {
			messages = append(messages, m)
		}
	}
	sort.Slice(messages, func(i, j int) bool { return messages[i].ID < messages[j].ID })

	partsByMessage := make(map[string][]repository.MessagePart)
	for _, p := range f.parts {
		if p.SessionID == sessionID {
			partsByMessage[p.MessageID] = append(partsByMessage[p.MessageID], p)
		}
	}
	for i := range messages {
		messages[i].Parts = partsByMessage[messages[i].ID]
	}
	return messages, nil
}

func (f *fakeMessageRepo) CreatePart(_ context.Context, p repository.MessagePart) (repository.MessagePart, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failCreatePart != nil {
		if err := f.failCreatePart(p); err != nil {
			return repository.MessagePart{}, err
		}
	}
	f.nextID++
	p.ID = fmt.Sprintf("p%d", f.nextID)
	p.CreatedAt = time.Now()
	f.parts[p.ID] = p
	return p, nil
}

func (f *fakeMessageRepo) ListPartsByMessage(_ context.Context, messageID string) ([]repository.MessagePart, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []repository.MessagePart
	for _, p := range f.parts {
		if p.MessageID == messageID {
			out = append(out, p)
		}
	}
	return out, nil
}

func (f *fakeMessageRepo) ListPartsBySession(_ context.Context, sessionID string) ([]repository.MessagePart, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []repository.MessagePart
	for _, p := range f.parts {
		if p.SessionID == sessionID {
			out = append(out, p)
		}
	}
	return out, nil
}

func (f *fakeMessageRepo) UpdatePart(_ context.Context, p repository.MessagePart) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.parts[p.ID]; !ok {
		return repository.ErrNotFound
	}
	f.parts[p.ID] = p
	return nil
}

func (f *fakeMessageRepo) DeletePart(_ context.Context, _ string) error { return nil }

func (f *fakeMessageRepo) BatchUpdateParts(_ context.Context, parts []repository.MessagePart) error {
	for _, p := range parts {
		if err := f.UpdatePart(context.Background(), p); err != nil {
			return err
		}
	}
	return nil
}

func (f *fakeMessageRepo) DeletePartsBySession(_ context.Context, sessionID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for id, p := range f.parts {
		if p.SessionID == sessionID {
			delete(f.parts, id)
		}
	}
	return nil
}

type fakeAttachmentRepo struct {
	blobs            map[string]repository.AttachmentBlob
	fileRefs         []repository.AttachmentFileRef
	failNegativeRefs error
	reconcileCalls   int
}

func newFakeAttachmentRepo() *fakeAttachmentRepo {
	return &fakeAttachmentRepo{blobs: make(map[string]repository.AttachmentBlob)}
}

func (f *fakeAttachmentRepo) ApplyDeltas(_ context.Context, deltas []repository.AttachmentRefDelta) ([]repository.AttachmentBlob, error) {
	var zero []repository.AttachmentBlob
	for _, delta := range deltas {
		if delta.StorageKey == "" || delta.Delta == 0 {
			continue
		}
		if delta.Delta < 0 && f.failNegativeRefs != nil {
			return nil, f.failNegativeRefs
		}
		blob := f.blobs[delta.StorageKey]
		blob.StorageKey = delta.StorageKey
		if delta.MimeType != "" {
			blob.MimeType = delta.MimeType
		}
		if delta.Size > 0 {
			blob.Size = delta.Size
		}
		blob.RefCount += delta.Delta
		blob.UpdatedAt = time.Now()
		f.blobs[delta.StorageKey] = blob
		if blob.RefCount <= 0 {
			zero = append(zero, blob)
		}
	}
	return zero, nil
}

func (f *fakeAttachmentRepo) List(_ context.Context) ([]repository.AttachmentBlob, error) {
	out := make([]repository.AttachmentBlob, 0, len(f.blobs))
	for _, blob := range f.blobs {
		out = append(out, blob)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].StorageKey < out[j].StorageKey })
	return out, nil
}

func (f *fakeAttachmentRepo) Delete(_ context.Context, storageKeys []string) error {
	for _, key := range storageKeys {
		delete(f.blobs, key)
	}
	return nil
}

func (f *fakeAttachmentRepo) Reconcile(_ context.Context) error {
	f.reconcileCalls++
	f.blobs = make(map[string]repository.AttachmentBlob)
	for _, ref := range f.fileRefs {
		if ref.StorageKey == "" {
			continue
		}
		blob := f.blobs[ref.StorageKey]
		blob.StorageKey = ref.StorageKey
		blob.MimeType = ref.MimeType
		blob.Size = ref.Size
		blob.RefCount++
		f.blobs[ref.StorageKey] = blob
	}
	return nil
}

func (f *fakeAttachmentRepo) ListFileRefs(_ context.Context) ([]repository.AttachmentFileRef, error) {
	out := make([]repository.AttachmentFileRef, len(f.fileRefs))
	copy(out, f.fileRefs)
	return out, nil
}

func (f *fakeAttachmentRepo) ReplaceAll(_ context.Context, blobs []repository.AttachmentBlob) error {
	f.blobs = make(map[string]repository.AttachmentBlob, len(blobs))
	for _, blob := range blobs {
		f.blobs[blob.StorageKey] = blob
	}
	return nil
}

type failingBranchTaskStore struct {
	mu         sync.Mutex
	clonedTo   string
	cleaned    map[string]bool
	branchData map[string][]string
}

func newFailingBranchTaskStore() *failingBranchTaskStore {
	return &failingBranchTaskStore{
		cleaned:    make(map[string]bool),
		branchData: make(map[string][]string),
	}
}

func (s *failingBranchTaskStore) CleanupSession(sessionID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cleaned[sessionID] = true
	delete(s.branchData, sessionID)
}

func (s *failingBranchTaskStore) CloneSession(_ context.Context, _, toSessionID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.clonedTo = toSessionID
	s.branchData[toSessionID] = []string{"Write tests"}
	return errors.New("task clone failed")
}

func (s *failingBranchTaskStore) clonedSessionID() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.clonedTo
}

func (s *failingBranchTaskStore) cleanedSession(sessionID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cleaned[sessionID]
}

func (s *failingBranchTaskStore) tasks(sessionID string) []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.branchData[sessionID]...)
}

type fakeProvider struct {
	id     string
	chunks []provider.StreamChunk
}

type subagentTestAgentLookup struct {
	cfg  config.AgentConfig
	mode string
	err  error
}

func (f subagentTestAgentLookup) Get(_ string) (config.AgentConfig, error) {
	return f.cfg, f.err
}

func (f subagentTestAgentLookup) Mode(_ string) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	return f.mode, nil
}

func (p *fakeProvider) ID() string   { return p.id }
func (p *fakeProvider) Name() string { return p.id }
func (p *fakeProvider) Models(_ context.Context) ([]provider.Model, error) {
	return []provider.Model{{ID: "fake-model", Name: "Fake", ContextSize: 8192}}, nil
}

func (p *fakeProvider) Chat(_ context.Context, model string, _ []provider.Message, _ provider.ChatOptions) (<-chan provider.StreamChunk, error) {
	_ = model
	ch := make(chan provider.StreamChunk, len(p.chunks))
	for _, c := range p.chunks {
		ch <- c
	}
	close(ch)
	return ch, nil
}

func f64ptr(v float64) *float64 { return &v }
func intptr(v int) *int         { return &v }
func boolPtr(v bool) *bool      { return &v }

func testCfg() *config.Config {
	return &config.Config{
		DefaultAgent: "default",
		Session: config.SessionConfig{
			CompactionThreshold: f64ptr(0.8),
			CompactionKeepTurns: intptr(10),
		},
	}
}

func testRegistry(p provider.Provider) *provider.Registry {
	reg := provider.NewRegistry()
	_ = reg.Register(p)
	return reg
}

func TestSessionService_Create(t *testing.T) {
	ctx := context.Background()
	svc := service.NewSessionService(
		newFakeSessionRepo(),
		newFakeMessageRepo(),
		testRegistry(&fakeProvider{id: "fake"}),
		testCfg(),
		nil,
		nil,
		t.TempDir(), pipeline.BuildChain(),
	)

	sess, err := svc.Create(ctx, "default", "fake/fake-model")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if sess.ID == "" {
		t.Error("Create() returned session with empty ID")
	}
	if sess.AgentID != "default" {
		t.Errorf("Create() AgentID = %q, want %q", sess.AgentID, "default")
	}
	if sess.ModelID != "fake/fake-model" {
		t.Errorf("Create() ModelID = %q, want %q", sess.ModelID, "fake/fake-model")
	}
}

func TestSessionService_Create_defaultsApplied(t *testing.T) {
	ctx := context.Background()
	svc := service.NewSessionService(
		newFakeSessionRepo(),
		newFakeMessageRepo(),
		testRegistry(&fakeProvider{id: "fake"}),
		testCfg(),
		nil,
		nil,
		t.TempDir(), pipeline.BuildChain(),
	)

	sess, err := svc.Create(ctx, "", "fake/fake-model")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if sess.AgentID != "default" {
		t.Errorf("Create() AgentID = %q, want %q", sess.AgentID, "default")
	}
	if sess.ModelID != "fake/fake-model" {
		t.Errorf("Create() ModelID = %q, want %q", sess.ModelID, "fake/fake-model")
	}
}

func TestSessionService_Get_existing(t *testing.T) {
	ctx := context.Background()
	svc := service.NewSessionService(
		newFakeSessionRepo(),
		newFakeMessageRepo(),
		testRegistry(&fakeProvider{id: "fake"}),
		testCfg(),
		nil,
		nil,
		t.TempDir(), pipeline.BuildChain(),
	)

	created, err := svc.Create(ctx, "default", "fake/fake-model")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	got, err := svc.Get(ctx, created.ID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	ignoreTime := cmpopts.IgnoreFields(repository.Session{}, "CreatedAt", "UpdatedAt")
	if diff := cmp.Diff(created, got, ignoreTime); diff != "" {
		t.Errorf("Get() mismatch (-want +got):\n%s", diff)
	}
}

func TestSessionService_Get_notFound(t *testing.T) {
	ctx := context.Background()
	svc := service.NewSessionService(
		newFakeSessionRepo(),
		newFakeMessageRepo(),
		testRegistry(&fakeProvider{id: "fake"}),
		testCfg(),
		nil,
		nil,
		t.TempDir(), pipeline.BuildChain(),
	)

	_, err := svc.Get(ctx, "nonexistent")
	if !errors.Is(err, repository.ErrNotFound) {
		t.Errorf("Get() error = %v, want ErrNotFound", err)
	}
}

func TestSessionService_List(t *testing.T) {
	ctx := context.Background()
	svc := service.NewSessionService(
		newFakeSessionRepo(),
		newFakeMessageRepo(),
		testRegistry(&fakeProvider{id: "fake"}),
		testCfg(),
		nil,
		nil,
		t.TempDir(), pipeline.BuildChain(),
	)

	_, _ = svc.Create(ctx, "default", "fake/fake-model")
	_, _ = svc.Create(ctx, "coder", "fake/fake-model")

	sessions, err := svc.List(ctx)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(sessions) != 2 {
		t.Errorf("List() returned %d sessions, want 2", len(sessions))
	}
}

func TestSessionService_Delete(t *testing.T) {
	ctx := context.Background()
	svc := service.NewSessionService(
		newFakeSessionRepo(),
		newFakeMessageRepo(),
		testRegistry(&fakeProvider{id: "fake"}),
		testCfg(),
		nil,
		nil,
		t.TempDir(), pipeline.BuildChain(),
	)

	sess, _ := svc.Create(ctx, "default", "fake/fake-model")

	if err := svc.Delete(ctx, sess.ID); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	_, err := svc.Get(ctx, sess.ID)
	if !errors.Is(err, repository.ErrNotFound) {
		t.Errorf("Get() after Delete() error = %v, want ErrNotFound", err)
	}
}

func TestSessionService_Delete_notFound(t *testing.T) {
	ctx := context.Background()
	svc := service.NewSessionService(
		newFakeSessionRepo(),
		newFakeMessageRepo(),
		testRegistry(&fakeProvider{id: "fake"}),
		testCfg(),
		nil,
		nil,
		t.TempDir(), pipeline.BuildChain(),
	)

	err := svc.Delete(ctx, "nonexistent")
	if !errors.Is(err, repository.ErrNotFound) {
		t.Errorf("Delete() error = %v, want ErrNotFound", err)
	}
}

func TestSessionService_Branch(t *testing.T) {
	ctx := context.Background()
	msgRepo := newFakeMessageRepo()
	svc := service.NewSessionService(
		newFakeSessionRepo(),
		msgRepo,
		testRegistry(&fakeProvider{id: "fake"}),
		testCfg(),
		nil,
		nil,
		t.TempDir(), pipeline.BuildChain(),
	)

	parent, _ := svc.Create(ctx, "default", "fake/fake-model")
	parent.WorkflowState = `{"phase":"approved","has_called_planner":true}`
	if err := svc.UpdateSession(ctx, parent); err != nil {
		t.Fatalf("UpdateSession() error = %v", err)
	}
	m1, _ := msgRepo.Create(ctx, repository.Message{SessionID: parent.ID, Role: "user"})
	if _, err := msgRepo.CreatePart(ctx, repository.MessagePart{
		MessageID: m1.ID, SessionID: parent.ID, Type: "text", Content: `{"text":"hello"}`,
	}); err != nil {
		t.Fatalf("CreatePart() error = %v", err)
	}
	m2, _ := msgRepo.Create(ctx, repository.Message{SessionID: parent.ID, Role: "assistant"})
	if _, err := msgRepo.CreatePart(ctx, repository.MessagePart{
		MessageID: m2.ID, SessionID: parent.ID, Type: "text", Content: `{"text":"hi there"}`,
	}); err != nil {
		t.Fatalf("CreatePart() error = %v", err)
	}
	if _, err := msgRepo.CreatePart(ctx, repository.MessagePart{
		MessageID: m2.ID, SessionID: parent.ID, Type: "tool_call",
		Content: `{"id":"c1","name":"bash","arguments":"{}"}`,
	}); err != nil {
		t.Fatalf("CreatePart() error = %v", err)
	}
	m3, _ := msgRepo.Create(ctx, repository.Message{SessionID: parent.ID, Role: "user"})
	if _, err := msgRepo.CreatePart(ctx, repository.MessagePart{
		MessageID: m3.ID, SessionID: parent.ID, Type: "tool_result",
		Content: `{"tool_call_id":"c1","output":"ok"}`,
	}); err != nil {
		t.Fatalf("CreatePart() error = %v", err)
	}

	branch, err := svc.Branch(ctx, parent.ID, m2.ID)
	if err != nil {
		t.Fatalf("Branch() error = %v", err)
	}
	if branch.ID == parent.ID {
		t.Error("Branch() returned same ID as parent")
	}
	if branch.ParentID != parent.ID {
		t.Errorf("Branch() ParentID = %q, want %q", branch.ParentID, parent.ID)
	}
	if branch.BranchPointMessageID != m2.ID {
		t.Errorf("Branch() BranchPointMessageID = %q, want %q", branch.BranchPointMessageID, m2.ID)
	}
	if branch.WorkflowState != parent.WorkflowState {
		t.Errorf("Branch() WorkflowState = %q, want %q", branch.WorkflowState, parent.WorkflowState)
	}

	msgs, err := msgRepo.ListMessagesWithParts(ctx, branch.ID)
	if err != nil {
		t.Fatalf("ListMessagesWithParts() error = %v", err)
	}
	if len(msgs) != 2 {
		t.Fatalf("Branch session has %d messages, want 2", len(msgs))
	}

	var roles []string
	for _, m := range msgs {
		roles = append(roles, m.Role)
	}
	wantRoles := []string{m1.Role, m2.Role}
	if diff := cmp.Diff(wantRoles, roles, cmpopts.SortSlices(func(a, b string) bool { return a < b })); diff != "" {
		t.Errorf("Branch message roles mismatch (-want +got):\n%s", diff)
	}

	partsByRole := make(map[string][]repository.MessagePart)
	for _, m := range msgs {
		partsByRole[m.Role] = m.Parts
	}

	if len(partsByRole["user"]) != 1 {
		t.Errorf("user clone has %d parts, want 1", len(partsByRole["user"]))
	} else if partsByRole["user"][0].Content != `{"text":"hello"}` {
		t.Errorf("user clone part content = %q, want hello text", partsByRole["user"][0].Content)
	}

	if len(partsByRole["assistant"]) != 2 {
		t.Errorf("assistant clone has %d parts, want 2", len(partsByRole["assistant"]))
	}

	branchParts, _ := msgRepo.ListPartsBySession(ctx, branch.ID)
	if len(branchParts) != 3 {
		t.Errorf("branch session has %d total parts, want 3", len(branchParts))
	}
	for _, p := range branchParts {
		if p.SessionID != branch.ID {
			t.Errorf("cloned part SessionID = %q, want %q", p.SessionID, branch.ID)
		}
	}
}

func TestSessionService_BranchCopiesPinsAndTasks(t *testing.T) {
	ctx := context.Background()
	sessRepo := newFakeSessionRepo()
	msgRepo := newFakeMessageRepo()
	settingsRepo := newFakeSettingsRepo()
	taskStore := tool.NewTaskStore(nil)
	svc := service.NewSessionService(
		sessRepo,
		msgRepo,
		testRegistry(&fakeProvider{id: "fake"}),
		testCfg(),
		nil,
		nil,
		t.TempDir(),
		pipeline.BuildChain(),
	)
	svc.SetSettings(settingsRepo)
	svc.SetTaskStore(taskStore)

	parent, _ := svc.Create(ctx, "default", "fake/fake-model")
	m1, _ := msgRepo.Create(ctx, repository.Message{SessionID: parent.ID, Role: "user"})
	textContent, _ := message.MarshalContent(message.TextContent{Text: "hello"})
	if _, err := msgRepo.CreatePart(ctx, repository.MessagePart{
		MessageID: m1.ID,
		SessionID: parent.ID,
		Type:      "text",
		Content:   textContent,
	}); err != nil {
		t.Fatalf("CreatePart() error = %v", err)
	}

	if err := settingsRepo.Set(ctx, "pins:"+parent.ID, `["stay focused","no shortcuts"]`); err != nil {
		t.Fatalf("Set(pins) error = %v", err)
	}
	taskTool := tool.NewTaskTool(taskStore)
	if _, err := taskTool.Execute(ctx, tool.ExecutionContext{SessionID: parent.ID}, []byte(`{"action":"create","title":"Write tests"}`)); err != nil {
		t.Fatalf("task create error = %v", err)
	}
	if _, err := taskTool.Execute(ctx, tool.ExecutionContext{SessionID: parent.ID}, []byte(`{"action":"create","title":"Review changes","status":"in_progress"}`)); err != nil {
		t.Fatalf("task create error = %v", err)
	}

	branch, err := svc.Branch(ctx, parent.ID, m1.ID)
	if err != nil {
		t.Fatalf("Branch() error = %v", err)
	}

	gotPins, err := settingsRepo.Get(ctx, "pins:"+branch.ID)
	if err != nil {
		t.Fatalf("Get(branch pins) error = %v", err)
	}
	if gotPins != `["stay focused","no shortcuts"]` {
		t.Fatalf("branch pins = %q, want parent pins", gotPins)
	}

	branchTasks := taskStore.GetTasks(branch.ID)
	if len(branchTasks) != 2 {
		t.Fatalf("branch tasks len = %d, want 2", len(branchTasks))
	}
	if branchTasks[0].Title != "Write tests" {
		t.Fatalf("branch task[0].Title = %q, want %q", branchTasks[0].Title, "Write tests")
	}
	if branchTasks[1].Status != "in_progress" {
		t.Fatalf("branch task[1].Status = %q, want %q", branchTasks[1].Status, "in_progress")
	}
}

func TestSessionService_BranchRemapsCompactionParentIDs(t *testing.T) {
	ctx := context.Background()
	msgRepo := newFakeMessageRepo()
	svc := service.NewSessionService(
		newFakeSessionRepo(),
		msgRepo,
		testRegistry(&fakeProvider{id: "fake"}),
		testCfg(),
		nil,
		nil,
		t.TempDir(),
		pipeline.BuildChain(),
	)

	parent, _ := svc.Create(ctx, "default", "fake/fake-model")
	oldUser, _ := msgRepo.Create(ctx, repository.Message{SessionID: parent.ID, Role: "user"})
	if _, err := msgRepo.CreatePart(ctx, repository.MessagePart{
		MessageID: oldUser.ID, SessionID: parent.ID, Type: "text", Content: `{"text":"old question"}`,
	}); err != nil {
		t.Fatalf("CreatePart() oldUser error = %v", err)
	}
	oldAnswer, _ := msgRepo.Create(ctx, repository.Message{SessionID: parent.ID, Role: "assistant"})
	if _, err := msgRepo.CreatePart(ctx, repository.MessagePart{
		MessageID: oldAnswer.ID, SessionID: parent.ID, Type: "text", Content: `{"text":"old answer"}`,
	}); err != nil {
		t.Fatalf("CreatePart() oldAnswer error = %v", err)
	}
	summary, _ := msgRepo.Create(ctx, repository.Message{
		SessionID:          parent.ID,
		Role:               "assistant",
		Summary:            true,
		CompactionParentID: oldAnswer.ID,
	})
	if _, err := msgRepo.CreatePart(ctx, repository.MessagePart{
		MessageID: summary.ID, SessionID: parent.ID, Type: "text", Content: `{"text":"summary"}`,
	}); err != nil {
		t.Fatalf("CreatePart() summary error = %v", err)
	}
	newUser, _ := msgRepo.Create(ctx, repository.Message{SessionID: parent.ID, Role: "user"})
	if _, err := msgRepo.CreatePart(ctx, repository.MessagePart{
		MessageID: newUser.ID, SessionID: parent.ID, Type: "text", Content: `{"text":"new question"}`,
	}); err != nil {
		t.Fatalf("CreatePart() newUser error = %v", err)
	}

	branch, err := svc.Branch(ctx, parent.ID, newUser.ID)
	if err != nil {
		t.Fatalf("Branch() error = %v", err)
	}

	msgs, err := msgRepo.ListMessagesWithParts(ctx, branch.ID)
	if err != nil {
		t.Fatalf("ListMessagesWithParts(branch) error = %v", err)
	}

	var clonedOldAnswerID string
	var clonedSummaryCutoff string
	for _, m := range msgs {
		if m.Summary {
			clonedSummaryCutoff = m.CompactionParentID
			continue
		}
		for _, p := range m.Parts {
			if p.Content == `{"text":"old answer"}` {
				clonedOldAnswerID = m.ID
			}
		}
	}

	if clonedOldAnswerID == "" {
		t.Fatal("did not find cloned old-answer message in branch")
	}
	if clonedSummaryCutoff != clonedOldAnswerID {
		t.Fatalf("branch summary cutoff = %q, want cloned old-answer ID %q", clonedSummaryCutoff, clonedOldAnswerID)
	}
}

func TestSessionService_Branch_parentNotFound(t *testing.T) {
	ctx := context.Background()
	svc := service.NewSessionService(
		newFakeSessionRepo(),
		newFakeMessageRepo(),
		testRegistry(&fakeProvider{id: "fake"}),
		testCfg(),
		nil,
		nil,
		t.TempDir(), pipeline.BuildChain(),
	)

	_, err := svc.Branch(ctx, "nope", "also-nope")
	if !errors.Is(err, repository.ErrNotFound) {
		t.Errorf("Branch() error = %v, want ErrNotFound", err)
	}
}

func TestSessionService_Branch_messageNotFound(t *testing.T) {
	ctx := context.Background()
	sessRepo := newFakeSessionRepo()
	msgRepo := newFakeMessageRepo()
	svc := service.NewSessionService(
		sessRepo,
		msgRepo,
		testRegistry(&fakeProvider{id: "fake"}),
		testCfg(),
		nil,
		nil,
		t.TempDir(), pipeline.BuildChain(),
	)

	parent, _ := svc.Create(ctx, "default", "fake/fake-model")
	msg, _ := msgRepo.Create(ctx, repository.Message{SessionID: parent.ID, Role: "user"})
	if _, err := msgRepo.CreatePart(ctx, repository.MessagePart{
		MessageID: msg.ID, SessionID: parent.ID, Type: "text", Content: `{"text":"hello"}`,
	}); err != nil {
		t.Fatalf("CreatePart() error = %v", err)
	}

	_, err := svc.Branch(ctx, parent.ID, "missing-message")
	if !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("Branch() error = %v, want ErrNotFound", err)
	}
	if len(sessRepo.sessions) != 1 {
		t.Fatalf("session count after failed branch = %d, want 1", len(sessRepo.sessions))
	}
}

func TestSessionService_Branch_returnsBusyWhenTurnActive(t *testing.T) {
	ctx := context.Background()
	sessRepo := newFakeSessionRepo()
	msgRepo := newFakeMessageRepo()
	svc := service.NewSessionService(
		sessRepo,
		msgRepo,
		testRegistry(&fakeProvider{id: "fake"}),
		testCfg(),
		nil,
		nil,
		t.TempDir(),
		pipeline.BuildChain(),
	)

	parent, _ := svc.Create(ctx, "default", "fake/fake-model")
	msg, _ := msgRepo.Create(ctx, repository.Message{SessionID: parent.ID, Role: "user"})
	if _, err := msgRepo.CreatePart(ctx, repository.MessagePart{
		MessageID: msg.ID, SessionID: parent.ID, Type: "text", Content: `{"text":"hello"}`,
	}); err != nil {
		t.Fatalf("CreatePart() error = %v", err)
	}

	res, err := svc.ReserveSend(parent.ID)
	if err != nil {
		t.Fatalf("ReserveSend() error = %v", err)
	}
	defer res.Release()

	_, err = svc.Branch(ctx, parent.ID, msg.ID)
	if !errors.Is(err, service.ErrSessionBusy) {
		t.Fatalf("Branch() error = %v, want ErrSessionBusy", err)
	}
	if len(sessRepo.sessions) != 1 {
		t.Fatalf("session count after busy branch = %d, want 1", len(sessRepo.sessions))
	}
}

func TestSessionService_Branch_rollsBackOnTaskCloneFailure(t *testing.T) {
	ctx := context.Background()
	sessRepo := newFakeSessionRepo()
	msgRepo := newFakeMessageRepo()
	settingsRepo := newFakeSettingsRepo()
	taskStore := newFailingBranchTaskStore()
	svc := service.NewSessionService(
		sessRepo,
		msgRepo,
		testRegistry(&fakeProvider{id: "fake"}),
		testCfg(),
		nil,
		nil,
		t.TempDir(),
		pipeline.BuildChain(),
	)
	svc.SetSettings(settingsRepo)
	svc.SetTaskStore(taskStore)

	parent, _ := svc.Create(ctx, "default", "fake/fake-model")
	msg, _ := msgRepo.Create(ctx, repository.Message{SessionID: parent.ID, Role: "user"})
	if _, err := msgRepo.CreatePart(ctx, repository.MessagePart{
		MessageID: msg.ID, SessionID: parent.ID, Type: "text", Content: `{"text":"hello"}`,
	}); err != nil {
		t.Fatalf("CreatePart() error = %v", err)
	}
	if err := settingsRepo.Set(ctx, "pins:"+parent.ID, `["stay focused"]`); err != nil {
		t.Fatalf("Set(parent pins) error = %v", err)
	}

	_, err := svc.Branch(ctx, parent.ID, msg.ID)
	if err == nil {
		t.Fatal("Branch() error = nil, want task clone failure")
	}
	if got := err.Error(); !strings.Contains(got, "copy tasks") {
		t.Fatalf("Branch() error = %q, want task clone failure", got)
	}

	branchID := taskStore.clonedSessionID()
	if branchID == "" {
		t.Fatal("expected failing task clone to capture branch session ID")
	}
	if !taskStore.cleanedSession(branchID) {
		t.Fatalf("CleanupSession(%q) was not called", branchID)
	}
	if got := taskStore.tasks(branchID); len(got) != 0 {
		t.Fatalf("branch in-memory tasks = %#v, want empty after rollback", got)
	}
	if _, ok := sessRepo.sessions[branchID]; ok {
		t.Fatalf("branch session %q still present after rollback", branchID)
	}
	msgs, err := msgRepo.ListMessagesWithParts(ctx, branchID)
	if err != nil {
		t.Fatalf("ListMessagesWithParts(branch) error = %v", err)
	}
	if len(msgs) != 0 {
		t.Fatalf("branch messages after rollback = %d, want 0", len(msgs))
	}
	parts, err := msgRepo.ListPartsBySession(ctx, branchID)
	if err != nil {
		t.Fatalf("ListPartsBySession(branch) error = %v", err)
	}
	if len(parts) != 0 {
		t.Fatalf("branch parts after rollback = %d, want 0", len(parts))
	}
	if _, err := settingsRepo.Get(ctx, "pins:"+branchID); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("branch pins error = %v, want ErrNotFound", err)
	}
}

func TestSessionService_Send_returnsNilWithNoopChain(t *testing.T) {
	ctx := context.Background()
	fp := &fakeProvider{
		id: "fake",
		chunks: []provider.StreamChunk{
			{Delta: "hello "},
			{Delta: "world"},
			{FinishReason: "stop", Usage: &provider.Usage{InputTokens: 10, OutputTokens: 5}},
		},
	}
	svc := service.NewSessionService(
		newFakeSessionRepo(),
		newFakeMessageRepo(),
		testRegistry(fp),
		testCfg(),
		nil,
		nil,
		t.TempDir(), pipeline.BuildChain(),
	)

	sess, _ := svc.Create(ctx, "default", "fake/fake-model")

	if err := svc.Send(ctx, sess.ID, "hi", nil, sandbox.OriginTUI); err != nil {
		t.Fatalf("Send() error = %v, want nil", err)
	}
}

func TestSessionService_Send_savesUserMessageToRepo(t *testing.T) {
	ctx := context.Background()
	fp := &fakeProvider{
		id: "fake",
		chunks: []provider.StreamChunk{
			{Delta: "hello"},
			{FinishReason: "stop", Usage: &provider.Usage{InputTokens: 5, OutputTokens: 2}},
		},
	}
	msgRepo := newFakeMessageRepo()
	svc := service.NewSessionService(
		newFakeSessionRepo(),
		msgRepo,
		testRegistry(fp),
		testCfg(),
		nil,
		nil,
		t.TempDir(), pipeline.BuildChain(),
	)

	sess, _ := svc.Create(ctx, "default", "fake/fake-model")

	if err := svc.Send(ctx, sess.ID, "ping", nil, sandbox.OriginTUI); err != nil {
		t.Fatalf("Send() error = %v", err)
	}

	msgs, _ := msgRepo.ListBySession(ctx, sess.ID)
	if len(msgs) != 1 {
		t.Errorf("after Send(), got %d messages in repo, want 1 (user only, noop chain)", len(msgs))
	}

	if len(msgs) > 0 && msgs[0].Role != "user" {
		t.Errorf("first message role = %q, want %q", msgs[0].Role, "user")
	}
}

func TestSessionService_Send_doesNotPersistUserMessageWhenModelValidationFails(t *testing.T) {
	ctx := context.Background()
	msgRepo := newFakeMessageRepo()
	svc := service.NewSessionService(
		newFakeSessionRepo(),
		msgRepo,
		testRegistry(&fakeProvider{id: "fake"}),
		testCfg(),
		nil,
		nil,
		t.TempDir(), pipeline.BuildChain(),
	)

	sess, _ := svc.Create(ctx, "default", "missing/fake-model")

	err := svc.Send(ctx, sess.ID, "ping", nil, sandbox.OriginTUI)
	if err == nil {
		t.Fatal("Send() error = nil, want provider validation failure")
	}
	if !strings.Contains(err.Error(), `provider "missing" not registered`) {
		t.Fatalf("Send() error = %q, want missing provider error", err)
	}

	msgs, err := msgRepo.ListBySession(ctx, sess.ID)
	if err != nil {
		t.Fatalf("ListBySession() error = %v", err)
	}
	if len(msgs) != 0 {
		t.Fatalf("persisted messages after failed Send() = %d, want 0", len(msgs))
	}

	parts, err := msgRepo.ListPartsBySession(ctx, sess.ID)
	if err != nil {
		t.Fatalf("ListPartsBySession() error = %v", err)
	}
	if len(parts) != 0 {
		t.Fatalf("persisted parts after failed Send() = %d, want 0", len(parts))
	}
}

func TestSessionService_Send_persistsAttachmentMetadataOnly(t *testing.T) {
	ctx := context.Background()
	projectDir := t.TempDir()
	msgRepo := newFakeMessageRepo()
	attRepo := newFakeAttachmentRepo()
	svc := service.NewSessionService(
		newFakeSessionRepo(),
		msgRepo,
		testRegistry(&fakeProvider{id: "fake"}),
		testCfg(),
		nil,
		nil,
		projectDir, pipeline.BuildChain(),
	)
	svc.SetAttachmentRepo(attRepo)

	sess, _ := svc.Create(ctx, "default", "fake/fake-model")
	att := service.FileAttachment{
		Path:     filepath.Join(projectDir, "diagram.png"),
		MimeType: "image/png",
		Data:     validPNGData(),
	}
	if err := svc.Send(ctx, sess.ID, "see attachment", []service.FileAttachment{att}, sandbox.OriginTUI); err != nil {
		t.Fatalf("Send() error = %v", err)
	}

	parts, err := msgRepo.ListPartsBySession(ctx, sess.ID)
	if err != nil {
		t.Fatalf("ListPartsBySession() error = %v", err)
	}
	if len(parts) != 2 {
		t.Fatalf("parts = %d, want 2", len(parts))
	}

	var filePart repository.MessagePart
	for _, p := range parts {
		if p.Type == "file" {
			filePart = p
			break
		}
	}
	if filePart.ID == "" {
		t.Fatal("expected persisted file part")
	}
	content, err := message.UnmarshalContent("file", filePart.Content)
	if err != nil {
		t.Fatalf("UnmarshalContent(file) error = %v", err)
	}
	fc := content.(message.FileContent)
	if fc.URL != "" {
		t.Fatalf("persisted file URL = %q, want empty metadata-only storage", fc.URL)
	}
	if fc.StorageKey == "" {
		t.Fatal("expected storage_key to be persisted")
	}
	if fc.Size != int64(len(att.Data)) {
		t.Fatalf("size = %d, want %d", fc.Size, len(att.Data))
	}
	storedPath := filepath.Join(projectDir, ".kodacode", "attachments", fc.StorageKey)
	if _, err := os.Stat(storedPath); err != nil {
		t.Fatalf("stored attachment %s missing: %v", storedPath, err)
	}
	blobs, err := attRepo.List(ctx)
	if err != nil {
		t.Fatalf("attachment refs List() error = %v", err)
	}
	if len(blobs) != 1 || blobs[0].RefCount != 1 {
		t.Fatalf("attachment refs = %#v, want one blob with refcount 1", blobs)
	}
}

func TestSessionService_AttachmentRefsSurviveBranchUntilLastSessionDelete(t *testing.T) {
	ctx := context.Background()
	projectDir := t.TempDir()
	msgRepo := newFakeMessageRepo()
	attRepo := newFakeAttachmentRepo()
	svc := service.NewSessionService(
		newFakeSessionRepo(),
		msgRepo,
		testRegistry(&fakeProvider{id: "fake"}),
		testCfg(),
		nil,
		nil,
		projectDir, pipeline.BuildChain(),
	)
	svc.SetAttachmentRepo(attRepo)

	parent, _ := svc.Create(ctx, "default", "fake/fake-model")
	att := service.FileAttachment{
		Path:     filepath.Join(projectDir, "diagram.png"),
		MimeType: "image/png",
		Data:     validPNGData(),
	}
	if err := svc.Send(ctx, parent.ID, "see attachment", []service.FileAttachment{att}, sandbox.OriginTUI); err != nil {
		t.Fatalf("Send() error = %v", err)
	}

	parentMsgs, err := msgRepo.ListBySession(ctx, parent.ID)
	if err != nil || len(parentMsgs) == 0 {
		t.Fatalf("ListBySession(parent) = %v, %d messages", err, len(parentMsgs))
	}
	branch, err := svc.Branch(ctx, parent.ID, parentMsgs[0].ID)
	if err != nil {
		t.Fatalf("Branch() error = %v", err)
	}

	parts, err := msgRepo.ListPartsBySession(ctx, parent.ID)
	if err != nil {
		t.Fatalf("ListPartsBySession(parent) error = %v", err)
	}
	var storageKey string
	for _, part := range parts {
		if part.Type != "file" {
			continue
		}
		content, err := message.UnmarshalContent("file", part.Content)
		if err != nil {
			t.Fatalf("UnmarshalContent(file) error = %v", err)
		}
		storageKey = content.(message.FileContent).StorageKey
		break
	}
	if storageKey == "" {
		t.Fatal("expected persisted attachment storage key")
	}

	blobs, err := attRepo.List(ctx)
	if err != nil {
		t.Fatalf("attachment refs after branch error = %v", err)
	}
	if len(blobs) != 1 || blobs[0].RefCount != 2 {
		t.Fatalf("attachment refs after branch = %#v, want one blob with refcount 2", blobs)
	}

	if err := svc.Delete(ctx, parent.ID); err != nil {
		t.Fatalf("Delete(parent) error = %v", err)
	}
	blobs, err = attRepo.List(ctx)
	if err != nil {
		t.Fatalf("attachment refs after parent delete error = %v", err)
	}
	if len(blobs) != 1 || blobs[0].RefCount != 1 {
		t.Fatalf("attachment refs after parent delete = %#v, want one blob with refcount 1", blobs)
	}
	storedPath := filepath.Join(projectDir, ".kodacode", "attachments", storageKey)
	if _, err := os.Stat(storedPath); err != nil {
		t.Fatalf("stored attachment should still exist after parent delete: %v", err)
	}

	if err := svc.Delete(ctx, branch.ID); err != nil {
		t.Fatalf("Delete(branch) error = %v", err)
	}
	blobs, err = attRepo.List(ctx)
	if err != nil {
		t.Fatalf("attachment refs after branch delete error = %v", err)
	}
	if len(blobs) != 0 {
		t.Fatalf("attachment refs after branch delete = %#v, want empty", blobs)
	}
	if _, err := os.Stat(storedPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("stored attachment should be removed after last delete, stat err = %v", err)
	}
}

func TestSessionService_BranchTriggersReconcileWhenAttachmentRollbackFails(t *testing.T) {
	ctx := context.Background()
	projectDir := t.TempDir()
	msgRepo := newFakeMessageRepo()
	attRepo := newFakeAttachmentRepo()
	svc := service.NewSessionService(
		newFakeSessionRepo(),
		msgRepo,
		testRegistry(&fakeProvider{id: "fake"}),
		testCfg(),
		nil,
		nil,
		projectDir, pipeline.BuildChain(),
	)
	svc.SetAttachmentRepo(attRepo)

	parent, _ := svc.Create(ctx, "default", "fake/fake-model")
	att := service.FileAttachment{
		Path:     filepath.Join(projectDir, "diagram.png"),
		MimeType: "image/png",
		Data:     validPNGData(),
	}
	if err := svc.Send(ctx, parent.ID, "see attachment", []service.FileAttachment{att}, sandbox.OriginTUI); err != nil {
		t.Fatalf("Send() error = %v", err)
	}

	parentMsgs, err := msgRepo.ListMessagesWithParts(ctx, parent.ID)
	if err != nil || len(parentMsgs) == 0 {
		t.Fatalf("ListMessagesWithParts(parent) = %v, %d messages", err, len(parentMsgs))
	}

	var fc message.FileContent
	for _, part := range parentMsgs[0].Parts {
		if part.Type != "file" {
			continue
		}
		content, err := message.UnmarshalContent("file", part.Content)
		if err != nil {
			t.Fatalf("UnmarshalContent(file) error = %v", err)
		}
		fc = content.(message.FileContent)
		break
	}
	if fc.StorageKey == "" {
		t.Fatal("expected persisted attachment storage key")
	}
	attRepo.fileRefs = []repository.AttachmentFileRef{{
		StorageKey: fc.StorageKey,
		MimeType:   fc.MimeType,
		Size:       fc.Size,
	}}

	msgRepo.failCreatePart = func(p repository.MessagePart) error {
		if p.SessionID != parent.ID && p.Type == "file" {
			return errors.New("copy failed")
		}
		return nil
	}
	attRepo.failNegativeRefs = errors.New("rollback failed")

	_, err = svc.Branch(ctx, parent.ID, parentMsgs[0].ID)
	if err == nil {
		t.Fatal("Branch() error = nil, want rollback failure")
	}
	if got := err.Error(); !strings.Contains(got, "attachment rollback failed") {
		t.Fatalf("Branch() error = %q, want attachment rollback failure", got)
	}
	if attRepo.reconcileCalls != 1 {
		t.Fatalf("reconcileCalls = %d, want 1", attRepo.reconcileCalls)
	}
	blobs, err := attRepo.List(ctx)
	if err != nil {
		t.Fatalf("attachment refs after reconcile error = %v", err)
	}
	if len(blobs) != 1 || blobs[0].RefCount != 1 {
		t.Fatalf("attachment refs after failed branch = %#v, want one blob with refcount 1", blobs)
	}
}

func TestSessionService_ReconcileAttachmentBlobsRepairsMetadataAndPrunesOrphans(t *testing.T) {
	ctx := context.Background()
	projectDir := t.TempDir()
	attRepo := newFakeAttachmentRepo()
	svc := service.NewSessionService(
		newFakeSessionRepo(),
		newFakeMessageRepo(),
		testRegistry(&fakeProvider{id: "fake"}),
		testCfg(),
		nil,
		nil,
		projectDir, pipeline.BuildChain(),
	)
	svc.SetAttachmentRepo(attRepo)

	livePath := filepath.Join(projectDir, ".kodacode", "attachments", "live.png")
	stalePath := filepath.Join(projectDir, ".kodacode", "attachments", "stale.png")
	if err := os.MkdirAll(filepath.Dir(livePath), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(livePath, []byte("live"), 0o644); err != nil {
		t.Fatalf("WriteFile(live) error = %v", err)
	}
	if err := os.WriteFile(stalePath, []byte("stale"), 0o644); err != nil {
		t.Fatalf("WriteFile(stale) error = %v", err)
	}

	attRepo.blobs["stale.png"] = repository.AttachmentBlob{
		StorageKey: "stale.png",
		MimeType:   "image/png",
		Size:       5,
		RefCount:   3,
	}
	attRepo.fileRefs = []repository.AttachmentFileRef{{
		StorageKey: "live.png",
		MimeType:   "image/png",
		Size:       4,
	}}

	if err := svc.ReconcileAttachmentBlobs(ctx); err != nil {
		t.Fatalf("ReconcileAttachmentBlobs() error = %v", err)
	}

	blobs, err := attRepo.List(ctx)
	if err != nil {
		t.Fatalf("attachment refs after reconcile error = %v", err)
	}
	if diff := cmp.Diff([]repository.AttachmentBlob{{
		StorageKey: "live.png",
		MimeType:   "image/png",
		Size:       4,
		RefCount:   1,
	}}, blobs, cmpopts.IgnoreFields(repository.AttachmentBlob{}, "UpdatedAt")); diff != "" {
		t.Fatalf("reconciled blobs mismatch (-want +got):\n%s", diff)
	}
	if _, err := os.Stat(stalePath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("stale attachment should be removed, stat err = %v", err)
	}
	if _, err := os.Stat(livePath); err != nil {
		t.Fatalf("live attachment should remain, stat err = %v", err)
	}
}

func TestSessionService_DeleteTriggersAttachmentReconcileOnReleaseFailure(t *testing.T) {
	ctx := context.Background()
	msgRepo := newFakeMessageRepo()
	attRepo := newFakeAttachmentRepo()
	attRepo.failNegativeRefs = errors.New("release failed")

	svc := service.NewSessionService(
		newFakeSessionRepo(),
		msgRepo,
		testRegistry(&fakeProvider{id: "fake"}),
		testCfg(),
		nil,
		nil,
		t.TempDir(), pipeline.BuildChain(),
	)
	svc.SetAttachmentRepo(attRepo)

	sess, _ := svc.Create(ctx, "default", "fake/fake-model")
	msg, err := msgRepo.Create(ctx, repository.Message{SessionID: sess.ID, Role: "user"})
	if err != nil {
		t.Fatalf("Create() message error = %v", err)
	}
	fileContent, _ := message.MarshalContent(message.FileContent{
		Path:       "diagram.png",
		MimeType:   "image/png",
		StorageKey: "diagram.png",
		Size:       4,
	})
	if _, err := msgRepo.CreatePart(ctx, repository.MessagePart{
		MessageID: msg.ID,
		SessionID: sess.ID,
		Type:      "file",
		Content:   fileContent,
	}); err != nil {
		t.Fatalf("CreatePart() error = %v", err)
	}
	attRepo.fileRefs = []repository.AttachmentFileRef{{
		StorageKey: "diagram.png",
		MimeType:   "image/png",
		Size:       4,
	}}

	if err := svc.Delete(ctx, sess.ID); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if attRepo.reconcileCalls != 1 {
		t.Fatalf("reconcileCalls = %d, want 1", attRepo.reconcileCalls)
	}
	if _, err := svc.Get(ctx, sess.ID); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("Get() after delete err = %v, want ErrNotFound", err)
	}
}

func TestSessionService_SpawnSubagentCleansUpEphemeralSessionState(t *testing.T) {
	ctx := context.Background()
	sessRepo := newFakeSessionRepo()
	msgRepo := newFakeMessageRepo()

	var svc *service.SessionService
	chain := pipeline.BuildChain(func(ctx context.Context, req *pipeline.TurnRequest, next pipeline.TurnHandler) error {
		svc.GetOrCreateCost(ctx, req.SessionID)
		svc.GetOrCreateTraces(req.SessionID).CommitTurn([]service.StepTrace{{Step: 1, ModelID: req.Model.ID}})
		return next(ctx, req)
	})

	svc = service.NewSessionService(
		sessRepo,
		msgRepo,
		testRegistry(&fakeProvider{id: "fake"}),
		testCfg(),
		nil,
		subagentTestAgentLookup{cfg: config.AgentConfig{SystemPrompt: "helper"}, mode: "subagent"},
		t.TempDir(), chain,
	)

	parent, err := svc.Create(ctx, "default", "fake/fake-model")
	if err != nil {
		t.Fatalf("Create(parent) error = %v", err)
	}

	if _, err := svc.SpawnSubagent(ctx, parent.ID, "default", "do the task", nil); err != nil {
		t.Fatalf("SpawnSubagent() error = %v", err)
	}

	const subagentSessionID = "s2"
	if _, ok := svc.GetSessionCost(subagentSessionID); ok {
		t.Fatalf("subagent cost for %s still present after cleanup", subagentSessionID)
	}
	if turns := svc.GetSessionTraces(subagentSessionID); len(turns) != 0 {
		t.Fatalf("subagent traces for %s still present after cleanup: %+v", subagentSessionID, turns)
	}
	if _, err := sessRepo.Get(ctx, subagentSessionID); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("subagent session delete err = %v, want ErrNotFound", err)
	}
}

func TestSpawnSubagentRejectsPrimaryAgentMode(t *testing.T) {
	ctx := context.Background()
	sessRepo := newFakeSessionRepo()
	svc := service.NewSessionService(
		sessRepo,
		newFakeMessageRepo(),
		testRegistry(&fakeProvider{id: "fake"}),
		testCfg(),
		nil,
		subagentTestAgentLookup{
			cfg:  config.AgentConfig{SystemPrompt: "helper"},
			mode: "primary",
		},
		t.TempDir(),
		pipeline.BuildChain(),
	)

	parent, err := svc.Create(ctx, "default", "fake/fake-model")
	if err != nil {
		t.Fatalf("Create(parent) error = %v", err)
	}

	if _, err := svc.SpawnSubagent(ctx, parent.ID, "default", "do the task", nil); err == nil {
		t.Fatal("SpawnSubagent() error = nil, want primary-agent rejection")
	}
}

func TestSpawnSubagentPlannerUsesParentModelWhenConfiguredUtility(t *testing.T) {
	ctx := context.Background()
	sessRepo := newFakeSessionRepo()
	cfg := testCfg()
	cfg.UtilityModel = "fake/utility-model"
	svc := service.NewSessionService(
		sessRepo,
		newFakeMessageRepo(),
		testRegistry(&fakeProvider{id: "fake"}),
		cfg,
		nil,
		subagentTestAgentLookup{
			cfg:  config.AgentConfig{SystemPrompt: "planner", Model: "utility"},
			mode: "subagent",
		},
		t.TempDir(),
		pipeline.BuildChain(),
	)

	parent, err := svc.Create(ctx, "engineer", "fake/fake-model")
	if err != nil {
		t.Fatalf("Create(parent) error = %v", err)
	}

	if _, err := svc.SpawnSubagent(ctx, parent.ID, "planner", "make a plan", nil); err != nil {
		t.Fatalf("SpawnSubagent() error = %v", err)
	}

	created := sessRepo.createdSessions()
	if len(created) != 2 {
		t.Fatalf("created session count = %d, want 2", len(created))
	}
	if got := created[1].ModelID; got != "fake/fake-model" {
		t.Fatalf("planner session model = %q, want %q", got, "fake/fake-model")
	}
}

func TestSpawnSubagentPlannerIgnoresExplicitModelOverride(t *testing.T) {
	ctx := context.Background()
	sessRepo := newFakeSessionRepo()
	svc := service.NewSessionService(
		sessRepo,
		newFakeMessageRepo(),
		testRegistry(&fakeProvider{id: "fake"}),
		testCfg(),
		nil,
		subagentTestAgentLookup{
			cfg:  config.AgentConfig{SystemPrompt: "planner", Model: "other/explicit-model"},
			mode: "subagent",
		},
		t.TempDir(),
		pipeline.BuildChain(),
	)

	parent, err := svc.Create(ctx, "engineer", "fake/fake-model")
	if err != nil {
		t.Fatalf("Create(parent) error = %v", err)
	}

	if _, err := svc.SpawnSubagent(ctx, parent.ID, "planner", "make a plan", nil); err != nil {
		t.Fatalf("SpawnSubagent() error = %v", err)
	}

	created := sessRepo.createdSessions()
	if len(created) != 2 {
		t.Fatalf("created session count = %d, want 2", len(created))
	}
	if got := created[1].ModelID; got != "fake/fake-model" {
		t.Fatalf("planner session model = %q, want %q", got, "fake/fake-model")
	}
}

func TestTitleUpdateAfterSessionDelete(t *testing.T) {
	ctx := context.Background()
	sessRepo := newFakeSessionRepo()
	var published []service.SSEEvent

	updateTitle := func(ctx context.Context, sessionID, title string) {
		sess, err := sessRepo.Get(ctx, sessionID)
		if err != nil {
			return
		}
		sess.Title = title
		if err := sessRepo.Update(ctx, sess); err != nil {
			return
		}
		published = append(published, service.SSEEvent{
			Type: "title_updated",
			Data: service.SSETitleData{Title: title},
		})
	}

	sess, err := sessRepo.Create(ctx, repository.Session{AgentID: "default", ModelID: "fake/model"})
	if err != nil {
		t.Fatal(err)
	}

	updateTitle(ctx, sess.ID, "First Title")
	got, _ := sessRepo.Get(ctx, sess.ID)
	if got.Title != "First Title" {
		t.Errorf("title = %q, want %q", got.Title, "First Title")
	}
	if len(published) != 1 {
		t.Fatalf("published events = %d, want 1", len(published))
	}

	if err := sessRepo.Delete(ctx, sess.ID); err != nil {
		t.Fatal(err)
	}

	updateTitle(ctx, sess.ID, "Ghost Title")

	if len(published) != 1 {
		t.Errorf("published events after delete = %d, want 1 (no new event)", len(published))
	}

	_, err = sessRepo.Get(ctx, sess.ID)
	if !errors.Is(err, repository.ErrNotFound) {
		t.Errorf("Get after delete: err = %v, want ErrNotFound", err)
	}
}
