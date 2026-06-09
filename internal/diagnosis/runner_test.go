package diagnosis

import (
	"context"
	"testing"
)

func TestRunnerLifecycle(t *testing.T) {
	r := NewRunner()
	ctx := context.Background()
	r.Start(ctx, "diag-1")
	r.PushEvent(ctx, "diag-1", StreamEvent{Type: "phase", Message: "collecting"})
	r.PushEvent(ctx, "diag-1", StreamEvent{Type: "phase", Message: "analyzing"})

	events := r.Finish(ctx, "diag-1")
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}
	if _, ok := r.active["diag-1"]; ok {
		t.Fatal("runner should clean up after Finish")
	}
}
