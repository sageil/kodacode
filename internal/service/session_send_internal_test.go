package service

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/sageil/kodacode/v1/internal/config"
	"github.com/sageil/kodacode/v1/internal/pipeline"
	"github.com/sageil/kodacode/v1/internal/provider"
	"github.com/sageil/kodacode/v1/internal/repository"
	"github.com/sageil/kodacode/v1/internal/sandbox"
)

type sendInternalSessionRepo struct {
	session repository.Session
}

type failingAtomicMessageRepo struct {
	turnLoopMessageRepo
	err error
}

func (r *failingAtomicMessageRepo) CreateWithParts(_ context.Context, _ repository.Message, _ []repository.MessagePart) (repository.Message, error) {
	if r.err == nil {
		r.err = errors.New("atomic write failed")
	}
	return repository.Message{}, r.err
}

type sendInternalAttachmentProvider struct {
	sendInternalProvider
	caps provider.AttachmentCapabilities
}

func (p *sendInternalAttachmentProvider) AttachmentCapabilities(_ provider.Model) provider.AttachmentCapabilities {
	return p.caps
}

type sendInternalProvider struct {
	id     string
	models []provider.Model
}

func (p *sendInternalProvider) ID() string   { return p.id }
func (p *sendInternalProvider) Name() string { return p.id }
func (p *sendInternalProvider) Models(_ context.Context) ([]provider.Model, error) {
	if len(p.models) > 0 {
		return p.models, nil
	}
	return []provider.Model{{ID: "fake-model", Name: "Fake Model", ContextSize: 8192}}, nil
}

func (p *sendInternalProvider) Chat(_ context.Context, _ string, _ []provider.Message, _ provider.ChatOptions) (<-chan provider.StreamChunk, error) {
	ch := make(chan provider.StreamChunk)
	close(ch)
	return ch, nil
}

func (r *sendInternalSessionRepo) Create(_ context.Context, s repository.Session) (repository.Session, error) {
	if s.ID == "" {
		s.ID = "sess-1"
	}
	if s.CreatedAt.IsZero() {
		now := time.Now()
		s.CreatedAt = now
		s.UpdatedAt = now
	}
	r.session = s
	return s, nil
}

func (r *sendInternalSessionRepo) Get(_ context.Context, id string) (repository.Session, error) {
	if r.session.ID == "" || r.session.ID != id {
		return repository.Session{}, repository.ErrNotFound
	}
	return r.session, nil
}

func (r *sendInternalSessionRepo) List(_ context.Context) ([]repository.Session, error) {
	if r.session.ID == "" {
		return nil, nil
	}
	return []repository.Session{r.session}, nil
}

func (r *sendInternalSessionRepo) Update(_ context.Context, s repository.Session) error {
	if r.session.ID == "" || r.session.ID != s.ID {
		return repository.ErrNotFound
	}
	r.session = s
	return nil
}

func (r *sendInternalSessionRepo) Delete(_ context.Context, id string) error {
	if r.session.ID == "" || r.session.ID != id {
		return repository.ErrNotFound
	}
	r.session = repository.Session{}
	return nil
}

func (r *sendInternalSessionRepo) DeleteEphemeral(_ context.Context) (int, error) { return 0, nil }

func (r *sendInternalSessionRepo) UpdateCost(_ context.Context, id string, _, _, _ int, _ float64) error {
	if r.session.ID == "" || r.session.ID != id {
		return repository.ErrNotFound
	}
	return nil
}

func (r *sendInternalSessionRepo) UpdateWorkflow(_ context.Context, id, workflowState string) error {
	if r.session.ID == "" || r.session.ID != id {
		return repository.ErrNotFound
	}
	r.session.WorkflowState = workflowState
	return nil
}

func validPNGData() []byte {
	return []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}
}

func TestBuildCurrentTurnPartsKeepsCurrentTurnAttachmentsAsFileParts(t *testing.T) {
	parts := buildCurrentTurnParts("see attachment", []FileAttachment{{
		Path:     "/tmp/diagram.png",
		MimeType: "image/png",
		Data:     []byte("png-bytes"),
	}})
	if len(parts) != 2 {
		t.Fatalf("parts len = %d, want 2", len(parts))
	}
	if _, ok := parts[0].(provider.TextPart); !ok {
		t.Fatalf("parts[0] type = %T, want provider.TextPart", parts[0])
	}
	filePart, ok := parts[1].(provider.FilePart)
	if !ok {
		t.Fatalf("parts[1] type = %T, want provider.FilePart", parts[1])
	}
	if filePart.MimeType != "image/png" {
		t.Fatalf("file mime_type = %q, want %q", filePart.MimeType, "image/png")
	}
	if !strings.HasPrefix(filePart.URL, "data:image/png;base64,") {
		t.Fatalf("file URL = %q, want data URL", filePart.URL)
	}
}

