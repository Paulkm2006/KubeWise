package model_test

import (
	"strings"
	"testing"

	"github.com/kubewise/kubewise/internal/conversation/domain"
	"github.com/kubewise/kubewise/internal/tui/model"
)

func TestRenderText(t *testing.T) {
	r := model.NewRenderer(80)
	out := r.RenderText("hello world")
	if !strings.Contains(out, "hello world") {
		t.Errorf("want 'hello world' in output, got: %q", out)
	}
}

func TestRenderKV(t *testing.T) {
	r := model.NewRenderer(80)
	pairs := []domain.KVPair{{Key: "namespace", Value: "default"}, {Key: "pods", Value: "5"}}
	out := r.RenderKV(pairs)
	if !strings.Contains(out, "namespace") || !strings.Contains(out, "default") {
		t.Errorf("unexpected KV output: %q", out)
	}
}

func TestRenderTable(t *testing.T) {
	r := model.NewRenderer(80)
	headers := []string{"Name", "Status"}
	rows := [][]string{{"pod-a", "Running"}, {"pod-b", "Pending"}}
	out := r.RenderTable(headers, rows)
	if !strings.Contains(out, "pod-a") || !strings.Contains(out, "Running") {
		t.Errorf("unexpected table output: %q", out)
	}
}

func TestRenderList(t *testing.T) {
	r := model.NewRenderer(80)
	items := []domain.ListItem{{Status: "ok", Text: "pod running"}, {Status: "error", Text: "pod crashed"}}
	out := r.RenderList(items)
	if !strings.Contains(out, "pod running") {
		t.Errorf("want 'pod running' in output: %q", out)
	}
}

func TestRenderDetail(t *testing.T) {
	r := model.NewRenderer(80)
	detail := domain.DetailPayload{
		Kind:      "pod",
		Name:      "my-pod",
		Namespace: "default",
		Status:    map[string]string{"phase": "Running", "ip": "10.0.0.1"},
		Labels:    map[string]string{"app": "web"},
		Containers: []domain.ContainerInfo{
			{Name: "app", Image: "nginx:latest", Ready: true, RestartCount: 0, State: "Running"},
			{Name: "sidecar", Image: "envoy:1.0", Ready: false, RestartCount: 3, State: "CrashLoopBackOff"},
		},
		Conditions: []domain.ConditionInfo{
			{Type: "Ready", Status: "True", Reason: "", Message: ""},
		},
		Events: []domain.EventInfo{
			{Type: "Normal", Reason: "Pulled", Message: "image pulled", Timestamp: "2024-01-01 00:00:00"},
			{Type: "Warning", Reason: "BackOff", Message: "back-off restarting", Timestamp: "2024-01-01 00:01:00"},
		},
	}
	out := r.RenderDetail(detail)
	if !strings.Contains(out, "pod/my-pod") {
		t.Errorf("expected header 'pod/my-pod', got: %q", out)
	}
	if !strings.Contains(out, "Running") {
		t.Errorf("expected 'Running' in output, got: %q", out)
	}
	if !strings.Contains(out, "app") {
		t.Errorf("expected container 'app' in output, got: %q", out)
	}
	if !strings.Contains(out, "sidecar") {
		t.Errorf("expected container 'sidecar' in output, got: %q", out)
	}
	if !strings.Contains(out, "CrashLoopBackOff") {
		t.Errorf("expected 'CrashLoopBackOff' in output, got: %q", out)
	}
	if !strings.Contains(out, "Ready") {
		t.Errorf("expected condition 'Ready' in output, got: %q", out)
	}
	if !strings.Contains(out, "BackOff") {
		t.Errorf("expected event 'BackOff' in output, got: %q", out)
	}
}
