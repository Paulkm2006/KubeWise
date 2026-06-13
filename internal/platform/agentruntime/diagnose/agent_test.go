package diagnose

import (
	"testing"

	corev1 "k8s.io/api/core/v1"

	"github.com/kubewise/kubewise/internal/platform/agentruntime/diagnose/casefile"
	"github.com/kubewise/kubewise/internal/platform/agentruntime/diagnose/evidence"
	"github.com/kubewise/kubewise/internal/platform/agentruntime/diagnose/report"
)

func TestDeterministicEvidenceBuildsKeySignals(t *testing.T) {
	pod := &corev1.Pod{
		Status: corev1.PodStatus{
			ContainerStatuses: []corev1.ContainerStatus{
				{
					Name: "app",
					LastTerminationState: corev1.ContainerState{
						Terminated: &corev1.ContainerStateTerminated{
							Reason:   "OOMKilled",
							ExitCode: 137,
						},
					},
				},
			},
		},
	}
	obs := casefile.Observation{
		Pod: pod,
		Events: []corev1.Event{
			{Reason: "FailedScheduling", Message: "0/3 nodes available"},
		},
	}
	got := evidence.DeterministicForTest(obs)
	if len(got) < 2 {
		t.Fatalf("expected at least 2 evidence entries, got %d", len(got))
	}
}

func TestValidateReportReferences(t *testing.T) {
	rep := report.DiagnosisReport{
		Verdict: report.VerdictConfirmed,
		Evidence: []report.Evidence{
			{ID: "e1", Source: "container_status", Strength: "strong", Summary: "oom"},
		},
		RootCause: report.RootCause{
			Category: "oom_killed", Title: "root", Summary: "desc",
			ConfidenceScore: 0.91, ConfidenceLabel: "high",
			EvidenceIDs: []string{"e1"},
		},
		Hypotheses: []report.Hypothesis{
			{ID: "h1", Title: "h", Rationale: "r", SupportingEvidence: []string{"e1"}, Status: "supported"},
		},
	}
	if err := report.Validate(rep); err != nil {
		t.Fatalf("expected valid report, got err: %v", err)
	}
	rep.RootCause.EvidenceIDs = []string{"missing"}
	if err := report.Validate(rep); err == nil {
		t.Fatal("expected error when root cause references unknown evidence")
	}
}
