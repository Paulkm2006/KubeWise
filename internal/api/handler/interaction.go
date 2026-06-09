package handler

import (
	"encoding/json"
	"net/http"

	"go.uber.org/zap"
	"github.com/kubewise/kubewise/internal/utils/log"
	"github.com/labstack/echo/v5"
)

// InteractionAnswerRequest is the body for POST /api/v1/chat/interaction.
type InteractionAnswerRequest struct {
	InteractionID string          `json:"interaction_id"`
	Payload       json.RawMessage `json:"payload"`
}

func (h *Handler) ChatInteraction(c *echo.Context) error {
	ctx := c.Request().Context()
	var req InteractionAnswerRequest
	if err := c.Bind(&req); err != nil {
		log.Ctx(ctx).Warn("interaction: invalid request body", zap.Error(err))
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "invalid request", Detail: err.Error()})
	}
	if req.InteractionID == "" {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "interaction_id is required"})
	}

	payload := req.Payload
	if len(payload) == 0 {
		payload = json.RawMessage("{}")
	}

	h.mu.Lock()
	pi, ok := h.pendingInteractions[req.InteractionID]
	if ok {
		delete(h.pendingInteractions, req.InteractionID)
	}
	h.mu.Unlock()

	if !ok {
		log.Ctx(ctx).Warn("interaction: unknown interaction_id", zap.String("interaction_id", req.InteractionID))
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: "interaction_id not found or already responded"})
	}

	select {
	case pi.respCh <- payload:
	default:
		return c.JSON(http.StatusConflict, ErrorResponse{Error: "agent no longer waiting for interaction"})
	}

	log.Ctx(ctx).Info("interaction received", zap.String("interaction_id", req.InteractionID))
	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}
