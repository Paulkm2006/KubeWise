package application

import (
	"context"

	"github.com/kubewise/kubewise/internal/diagnosis/domain"
)

// Repository persists diagnosis aggregates and their event log.
type Repository interface {
	Create(ctx context.Context, id string, target domain.Target) error
	AppendEvent(ctx context.Context, diagID string, ev domain.EventAppend) error
	SetCompleted(ctx context.Context, diagID string, result *domain.Result) error
	SetFailed(ctx context.Context, diagID, errMsg string) error
	SetCancelled(ctx context.Context, diagID string) error
	GetByID(ctx context.Context, diagID string) (*domain.Diagnosis, error)
	GetLatestByTarget(ctx context.Context, target domain.Target) (*domain.Diagnosis, error)
	ListEventsSince(ctx context.Context, diagID string, sinceSeqNum int) ([]domain.EventRecord, error)
	List(ctx context.Context, limit, offset int) ([]domain.Diagnosis, int, error)
}
