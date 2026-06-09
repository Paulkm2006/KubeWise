package handler

import (
	"net/http"
	"strconv"

	"github.com/kubewise/kubewise/internal/activity"
	"github.com/labstack/echo/v5"
)

func (h *Handler) ListActivities(c *echo.Context) error {
	if h.activityService == nil {
		return c.JSON(http.StatusOK, []any{})
	}
	limit := 20
	offset := 0
	if l := c.QueryParam("limit"); l != "" {
		if v, err := strconv.Atoi(l); err == nil && v > 0 {
			limit = v
		}
	}
	if o := c.QueryParam("offset"); o != "" {
		if v, err := strconv.Atoi(o); err == nil && v >= 0 {
			offset = v
		}
	}

	activities, err := h.activityService.List(limit, offset)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}
	if activities == nil {
		activities = []activity.Activity{}
	}
	return c.JSON(http.StatusOK, activities)
}
