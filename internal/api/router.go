// Package api wires the Echo HTTP server together with all handlers and middleware.
package api

import (
	"io"
	"net/http"
	"net/http/pprof"

	"github.com/labstack/echo/v4"
	echomiddleware "github.com/labstack/echo/v4/middleware"

	"github.com/sageil/kodacode/v1/internal/api/handler"
	"github.com/sageil/kodacode/v1/internal/api/middleware"
	"github.com/sageil/kodacode/v1/internal/config"
)

// NewRouter creates an Echo instance configured with all middleware and routes.
//
// The returned router has:
//   - Banner and logging suppressed (the app runs inside a Bubble Tea TUI;
//     stdout writes would corrupt the terminal rendering)
//   - Custom ErrorHandler that maps domain errors to HTTP status codes
//   - Panic recover middleware
//   - All session, agent, settings, and health routes registered
func NewRouter(sessions handler.SessionService, agents handler.AgentService, settings handler.SettingsService, cfg *config.Config, registry handler.ProviderRegistry, sbInfo ...handler.StatusBarInfo) (*echo.Echo, *handler.Handler) {
	e := echo.New()
	e.HideBanner = true
	e.Logger.SetOutput(io.Discard)
	e.HTTPErrorHandler = middleware.ErrorHandler
	e.Use(echomiddleware.Recover())
	h := handler.RegisterRoutes(e, sessions, agents, settings, cfg, registry, sbInfo...)

	if cfg != nil && config.Bool(cfg.Debug) {
		e.GET("/debug/pprof/*", echo.WrapHandler(http.HandlerFunc(pprof.Index)))
		e.GET("/debug/pprof/heap", echo.WrapHandler(http.HandlerFunc(pprof.Handler("heap").ServeHTTP)))
		e.GET("/debug/pprof/goroutine", echo.WrapHandler(http.HandlerFunc(pprof.Handler("goroutine").ServeHTTP)))
		e.GET("/debug/pprof/allocs", echo.WrapHandler(http.HandlerFunc(pprof.Handler("allocs").ServeHTTP)))
	}

	return e, h
}
