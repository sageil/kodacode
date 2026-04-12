package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/sageil/kodacode/v1/internal/apitypes"
	"github.com/sageil/kodacode/v1/internal/appservice"
	"github.com/sageil/kodacode/v1/internal/logging"
	"github.com/sageil/kodacode/v1/internal/repository"
	"github.com/sageil/kodacode/v1/internal/service"
)

// createSessionRequest is the JSON body for POST /sessions.
type createSessionRequest struct {
	AgentID string `json:"agent_id"`
	ModelID string `json:"model_id"`
}

func (h *Handler) createSession(c echo.Context) error {
	var req createSessionRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}

	sess, err := h.appService().CreateSession(c.Request().Context(), req.AgentID, req.ModelID)
	if err != nil {
		return fmt.Errorf("create session: %w", err)
	}
	return c.JSON(http.StatusCreated, apitypes.SessionFromRepository(sess))
}

func (h *Handler) listSessions(c echo.Context) error {
	sessions, err := h.appService().ListSessions(c.Request().Context())
	if err != nil {
		return fmt.Errorf("list sessions: %w", err)
	}
	return c.JSON(http.StatusOK, apitypes.SessionsFromRepository(sessions))
}

func (h *Handler) getSession(c echo.Context) error {
	id := c.Param("id")
	sess, err := h.appService().GetSession(c.Request().Context(), id)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return echo.NewHTTPError(http.StatusNotFound, "session not found")
		}
		return fmt.Errorf("get session: %w", err)
	}
	return c.JSON(http.StatusOK, apitypes.SessionFromRepository(sess))
}

type updateSessionRequest struct {
	AgentID string `json:"agent_id"`
	ModelID string `json:"model_id"`
}

func (h *Handler) updateSession(c echo.Context) error {
	id := c.Param("id")
	var req updateSessionRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}
	sess, err := h.appService().UpdateSession(c.Request().Context(), id, req.AgentID, req.ModelID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return echo.NewHTTPError(http.StatusNotFound, "session not found")
		}
		return fmt.Errorf("update session: %w", err)
	}
	return c.JSON(http.StatusOK, apitypes.SessionFromRepository(sess))
}

func (h *Handler) deleteSession(c echo.Context) error {
	id := c.Param("id")
	if err := h.appService().DeleteSession(c.Request().Context(), id); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return echo.NewHTTPError(http.StatusNotFound, "session not found")
		}
		return fmt.Errorf("delete session: %w", err)
	}
	return c.NoContent(http.StatusNoContent)
}

// attachmentRequest is the JSON shape for a file attachment in the send-message request.
type attachmentRequest struct {
	Path     string `json:"path"`
	MimeType string `json:"mime_type"`
}

// sendMessageRequest is the JSON body for POST /sessions/:id/messages.
type sendMessageRequest struct {
	Content     string              `json:"content"`
	Attachments []attachmentRequest `json:"attachments,omitempty"`
	AgentID     string              `json:"agent_id,omitempty"`
	Variant     string              `json:"variant,omitempty"`
}

func (h *Handler) listMessages(c echo.Context) error {
	id := c.Param("id")
	msgs, err := h.appService().ListMessages(c.Request().Context(), id)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return echo.NewHTTPError(http.StatusNotFound, "session not found")
		}
		return fmt.Errorf("list messages: %w", err)
	}
	return c.JSON(http.StatusOK, apitypes.MessagesFromViews(msgs))
}

func (h *Handler) sendMessage(c echo.Context) error {
	id := c.Param("id")
	var req sendMessageRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}
	status, err := h.appService().SendMessageOperation(c.Request().Context(), id, req.Content, toAppAttachments(req.Attachments), req.AgentID, req.Variant)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return echo.NewHTTPError(http.StatusNotFound, "session not found")
		}
		if errors.Is(err, service.ErrSessionBusy) {
			return echo.NewHTTPError(http.StatusConflict, err.Error())
		}
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	return c.JSON(http.StatusAccepted, apitypes.TurnStatusFromDomain(status))
}

func (h *Handler) getTurnStatus(c echo.Context) error {
	id := c.Param("id")
	operationID := c.QueryParam("operation_id")
	var (
		status apitypes.TurnStatus
		err    error
	)
	if operationID != "" {
		var domainStatus service.TurnStatus
		domainStatus, err = h.appService().TurnStatusByOperation(c.Request().Context(), id, operationID)
		status = apitypes.TurnStatusFromDomain(domainStatus)
	} else {
		var domainStatus service.TurnStatus
		domainStatus, err = h.appService().TurnStatus(c.Request().Context(), id)
		status = apitypes.TurnStatusFromDomain(domainStatus)
	}
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return echo.NewHTTPError(http.StatusNotFound, "session not found")
		}
		if errors.Is(err, service.ErrTurnOperationNotFound) {
			return echo.NewHTTPError(http.StatusNotFound, "turn operation not found")
		}
		return fmt.Errorf("get turn status: %w", err)
	}
	return c.JSON(http.StatusOK, status)
}

