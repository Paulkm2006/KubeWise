package handler

import (
	"context"
	"fmt"
	"net/http"
	"time"

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

	buf := h.diagnosisRunner.GetBuffer(id)
	if buf == nil {
		log.Ctx(ctx).Warn("diagnosis not found",
			zap.String("diagnosis_id", id),
		)
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: "diagnosis not found"})
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"diagnosis_id": id,
		"status":       "running",
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
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "id parameter is required"})
	}

	sse, err := ssestream.NewSSEWriter(c.Response())
	if err != nil {
		log.Ctx(ctx).Error("diagnosis stream: failed to create SSE writer", zap.Error(err))
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}

	log.Ctx(ctx).Debug("diagnosis events streaming started",
		zap.String("diagnosis_id", id),
	)

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	sent := 0

	for {
		select {
		case <-ctx.Done():
			log.Ctx(ctx).Debug("diagnosis stream: context cancelled")
			return nil
		case <-ticker.C:
			buf := h.diagnosisRunner.GetBuffer(id)
			if buf == nil {
				log.Ctx(ctx).Debug("diagnosis stream: buffer gone, closing")
				return nil
			}

			events := buf.ReadSince(sent)
			for _, ev := range events {
				if err := sse.WriteEvent("diagnosis_event", ev); err != nil {
					log.Ctx(ctx).Warn("diagnosis stream: write error, closing", zap.Error(err))
					return nil
				}
				sent++
				if ev.Type == "stream_done" || ev.Type == "error" {
					return nil
				}
			}
		}
	}
}