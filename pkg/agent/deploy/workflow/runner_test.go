package workflow

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kubewise/kubewise/pkg/tui/events"
)

type recordingEmitter struct {
	events []events.TUIEvent
}

func (e *recordingEmitter) Emit(ev events.TUIEvent) {
	e.events = append(e.events, ev)
}

func TestRunner_Run_EmitsToolEvents(t *testing.T) {
	rec := &recordingEmitter{}
	r := &Runner{QueryID: "q1", Emit: rec}
	err := r.Run(context.Background(), Meta{Name: "helm repo add", Step: 1}, func(ctx context.Context) error {
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(rec.events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(rec.events))
	}
	if _, ok := rec.events[0].(events.ToolCallEvent); !ok {
		t.Fatalf("expected ToolCallEvent, got %T", rec.events[0])
	}
	if done, ok := rec.events[1].(events.ToolDoneEvent); !ok {
		t.Fatalf("expected ToolDoneEvent, got %T", rec.events[1])
	} else if done.ToolName != "helm repo add" {
		t.Fatalf("unexpected tool name %q", done.ToolName)
	}
}

func TestRunner_Run_PropagatesError(t *testing.T) {
	r := &Runner{QueryID: "q1"}
	err := r.Run(context.Background(), Meta{Name: "test", Step: 1}, func(ctx context.Context) error {
		return errors.New("boom")
	})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestRunWithResult_ReturnsValue(t *testing.T) {
	r := &Runner{QueryID: "q1"}
	out, err := RunWithResult(r, context.Background(), Meta{Name: "helm show values", Step: 2},
		func(ctx context.Context) (string, error) {
			time.Sleep(time.Millisecond)
			return "values", nil
		},
	)
	if err != nil || out != "values" {
		t.Fatalf("got %q, %v", out, err)
	}
}
