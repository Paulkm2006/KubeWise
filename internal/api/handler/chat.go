package handler

import (
	"net/http"

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
	var req ChatRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "invalid request", Detail: err.Error()})
	}
	if req.Query == "" {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "query is required"})
	}

	result, err := h.querier.HandleQuery(req.Query)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "query failed", Detail: err.Error()})
	}

	return c.JSON(http.StatusOK, ChatResponse{
		QueryID: req.QueryID,
		Result:  result,
	})
}
