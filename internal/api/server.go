package api

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/sageil/kodacode/v1/internal/api/handler"
	"github.com/sageil/kodacode/v1/internal/config"
)

// Server wraps an Echo instance and manages its lifecycle.
// It listens on a random available port when port 0 is passed to NewServer,
// so the TUI can discover the actual address via Addr() after Start returns.
type Server struct {
	echo    *echo.Echo
	handler *handler.Handler
	addr    string
}

func NewServer(sessions handler.SessionService, agents handler.AgentService, settings handler.SettingsService, cfg *config.Config, registry handler.ProviderRegistry, sbInfo ...handler.StatusBarInfo) *Server {
	e, h := NewRouter(sessions, agents, settings, cfg, registry, sbInfo...)
	return &Server{echo: e, handler: h}
}

func (s *Server) Handler() *handler.Handler { return s.handler }

// Start binds to the configured address and serves requests until ctx is
// cancelled. The actual bound address is available via Addr() after Start
// has begun serving.
//
// Start blocks until the server shuts down. A context-cancellation shutdown is
// not treated as an error; other shutdown errors are returned.
func (s *Server) Start(ctx context.Context) error {
	// Bind to loopback on a random available port from the OS.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return fmt.Errorf("server listen: %w", err)
	}
	s.addr = ln.Addr().String()

	// Shut down when ctx is cancelled, with a timeout to avoid hanging
	// on open connections (e.g. SSE streams from the TUI).
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
		defer cancel()
		_ = s.echo.Shutdown(shutdownCtx) //nolint:contextcheck
	}()

	err = s.echo.Server.Serve(ln)
	if err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("server serve: %w", err)
	}
	return nil
}

// Addr returns the actual TCP address the server is listening on, for example
// "127.0.0.1:54321". It is only populated after Start has been called.
func (s *Server) Addr() string {
	return s.addr
}
