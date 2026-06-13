package application

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/kubewise/kubewise/internal/config"
	"github.com/kubewise/kubewise/internal/diagnosis/domain"
	"github.com/kubewise/kubewise/internal/platform/agentruntime"
	"go.uber.org/zap"
)

var (
	ErrUnavailable = errors.New("diagnosis unavailable")
	ErrNotFound    = errors.New("diagnosis not found")
)

type ActivityRecorder interface {
	RecordDiagnosisStart(ctx context.Context, clusterDisplay, diagnosisID, pod, namespace string) error
}

type Service struct {
	repo     Repository
	agent    agentruntime.DiagnosisRunner
	activity ActivityRecorder

	mu      sync.Mutex
	cancels map[string]context.CancelFunc
}

func NewService(repo Repository, agent agentruntime.DiagnosisRunner, activity ActivityRecorder) *Service {
	return &Service{
		repo:     repo,
		agent:    agent,
		activity: activity,
		cancels:  make(map[string]context.CancelFunc),
	}
}

func (s *Service) Start(ctx context.Context, target domain.Target) (string, error) {
	if s == nil || s.repo == nil {
		return "", ErrUnavailable
	}

	id := uuid.New().String()
	if err := s.repo.Create(ctx, id, target); err != nil {
		return "", err
	}

	if s.activity != nil {
		_ = s.activity.RecordDiagnosisStart(ctx, target.ClusterDisplay, id, target.Pod, target.Namespace)
	}

	go s.run(id, target)
	return id, nil
}

func (s *Service) run(id string, target domain.Target) {
	ctx, cancel := context.WithCancel(context.Background())
	s.setCancel(id, cancel)
	defer s.clearCancel(id)

	eventCh := make(chan agentruntime.ProgressEvent, 64)
	queryID := fmt.Sprintf("diag-%s", uuid.New().String()[:8])
	start := time.Now()

	var finalEvent agentruntime.ProgressEvent
	hasFinal := false
	done := make(chan struct{})
	go func() {
		defer close(done)
		for ev := range eventCh {
			appendEv := mapProgressEvent(ev)
			logDiagnosisProgressEvent(id, target, appendEv)
			_ = s.repo.AppendEvent(ctx, id, appendEv)
			if ev.Type == "agent_done" {
				finalEvent = ev
				hasFinal = true
			}
		}
	}()

	err := s.agent.DiagnosePod(ctx, agentruntime.DiagnoseParams{
		Cluster: target.Cluster, Namespace: target.Namespace, Pod: target.Pod,
	}, queryID, eventCh)
	close(eventCh)
	<-done

	durationMs := time.Since(start).Milliseconds()

	if err != nil {
		if errors.Is(err, context.Canceled) {
			if d, getErr := s.repo.GetByID(ctx, id); getErr == nil && d.Status == domain.StatusCancelled {
				return
			}
			_ = s.repo.SetCancelled(ctx, id)
			return
		}
		_ = s.repo.AppendEvent(ctx, id, domain.EventAppend{EventType: "stream_err", Detail: err.Error()})
		_ = s.repo.SetFailed(ctx, id, err.Error())
		return
	}

	if !hasFinal {
		_ = s.repo.SetFailed(ctx, id, "diagnosis finished without agent_done event")
		return
	}

	result := parseResultFromProgress(finalEvent, durationMs)
	if result == nil {
		_ = s.repo.SetFailed(ctx, id, "diagnosis finished without structured report")
		return
	}
	if result.Enrichment.Status == "degraded" {
		config.L().Warn("diagnosis completed with degraded enrichment",
			zap.String("diagnosis_id", id),
			zap.String("cluster", target.Cluster),
			zap.String("namespace", target.Namespace),
			zap.String("pod", target.Pod),
			zap.Strings("degraded_steps", result.Enrichment.DegradedSteps),
			zap.String("enrichment_message", result.Enrichment.Message),
		)
	}
	_ = s.repo.SetCompleted(ctx, id, result)
}

func (s *Service) Cancel(ctx context.Context, id string) error {
	if s == nil || s.repo == nil {
		return ErrUnavailable
	}

	if _, err := s.repo.GetByID(ctx, id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}

	s.mu.Lock()
	cancel := s.cancels[id]
	s.mu.Unlock()
	if cancel != nil {
		cancel()
	}

	if err := s.repo.SetCancelled(ctx, id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	return nil
}

func (s *Service) Latest(ctx context.Context, target domain.Target) (*domain.Diagnosis, []domain.EventRecord, error) {
	if s == nil || s.repo == nil {
		return nil, nil, ErrUnavailable
	}

	d, err := s.repo.GetLatestByTarget(ctx, target)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil, ErrNotFound
		}
		return nil, nil, err
	}
	events, err := s.repo.ListEventsSince(ctx, d.ID, 0)
	return d, events, err
}

func (s *Service) Get(ctx context.Context, id string) (*domain.Diagnosis, []domain.EventRecord, error) {
	if s == nil || s.repo == nil {
		return nil, nil, ErrUnavailable
	}
	d, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, nil, err
	}
	events, err := s.repo.ListEventsSince(ctx, id, 0)
	return d, events, err
}

func (s *Service) List(ctx context.Context, limit, offset int) ([]domain.Diagnosis, int, error) {
	if s == nil || s.repo == nil {
		return nil, 0, ErrUnavailable
	}
	return s.repo.List(ctx, limit, offset)
}

func (s *Service) EventsSince(ctx context.Context, id string, since int) ([]domain.EventRecord, error) {
	if s == nil || s.repo == nil {
		return nil, ErrUnavailable
	}
	return s.repo.ListEventsSince(ctx, id, since)
}

func (s *Service) Status(ctx context.Context, id string) (*domain.Diagnosis, error) {
	if s == nil || s.repo == nil {
		return nil, ErrUnavailable
	}
	return s.repo.GetByID(ctx, id)
}

func (s *Service) setCancel(id string, cancel context.CancelFunc) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cancels[id] = cancel
}

func (s *Service) clearCancel(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.cancels, id)
}
