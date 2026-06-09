package diagnosis

import (
	"context"
	"sync"

	"github.com/kubewise/kubewise/internal/utils/log"
	"go.uber.org/zap"
)

type Runner struct {
	mu        sync.Mutex
	active    map[string]*RingBuffer
	diagnoses map[string]*Diagnosis
}

func NewRunner() *Runner {
	return &Runner{
		active:    make(map[string]*RingBuffer),
		diagnoses: make(map[string]*Diagnosis),
	}
}

func (r *Runner) Start(ctx context.Context, id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.active[id] = NewRingBuffer(100)
	log.Ctx(ctx).Info("diagnosis runner started",
		zap.String("event", "diagnosis.runner.started"),
		zap.String("diagnosis_id", id),
	)
}

func (r *Runner) PushEvent(ctx context.Context, id string, ev StreamEvent) {
	r.mu.Lock()
	defer r.mu.Unlock()
	buf, ok := r.active[id]
	if !ok {
		log.Ctx(ctx).Warn("diagnosis runner: push to unknown id",
			zap.String("diagnosis_id", id),
		)
		return
	}
	if ok := buf.Push(ev); !ok {
		log.Ctx(ctx).Warn("diagnosis runner: ring buffer overflow, dropping event",
			zap.String("diagnosis_id", id),
			zap.String("event_type", ev.Type),
		)
	} else {
		log.Ctx(ctx).Debug("diagnosis runner: event pushed",
			zap.String("diagnosis_id", id),
			zap.String("event_type", ev.Type),
		)
	}
}

func (r *Runner) GetBuffer(id string) *RingBuffer {
	return r.active[id]
}

func (r *Runner) Finish(ctx context.Context, id string) []StreamEvent {
	r.mu.Lock()
	defer r.mu.Unlock()
	buf, ok := r.active[id]
	if !ok {
		log.Ctx(ctx).Warn("diagnosis runner: finish called for unknown id",
			zap.String("diagnosis_id", id),
		)
		return nil
	}
	delete(r.active, id)
	events := buf.Drain()
	log.Ctx(ctx).Info("diagnosis runner finished",
		zap.String("event", "diagnosis.runner.finished"),
		zap.String("diagnosis_id", id),
		zap.Int("events", len(events)),
	)
	return events
}
