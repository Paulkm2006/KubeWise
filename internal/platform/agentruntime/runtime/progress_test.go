package runtime

import (
	"encoding/json"
	"testing"

	"github.com/kubewise/kubewise/internal/platform/agentruntime/event"
)

func TestToProgressEventMapsLLMStepDegraded(t *testing.T) {
	pe, ok := toProgressEvent(event.LLMStepDegraded{
		Step: "llm_report_composition", Phase: "report",
		Err:       "chat completion failed: 503 service unavailable",
		Transient: true, Fallback: "deterministic report composition",
	})
	if !ok {
		t.Fatal("expected llm step degraded to map")
	}
	if pe.Type != "llm_step_degraded" {
		t.Fatalf("unexpected type %q", pe.Type)
	}
	if pe.Message != "llm_report_composition" {
		t.Fatalf("unexpected message %q", pe.Message)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(pe.PayloadJSON), &payload); err != nil {
		t.Fatalf("payload json: %v", err)
	}
	if payload["transient"] != true {
		t.Fatalf("expected transient payload, got %#v", payload)
	}
}
