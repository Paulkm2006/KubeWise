package model_test

import (
	"strings"
	"testing"

	"github.com/kubewise/kubewise/pkg/tui/events"
	"github.com/kubewise/kubewise/pkg/tui/model"
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
	pairs := []events.KVPair{{Key: "namespace", Value: "default"}, {Key: "pods", Value: "5"}}
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
	items := []events.ListItem{{Status: "ok", Text: "pod running"}, {Status: "error", Text: "pod crashed"}}
	out := r.RenderList(items)
	if !strings.Contains(out, "pod running") {
		t.Errorf("want 'pod running' in output: %q", out)
	}
}