func TestSendRejectsAttachmentsForEphemeralSessions(t *testing.T) {
	ctx := context.WithValue(context.Background(), ephemeralKey{}, true)
	sessRepo := &sendInternalSessionRepo{
		session: repository.Session{
			ID:        "sess-1",
			AgentID:   "default",
			ModelID:   "fake/fake-model",
			Ephemeral: true,
		},
	}
	svc := NewSessionService(
		sessRepo,
		&turnLoopMessageRepo{},
		nil,
		nil,
		nil,
		nil,
		t.TempDir(), nil,
	)

	err := svc.Send(ctx, sessRepo.session.ID, "see attachment", []FileAttachment{{
		Path:     "/tmp/diagram.png",
		MimeType: "image/png",
		Data:     validPNGData(),
	}}, sandbox.OriginTUI)
	if err == nil {
		t.Fatal("Send() error = nil, want ephemeral attachment rejection")
	}
	if !strings.Contains(err.Error(), "ephemeral sessions do not support attachments") {
		t.Fatalf("Send() error = %q, want attachment rejection", err)
	}

	msgs, err := svc.ListMessages(context.Background(), sessRepo.session.ID)
	if !errors.Is(err, repository.ErrNotFound) && len(msgs) != 0 {
		t.Fatalf("ListMessages() = %v, %v; want no persisted ephemeral messages", msgs, err)
	}
}

func TestSendRejectsConcurrentTurnsForSameSession(t *testing.T) {
	ctx := context.Background()
	sessRepo := &sendInternalSessionRepo{
		session: repository.Session{
			ID:      "sess-1",
			AgentID: "default",
			ModelID: "fake/fake-model",
		},
	}

	registry := provider.NewRegistry()
	if err := registry.Register(&sendInternalProvider{id: "fake"}); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	started := make(chan struct{}, 1)
	release := make(chan struct{})
	var once sync.Once
	chain := pipeline.BuildChain(func(ctx context.Context, req *pipeline.TurnRequest, next pipeline.TurnHandler) error {
		once.Do(func() { started <- struct{}{} })
		<-release
		return next(ctx, req)
	})

	svc := NewSessionService(
		sessRepo,
		&turnLoopMessageRepo{},
		registry,
		nil,
		nil,
		nil,
		t.TempDir(),
		chain,
	)

	errCh := make(chan error, 1)
	go func() {
		errCh <- svc.Send(ctx, sessRepo.session.ID, "first", nil, sandbox.OriginTUI)
	}()

	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for first Send() to reserve the session")
	}

	err := svc.Send(ctx, sessRepo.session.ID, "second", nil, sandbox.OriginTUI)
	if !errors.Is(err, ErrSessionBusy) {
		t.Fatalf("second Send() error = %v, want ErrSessionBusy", err)
	}

	close(release)
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("first Send() error = %v, want nil", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for first Send() to finish")
	}
}

func TestSendRejectsUnsupportedAttachmentsBeforePersistence(t *testing.T) {
	ctx := context.Background()
	sessRepo := &sendInternalSessionRepo{
		session: repository.Session{
			ID:      "sess-1",
			AgentID: "default",
			ModelID: "fake/fake-model",
		},
	}

	registry := provider.NewRegistry()
	if err := registry.Register(&sendInternalAttachmentProvider{
		sendInternalProvider: sendInternalProvider{id: "fake"},
		caps: provider.AttachmentCapabilities{
			Images: true,
			Text:   true,
		},
	}); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	msgRepo := &turnLoopMessageRepo{}
	svc := NewSessionService(
		sessRepo,
		msgRepo,
		registry,
		nil,
		nil,
		nil,
		t.TempDir(),
		nil,
	)

	err := svc.Send(ctx, sessRepo.session.ID, "see attachment", []FileAttachment{{
		Path:     "/tmp/archive.bin",
		MimeType: "application/octet-stream",
		Data:     []byte("bin"),
	}}, sandbox.OriginTUI)
	if err == nil {
		t.Fatal("Send() error = nil, want unsupported attachment rejection")
	}
	if !strings.Contains(err.Error(), "does not support application/octet-stream attachments") {
		t.Fatalf("Send() error = %q, want capability rejection", err)
	}
	if len(msgRepo.messages) != 0 {
		t.Fatalf("persisted messages after attachment rejection = %d, want 0", len(msgRepo.messages))
	}
}

