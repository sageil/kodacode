package search

import (
	"context"
	"errors"
	"strings"
	"sync"

	"github.com/sageil/kodacode/internal/observability"
	"github.com/sageil/kodacode/internal/provider"
)

var ErrSemanticScopeTooLarge = errors.New("hybrid search scope is too large; narrow path or glob")

type Service struct {
	embedder   provider.Embedder
	model      provider.ModelRef
	dimensions int
	indexDir   string
	logger     *observability.Logger
	rrfK       int
	pathBoosts pathBoostConfig
	skipDirs   skipDirMatcher

	mu       sync.Mutex
	files    map[string]cachedFile
	trackers map[string]*workspaceTracker
	closed   bool
	wg       sync.WaitGroup
}

type Option func(*Service)

func WithSkipDirs(skipDirs []string) Option {
	return func(s *Service) {
		if s == nil {
			return
		}
		s.skipDirs = newSkipDirMatcher(skipDirs)
	}
}

func NewService(embedder provider.Embedder, model provider.ModelRef, dimensions int, indexDir string, logger *observability.Logger, options ...Option) *Service {
	service := &Service{
		embedder:   embedder,
		model:      model,
		dimensions: dimensions,
		indexDir:   indexDir,
		logger:     logger,
		rrfK:       standardRRFK,
		pathBoosts: defaultPathBoostConfig(),
		skipDirs:   defaultSkipDirMatcher(),
		files:      map[string]cachedFile{},
		trackers:   map[string]*workspaceTracker{},
	}
	for _, option := range options {
		if option != nil {
			option(service)
		}
	}
	return service
}

func (s *Service) shouldSkipDir(name string) bool {
	if s == nil {
		return defaultSkipDirMatcher().shouldSkip(name)
	}
	return s.skipDirs.shouldSkip(name)
}

func (s *Service) HybridConfigured() bool {
	return s != nil && s.embedder != nil && strings.TrimSpace(s.model.ProviderID) != ""
}

func (s *Service) ResolveMode(req Request) Mode {
	if req.Mode != "" {
		return req.Mode
	}
	if req.Regex || !s.HybridConfigured() {
		return ModeLexical
	}
	return ModeHybrid
}

func (s *Service) Search(ctx context.Context, req Request) (Response, error) {
	if err := req.Validate(); err != nil {
		return Response{}, err
	}
	mode := s.ResolveMode(req)
	req.Mode = mode
	if mode == ModeLexical {
		return s.lexicalObserved(req)
	}
	if !s.HybridConfigured() {
		lexicalResp, err := s.lexicalObserved(req)
		if err != nil {
			return Response{}, err
		}
		return Response{
			Mode:     ModeHybrid,
			Notice:   "semantic search is not configured; using lexical results",
			Results:  lexicalResp.Results,
			Fallback: true,
		}, nil
	}

	results, notice, err := s.hybrid(ctx, req)
	if err != nil {
		s.logFallback(err)
		autoWarmNotice := ""
		if errors.Is(err, ErrSemanticScopeTooLarge) && s.scheduleAutoWarmWorkspace(req.WorkspaceRoot, "hybrid_fallback") {
			autoWarmNotice = "warming workspace index in background"
		}
		lexicalResp, lexErr := s.lexicalObserved(req)
		if lexErr != nil {
			return Response{}, lexErr
		}
		noticeText := err.Error()
		if strings.TrimSpace(autoWarmNotice) != "" {
			noticeText += "; " + autoWarmNotice
		}
		return Response{
			Mode:     ModeHybrid,
			Notice:   noticeText,
			Results:  lexicalResp.Results,
			Fallback: true,
		}, nil
	}
	return Response{
		Mode:    ModeHybrid,
		Notice:  notice,
		Results: results,
	}, nil
}

func (s *Service) logFallback(err error) {
	if s == nil || s.logger == nil || err == nil {
		return
	}
	s.logger.Debug("hybrid search fallback", "reason", err.Error())
}
