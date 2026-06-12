package application

import (
	"context"
	"errors"

	"github.com/kubewise/kubewise/internal/activityfeed/domain"
	"github.com/kubewise/kubewise/internal/activityfeed/infrastructure/sqlite"
)

var ErrUnavailable = errors.New("activity feed unavailable")

type Service struct {
	repo *sqlite.Repository
}

func NewService(repo *sqlite.Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) List(ctx context.Context, limit, offset int) ([]domain.Activity, error) {
	if s == nil || s.repo == nil {
		return nil, ErrUnavailable
	}
	return s.repo.List(ctx, limit, offset)
}

func (s *Service) RecordDiagnosisStart(ctx context.Context, clusterDisplay, diagnosisID, pod, namespace string) error {
	if s == nil || s.repo == nil {
		return ErrUnavailable
	}
	text := "Started diagnosis for pod " + pod + " in namespace " + namespace
	_, err := s.repo.Insert(ctx, domain.TypeDiagnosis, text, clusterDisplay, diagnosisID)
	return err
}
