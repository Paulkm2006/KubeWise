package handler

import (
	"net/http"

	"github.com/kubewise/kubewise/internal/utils/log"
	"github.com/labstack/echo/v5"
)

func (h *Handler) Health(c *echo.Context) error {
	log.Ctx(c.Request().Context()).Debug("health check")
	return c.JSON(http.StatusOK, HealthResponse{
		Status:  "ok",
		Version: "dev",
	})
}
