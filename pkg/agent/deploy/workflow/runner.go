package workflow

import (
	"context"
	"time"

	"github.com/kubewise/kubewise/pkg/tui/events"
)

// Emitter sends TUI events for workflow tool progress.
type Emitter interface {
	Emit(e events.TUIEvent)
}

// Runner executes deploy-internal workflow tools and emits tool call/done events.
type Runner struct {
	QueryID string
	Emit    Emitter
}

// Meta describes a workflow tool for progress display.
type Meta struct {
	Name string
	Step int
}

// Run executes fn as a workflow tool and emits ToolCall/ToolDone events.
func (r *Runner) Run(ctx context.Context, meta Meta, fn func(context.Context) error) error {
	if r.Emit != nil {
		r.Emit.Emit(events.ToolCallEvent{QueryID: r.QueryID, ToolName: meta.Name, Step: meta.Step})
	}
	start := time.Now()
	err := fn(ctx)
	if err == nil && r.Emit != nil {
		r.Emit.Emit(events.ToolDoneEvent{
			QueryID:  r.QueryID,
			ToolName: meta.Name,
			Step:     meta.Step,
			Elapsed:  time.Since(start),
		})
	}
	return err
}

// RunWithResult executes fn and returns its result with progress events.
func RunWithResult[T any](r *Runner, ctx context.Context, meta Meta, fn func(context.Context) (T, error)) (T, error) {
	var zero T
	if r.Emit != nil {
		r.Emit.Emit(events.ToolCallEvent{QueryID: r.QueryID, ToolName: meta.Name, Step: meta.Step})
	}
	start := time.Now()
	out, err := fn(ctx)
	if err != nil {
		return zero, err
	}
	if r.Emit != nil {
		r.Emit.Emit(events.ToolDoneEvent{
			QueryID:  r.QueryID,
			ToolName: meta.Name,
			Step:     meta.Step,
			Elapsed:  time.Since(start),
		})
	}
	return out, nil
}
