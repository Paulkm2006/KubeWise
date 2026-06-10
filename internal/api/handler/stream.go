package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"github.com/kubewise/kubewise/internal/agent/event"
	"github.com/kubewise/kubewise/internal/api/ssestream"
		"github.com/kubewise/kubewise/internal/diagnosis"
	"github.com/kubewise/kubewise/internal/utils/log"
	"github.com/labstack/echo/v5"
	"go.uber.org/zap"
	"net/http"
)

func (h *Handler) ChatStream(c *echo.Context) error {
	ctx := c.Request().Context()
	query := c.QueryParam("query")
	if query == "" {
		log.Ctx(ctx).Warn("chat stream: missing query parameter")
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "query parameter is required"})
	}

	sse, err := ssestream.NewSSEWriter(c.Response())
	if err != nil {
		log.Ctx(ctx).Error("chat stream: failed to create SSE writer", zap.Error(err))
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	log.Ctx(ctx).Info("chat stream started", zap.String("query_preview", truncate(query, 80)))

	queryID := fmt.Sprintf("q-%s", uuid.New().String()[:8])
	eventCh := make(chan event.Event, 64)

	go func() {
		defer close(eventCh)
		_ = h.querier.HandleQueryStream(ctx, query, queryID, eventCh)
	}()

	defer h.cleanupPendingInteractions(queryID)

	for ev := range eventCh {
		if ctx.Err() != nil {
			log.Ctx(ctx).Warn("chat stream: context cancelled, stopping event loop")
			break
		}
		if err := h.bridgeStreamEvent(sse, ev); err != nil {
			log.Ctx(ctx).Warn("chat stream: bridge write error, closing", zap.Error(err))
			break
		}
	}

	log.Ctx(ctx).Info("chat stream completed", zap.String("query_preview", truncate(query, 80)))
	return nil
}

func (h *Handler) bridgeStreamEvent(sse *ssestream.SSEWriter, ev event.Event) error {
	switch e := ev.(type) {
	case event.InteractionRequest:
		return h.handleStreamInteractionSSE(sse, e)

	case event.Phase:
		return sse.WriteEvent("phase", PhaseData{QueryID: e.QueryID, Phase: e.Phase})

	case event.AgentStart:
		return sse.WriteEvent("agent_start", AgentStartData{QueryID: e.QueryID, AgentName: e.AgentName})

	case event.AgentDone:
		return sse.WriteEvent("agent_done", AgentDoneData{
			QueryID: e.QueryID, Result: e.Result,
			Duration: e.Duration, InTokens: e.InTokens, OutTokens: e.OutTokens,
		})

	case event.ToolCall:
		return sse.WriteEvent("tool_call", ToolCallData{
			QueryID: e.QueryID, ToolName: e.ToolName, Step: e.Step,
		})

	case event.ToolDone:
		return sse.WriteEvent("tool_done", ToolDoneData{
			QueryID: e.QueryID, ToolName: e.ToolName, Step: e.Step, Elapsed: e.Elapsed,
		})

	case event.ToolFail:
		return sse.WriteEvent("tool_fail", ToolFailData{
			QueryID: e.QueryID, ToolName: e.ToolName, Step: e.Step, Elapsed: e.Elapsed, Error: e.Err,
		})

	case event.LLMTextDelta:
		return sse.WriteEvent("llm_text_delta", LLMTextDeltaData{QueryID: e.QueryID, Delta: e.Delta})

	case event.Supervisor:
		return sse.WriteEvent("supervisor", SupervisorData{
			QueryID: e.QueryID, Reason: e.Reason, Decision: e.Decision, Detail: e.Detail,
		})

	case event.StreamDone:
		return sse.WriteEvent("stream_done", StreamDoneData{QueryID: e.QueryID})

	case event.StreamErr:
		msg := ""
		if e.Err != nil {
			msg = e.Err.Error()
		}
		return sse.WriteEvent("stream_err", StreamErrData{QueryID: e.QueryID, Error: msg})

	default:
		return sse.WriteEvent("unknown_stream_event", UnknownStreamEventData{EventType: fmt.Sprintf("%T", ev)})
	}
}

func (h *Handler) handleStreamInteractionSSE(sse *ssestream.SSEWriter, e event.InteractionRequest) error {
	interactionID := uuid.New().String()
	h.mu.Lock()
	h.pendingInteractions[interactionID] = &pendingInteraction{queryID: e.QueryID, respCh: e.RespCh}
	h.mu.Unlock()

	payload := json.RawMessage(e.Payload)
	if len(payload) == 0 {
		payload = json.RawMessage("{}")
	}
	data := InteractionRequestData{
		InteractionID: interactionID,
		QueryID:       e.QueryID,
		Kind:          string(e.Kind),
		Payload:       payload,
		TotalSteps:    e.TotalSteps,
	}

	return sse.WriteEvent("interaction_request", data)
}

// bridgeAgentEventToDiagnosis converts agent event.Event types to
// diagnosis.StreamEvent for SSE streaming to the DiagnosisOverlay.
func bridgeAgentEventToDiagnosis(ev event.Event) *diagnosis.StreamEvent {
	switch e := ev.(type) {
	case event.Phase:
		return &diagnosis.StreamEvent{Type: "phase", Message: e.Phase}
	case event.AgentStart:
		return &diagnosis.StreamEvent{Type: "phase", Message: e.AgentName + " started"}
	case event.AgentDone:
		return &diagnosis.StreamEvent{Type: "done", Message: e.Result}
	case event.ToolCall:
		return &diagnosis.StreamEvent{Type: "tool", Message: e.ToolName}
	case event.ToolDone:
		return &diagnosis.StreamEvent{Type: "tool_done", Message: e.ToolName,
			Detail: fmt.Sprintf("%.0fms", e.Elapsed.Seconds()*1000)}
	case event.ToolFail:
		return &diagnosis.StreamEvent{Type: "tool_fail", Message: e.ToolName, Detail: e.Err}
	case event.LLMTextDelta:
		return &diagnosis.StreamEvent{Type: "thought", Message: e.Delta}
	case event.StreamDone:
		return &diagnosis.StreamEvent{Type: "stream_done", Message: "completed"}
	case event.StreamErr:
		msg := ""
		if e.Err != nil {
			msg = e.Err.Error()
		}
		return &diagnosis.StreamEvent{Type: "error", Message: msg}
	default:
		return nil
	}
}
