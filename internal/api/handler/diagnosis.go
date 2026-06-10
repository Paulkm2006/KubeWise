package handler

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
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

var runner = diagnosis.NewRunner()

// POST /api/v1/diagnose
func (h *Handler) StartDiagnose(c *echo.Context) error {
	ctx := c.Request().Context()
	var req DiagnoseRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "invalid request"})
	}
	if req.Cluster == "" || req.Namespace == "" || req.Pod == "" {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "cluster, namespace, and pod are required"})
	}

	diagID := uuid.New().String()
	target := diagnosis.DiagnosisTarget{
		Cluster:        req.Cluster,
		ClusterDisplay: req.Cluster,
		Namespace:      req.Namespace,
		Pod:            req.Pod,
	}

	if err := runner.Start(ctx, diagID, target); err != nil {
		log.Ctx(ctx).Error("diagnose: failed to start", zap.Error(err))
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "failed to start diagnosis"})
	}

	log.Ctx(ctx).Info("diagnosis started",
		zap.String("diagnosis_id", diagID),
		zap.String("cluster", req.Cluster),
		zap.String("namespace", req.Namespace),
		zap.String("pod", req.Pod),
	)

	// Async: run diagnosis via troubleshooting agent
	go func() {
		eventCh := make(chan event.Event, 64)
		queryID := fmt.Sprintf("diag-%s", uuid.New().String()[:8])
		query := fmt.Sprintf("I see pod '%s' in namespace '%s' on cluster '%s' is unhealthy. Diagnose the issue.",
			req.Pod, req.Namespace, req.Cluster)
		diagCtx := context.Background()

		err := h.querier.HandleQueryStream(diagCtx, query, queryID, eventCh)
		close(eventCh)

		if err != nil {
			runner.PushEvent(diagCtx, diagID, event.StreamErr{
				QueryID: queryID, Err: err,
			})
			runner.SetFailed(diagCtx, diagID, err.Error())
			return
		}

		var finalResult string
		for ev := range eventCh {
			runner.PushEvent(diagCtx, diagID, ev)
			if done, ok := ev.(event.AgentDone); ok {
				finalResult = done.Result
			}
		}

		result := parseAgentResult(finalResult)
		runner.SetCompleted(diagCtx, diagID, result)
	}()

	return c.JSON(http.StatusAccepted, DiagnoseResponse{
		DiagnosisID: diagID,
		Status:      "running",
	})
}

// GET /api/v1/diagnose/stream?id=X&since=N
func (h *Handler) StreamDiagnosisEvents(c *echo.Context) error {
	ctx := c.Request().Context()
	id := c.QueryParam("id")
	if id == "" {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "id parameter is required"})
	}

	since := 0
	if lastID := c.Request().Header.Get("Last-Event-ID"); lastID != "" {
		since, _ = strconv.Atoi(lastID)
	}
	if s := c.QueryParam("since"); s != "" {
		since, _ = strconv.Atoi(s)
	}

	sse, err := ssestream.NewSSEWriter(c.Response())
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}

	log.Ctx(ctx).Debug("diagnosis stream: started",
		zap.String("diagnosis_id", id),
		zap.Int("since", since),
	)

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	hasSentEvents := false

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			events, err := runner.GetEventsSince(ctx, id, since)
			if err != nil {
				log.Ctx(ctx).Warn("diagnosis stream: query error", zap.Error(err))
				continue
			}

			for _, ev := range events {
				if err := sse.WriteEventWithID("diagnosis_event", ev.SeqNum, ev); err != nil {
					return nil
				}
				since = ev.SeqNum
				hasSentEvents = true
			}

			status, err := runner.GetStatus(ctx, id)
			if err == nil {
				if (status.Status == diagnosis.StatusCompleted || status.Status == diagnosis.StatusFailed) && len(events) == 0 && hasSentEvents {
					sse.WriteEvent("stream_complete", map[string]string{
						"status":       string(status.Status),
						"diagnosis_id": id,
					})
					return nil
				}
			}
		}
	}
}

// GET /api/v1/diagnose/:id
func (h *Handler) GetDiagnosis(c *echo.Context) error {
	ctx := c.Request().Context()
	id := c.Param("id")

	diag, err := runner.GetStatus(ctx, id)
	if err != nil {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: "diagnosis not found"})
	}

	events, _ := runner.GetEventsSince(ctx, id, 0)

	resp := DiagnosisStatusResponse{
		DiagnosisID: id,
		Status:      string(diag.Status),
		Target: diagnosis.DiagnosisTarget{
			Cluster:        diag.ClusterFingerprint,
			ClusterDisplay: diag.ClusterDisplay,
			Namespace:      diag.Namespace,
			Pod:            diag.Pod,
		},
		Events: events,
	}

	if diag.Status == diagnosis.StatusCompleted && diag.RootCause != "" {
		resp.Result = &diagnosis.DiagnosisResult{
			RootCause:  diag.RootCause,
			Confidence: diag.Confidence,
			Evidence:   diag.Evidence,
			FixActions: diag.FixActions,
			Impact:     diag.Impact,
			DurationMs: diag.DurationMs,
		}
	}

	return c.JSON(http.StatusOK, resp)
}

// GET /api/v1/diagnoses
func (h *Handler) ListDiagnoses(c *echo.Context) error {
	ctx := c.Request().Context()
	limit, _ := strconv.Atoi(c.QueryParam("limit"))
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	offset, _ := strconv.Atoi(c.QueryParam("offset"))

	list, total, err := runner.List(ctx, limit, offset)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "failed to list diagnoses"})
	}

	return c.JSON(http.StatusOK, DiagnosisListResponse{Diagnoses: list, Total: total})
}

func parseAgentResult(output string) *diagnosis.DiagnosisResult {
	result := &diagnosis.DiagnosisResult{
		RootCause:  output,
		Confidence: "medium",
	}
	if len(output) > 500 {
		result.RootCause = output[:500] + "..."
	}
	if output != "" {
		result.Evidence = []diagnosis.Evidence{
			{Num: 1, Text: "Agent analysis completed"},
		}
	}
	return result
}