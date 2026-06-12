package application

import (
	"context"

	corev1 "k8s.io/api/core/v1"

	"github.com/kubewise/kubewise/internal/observability/infrastructure/cluster"
	platformcluster "github.com/kubewise/kubewise/internal/platform/cluster"
)

type Service struct {
	reader cluster.Reader
}

func NewService(reader cluster.Reader) *Service {
	return &Service{reader: reader}
}

func (s *Service) ListClusters(ctx context.Context) []platformcluster.ClusterSummary {
	if s == nil || s.reader == nil {
		return []platformcluster.ClusterSummary{}
	}
	return s.reader.ListClusters(ctx)
}

func (s *Service) ListIssues(ctx context.Context, clusterName string) ([]platformcluster.Issue, error) {
	if s == nil || s.reader == nil {
		return nil, cluster.ErrUnavailable
	}
	return s.reader.ListIssues(ctx, clusterName)
}

func (s *Service) ListEvents(ctx context.Context, clusterName string) ([]corev1.Event, error) {
	if s == nil || s.reader == nil {
		return nil, cluster.ErrUnavailable
	}
	return s.reader.ListEvents(ctx, clusterName)
}
