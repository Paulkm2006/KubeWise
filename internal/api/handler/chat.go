package handler

import (
	"net/http"

	"go.uber.org/zap"
	"github.com/kubewise/kubewise/internal/utils/log"
	"github.com/labstack/echo/v5"
)

type ChatRequest struct {
	Query   string `json:"query"`
	QueryID string `json:"query_id,omitempty"`
}

type ChatResponse struct {
	QueryID string `json:"query_id"`
	Result  string `json:"result"`
}

func (h *Handler) ChatSync(c *echo.Context) error {
	ctx := c.Request().Context()
	var req ChatRequest
	if err := c.Bind(&req); err != nil {
		log.Ctx(ctx).Warn("chat sync: invalid request body", zap.Error(err))
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "invalid request", Detail: err.Error()})
	}
	if req.Query == "" {
		log.Ctx(ctx).Warn("chat sync: empty query")
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "query is required"})
	}

	result, err := h.querier.HandleQuery(req.Query)
	if err != nil {
		log.Ctx(ctx).Error("chat sync: query failed", zap.String("query_preview", truncate(req.Query, 80)), zap.Error(err))
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "query failed", Detail: err.Error()})
	}

	log.Ctx(ctx).Info("chat sync completed", zap.String("query_preview", truncate(req.Query, 80)))
	return c.JSON(http.StatusOK, ChatResponse{
		QueryID: req.QueryID,
		Result:  result,
	})
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
