package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"

	"github.com/kubewise/kubewise/pkg/tui/events"
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
	eventCh := make(chan events.TUIEvent, 64)

	go func() {
		defer close(eventCh)
		_ = h.querier.HandleQueryStream(ctx, query, queryID, eventCh)
	}()

	defer h.cleanupPendingConfirms(queryID)

	for ev := range eventCh {
		if ctx.Err() != nil {
			break
		}
		if err := h.bridgeEvent(sse, ev); err != nil {
			break
		}
	}

	return nil
}

func (h *Handler) bridgeEvent(sse *SSEWriter, ev events.TUIEvent) error {
	switch e := ev.(type) {
	case events.PhaseEvent:
		return sse.WriteEvent("phase", PhaseData{QueryID: e.QueryID, Phase: e.Phase})

	case events.AgentStartEvent:
		return sse.WriteEvent("agent_start", AgentStartData{QueryID: e.QueryID, AgentName: e.AgentName})

	case events.AgentDoneEvent:
		return sse.WriteEvent("agent_done", AgentDoneData{
			QueryID: e.QueryID, Duration: e.Duration,
			InTokens: e.InTokens, OutTokens: e.OutTokens,
		})

	case events.ToolCallEvent:
		return sse.WriteEvent("tool_call", ToolCallData{
			QueryID: e.QueryID, ToolName: e.ToolName, Step: e.Step,
		})

	case events.ToolDoneEvent:
		return sse.WriteEvent("tool_done", ToolDoneData{
			QueryID: e.QueryID, ToolName: e.ToolName, Step: e.Step, Elapsed: e.Elapsed,
		})

	case events.RenderTextEvent:
		return sse.WriteEvent("render_text", RenderTextData{QueryID: e.QueryID, Text: e.Text})

	case events.RenderTableEvent:
		return sse.WriteEvent("render_table", RenderTableData{
			QueryID: e.QueryID, Headers: e.Headers, Rows: e.Rows,
		})

	case events.RenderCodeEvent:
		return sse.WriteEvent("render_code", RenderCodeData{
			QueryID: e.QueryID, Language: e.Language, Content: e.Content,
		})

	case events.RenderKVEvent:
		pairs := make([]KVPair, len(e.Pairs))
		for i, p := range e.Pairs {
			pairs[i] = KVPair{Key: p.Key, Value: p.Value}
		}
		return sse.WriteEvent("render_kv", RenderKVData{QueryID: e.QueryID, Pairs: pairs})

	case events.RenderListEvent:
		items := make([]ListItem, len(e.Items))
		for i, it := range e.Items {
			items[i] = ListItem{Status: it.Status, Text: it.Text}
		}
		return sse.WriteEvent("render_list", RenderListData{QueryID: e.QueryID, Items: items})

	case events.ConfirmRequestEvent:
		return h.handleConfirmRequestSSE(sse, e)

	case events.StreamDoneEvent:
		return sse.WriteEvent("stream_done", StreamDoneData{QueryID: e.QueryID, Result: e.Result})

	case events.StreamErrEvent:
		return sse.WriteEvent("stream_err", StreamErrData{QueryID: e.QueryID, Error: e.Err.Error()})

	case events.SupervisorEvent:
		return sse.WriteEvent("supervisor", SupervisorData{
			QueryID: e.QueryID, Reason: e.Reason, Decision: e.Decision, Detail: e.Detail,
		})

	case events.RenderDetailEvent:
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

	default:
		return nil
	}
}

func (h *Handler) handleConfirmRequestSSE(sse *SSEWriter, ev events.ConfirmRequestEvent) error {
	confirmID := uuid.New().String()

	h.mu.Lock()
	h.pendingConfirms[confirmID] = &pendingConfirm{queryID: ev.QueryID, respCh: ev.RespCh}
	h.mu.Unlock()

	stepJSON, err := json.Marshal(ev.Step)
	if err != nil {
		stepJSON = []byte("{}")
	}

	data := ConfirmRequestData{
		ConfirmID:  confirmID,
		QueryID:    ev.QueryID,
		Step:       stepJSON,
		TotalSteps: ev.TotalSteps,
	}

	return sse.WriteEvent("confirm_request", data)
}
