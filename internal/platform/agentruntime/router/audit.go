package router

import (
	"context"
	"encoding/json"
	"fmt"

	auditsubagent "github.com/kubewise/kubewise/internal/platform/agentruntime/subagent/audit"
	"github.com/kubewise/kubewise/internal/platform/agentruntime/event"
	"github.com/kubewise/kubewise/internal/platform/cluster"
	"github.com/kubewise/kubewise/internal/utils/log"
	"go.uber.org/zap"
)

func (a *Agent) AuditClusterStream(ctx context.Context, clusterName, queryID string, eventCh chan<- event.Event) error {
	a.streamMu.Lock()
	defer a.streamMu.Unlock()

	k8sClient, err := a.auditK8sClient(ctx, clusterName)
	if err != nil {
		log.Ctx(ctx).Error("cluster audit selection failed",
			zap.String("event", "agent.error"),
			zap.String("cluster", clusterName),
			zap.Error(err),
		)
		se := event.NewEmitter(eventCh, queryID)
		_ = se.Emit(ctx, event.StreamErr{QueryID: queryID, Err: err})
		return err
	}

	auditAgent := auditsubagent.NewAgent(k8sClient, a.llmClient, a.maxSteps, a.supervisorCfg)
	if err := auditAgent.RunClusterAudit(ctx, clusterName, queryID, eventCh); err != nil {
		log.Ctx(ctx).Error("cluster audit failed",
			zap.String("event", "agent.error"),
			zap.String("cluster", clusterName),
			zap.Error(err),
		)
		return err
	}

	se := event.NewEmitter(eventCh, queryID)
	_ = se.Emit(ctx, event.StreamDone{QueryID: queryID})
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

// decodePayload kept for tests or future adapters.
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
