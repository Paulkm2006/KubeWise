package runtime

import (
	"encoding/json"

	"github.com/kubewise/kubewise/internal/platform/agentruntime"
	"github.com/kubewise/kubewise/internal/platform/agentruntime/event"
)

func toProgressEvent(ev event.Event) (agentruntime.ProgressEvent, bool) {
	switch e := ev.(type) {
	case event.Phase:
		return withPayload(agentruntime.ProgressEvent{Type: "phase", Message: e.Phase, Summary: e.Summary}, e.Payload), true
	case event.AgentStart:
		return agentruntime.ProgressEvent{Type: "agent_start", Message: e.AgentName}, true
	case event.AgentDone:
		return withPayload(agentruntime.ProgressEvent{
			Type: "agent_done", Result: e.Result,
			Summary: e.Summary,
			TokenIn: e.InTokens, TokenOut: e.OutTokens, ElapsedMs: int(e.Duration.Milliseconds()),
		}, e.Payload), true
	case event.ToolCall:
		return agentruntime.ProgressEvent{Type: "tool_call", Message: e.ToolName}, true
	case event.ToolDone:
		return withPayload(agentruntime.ProgressEvent{
			Type: "tool_done", Message: e.ToolName, Summary: e.Summary, ElapsedMs: int(e.Elapsed.Milliseconds()),
		}, e.Payload), true
	case event.ToolFail:
		return agentruntime.ProgressEvent{Type: "tool_fail", Message: e.ToolName, Detail: e.Err}, true
	case event.LLMTextDelta:
		return agentruntime.ProgressEvent{Type: "llm_text_delta", Message: e.Delta}, true
	case event.Supervisor:
		return agentruntime.ProgressEvent{Type: "supervisor", Message: e.Decision, Detail: e.Detail}, true
	case event.StreamDone:
		return agentruntime.ProgressEvent{Type: "stream_done"}, true
	case event.StreamErr:
		detail := ""
		if e.Err != nil {
			detail = e.Err.Error()
		}
		return agentruntime.ProgressEvent{Type: "stream_err", Detail: detail}, true
	case event.LLMStepDegraded:
		payload := map[string]any{
			"step": e.Step, "phase": e.Phase, "error": e.Err,
			"transient": e.Transient, "fallback": e.Fallback,
		}
		bs, _ := json.Marshal(payload)
		return agentruntime.ProgressEvent{
			Type:        "llm_step_degraded",
			Message:     e.Step,
			Summary:     e.Fallback,
			Detail:      e.Err,
			PayloadKind: event.PayloadKindDiagnosisLLMStep,
			PayloadJSON: string(bs),
		}, true
	default:
		return agentruntime.ProgressEvent{}, false
	}
}

func withPayload(pe agentruntime.ProgressEvent, p *event.Payload) agentruntime.ProgressEvent {
	if p == nil {
		return pe
	}
	pe.PayloadKind = p.Kind
	if bs, err := json.Marshal(p.Data); err == nil {
		pe.PayloadJSON = string(bs)
	}
	return pe
}
