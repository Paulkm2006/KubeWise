package cluster

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestIssueSeverityCrashLoop(t *testing.T) {
	pod := corev1.Pod{
		Status: corev1.PodStatus{
			Phase: corev1.PodPending,
			ContainerStatuses: []corev1.ContainerStatus{{
				State: corev1.ContainerState{
					Waiting: &corev1.ContainerStateWaiting{Reason: "CrashLoopBackOff"},
				},
			}},
		},
	}
	if issueSeverity(pod) != "high" {
		t.Fatalf("expected high severity")
	}
}

func issueSeverity(p corev1.Pod) string {
	severity := "low"
	if p.Status.Phase == corev1.PodPending {
		severity = "medium"
	}
	for _, cs := range p.Status.ContainerStatuses {
		if cs.State.Waiting != nil && cs.State.Waiting.Reason == "CrashLoopBackOff" {
			severity = "high"
			break
		}
	}
	return severity
}

func TestIssueSeverityPending(t *testing.T) {
	pod := corev1.Pod{ObjectMeta: metav1.ObjectMeta{}, Status: corev1.PodStatus{Phase: corev1.PodPending}}
	if issueSeverity(pod) != "medium" {
		t.Fatalf("expected medium severity")
	}
}
