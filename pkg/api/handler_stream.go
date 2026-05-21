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
			QueryID: e.QueryID, Duration: e.Duration,
			InTokens: e.InTokens, OutTokens: e.OutTokens,
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

	case stream.RenderText:
		return sse.WriteEvent("render_text", RenderTextData{QueryID: e.QueryID, Text: e.Text})

	case stream.RenderTable:
		return sse.WriteEvent("render_table", RenderTableData{
			QueryID: e.QueryID, Headers: e.Headers, Rows: e.Rows,
		})

	case stream.RenderCode:
		return sse.WriteEvent("render_code", RenderCodeData{
			QueryID: e.QueryID, Language: e.Language, Content: e.Content,
		})

	case stream.RenderKV:
		pairs := make([]KVPair, len(e.Pairs))
		for i, p := range e.Pairs {
			pairs[i] = KVPair{Key: p.Key, Value: p.Value}
		}
		return sse.WriteEvent("render_kv", RenderKVData{QueryID: e.QueryID, Pairs: pairs})

	case stream.RenderList:
		items := make([]ListItem, len(e.Items))
		for i, it := range e.Items {
			items[i] = ListItem{Status: it.Status, Text: it.Text}
		}
		return sse.WriteEvent("render_list", RenderListData{QueryID: e.QueryID, Items: items})

	case stream.RenderDetail:
		d := e.Detail
		data := RenderDetailData{
			QueryID: e.QueryID,
			Detail: ResourceDetailData{
				Kind: d.Kind, Name: d.Name, Namespace: d.Namespace,
				Status: d.Status, RecentLogs: d.RecentLogs, Labels: d.Labels,
			},
		}
		for _, c := range d.Containers {
			data.Detail.Containers = append(data.Detail.Containers, ContainerInfoData{
				Name: c.Name, Image: c.Image, Ready: c.Ready,
				RestartCount: c.RestartCount, State: c.State, Resources: c.Resources,
			})
		}
		for _, c := range d.Conditions {
			data.Detail.Conditions = append(data.Detail.Conditions, ConditionInfoData{
				Type: c.Type, Status: c.Status, Reason: c.Reason, Message: c.Message,
			})
		}
		for _, ev := range d.Events {
			data.Detail.Events = append(data.Detail.Events, EventInfoData{
				Type: ev.Type, Reason: ev.Reason, Message: ev.Message, Timestamp: ev.Timestamp,
			})
		}
		return sse.WriteEvent("render_detail", data)

	case stream.Supervisor:
		return sse.WriteEvent("supervisor", SupervisorData{
			QueryID: e.QueryID, Reason: e.Reason, Decision: e.Decision, Detail: e.Detail,
		})

	case stream.StreamDone:
		return sse.WriteEvent("stream_done", StreamDoneData{QueryID: e.QueryID, Result: e.Result})

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