func (h *Handler) cancelTurn(c echo.Context) error {
	id := c.Param("id")
	status, err := h.appService().CancelTurnOperation(c.Request().Context(), id)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return echo.NewHTTPError(http.StatusNotFound, "session not found")
		}
		if errors.Is(err, service.ErrNoActiveTurn) {
			return echo.NewHTTPError(http.StatusConflict, err.Error())
		}
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	return c.JSON(http.StatusAccepted, apitypes.TurnStatusFromDomain(status))
}

func (h *Handler) streamSession(c echo.Context) error {
	id := c.Param("id")
	sub, cancel, err := h.appService().Subscribe(c.Request().Context(), id)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return echo.NewHTTPError(http.StatusNotFound, "session not found")
		}
		return fmt.Errorf("get session: %w", err)
	}

	w := c.Response()
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)

	defer cancel()

	flusher, hasFlusher := w.Writer.(http.Flusher)

	ctx := c.Request().Context()
	for {
		select {
		case <-ctx.Done():
			return nil
		case ev, ok := <-sub:
			if !ok {
				return nil
			}
			data, err := json.Marshal(ev.Data)
			if err != nil {
				log.Printf("sse: failed to marshal %s event: %v", ev.Type, err)
				continue
			}
			if ev.Type == "reasoning_delta" || ev.Type == "reasoning_done" {
				logging.Debugf("[4-http] writing SSE %s event: %d bytes", ev.Type, len(data))
			}
			if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", ev.Type, data); err != nil {
				return err
			}
			if hasFlusher {
				flusher.Flush()
			}
			if ev.Type == "done" || ev.Type == "error" {
				return nil
			}
		}
	}
}

// branchRequest is the JSON body for POST /sessions/:id/branch.
type branchRequest struct {
	MessageID string `json:"message_id"`
}

func (h *Handler) branchSession(c echo.Context) error {
	id := c.Param("id")

	var req branchRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}

	branch, err := h.appService().BranchSession(c.Request().Context(), id, req.MessageID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return echo.NewHTTPError(http.StatusNotFound, "session not found")
		}
		return fmt.Errorf("branch session: %w", err)
	}
	return c.JSON(http.StatusCreated, apitypes.SessionFromRepository(branch))
}

// answerRequest is the JSON body for POST /sessions/:id/answer.
type answerRequest struct {
	QuestionID string `json:"question_id"`
	Response   string `json:"response"` // "once", "always", or "reject"
}

func (h *Handler) answerQuestion(c echo.Context) error {
	id := c.Param("id")
	var req answerRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}
	if err := h.appService().AnswerQuestion(c.Request().Context(), id, req.QuestionID, req.Response); err != nil {
		if errors.Is(err, appservice.ErrInvalidQuestionResponse) {
			return echo.NewHTTPError(http.StatusBadRequest, "invalid response value: must be \"once\", \"always\", or \"reject\"")
		}
		if errors.Is(err, repository.ErrNotFound) {
			return echo.NewHTTPError(http.StatusNotFound, err.Error())
		}
		return fmt.Errorf("answer question: %w", err)
	}
	return c.NoContent(http.StatusNoContent)
}

// spawnSubagentRequest is the JSON body for POST /sessions/:id/subagent.
type spawnSubagentRequest struct {
	AgentID string `json:"agent_id"`
	Task    string `json:"task"`
}

func (h *Handler) spawnSubagent(c echo.Context) error {
	parentID := c.Param("id")
	var req spawnSubagentRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}
	if err := h.appService().SpawnSubagent(c.Request().Context(), parentID, req.AgentID, req.Task); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return echo.NewHTTPError(http.StatusNotFound, "session not found")
		}
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	return c.NoContent(http.StatusAccepted)
}

func toAppAttachments(in []attachmentRequest) []appservice.AttachmentInput {
	out := make([]appservice.AttachmentInput, 0, len(in))
	for _, a := range in {
		out = append(out, appservice.AttachmentInput{Path: a.Path, MimeType: a.MimeType})
	}
	return out
}
