package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"

	"github.com/kubewise/kubewise/pkg/stream"
)

func (h *Handler) ChatStream(c *echo.Context) error {
	query := c.QueryParam("query")
	if query == "" {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "query parameter is required"})
	}

	sse, err := NewSSEWriter(c.Response())
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}

	ctx, cancel := context.WithCancel(c.Request().Context())
	defer cancel()

	queryID := fmt.Sprintf("q-%s", uuid.New().String()[:8])
	eventCh := make(chan stream.Event, 64)

	go func() {
		defer close(eventCh)
		_ = h.querier.HandleQueryStream(ctx, query, queryID, eventCh)
	}()

	defer h.cleanupPendingInteractions(queryID)

	for ev := range eventCh {
		if ctx.Err() != nil {
			break
		}
		if err := h.bridgeStreamEvent(sse, ev); err != nil {
			break
		}
	}

	return nil
}

func (h *Handler) bridgeStreamEvent(sse *SSEWriter, ev stream.Event) error {
	switch e := ev.(type) {
	case stream.InteractionRequest:
		return h.handleStreamInteractionSSE(sse, e)

	case stream.Phase:
		return sse.WriteEvent("phase", PhaseData{QueryID: e.QueryID, Phase: e.Phase})

	case stream.AgentStart:
		return sse.WriteEvent("agent_start", AgentStartData{QueryID: e.QueryID, AgentName: e.AgentName})

	case stream.AgentDone:
		return sse.WriteEvent("agent_done", AgentDoneData{
			QueryID: e.QueryID, Result: e.Result,
			Duration: e.Duration, InTokens: e.InTokens, OutTokens: e.OutTokens,
		})

	case stream.ToolCall:
		return sse.WriteEvent("tool_call", ToolCallData{
			QueryID: e.QueryID, ToolName: e.ToolName, Step: e.Step,
		})

	case stream.ToolDone:
		return sse.WriteEvent("tool_done", ToolDoneData{
			QueryID: e.QueryID, ToolName: e.ToolName, Step: e.Step, Elapsed: e.Elapsed,
		})

	case stream.ToolFail:
		return sse.WriteEvent("tool_fail", ToolFailData{
			QueryID: e.QueryID, ToolName: e.ToolName, Step: e.Step, Elapsed: e.Elapsed, Error: e.Err,
		})

	case stream.LLMTextDelta:
		return sse.WriteEvent("llm_text_delta", LLMTextDeltaData{QueryID: e.QueryID, Delta: e.Delta})

	case stream.Supervisor:
		return sse.WriteEvent("supervisor", SupervisorData{
			QueryID: e.QueryID, Reason: e.Reason, Decision: e.Decision, Detail: e.Detail,
		})

	case stream.StreamDone:
		return sse.WriteEvent("stream_done", StreamDoneData{QueryID: e.QueryID})

	case stream.StreamErr:
		msg := ""
		if e.Err != nil {
			msg = e.Err.Error()
		}
		return sse.WriteEvent("stream_err", StreamErrData{QueryID: e.QueryID, Error: msg})

	default:
		return sse.WriteEvent("unknown_stream_event", UnknownStreamEventData{EventType: fmt.Sprintf("%T", ev)})
	}
}

func (h *Handler) handleStreamInteractionSSE(sse *SSEWriter, e stream.InteractionRequest) error {
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