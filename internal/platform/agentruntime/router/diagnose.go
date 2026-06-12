package router

import (
	"context"

	"github.com/kubewise/kubewise/internal/platform/agentruntime"
	"github.com/kubewise/kubewise/internal/platform/agentruntime/diagnose"
	"github.com/kubewise/kubewise/internal/platform/agentruntime/diagnose/runtime"
	"github.com/kubewise/kubewise/internal/platform/agentruntime/event"
	"github.com/kubewise/kubewise/internal/utils/log"
	"go.uber.org/zap"
)

// DiagnosePodStream runs pod troubleshooting without intent classification.
func (a *Agent) DiagnosePodStream(ctx context.Context, params agentruntime.DiagnoseParams, queryID string, eventCh chan<- event.Event) error {
	a.streamMu.Lock()
	defer a.streamMu.Unlock()

	se := event.NewEmitter(eventCh, queryID)
	emit := func(ev event.Event) {
		_ = se.Emit(ctx, ev)
	}

	emit(event.Phase{
		QueryID: queryID,
		Phase:   "starting pod diagnosis",
		Summary: "running deterministic diagnosis pipeline",
		Payload: &event.Payload{
			Kind: event.PayloadKindTarget,
			Data: params,
		},
	})
	k8sClient, err := a.k8sClientForCluster(ctx, params.Cluster)
	if err != nil {
		log.Ctx(ctx).Error("pod diagnosis cluster selection failed",
			zap.String("event", "agent.error"),
			zap.String("cluster", params.Cluster),
			zap.Error(err),
		)
		emit(event.StreamErr{QueryID: queryID, Err: err})
		return err
	}

	diagAgent := diagnose.NewAgent(k8sClient, a.llmClient)
	_, _, err = diagAgent.Run(ctx, queryID, runtime.Target{
		Cluster:   params.Cluster,
		Namespace: params.Namespace,
		Pod:       params.Pod,
	}, diagnose.DashboardStrictProfile(), eventCh)
	if err != nil {
		log.Ctx(ctx).Error("pod diagnosis failed",
			zap.String("event", "agent.error"),
			zap.String("cluster", params.Cluster),
			zap.String("namespace", params.Namespace),
			zap.String("pod", params.Pod),
			zap.Error(err),
		)
		emit(event.StreamErr{QueryID: queryID, Err: err})
		return err
	}

	emit(event.StreamDone{QueryID: queryID})
	return nil
}

