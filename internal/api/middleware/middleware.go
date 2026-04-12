// Package middleware provides Echo middleware for the kodacode API server.
package middleware

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/sageil/kodacode/v1/internal/repository"
)

// ErrorHandler is an Echo HTTPErrorHandler that maps domain errors to HTTP
// status codes and returns a consistent JSON error body.
//
// Usage:
//
//	e.HTTPErrorHandler = middleware.ErrorHandler
func ErrorHandler(err error, c echo.Context) {
	if c.Response().Committed {
		return
	}

	code := http.StatusInternalServerError
	msg := "internal server error"

	var he *echo.HTTPError
	switch {
	case errors.As(err, &he):
		code = he.Code
		if s, ok := he.Message.(string); ok {
			msg = s
		}
	case errors.Is(err, repository.ErrNotFound):
		code = http.StatusNotFound
		msg = "not found"
	}

	if code == http.StatusInternalServerError {
		slog.Error("unhandled error", "err", err, "path", c.Request().URL.Path)
	}

	if err := c.JSON(code, map[string]string{"error": msg}); err != nil {
		slog.Error("error handler: write response", "err", err)
	}
}
