package httpapi

import (
	"errors"
	"net/http"
	"strconv"

	feedapp "github.com/kubewise/kubewise/internal/activityfeed/application"
	httputil "github.com/kubewise/kubewise/internal/transport/httputil"
	"github.com/kubewise/kubewise/internal/utils/log"
	"github.com/labstack/echo/v5"
	"go.uber.org/zap"
)

type Handler struct {
	Service *feedapp.Service
}

func (h *Handler) List(c *echo.Context) error {
	ctx := c.Request().Context()
	limit := int64(20)
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

	activities, err := h.Service.List(ctx, int(limit), int(offset))
	if err != nil {
		if errors.Is(err, feedapp.ErrUnavailable) {
			return c.JSON(http.StatusServiceUnavailable, httputil.ErrorResponse{Error: "activity feed unavailable"})
		}
		log.Ctx(ctx).Error("list activities failed", zap.Error(err))
		return c.JSON(http.StatusInternalServerError, httputil.ErrorResponse{Error: "query failed", Detail: err.Error()})
	}
	return c.JSON(http.StatusOK, activities)
}
