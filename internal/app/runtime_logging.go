package app

import (
	"errors"
	"io"

	"github.com/sageil/kodacode/internal/observability"
	"github.com/sageil/kodacode/internal/provider"
)

func (r *Runtime) SetLogger(logger *observability.Logger) {
	r.Logger = logger
	attachProviderRouteLogging(r.Provider, logger.With("component", "provider"))
	if r.Sessions != nil {
		r.Sessions.SetLogger(logger.With("component", "session_service"))
	}
	if r.Runner != nil {
		r.Runner.SetLogger(logger.With("component", "turn_runner"))
	}
	if r.Tools != nil {
		r.Tools.SetLogger(logger.With("component", "tool_executor"))
	}
	if r.Tools != nil {
		r.Tools.SetSearchService(r.Search)
	}
	if r.Tools != nil {
		r.Tools.SetWebSearchService(r.WebSearch)
	}
}

func (r *Runtime) Close() error {
	if r == nil {
		return nil
	}
	var errs []error
	if r.Search != nil {
		if err := r.Search.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if r.mcpRegistry != nil {
		if err := r.mcpRegistry.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if r.Trusts != nil {
		if err := r.Trusts.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if closer, ok := r.Store.(io.Closer); ok {
		if err := closer.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if r.Logger != nil {
		if err := r.Logger.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (r *Runtime) log(component string) *observability.Logger {
	if r == nil || r.Logger == nil {
		return nil
	}
	return r.Logger.With("component", component)
}

func attachProviderRouteLogging(client provider.Client, logger *observability.Logger) {
	router, ok := client.(*provider.RoutedClient)
	if !ok || router == nil {
		return
	}
	if logger == nil {
		router.SetRouteObserver(nil)
		return
	}
	router.SetRouteObserver(func(attempt provider.RouteAttempt) {
		args := []any{
			"session_id", attempt.SessionID,
			"turn_id", attempt.TurnID,
			"agent_id", attempt.AgentID,
			"attempt", attempt.Attempt,
			"model", attempt.Model.String(),
		}
		if attempt.Selected {
			logger.Debug("provider route selected", args...)
			return
		}
		if attempt.Error != nil {
			args = append(args, "error", attempt.Error.Error())
			args = append(args, providerErrorLogFields(attempt.Error)...)
			logger.Debug("provider route attempt failed", args...)
		}
	})
}
