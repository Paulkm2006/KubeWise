package application

import (
	"testing"

	"github.com/kubewise/kubewise/internal/platform/agentruntime"
	"github.com/kubewise/kubewise/internal/platform/agentruntime/event"
)

func TestMapProgressEventOmitsMarkdownDetailForStructuredAgentDone(t *testing.T) {
	got := mapProgressEvent(agentruntime.ProgressEvent{
		Type:        "agent_done",
		Result:      "## Diagnosis\n\nLong markdown report",
		Summary:     "diagnosis pipeline finished",
		PayloadKind: event.PayloadKindDiagnosisReport,
		PayloadJSON: `{"verdict":"confirmed"}`,
	})
	if got.Detail != "" {
		t.Fatalf("expected empty detail for structured agent_done, got %q", got.Detail)
	}
}

func TestMapProgressEventTruncatesUnstructuredAgentDoneDetail(t *testing.T) {
	long := string(make([]byte, 400))
	got := mapProgressEvent(agentruntime.ProgressEvent{
		Type:    "agent_done",
		Result:  long,
		Summary: "done",
	})
	if len(got.Detail) > 243 {
		t.Fatalf("expected truncated detail, got len=%d", len(got.Detail))
	}
}
