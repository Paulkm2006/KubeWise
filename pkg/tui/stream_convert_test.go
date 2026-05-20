package tui

import (
	"testing"
	"time"

	"github.com/kubewise/kubewise/pkg/stream"
	"github.com/kubewise/kubewise/pkg/tui/events"
)

func TestToTUI_ToolFailOmitsErrorBody(t *testing.T) {
	te, ok := ToTUI(stream.ToolFail{
		QueryID: "q-1", ToolName: "helm preflight", Step: 5,
		Elapsed: time.Second, Err: "Helm 预检未通过: something long",
	})
	if !ok {
		t.Fatal("expected conversion")
	}
	fail, ok := te.(events.ToolFailEvent)
	if !ok {
		t.Fatalf("got %T", te)
	}
	if fail.ToolName != "helm preflight" || fail.Step != 5 {
		t.Fatalf("unexpected %+v", fail)
	}
}
