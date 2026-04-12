package handler

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/sageil/kodacode/v1/internal/repository"
)

// settingResponse is the JSON shape for GET/PUT /settings/:key.
type settingResponse struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// putSettingRequest is the JSON body for PUT /settings/:key.
type putSettingRequest struct {
	Value string `json:"value"`
}

func (h *Handler) getSetting(c echo.Context) error {
	key := c.Param("key")
	value, err := h.appService().GetSetting(c.Request().Context(), key)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return echo.NewHTTPError(http.StatusNotFound, "setting not found")
		}
		return fmt.Errorf("get setting: %w", err)
	}
	return c.JSON(http.StatusOK, settingResponse{Key: key, Value: value})
}

func (h *Handler) putSetting(c echo.Context) error {
	key := c.Param("key")
	var req putSettingRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}
	if err := h.appService().SetSetting(c.Request().Context(), key, req.Value); err != nil {
		return fmt.Errorf("set setting: %w", err)
	}
	return c.JSON(http.StatusOK, settingResponse{Key: key, Value: req.Value})
}
