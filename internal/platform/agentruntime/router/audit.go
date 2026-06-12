package router

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/kubewise/kubewise/internal/platform/agentruntime"
	"github.com/kubewise/kubewise/internal/platform/agentruntime/audit"
	"github.com/kubewise/kubewise/internal/platform/agentruntime/event"
	"github.com/kubewise/kubewise/internal/platform/cluster"
	"github.com/kubewise/kubewise/internal/utils/log"
	"go.uber.org/zap"
)

func (a *Agent) AuditClusterStream(ctx context.Context, clusterName, queryID string, eventCh chan<- event.Event) error {
	a.streamMu.Lock()
	defer a.streamMu.Unlock()

	se := event.NewEmitter(eventCh, queryID)
	emit := func(ev event.Event) {
		_ = se.Emit(ctx, ev)
	}

	emit(event.Phase{
		QueryID: queryID,
		Phase:   "starting cluster audit",
		Summary: "running deterministic security scan",
		Payload: &event.Payload{
			Kind: event.PayloadKindTarget,
			Data: map[string]string{"cluster": clusterName},
		},
	})

	k8sClient, err := a.auditK8sClient(ctx, clusterName)
	if err != nil {
		log.Ctx(ctx).Error("cluster audit selection failed",
			zap.String("event", "agent.error"),
			zap.String("cluster", clusterName),
			zap.Error(err),
		)
		emit(event.StreamErr{QueryID: queryID, Err: err})
		return err
	}

	progressCh := make(chan agentruntime.ProgressEvent, 32)
	done := make(chan error, 1)
	go func() {
		done <- audit.NewRunner(k8sClient).Run(ctx, clusterName, queryID, progressCh)
		close(progressCh)
	}()

	for pe := range progressCh {
		if mapped, ok := auditProgressToEvent(queryID, pe); ok {
			emit(mapped)
		}
	}

	if err := <-done; err != nil {
		log.Ctx(ctx).Error("cluster audit failed",
			zap.String("event", "agent.error"),
			zap.String("cluster", clusterName),
			zap.Error(err),
		)
		emit(event.StreamErr{QueryID: queryID, Err: err})
		return err
	}

	emit(event.StreamDone{QueryID: queryID})
	return nil
}

func (a *Agent) auditK8sClient(ctx context.Context, clusterName string) (*cluster.Client, error) {
	if clusterName == "" || a.clusterManager == nil {
		if a.k8sClient == nil {
			return nil, fmt.Errorf("single-context Kubernetes client is not configured")
		}
		return a.k8sClient, nil
	}
	cc, err := a.clusterManager.GetClient(ctx, clusterName)
	if err != nil {
		return nil, err
	}
	return cluster.NewClientFromClusterClient(cc)
}

func auditProgressToEvent(queryID string, pe agentruntime.ProgressEvent) (event.Event, bool) {
	switch pe.Type {
	case "phase_start":
		return event.Phase{QueryID: queryID, Phase: pe.Message, Summary: pe.Summary}, true
	case "phase_done":
		return event.ToolDone{
			QueryID: queryID, ToolName: pe.Message, Summary: pe.Summary,
			Payload: &event.Payload{Kind: pe.PayloadKind, Data: decodePayload(pe.PayloadJSON)},
		}, true
	case "phase_fail":
		return event.ToolFail{QueryID: queryID, ToolName: pe.Message, Err: pe.Detail}, true
	case "audit_complete":
		return event.AgentDone{
			QueryID: queryID, Summary: pe.Summary,
			Payload: &event.Payload{Kind: pe.PayloadKind, Data: decodePayload(pe.PayloadJSON)},
		}, true
	default:
		return nil, false
	}
}

func decodePayload(raw string) any {
	if raw == "" {
		return nil
	}
	var data any
	if err := json.Unmarshal([]byte(raw), &data); err != nil {
		return raw
	}
	return data
}
