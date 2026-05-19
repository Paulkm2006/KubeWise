package api

import (
	"net/http"

	"github.com/labstack/echo/v5"
)

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
