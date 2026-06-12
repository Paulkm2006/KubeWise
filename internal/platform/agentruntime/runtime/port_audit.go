package runtime

import (
	"context"
	"sync"

	"github.com/kubewise/kubewise/internal/platform/agentruntime"
	"github.com/kubewise/kubewise/internal/platform/agentruntime/event"
)

var _ agentruntime.AuditRunner = (*Runtime)(nil)

func (r *Runtime) AuditCluster(ctx context.Context, cluster, queryID string, out chan<- agentruntime.ProgressEvent) error {
	internalCh := make(chan event.Event, 64)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for ev := range internalCh {
			if pe, ok := toAuditProgressEvent(ev); ok {
				select {
				case out <- pe:
				case <-ctx.Done():
					return
				}
			}
		}
	}()

	err := r.Router.AuditClusterStream(ctx, cluster, queryID, internalCh)
	close(internalCh)
	wg.Wait()
	return err
}

func toAuditProgressEvent(ev event.Event) (agentruntime.ProgressEvent, bool) {
	switch e := ev.(type) {
	case event.AgentStart:
		return agentruntime.ProgressEvent{
			Type: "agent_start", Message: e.AgentName, Summary: "audit agent started",
		}, true
	case event.Phase:
		if e.Phase == "starting cluster audit" {
			return agentruntime.ProgressEvent{Type: "phase_start", Message: e.Phase, Summary: e.Summary}, true
		}
		return agentruntime.ProgressEvent{Type: "phase_start", Message: e.Phase, Summary: e.Summary}, true
	case event.ToolCall:
		return agentruntime.ProgressEvent{Type: "tool_call", Message: e.ToolName}, true
	case event.ToolDone:
		return withPayload(agentruntime.ProgressEvent{
			Type: "phase_done", Message: e.ToolName, Summary: e.Summary,
			ElapsedMs: int(e.Elapsed.Milliseconds()),
		}, e.Payload), true
	case event.ToolFail:
		return agentruntime.ProgressEvent{Type: "phase_fail", Message: e.ToolName, Detail: e.Err}, true
	case event.AgentDone:
		return withPayload(agentruntime.ProgressEvent{
			Type: "audit_complete", Summary: e.Summary,
			ElapsedMs: int(e.Duration.Milliseconds()),
		}, e.Payload), true
	case event.StreamDone:
		return agentruntime.ProgressEvent{Type: "stream_done"}, true
	case event.StreamErr:
		detail := ""
		if e.Err != nil {
			detail = e.Err.Error()
		}
		return agentruntime.ProgressEvent{Type: "stream_err", Detail: detail}, true
	default:
		return agentruntime.ProgressEvent{}, false
	}
}