func TestPrepareSendRejectsUnsupportedAttachmentsAndReleasesReservation(t *testing.T) {
	ctx := context.Background()
	sessRepo := &sendInternalSessionRepo{
		session: repository.Session{
			ID:      "sess-1",
			AgentID: "default",
			ModelID: "fake/fake-model",
		},
	}

	registry := provider.NewRegistry()
	if err := registry.Register(&sendInternalAttachmentProvider{
		sendInternalProvider: sendInternalProvider{id: "fake"},
		caps: provider.AttachmentCapabilities{
			Images: true,
			Text:   true,
		},
	}); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	svc := NewSessionService(
		sessRepo,
		&turnLoopMessageRepo{},
		registry,
		nil,
		nil,
		nil,
		t.TempDir(),
		nil,
	)

	att := []FileAttachment{{
		Path:     "/tmp/archive.bin",
		MimeType: "application/octet-stream",
		Data:     []byte("bin"),
	}}
	if _, err := svc.PrepareSend(ctx, sessRepo.session.ID, att); err == nil {
		t.Fatal("PrepareSend() error = nil, want unsupported attachment rejection")
	}
	status, err := svc.TurnStatus(ctx, sessRepo.session.ID)
	if err != nil {
		t.Fatalf("TurnStatus() error = %v", err)
	}
	if status.State != TurnStateFailed {
		t.Fatalf("State = %q, want %q after validation failure", status.State, TurnStateFailed)
	}
	if status.Active {
		t.Fatal("Active = true, want false after validation failure")
	}
	if _, err := svc.PrepareSend(ctx, sessRepo.session.ID, att); err == nil {
		t.Fatal("second PrepareSend() error = nil, want unsupported attachment rejection")
	} else if errors.Is(err, ErrSessionBusy) {
		t.Fatalf("second PrepareSend() error = %v, want reservation to be released after validation failure", err)
	}
}

func TestSendRejectsAttachmentsWhenModelMetadataDisablesThem(t *testing.T) {
	ctx := context.Background()
	sessRepo := &sendInternalSessionRepo{
		session: repository.Session{
			ID:      "sess-1",
			AgentID: "default",
			ModelID: "fake/fake-model",
		},
	}

	registry := provider.NewRegistry()
	if err := registry.Register(&sendInternalAttachmentProvider{
		sendInternalProvider: sendInternalProvider{
			id: "fake",
			models: []provider.Model{{
				ID:              "fake-model",
				Name:            "Fake Model",
				ContextSize:     8192,
				AttachmentKnown: true,
				Attachment:      false,
				VisionKnown:     true,
				Vision:          false,
			}},
		},
		caps: provider.AttachmentCapabilities{
			Images: true,
			PDFs:   true,
			Text:   true,
			Binary: true,
		},
	}); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	msgRepo := &turnLoopMessageRepo{}
	svc := NewSessionService(
		sessRepo,
		msgRepo,
		registry,
		nil,
		nil,
		nil,
		t.TempDir(),
		nil,
	)

	err := svc.Send(ctx, sessRepo.session.ID, "see attachment", []FileAttachment{{
		Path:     "/tmp/diagram.png",
		MimeType: "image/png",
		Data:     validPNGData(),
	}}, sandbox.OriginTUI)
	if err == nil {
		t.Fatal("Send() error = nil, want model-specific attachment rejection")
	}
	if !strings.Contains(err.Error(), `model "fake-model" does not support image/png attachments`) {
		t.Fatalf("Send() error = %q, want model-specific rejection", err)
	}
	if len(msgRepo.messages) != 0 {
		t.Fatalf("persisted messages after model-specific rejection = %d, want 0", len(msgRepo.messages))
	}
}

func TestPrepareSendRejectsAttachmentMimeMismatch(t *testing.T) {
	sessRepo := &sendInternalSessionRepo{
		session: repository.Session{
			ID:      "sess-1",
			AgentID: "default",
			ModelID: "fake/fake-model",
		},
	}
	svc := NewSessionService(
		sessRepo,
		&turnLoopMessageRepo{},
		nil,
		nil,
		nil,
		nil,
		t.TempDir(),
		nil,
	)

	_, err := svc.PrepareSend(context.Background(), sessRepo.session.ID, []FileAttachment{{
		Path:     "/tmp/diagram.png",
		MimeType: "image/png",
		Data:     []byte("not a real png"),
	}})
	if err == nil || !strings.Contains(err.Error(), "content does not match declared MIME type") {
		t.Fatalf("PrepareSend() error = %v, want MIME mismatch rejection", err)
	}
}

