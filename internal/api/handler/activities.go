package handler

import (
	"net/http"
	"strconv"

	"github.com/kubewise/kubewise/internal/utils/log"
	"github.com/labstack/echo/v5"
	"go.uber.org/zap"
)

func (h *Handler) ListActivities(c *echo.Context) error {
	ctx := c.Request().Context()
	limit := int64(20) // default
	offset := int64(0)
	if l := c.QueryParam("limit"); l != "" {
		if parsed, err := strconv.ParseInt(l, 10, 64); err == nil {
			limit = parsed
		}
	}
	if o := c.QueryParam("offset"); o != "" {
		if parsed, err := strconv.ParseInt(o, 10, 64); err == nil {
			offset = parsed
		}
	}

	log.Ctx(ctx).Debug("listing activities",
		zap.Int64("limit", limit),
		zap.Int64("offset", offset),
	)

	activities, err := h.activityService.List(ctx, int(limit), int(offset))
	if err != nil {
		log.Ctx(ctx).Error("failed to list activities", zap.Error(err))
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "query failed", Detail: err.Error()})
	}

	log.Ctx(ctx).Info("listed activities", zap.Int("count", len(activities)))
	return c.JSON(http.StatusOK, activities)
}