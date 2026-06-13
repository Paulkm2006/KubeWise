package httpapi

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	diagapp "github.com/kubewise/kubewise/internal/diagnosis/application"
	"github.com/kubewise/kubewise/internal/diagnosis/domain"
	httputil "github.com/kubewise/kubewise/internal/transport/httputil"
	"github.com/kubewise/kubewise/internal/utils/log"
	"github.com/labstack/echo/v5"
	"go.uber.org/zap"
)

type Handler struct {
	Service *diagapp.Service
}

type diagnoseRequest struct {
	Cluster   string `json:"cluster"`
	Namespace string `json:"namespace"`
	Pod       string `json:"pod"`
}

type diagnoseResponse struct {
	DiagnosisID string `json:"diagnosis_id"`
	Status      string `json:"status"`
}

type diagnosisStatusResponse struct {
	DiagnosisID string               `json:"diagnosis_id"`
	Status      string               `json:"status"`
	CreatedAt   time.Time            `json:"created_at"`
	Target      domain.Target        `json:"target"`
	Events      []domain.EventRecord `json:"events"`
	Result      *domain.Result       `json:"result,omitempty"`
}

type diagnosisListResponse struct {
	Diagnoses []domain.Diagnosis `json:"diagnoses"`
	Total     int                `json:"total"`
}

func (h *Handler) Start(c *echo.Context) error {
	ctx := c.Request().Context()
	var req diagnoseRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, httputil.ErrorResponse{Error: "invalid request"})
	}
	if req.Cluster == "" || req.Namespace == "" || req.Pod == "" {
		return c.JSON(http.StatusBadRequest, httputil.ErrorResponse{Error: "cluster, namespace, and pod are required"})
	}

	id, err := h.Service.Start(ctx, domain.Target{
		Cluster: req.Cluster, ClusterDisplay: req.Cluster,
		Namespace: req.Namespace, Pod: req.Pod,
	})
	if err != nil {
		if errors.Is(err, diagapp.ErrUnavailable) {
			return c.JSON(http.StatusServiceUnavailable, httputil.ErrorResponse{Error: "diagnosis unavailable"})
		}
		log.Ctx(ctx).Error("start diagnosis failed", zap.Error(err))
		return c.JSON(http.StatusInternalServerError, httputil.ErrorResponse{Error: "failed to start diagnosis"})
	}

	log.Ctx(ctx).Info("diagnosis started",
		zap.String("diagnosis_id", id),
		zap.String("cluster", req.Cluster),
		zap.String("namespace", req.Namespace),
		zap.String("pod", req.Pod),
	)
	return c.JSON(http.StatusAccepted, diagnoseResponse{DiagnosisID: id, Status: "running"})
}

func (h *Handler) StreamEvents(c *echo.Context) error {
	ctx := c.Request().Context()
	id := c.Param("id")
	if id == "" {
		return c.JSON(http.StatusBadRequest, httputil.ErrorResponse{Error: "diagnosis id is required"})
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
				log.Ctx(ctx).Warn("diagnosis stream query error", zap.Error(err))
				continue
			}
			for _, ev := range events {
				if err := sse.WriteEventWithID("diagnosis_event", ev.SeqNum, ev); err != nil {
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
					"status": string(status.Status), "diagnosis_id": id,
				})
				return nil
			}
		}
	}
}

func (h *Handler) Get(c *echo.Context) error {
	ctx := c.Request().Context()
	id := c.Param("id")

	diag, events, err := h.Service.Get(ctx, id)
	if err != nil {
		return c.JSON(http.StatusNotFound, httputil.ErrorResponse{Error: "diagnosis not found"})
	}

	return c.JSON(http.StatusOK, buildStatusResponse(diag, events))
}

