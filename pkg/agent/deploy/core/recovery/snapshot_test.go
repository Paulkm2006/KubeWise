package recovery

import (
	"context"
	"fmt"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/kubewise/kubewise/pkg/catalog"
	"github.com/kubewise/kubewise/pkg/helm"
)

type fakeK8s struct {
	pods   []corev1.Pod
	events []corev1.Event
}

func (f *fakeK8s) ListPods(ctx context.Context, namespace string) ([]corev1.Pod, error) {
	return f.pods, nil
}

func (f *fakeK8s) GetEvents(ctx context.Context, namespace, name string) ([]corev1.Event, error) {
	var out []corev1.Event
	for _, e := range f.events {
		if e.InvolvedObject.Name == name {
			out = append(out, e)
		}
	}
	return out, nil
}

type mockStatusClient struct {
	statusFunc func(ctx context.Context, releaseName, namespace string) (*helm.Release, error)
}

func (m *mockStatusClient) Status(ctx context.Context, releaseName, namespace string) (*helm.Release, error) {
	return m.statusFunc(ctx, releaseName, namespace)
}

func TestSummarizePods(t *testing.T) {
	fk := &fakeK8s{
		pods: []corev1.Pod{{
			ObjectMeta: metav1.ObjectMeta{Name: "app-1"},
			Status:     corev1.PodStatus{Phase: corev1.PodPending},
		}},
		events: []corev1.Event{{
			ObjectMeta:     metav1.ObjectMeta{Name: "ev1"},
			InvolvedObject: corev1.ObjectReference{Name: "app-1"},
			Reason:         "FailedScheduling",
			Message:        "0/1 nodes available",
			Type:           "Warning",
			LastTimestamp:  metav1.Now(),
		}},
	}

	snap := summarizePods(fk.pods, 10)
	if len(snap) != 1 || snap[0]["name"] != "app-1" {
		t.Fatalf("unexpected pod summary: %+v", snap)
	}
	evs := collectUnhealthyPodEvents(context.Background(), "default", fk.pods, fk)
	if len(evs) == 0 {
		t.Fatal("expected events for unhealthy pod")
	}
}

func TestBuildDiagnosticsSnapshot_JSON(t *testing.T) {
	hc := &mockStatusClient{
		statusFunc: func(ctx context.Context, releaseName, namespace string) (*helm.Release, error) {
			return &helm.Release{Name: releaseName, Namespace: namespace, Status: "failed"}, nil
		},
	}
	out := BuildDiagnosticsSnapshot(context.Background(),
		fmt.Errorf("timeout"), "nginx", "default",
		&catalog.ChartInfo{ChartName: "nginx"},
		hc, nil,
	)
	if !containsStr(out, "helmRelease") || !containsStr(out, "timeout") {
		t.Fatalf("snapshot missing expected fields: %s", out)
	}
}

func containsStr(s, sub string) bool {
	return len(sub) == 0 || (len(s) >= len(sub) && indexStr(s, sub) >= 0)
}

func indexStr(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
