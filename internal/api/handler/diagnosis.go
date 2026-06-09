package handler

import (
	"net/http"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"
	"github.com/kubewise/kubewise/internal/api/ssestream"
)

type DiagnoseRequest struct {
	Cluster   string `json:"cluster"`
	Namespace string `json:"namespace"`
	Pod       string `json:"pod"`
}

func (h *Handler) StartDiagnose(c *echo.Context) error {
	var req DiagnoseRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "invalid request body"})
	}
	if req.Cluster == "" || req.Namespace == "" || req.Pod == "" {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "cluster, namespace, and pod are required"})
	}

	diagID := uuid.New().String()
	if h.diagnosisRunner != nil {
		h.diagnosisRunner.Start(diagID)
	}

	return c.JSON(http.StatusAccepted, map[string]string{
		"diagnosis_id": diagID,
		"status":       "pending",
	})
}

func (h *Handler) GetDiagnosis(c *echo.Context) error {
	id := c.Param("id")
	if h.diagnosisRunner == nil {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: "diagnosis not found"})
	}
	buf := h.diagnosisRunner.GetBuffer(id)
	if buf == nil {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: "diagnosis not found"})
	}
	return c.JSON(http.StatusOK, map[string]any{
		"id":     id,
		"status": "running",
		"events": buf.Drain(),
	})
}

func (h *Handler) ListDiagnoses(c *echo.Context) error {
	return c.JSON(http.StatusOK, []any{})
}

func (h *Handler) StreamDiagnosisEvents(c *echo.Context) error {
	id := c.QueryParam("id")
	if id == "" {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "id query parameter required"})
	}

	sse, err := ssestream.NewSSEWriter(c.Response())
	if err != nil {
		return err
	}

	if h.diagnosisRunner != nil {
		buf := h.diagnosisRunner.GetBuffer(id)
		if buf != nil {
			for _, ev := range buf.Drain() {
				if err := sse.WriteEvent("diagnosis_event", ev); err != nil {
					return nil
				}
			}
		}
	}

	return nil
}