func TestPrepareSendRejectsAttachmentOverConfiguredLimit(t *testing.T) {
	sessRepo := &sendInternalSessionRepo{
		session: repository.Session{
			ID:      "sess-1",
			AgentID: "default",
			ModelID: "fake/fake-model",
		},
	}
	cfg := &config.Config{}
	cfg.TUI.MaxAttachmentSize = 4
	svc := NewSessionService(
		sessRepo,
		&turnLoopMessageRepo{},
		nil,
		cfg,
		nil,
		nil,
		t.TempDir(),
		nil,
	)

	_, err := svc.PrepareSend(context.Background(), sessRepo.session.ID, []FileAttachment{{
		Path:     "/tmp/large.txt",
		MimeType: "text/plain",
		Data:     []byte("too large"),
	}})
	if err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("PrepareSend() error = %v, want size rejection", err)
	}
}

func TestPrepareSendRejectsSymlinkAttachment(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target.txt")
	if err := os.WriteFile(target, []byte("hello"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	link := filepath.Join(dir, "link.txt")
	if err := os.Symlink(target, link); err != nil {
		t.Fatalf("Symlink() error = %v", err)
	}

	sessRepo := &sendInternalSessionRepo{
		session: repository.Session{
			ID:      "sess-1",
			AgentID: "default",
			ModelID: "fake/fake-model",
		},
	}
	svc := NewSessionService(
		sessRepo,
		&turnLoopMessageRepo{},
		nil,
		nil,
		nil,
		nil,
		dir,
		nil,
	)

	_, err := svc.PrepareSend(context.Background(), sessRepo.session.ID, []FileAttachment{{
		Path:     link,
		MimeType: "text/plain",
		Data:     []byte("hello"),
	}})
	if err == nil || !strings.Contains(err.Error(), "cannot be a symlink") {
		t.Fatalf("PrepareSend() error = %v, want symlink rejection", err)
	}
}

func TestSendDoesNotPersistPartialMessageWhenAtomicCreateFails(t *testing.T) {
	sessRepo := &sendInternalSessionRepo{
		session: repository.Session{
			ID:      "sess-1",
			AgentID: "default",
			ModelID: "fake/fake-model",
		},
	}
	msgRepo := &failingAtomicMessageRepo{}
	registry := provider.NewRegistry()
	if err := registry.Register(&sendInternalProvider{id: "fake"}); err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	svc := NewSessionService(
		sessRepo,
		msgRepo,
		registry,
		nil,
		nil,
		nil,
		t.TempDir(),
		nil,
	)

	err := svc.Send(context.Background(), sessRepo.session.ID, "hello", nil, sandbox.OriginTUI)
	if err == nil || !strings.Contains(err.Error(), "atomic write failed") {
		t.Fatalf("Send() error = %v, want atomic write failure", err)
	}
	if len(msgRepo.messages) != 0 {
		t.Fatalf("persisted messages = %d, want 0", len(msgRepo.messages))
	}
}

func TestResolveSendModel_PrefersQualifiedSessionModelConfig(t *testing.T) {
	registry := provider.NewRegistry()
	if err := registry.Register(&sendInternalProvider{
		id: "fake",
		models: []provider.Model{{
			ID:          "fake-model",
			Name:        "Fake Model",
			ContextSize: 8192,
		}},
	}); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	cfg := &config.Config{
		Session: config.SessionConfig{
			Models: map[string]config.ModelSessionConfig{
				"fake/fake-model": {MaxInputTokens: 4096},
				"fake-model":      {MaxInputTokens: 2048},
			},
		},
	}

	svc := NewSessionService(
		&sendInternalSessionRepo{},
		&turnLoopMessageRepo{},
		registry,
		cfg,
		nil,
		nil,
		t.TempDir(),
		nil,
	)

	_, model, err := svc.resolveSendModel(context.Background(), "fake", "fake-model")
	if err != nil {
		t.Fatalf("resolveSendModel() error = %v", err)
	}
	if model.MaxInputTokens != 4096 {
		t.Fatalf("MaxInputTokens = %d, want 4096", model.MaxInputTokens)
	}
}
