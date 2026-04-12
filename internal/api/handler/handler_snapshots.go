package handler

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/sageil/kodacode/v1/internal/apitypes"
	"github.com/sageil/kodacode/v1/internal/appservice"
)

func (h *Handler) listSnapshots(c echo.Context) error {
	snapshots, err := h.appService().ListSnapshots(c.Param("id"))
	if err != nil {
		return fmt.Errorf("list snapshots: %w", err)
	}
	return c.JSON(http.StatusOK, apitypes.SnapshotsFromDomain(snapshots))
}

func (h *Handler) restoreSnapshot(c echo.Context) error {
	var turn int
	if _, err := fmt.Sscanf(c.Param("turn"), "%d", &turn); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid turn number")
	}
	if err := h.appService().RestoreSnapshot(c.Param("id"), turn); err != nil {
		if errors.Is(err, appservice.ErrSnapshotsDisabled) {
			return echo.NewHTTPError(http.StatusNotFound, "snapshots not enabled")
		}
		return fmt.Errorf("restore snapshot: %w", err)
	}
	return c.NoContent(http.StatusNoContent)
}
