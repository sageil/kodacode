package handler

import (
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/sageil/kodacode/v1/internal/apitypes"
)

func (h *Handler) listModels(c echo.Context) error {
	models := h.appService().ListModels(c.Request().Context())
	return c.JSON(http.StatusOK, apitypes.ProviderModelsFromDomain(models))
}

func (h *Handler) refreshModels(c echo.Context) error {
	models := h.appService().RefreshModels(c.Request().Context())
	return c.JSON(http.StatusOK, apitypes.ProviderModelsFromDomain(models))
}
