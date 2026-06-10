package handler

import (
	"context"
	"fmt"
	"net/http"

	"github.com/google/uuid"
	"github.com/kubewise/kubewise/internal/agent/event"
	"github.com/kubewise/kubewise/internal/api/ssestream"
	"github.com/kubewise/kubewise/internal/diagnosis"
	"github.com/kubewise/kubewise/internal/utils/log"
	"github.com/labstack/echo/v5"
	"go.uber.org/zap"
)

type DiagnoseRequest struct {
	Cluster   string `json:"cluster"`
	Namespace string `json:"namespace"`
	Pod       string `json:"pod"`
}

func (h *Handler) StartDiagnose(c *echo.Context) error {
	ctx := c.Request().Context()
	var req DiagnoseRequest
	if err := c.Bind(&req); err != nil {
		log.Ctx(ctx).Warn("diagnose: invalid request body", zap.Error(err))
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "invalid request", Detail: err.Error()})
	}

	if req.Cluster == "" || req.Namespace == "" || req.Pod == "" {
		log.Ctx(ctx).Warn("diagnose: missing required fields")
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "cluster, namespace, and pod are required"})
	}

	diagID := uuid.New().String()
	h.diagnosisRunner.Start(ctx, diagID)

	log.Ctx(ctx).Info("diagnosis started",
		zap.String("event", "diagnosis.started"),
		zap.String("diagnosis_id", diagID),
		zap.String("cluster", req.Cluster),
		zap.String("namespace", req.Namespace),
		zap.String("pod", req.Pod),
	)

	// Async: run diagnosis via troubleshooting agent
	go func() {
		eventCh := make(chan event.Event, 64)
		queryID := fmt.Sprintf("diag-%s", uuid.New().String()[:8])
		query := fmt.Sprintf("I see pod '%s' in namespace '%s' on cluster '%s' is unhealthy. Diagnose the issue.", req.Pod, req.Namespace, req.Cluster)
		diagCtx := context.Background()

		err := h.querier.HandleQueryStream(diagCtx, query, queryID, eventCh)
		if err != nil {
			h.diagnosisRunner.PushEvent(diagCtx, diagID, diagnosis.StreamEvent{
				Type: "error", Message: "Agent error", Detail: err.Error(),
			})
			h.diagnosisRunner.Finish(diagCtx, diagID)
			return
		}

		for ev := range eventCh {
			if se := bridgeAgentEventToDiagnosis(ev); se != nil {
				h.diagnosisRunner.PushEvent(diagCtx, diagID, *se)
			}
		}

		h.diagnosisRunner.Finish(diagCtx, diagID)
	}()

	return c.JSON(http.StatusAccepted, DiagnoseResponse{
		DiagnosisID: diagID,
		Status:      "running",
	})
}

func (h *Handler) GetDiagnosis(c *echo.Context) error {
	ctx := c.Request().Context()
	id := c.Param("id")
	if h.diagnosisRunner == nil {
		log.Ctx(ctx).Error("diagnosis runner not available", zap.String("diagnosis_id", id))
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: "diagnosis not found"})
	}
	buf := h.diagnosisRunner.GetBuffer(id)
	if buf == nil {
		log.Ctx(ctx).Warn("diagnosis buffer not found", zap.String("diagnosis_id", id))
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: "diagnosis not found"})
	}
	log.Ctx(ctx).Info("diagnosis retrieved",
		zap.String("diagnosis_id", id),
		zap.String("status", "running"),
	)
	return c.JSON(http.StatusOK, map[string]any{
		"id":     id,
		"status": "running",
		"events": buf.Drain(),
	})
}

func (h *Handler) ListDiagnoses(c *echo.Context) error {
	ctx := c.Request().Context()
	log.Ctx(ctx).Info("listed diagnoses", zap.Int("count", 0))
	return c.JSON(http.StatusOK, []any{})
}

func (h *Handler) StreamDiagnosisEvents(c *echo.Context) error {
	ctx := c.Request().Context()
	id := c.QueryParam("id")
	if id == "" {
		log.Ctx(ctx).Warn("diagnosis stream: missing id parameter")
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "id query parameter required"})
	}

	log.Ctx(ctx).Debug("diagnosis events requested",
		zap.String("diagnosis_id", id),
	)

	sse, err := ssestream.NewSSEWriter(c.Response())
	if err != nil {
		log.Ctx(ctx).Error("failed to create SSE writer", zap.Error(err))
		return err
	}

	if h.diagnosisRunner != nil {
		buf := h.diagnosisRunner.GetBuffer(id)
		if buf != nil {
			for _, ev := range buf.Drain() {
				if err := sse.WriteEvent("diagnosis_event", ev); err != nil {
					log.Ctx(ctx).Warn("SSE write error during diagnosis stream", zap.Error(err))
					return nil
				}
			}
		}
	}

	return nil
}