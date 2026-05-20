package stream

import (
	"context"

	"github.com/kubewise/kubewise/pkg/tui/events"
)

// Emitter sends stream events without dropping interactions, stream terminals, or render blocks.
type Emitter struct {
	ch     chan<- Event
	queryID string
}

// NewEmitter returns an emitter targeting ch. QueryID is used where events lack it (optional).
func NewEmitter(ch chan<- Event, queryID string) Emitter {
	return Emitter{ch: ch, queryID: queryID}
}

func (e Emitter) notify(ev Event) bool {
	select {
	case e.ch <- ev:
		return true
	default:
		return false
	}
}

func (e Emitter) notifyBlocking(ctx context.Context, ev Event) error {
	select {
	case e.ch <- ev:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Emit sends ev. InteractionRequest, StreamDone, StreamErr, Render*, WorkflowStep never drop.
func (e Emitter) Emit(ctx context.Context, ev Event) error {
	if e.ch == nil {
		return nil
	}
	switch ev.(type) {
	case InteractionRequest, StreamDone, StreamErr, WorkflowStep:
		return e.notifyBlocking(ctx, ev)
	case RenderText, RenderTable, RenderCode, RenderKV, RenderList, RenderDetail:
		return e.notifyBlocking(ctx, ev)
	default:
		_ = e.notify(ev)
		return nil
	}
}

// EmitLegacy wraps a TUI event.
func (e Emitter) EmitLegacy(ctx context.Context, te events.TUIEvent) error {
	return e.Emit(ctx, Legacy{TUI: te})
}
