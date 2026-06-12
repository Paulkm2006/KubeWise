package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/kubewise/kubewise/internal/conversation/application"
	httputil "github.com/kubewise/kubewise/internal/transport/httputil"
	"github.com/kubewise/kubewise/internal/utils/log"
	"github.com/labstack/echo/v5"
	"go.uber.org/zap"
)

type Handler struct {
	Chat    *application.ChatService
	Session *application.SessionService
}

type chatRequest struct {
	Query     string `json:"query"`
	QueryID   string `json:"query_id,omitempty"`
	SessionID string `json:"session_id,omitempty"`
	Cluster   string `json:"cluster,omitempty"`
}

type chatResponse struct {
	QueryID string `json:"query_id"`
	Result  string `json:"result"`
}

type createSessionRequest struct {
	Title string `json:"title,omitempty"`
}

type interactionAnswerRequest struct {
	InteractionID string          `json:"interaction_id"`
	Payload       json.RawMessage `json:"payload"`
}

func (h *Handler) ChatSync(c *echo.Context) error {
	ctx := c.Request().Context()
	var req chatRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, httputil.ErrorResponse{Error: "invalid request", Detail: err.Error()})
	}

	resp, err := h.Chat.QuerySync(ctx, application.SyncRequest{
		Query: req.Query, QueryID: req.QueryID, SessionID: req.SessionID, Cluster: req.Cluster,
	})
	if err != nil {
		if err.Error() == "query is required" {
			return c.JSON(http.StatusBadRequest, httputil.ErrorResponse{Error: err.Error()})
		}
		log.Ctx(ctx).Error("chat sync failed", zap.Error(err))
		return c.JSON(http.StatusInternalServerError, httputil.ErrorResponse{Error: "query failed", Detail: err.Error()})
	}
	return c.JSON(http.StatusOK, chatResponse{QueryID: resp.QueryID, Result: resp.Result})
}

func (h *Handler) ChatStream(c *echo.Context) error {
	ctx := c.Request().Context()
	query := c.QueryParam("query")
	if query == "" {
		return c.JSON(http.StatusBadRequest, httputil.ErrorResponse{Error: "query parameter is required"})
	}

	sse, err := httputil.NewSSE(c)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, httputil.ErrorResponse{Error: err.Error()})
	}

	ctx, cancel := httputil.ContextWithCancel(c)
	defer cancel()

	_ = h.Chat.Stream(ctx, application.StreamRequest{
		Query: query, QueryID: c.QueryParam("query_id"),
		SessionID: c.QueryParam("session_id"), Cluster: c.QueryParam("cluster"),
	}, sse)
	return nil
}

func (h *Handler) ChatInteraction(c *echo.Context) error {
	ctx := c.Request().Context()
	var req interactionAnswerRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, httputil.ErrorResponse{Error: "invalid request", Detail: err.Error()})
	}

	err := h.Chat.AnswerInteraction(ctx, application.InteractionAnswer{
		InteractionID: req.InteractionID, Payload: req.Payload,
	})
	if err != nil {
		switch {
		case err.Error() == "interaction_id is required":
			return c.JSON(http.StatusBadRequest, httputil.ErrorResponse{Error: err.Error()})
		case errors.Is(err, application.ErrInteractionNotFound):
			return c.JSON(http.StatusNotFound, httputil.ErrorResponse{Error: "interaction_id not found or already responded"})
		case errors.Is(err, application.ErrInteractionClosed):
			return c.JSON(http.StatusConflict, httputil.ErrorResponse{Error: err.Error()})
		default:
			return c.JSON(http.StatusInternalServerError, httputil.ErrorResponse{Error: err.Error()})
		}
	}
	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) ListSessions(c *echo.Context) error {
	ctx := c.Request().Context()
	sessions, err := h.Session.ListRecent(50)
	if err != nil {
		log.Ctx(ctx).Error("list sessions failed", zap.Error(err))
		return c.JSON(http.StatusInternalServerError, httputil.ErrorResponse{Error: "failed to load sessions"})
	}
	return c.JSON(http.StatusOK, map[string]any{"sessions": sessions})
}

func (h *Handler) CreateSession(c *echo.Context) error {
	ctx := c.Request().Context()
	var req createSessionRequest
	_ = c.Bind(&req)

	summary, err := h.Session.Create(req.Title)
	if err != nil {
		log.Ctx(ctx).Error("create session failed", zap.Error(err))
		return c.JSON(http.StatusInternalServerError, httputil.ErrorResponse{Error: "failed to create session"})
	}
	return c.JSON(http.StatusCreated, summary)
}

func (h *Handler) GetSession(c *echo.Context) error {
	ctx := c.Request().Context()
	id := c.Param("id")

	detail, err := h.Session.Get(id)
	if err != nil {
		if errors.Is(err, application.ErrSessionNotFound) {
			return c.JSON(http.StatusNotFound, httputil.ErrorResponse{Error: "session not found"})
		}
		log.Ctx(ctx).Error("get session failed", zap.String("session_id", id), zap.Error(err))
		return c.JSON(http.StatusInternalServerError, httputil.ErrorResponse{Error: "failed to load sessions"})
	}
	return c.JSON(http.StatusOK, detail)
}

func (h *Handler) DeleteSession(c *echo.Context) error {
	ctx := c.Request().Context()
	id := c.Param("id")
	if err := h.Session.Delete(id); err != nil {
		log.Ctx(ctx).Error("delete session failed", zap.String("session_id", id), zap.Error(err))
		return c.JSON(http.StatusNotFound, httputil.ErrorResponse{Error: "session not found"})
	}
	return c.JSON(http.StatusOK, map[string]string{"status": "deleted"})
}
