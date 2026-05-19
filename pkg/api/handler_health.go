package api

import (
	"net/http"

	"github.com/labstack/echo/v5"
)

func (h *Handler) Health(c *echo.Context) error {
	return c.JSON(http.StatusOK, HealthResponse{
		Status:  "ok",
		Version: "dev",
	})
}