func (h *Handler) Latest(c *echo.Context) error {
	ctx := c.Request().Context()
	cluster := c.QueryParam("cluster")
	namespace := c.QueryParam("namespace")
	pod := c.QueryParam("pod")
	if cluster == "" || namespace == "" || pod == "" {
		return c.JSON(http.StatusBadRequest, httputil.ErrorResponse{Error: "cluster, namespace, and pod are required"})
	}

	diag, events, err := h.Service.Latest(ctx, domain.Target{
		Cluster: cluster, ClusterDisplay: cluster,
		Namespace: namespace, Pod: pod,
	})
	if err != nil {
		if errors.Is(err, diagapp.ErrNotFound) {
			return c.JSON(http.StatusNotFound, httputil.ErrorResponse{Error: "diagnosis not found"})
		}
		if errors.Is(err, diagapp.ErrUnavailable) {
			return c.JSON(http.StatusServiceUnavailable, httputil.ErrorResponse{Error: "diagnosis unavailable"})
		}
		return c.JSON(http.StatusInternalServerError, httputil.ErrorResponse{Error: "failed to get latest diagnosis"})
	}

	return c.JSON(http.StatusOK, buildStatusResponse(diag, events))
}

func (h *Handler) Cancel(c *echo.Context) error {
	ctx := c.Request().Context()
	id := c.Param("id")
	if id == "" {
		return c.JSON(http.StatusBadRequest, httputil.ErrorResponse{Error: "diagnosis id is required"})
	}

	if err := h.Service.Cancel(ctx, id); err != nil {
		if errors.Is(err, diagapp.ErrNotFound) {
			return c.JSON(http.StatusNotFound, httputil.ErrorResponse{Error: "diagnosis not found"})
		}
		if errors.Is(err, diagapp.ErrUnavailable) {
			return c.JSON(http.StatusServiceUnavailable, httputil.ErrorResponse{Error: "diagnosis unavailable"})
		}
		return c.JSON(http.StatusConflict, httputil.ErrorResponse{Error: "diagnosis cannot be cancelled"})
	}

	return c.JSON(http.StatusOK, map[string]string{"status": "cancelled", "diagnosis_id": id})
}

func buildStatusResponse(diag *domain.Diagnosis, events []domain.EventRecord) diagnosisStatusResponse {
	resp := diagnosisStatusResponse{
		DiagnosisID: diag.ID,
		Status:      string(diag.Status),
		CreatedAt:   diag.CreatedAt,
		Target: domain.Target{
			Cluster: diag.ClusterFingerprint, ClusterDisplay: diag.ClusterDisplay,
			Namespace: diag.Namespace, Pod: diag.Pod,
		},
		Events: events,
	}
	if diag.Status == domain.StatusCompleted && diag.Report != nil {
		resp.Result = diag.Report
	} else if diag.Status == domain.StatusCompleted && diag.RootCause != "" {
		resp.Result = &domain.Result{
			Verdict: domain.VerdictInconclusive,
			RootCause: domain.RootCause{
				Title: diag.RootCause, Summary: diag.RootCause, ConfidenceLabel: diag.Confidence,
			},
			Evidence: diag.Evidence, Actions: diag.FixActions,
			Impact: domain.Impact{Description: diag.Impact}, DurationMs: diag.DurationMs,
		}
	}
	if diag.Status == domain.StatusFailed && diag.RootCause != "" {
		resp.Result = &domain.Result{
			Verdict:   domain.VerdictInconclusive,
			RootCause: domain.RootCause{Summary: diag.RootCause},
		}
	}
	return resp
}

func isTerminalStatus(status domain.Status) bool {
	return status == domain.StatusCompleted ||
		status == domain.StatusFailed ||
		status == domain.StatusCancelled
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
		if errors.Is(err, diagapp.ErrUnavailable) {
			return c.JSON(http.StatusServiceUnavailable, httputil.ErrorResponse{Error: "diagnosis unavailable"})
		}
		return c.JSON(http.StatusInternalServerError, httputil.ErrorResponse{Error: "failed to list diagnoses"})
	}
	return c.JSON(http.StatusOK, diagnosisListResponse{Diagnoses: list, Total: total})
}
