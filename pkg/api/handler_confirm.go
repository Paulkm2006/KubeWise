package api

import (
	"net/http"

	"github.com/labstack/echo/v5"

	"github.com/kubewise/kubewise/pkg/agent/operation"
)

func (h *Handler) ChatConfirm(c *echo.Context) error {
	var req ConfirmRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "invalid request", Detail: err.Error()})
	}
	if req.ConfirmID == "" {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "confirm_id is required"})
	}

	h.mu.Lock()
	pc, ok := h.pendingConfirms[req.ConfirmID]
	if ok {
		delete(h.pendingConfirms, req.ConfirmID)
	}
	h.mu.Unlock()

	if !ok {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: "confirm_id not found or already responded"})
	}

	select {
	case pc.respCh <- operation.ConfirmResponse{
		Confirmed:  req.Confirmed,
		Correction: req.Correction,
	}:
	default:
		return c.JSON(http.StatusConflict, ErrorResponse{Error: "agent no longer waiting for confirmation"})
	}

	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}
