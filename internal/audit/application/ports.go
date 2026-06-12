package application

import (
	"context"

	"github.com/kubewise/kubewise/internal/audit/domain"
)

type Repository interface {
	Create(ctx context.Context, id string, target domain.Target) error
	AppendEvent(ctx context.Context, auditID string, ev domain.EventAppend) error
	SetCompleted(ctx context.Context, auditID string, result *domain.Result) error
	SetFailed(ctx context.Context, auditID, errMsg string) error
	SetCancelled(ctx context.Context, auditID string) error
	GetByID(ctx context.Context, auditID string) (*domain.Audit, error)
	GetLatestByCluster(ctx context.Context, cluster string) (*domain.Audit, error)
	ListEventsSince(ctx context.Context, auditID string, sinceSeqNum int) ([]domain.EventRecord, error)
	List(ctx context.Context, limit, offset int) ([]domain.Audit, int, error)
}
