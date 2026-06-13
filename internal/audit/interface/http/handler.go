package httpapi

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	auditapp "github.com/kubewise/kubewise/internal/audit/application"
	"github.com/kubewise/kubewise/internal/audit/domain"
	httputil "github.com/kubewise/kubewise/internal/transport/httputil"
	"github.com/kubewise/kubewise/internal/utils/log"
	"github.com/labstack/echo/v5"
	"go.uber.org/zap"
)

type Handler struct {
	Service *auditapp.Service
}

type auditRequest struct {
	Cluster string `json:"cluster"`
}

type auditResponse struct {
	AuditID string `json:"audit_id"`
	Status  string `json:"status"`
}

type auditStatusResponse struct {
	AuditID   string               `json:"audit_id"`
	Status    string               `json:"status"`
	CreatedAt time.Time            `json:"created_at"`
	Target    domain.Target        `json:"target"`
	Events    []domain.EventRecord `json:"events"`
	Result    *domain.Result       `json:"result,omitempty"`
	Error     string               `json:"error_message,omitempty"`
}

type auditListResponse struct {
	Audits []domain.Audit `json:"audits"`
	Total  int            `json:"total"`
}

func (h *Handler) Start(c *echo.Context) error {
	ctx := c.Request().Context()
	var req auditRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, httputil.ErrorResponse{Error: "invalid request"})
	}
	if req.Cluster == "" {
		return c.JSON(http.StatusBadRequest, httputil.ErrorResponse{Error: "cluster is required"})
	}

	id, err := h.Service.Start(ctx, domain.Target{Cluster: req.Cluster, ClusterDisplay: req.Cluster})
	if err != nil {
		if errors.Is(err, auditapp.ErrUnavailable) {
			return c.JSON(http.StatusServiceUnavailable, httputil.ErrorResponse{Error: "audit unavailable"})
		}
		log.Ctx(ctx).Error("start audit failed", zap.Error(err))
		return c.JSON(http.StatusInternalServerError, httputil.ErrorResponse{Error: "failed to start audit"})
	}

	log.Ctx(ctx).Info("audit started", zap.String("audit_id", id), zap.String("cluster", req.Cluster))
	return c.JSON(http.StatusAccepted, auditResponse{AuditID: id, Status: "running"})
}

func (h *Handler) StreamEvents(c *echo.Context) error {
	ctx := c.Request().Context()
	id := c.Param("id")
	if id == "" {
		return c.JSON(http.StatusBadRequest, httputil.ErrorResponse{Error: "audit id is required"})
	}

	since := 0
	if lastID := c.Request().Header.Get("Last-Event-ID"); lastID != "" {
		since, _ = strconv.Atoi(lastID)
	}
	if s := c.QueryParam("since"); s != "" {
		since, _ = strconv.Atoi(s)
	}

	sse, err := httputil.NewSSE(c)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, httputil.ErrorResponse{Error: err.Error()})
	}

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	hasSent := false
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			events, err := h.Service.EventsSince(ctx, id, since)
			if err != nil {
				log.Ctx(ctx).Warn("audit stream query error", zap.Error(err))
				continue
			}
			for _, ev := range events {
				if err := sse.WriteEventWithID("audit_event", ev.SeqNum, ev); err != nil {
					return nil
				}
				since = ev.SeqNum
				hasSent = true
			}

			status, err := h.Service.Status(ctx, id)
			if err != nil {
				continue
			}
			if isTerminalStatus(status.Status) && len(events) == 0 && hasSent {
				_ = sse.WriteEvent("stream_complete", map[string]string{
					"status": string(status.Status), "audit_id": id,
				})
				return nil
			}
		}
	}
}

func (h *Handler) Get(c *echo.Context) error {
	ctx := c.Request().Context()
	id := c.Param("id")
	audit, events, err := h.Service.Get(ctx, id)
	if err != nil {
		return c.JSON(http.StatusNotFound, httputil.ErrorResponse{Error: "audit not found"})
	}
	return c.JSON(http.StatusOK, buildStatusResponse(audit, events))
}

func (h *Handler) Latest(c *echo.Context) error {
	ctx := c.Request().Context()
	cluster := c.QueryParam("cluster")
	if cluster == "" {
		return c.JSON(http.StatusBadRequest, httputil.ErrorResponse{Error: "cluster is required"})
	}

	audit, events, err := h.Service.Latest(ctx, cluster)
	if err != nil {
		if errors.Is(err, auditapp.ErrNotFound) {
			return c.JSON(http.StatusNotFound, httputil.ErrorResponse{Error: "audit not found"})
		}
		if errors.Is(err, auditapp.ErrUnavailable) {
			return c.JSON(http.StatusServiceUnavailable, httputil.ErrorResponse{Error: "audit unavailable"})
		}
		return c.JSON(http.StatusInternalServerError, httputil.ErrorResponse{Error: "failed to get latest audit"})
	}
	return c.JSON(http.StatusOK, buildStatusResponse(audit, events))
}

func (h *Handler) Cancel(c *echo.Context) error {
	ctx := c.Request().Context()
	id := c.Param("id")
	if id == "" {
		return c.JSON(http.StatusBadRequest, httputil.ErrorResponse{Error: "audit id is required"})
	}

	if err := h.Service.Cancel(ctx, id); err != nil {
		if errors.Is(err, auditapp.ErrNotFound) {
			return c.JSON(http.StatusNotFound, httputil.ErrorResponse{Error: "audit not found"})
		}
		if errors.Is(err, auditapp.ErrUnavailable) {
			return c.JSON(http.StatusServiceUnavailable, httputil.ErrorResponse{Error: "audit unavailable"})
		}
		return c.JSON(http.StatusConflict, httputil.ErrorResponse{Error: "audit cannot be cancelled"})
	}
	return c.JSON(http.StatusOK, map[string]string{"status": "cancelled", "audit_id": id})
}

func (h *Handler) List(c *echo.Context) error {
	ctx := c.Request().Context()
	limit, _ := strconv.Atoi(c.QueryParam("limit"))
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	offset, _ := strconv.Atoi(c.QueryParam("offset"))

	list, total, err := h.Service.List(ctx, limit, offset)
	if err != nil {
		if errors.Is(err, auditapp.ErrUnavailable) {
			return c.JSON(http.StatusServiceUnavailable, httputil.ErrorResponse{Error: "audit unavailable"})
		}
		return c.JSON(http.StatusInternalServerError, httputil.ErrorResponse{Error: "failed to list audits"})
	}
	return c.JSON(http.StatusOK, auditListResponse{Audits: list, Total: total})
}

func buildStatusResponse(audit *domain.Audit, events []domain.EventRecord) auditStatusResponse {
	return auditStatusResponse{
		AuditID:   audit.ID,
		Status:    string(audit.Status),
		CreatedAt: audit.CreatedAt,
		Target: domain.Target{
			Cluster: audit.ClusterFingerprint, ClusterDisplay: audit.ClusterDisplay,
		},
		Events: events,
		Result: audit.Result,
		Error:  audit.ErrorMessage,
	}
}

func isTerminalStatus(status domain.Status) bool {
	return status == domain.StatusCompleted ||
		status == domain.StatusFailed ||
		status == domain.StatusCancelled
}
