package middleware_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/sageil/kodacode/v1/internal/api/middleware"
	"github.com/sageil/kodacode/v1/internal/repository"
)

func TestErrorHandler(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		wantCode int
		wantBody string
	}{
		{
			name:     "echo.HTTPError maps code and message",
			err:      echo.NewHTTPError(http.StatusBadRequest, "invalid request body"),
			wantCode: http.StatusBadRequest,
			wantBody: `{"error":"invalid request body"}` + "\n",
		},
		{
			name:     "repository.ErrNotFound maps to 404",
			err:      repository.ErrNotFound,
			wantCode: http.StatusNotFound,
			wantBody: `{"error":"not found"}` + "\n",
		},
		{
			name:     "wrapped ErrNotFound maps to 404",
			err:      errors.Join(repository.ErrNotFound, errors.New("some context")),
			wantCode: http.StatusNotFound,
			wantBody: `{"error":"not found"}` + "\n",
		},
		{
			name:     "generic error maps to 500",
			err:      errors.New("unexpected database failure"),
			wantCode: http.StatusInternalServerError,
			wantBody: `{"error":"internal server error"}` + "\n",
		},
		{
			name:     "echo.HTTPError with 404 code",
			err:      echo.NewHTTPError(http.StatusNotFound, "session not found"),
			wantCode: http.StatusNotFound,
			wantBody: `{"error":"session not found"}` + "\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := echo.New()
			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)

			middleware.ErrorHandler(tt.err, c)

			if rec.Code != tt.wantCode {
				t.Errorf("ErrorHandler(%v) status = %d, want %d", tt.err, rec.Code, tt.wantCode)
			}
			if got := rec.Body.String(); got != tt.wantBody {
				t.Errorf("ErrorHandler(%v) body = %q, want %q", tt.err, got, tt.wantBody)
			}
		})
	}
}

func TestErrorHandler_AlreadyCommitted(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	// Mark response as already committed — handler must be a no-op.
	c.Response().Committed = true

	middleware.ErrorHandler(errors.New("some error"), c)

	// No write should have occurred.
	if rec.Code != http.StatusOK {
		t.Errorf("ErrorHandler on committed response wrote status %d, want %d (no-op)", rec.Code, http.StatusOK)
	}
	if rec.Body.Len() != 0 {
		t.Errorf("ErrorHandler on committed response wrote body %q, want empty", rec.Body.String())
	}
}
