package cluster

import (
	"context"

	corev1 "k8s.io/api/core/v1"

	"github.com/kubewise/kubewise/internal/platform/cluster"
)

// Reader is the infrastructure port to Kubernetes cluster data.
type Reader interface {
	ListClusters(ctx context.Context) []cluster.ClusterSummary
	ListIssues(ctx context.Context, clusterName string) ([]cluster.Issue, error)
	ListEvents(ctx context.Context, clusterName string) ([]corev1.Event, error)
}

type ManagerReader struct {
	manager *cluster.ClusterClientManager
}

func NewManagerReader(manager *cluster.ClusterClientManager) *ManagerReader {
	return &ManagerReader{manager: manager}
}

func (g *ManagerReader) ListClusters(ctx context.Context) []cluster.ClusterSummary {
	if g.manager == nil {
		return []cluster.ClusterSummary{}
	}
	return g.manager.ListClusters(ctx)
}

func (g *ManagerReader) ListIssues(ctx context.Context, clusterName string) ([]cluster.Issue, error) {
	if g.manager == nil {
		return nil, ErrUnavailable
	}
	return listIssues(ctx, g.manager, clusterName)
}

func (g *ManagerReader) ListEvents(ctx context.Context, clusterName string) ([]corev1.Event, error) {
	if g.manager == nil {
		return nil, ErrUnavailable
	}
	return listClusterEvents(ctx, g.manager, clusterName)
}
