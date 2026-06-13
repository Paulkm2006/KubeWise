package application

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/kubewise/kubewise/internal/platform/agentruntime/event"
	"github.com/kubewise/kubewise/internal/transport/http/ssestream"
)

type phaseData struct {
	QueryID string `json:"query_id"`
	Phase   string `json:"phase"`
}

type agentStartData struct {
	QueryID   string `json:"query_id"`
	AgentName string `json:"agent_name"`
}

type agentDoneData struct {
	QueryID   string        `json:"query_id"`
	Result    string        `json:"result"`
	Duration  time.Duration `json:"duration"`
	InTokens  int           `json:"in_tokens"`
	OutTokens int           `json:"out_tokens"`
}

type toolCallData struct {
	QueryID  string `json:"query_id"`
	ToolName string `json:"tool_name"`
	Step     int    `json:"step"`
}

type toolDoneData struct {
	QueryID  string        `json:"query_id"`
	ToolName string        `json:"tool_name"`
	Step     int           `json:"step"`
	Elapsed  time.Duration `json:"elapsed"`
}

type toolFailData struct {
	QueryID  string        `json:"query_id"`
	ToolName string        `json:"tool_name"`
	Step     int           `json:"step"`
	Elapsed  time.Duration `json:"elapsed"`
	Error    string        `json:"error"`
}

type llmTextDeltaData struct {
	QueryID string `json:"query_id"`
	Delta   string `json:"delta"`
}

type interactionRequestData struct {
	InteractionID string          `json:"interaction_id"`
	QueryID       string          `json:"query_id"`
	Kind          string          `json:"kind"`
	Payload       json.RawMessage `json:"payload"`
	TotalSteps    int             `json:"total_steps,omitempty"`
}

type streamDoneData struct {
	QueryID string `json:"query_id"`
}

type streamErrData struct {
	QueryID string `json:"query_id"`
	Error   string `json:"error"`
}

type supervisorData struct {
	QueryID  string `json:"query_id"`
	Reason   string `json:"reason"`
	Decision string `json:"decision"`
	Detail   string `json:"detail"`
}

func (s *ChatService) bridgeEvent(sse *ssestream.SSEWriter, ev event.Event) error {
	switch e := ev.(type) {
	case event.InteractionRequest:
		return s.handleInteractionSSE(sse, e)
	case event.Phase:
		return sse.WriteEvent("phase", phaseData{QueryID: e.QueryID, Phase: e.Phase})
	case event.AgentStart:
		return sse.WriteEvent("agent_start", agentStartData{QueryID: e.QueryID, AgentName: e.AgentName})
	case event.AgentDone:
		return sse.WriteEvent("agent_done", agentDoneData{
			QueryID: e.QueryID, Result: e.Result,
			Duration: e.Duration, InTokens: e.InTokens, OutTokens: e.OutTokens,
		})
	case event.ToolCall:
		return sse.WriteEvent("tool_call", toolCallData{QueryID: e.QueryID, ToolName: e.ToolName, Step: e.Step})
	case event.ToolDone:
		return sse.WriteEvent("tool_done", toolDoneData{
			QueryID: e.QueryID, ToolName: e.ToolName, Step: e.Step, Elapsed: e.Elapsed,
		})
	case event.ToolFail:
		return sse.WriteEvent("tool_fail", toolFailData{
			QueryID: e.QueryID, ToolName: e.ToolName, Step: e.Step, Elapsed: e.Elapsed, Error: e.Err,
		})
	case event.LLMTextDelta:
		return sse.WriteEvent("llm_text_delta", llmTextDeltaData{QueryID: e.QueryID, Delta: e.Delta})
	case event.Supervisor:
		return sse.WriteEvent("supervisor", supervisorData{
			QueryID: e.QueryID, Reason: e.Reason, Decision: e.Decision, Detail: e.Detail,
		})
	case event.StreamDone:
		return sse.WriteEvent("stream_done", streamDoneData{QueryID: e.QueryID})
	case event.StreamErr:
		msg := ""
		if e.Err != nil {
			msg = e.Err.Error()
		}
		return sse.WriteEvent("stream_err", streamErrData{QueryID: e.QueryID, Error: msg})
	default:
		return sse.WriteEvent("unknown_stream_event", map[string]string{"event_type": fmt.Sprintf("%T", ev)})
	}
}

func (s *ChatService) handleInteractionSSE(sse *ssestream.SSEWriter, e event.InteractionRequest) error {
	interactionID := uuid.New().String()
	s.mu.Lock()
	s.pending[interactionID] = &pendingInteraction{queryID: e.QueryID, respCh: e.RespCh}
	s.mu.Unlock()

	payload := json.RawMessage(e.Payload)
	if len(payload) == 0 {
		payload = json.RawMessage("{}")
	}
	return sse.WriteEvent("interaction_request", interactionRequestData{
		InteractionID: interactionID,
		QueryID:       e.QueryID,
		Kind:          string(e.Kind),
		Payload:       payload,
		TotalSteps:    e.TotalSteps,
	})
}
