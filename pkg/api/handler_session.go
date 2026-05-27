package api

import (
	"net/http"

	"github.com/labstack/echo/v5"

	"github.com/kubewise/kubewise/pkg/session"
)

func (h *Handler) ListSessions(c *echo.Context) error {
	sessions, err := h.sessionStore.LoadRecent(50)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "failed to load sessions"})
	}
	resp := make([]SessionResponse, 0, len(sessions))
	for _, s := range sessions {
		resp = append(resp, SessionResponse{
			ID:           s.ID,
			Title:        s.Title,
			CreatedAt:    s.CreatedAt,
			UpdatedAt:    s.UpdatedAt,
			MessageCount: len(s.Messages),
		})
	}
	return c.JSON(http.StatusOK, SessionListResponse{Sessions: resp})
}

func (h *Handler) CreateSession(c *echo.Context) error {
	var req CreateSessionRequest
	_ = c.Bind(&req)

	sess := session.NewConversation()
	if req.Title != "" {
		sess.Title = req.Title
	}
	if err := h.sessionStore.Save(sess); err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "failed to create session"})
	}
	return c.JSON(http.StatusCreated, SessionResponse{
		ID:        sess.ID,
		Title:     sess.Title,
		CreatedAt: sess.CreatedAt,
		UpdatedAt: sess.UpdatedAt,
	})
}

func (h *Handler) GetSession(c *echo.Context) error {
	id := c.Param("id")
	sessions, err := h.sessionStore.LoadRecent(200)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "failed to load sessions"})
	}
	for _, s := range sessions {
		if s.ID == id {
			msgs := make([]Message, 0, len(s.Messages))
			for _, m := range s.Messages {
				msgs = append(msgs, Message{
					Role:      m.Role,
					Content:   m.Content,
					Timestamp: m.Timestamp,
				})
			}
			return c.JSON(http.StatusOK, SessionDetailResponse{
				ID:        s.ID,
				Title:     s.Title,
				CreatedAt: s.CreatedAt,
				UpdatedAt: s.UpdatedAt,
				Messages:  msgs,
			})
		}
	}
	return c.JSON(http.StatusNotFound, ErrorResponse{Error: "session not found"})
}

func (h *Handler) DeleteSession(c *echo.Context) error {
	id := c.Param("id")
	if err := h.sessionStore.Delete(id); err != nil {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: "session not found"})
	}
	return c.JSON(http.StatusOK, map[string]string{"status": "deleted"})
}
