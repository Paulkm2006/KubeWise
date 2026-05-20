package router

import (
	"testing"

	"github.com/kubewise/kubewise/pkg/stream"
)

func TestExtractDetailMarker_PodDetail(t *testing.T) {
	input := `__KUBEWISE_DETAIL:pod__
{"kind":"pod","name":"my-pod","namespace":"default","status":{"phase":"Running"},"containers":[{"name":"app","image":"nginx:latest","ready":true,"restart_count":0,"state":"Running"}],"conditions":[{"type":"Ready","status":"True","reason":"","message":""}],"events":[{"type":"Normal","reason":"Pulled","message":"image pulled","timestamp":"2024-01-01 00:00:00"}],"labels":{"app":"myapp"}}
__END__`

	detail, stripped, ok := extractDetailMarker(input)
	if !ok {
		t.Fatal("expected ok=true")
	}
	if detail.Kind != "pod" {
		t.Errorf("expected kind=pod, got %s", detail.Kind)
	}
	if detail.Name != "my-pod" {
		t.Errorf("expected name=my-pod, got %s", detail.Name)
	}
	if detail.Namespace != "default" {
		t.Errorf("expected namespace=default, got %s", detail.Namespace)
	}
	if detail.Status["phase"] != "Running" {
		t.Errorf("expected phase=Running, got %s", detail.Status["phase"])
	}
	if len(detail.Containers) != 1 {
		t.Fatalf("expected 1 container, got %d", len(detail.Containers))
	}
	if detail.Containers[0].Name != "app" {
		t.Errorf("expected container name=app, got %s", detail.Containers[0].Name)
	}
	if len(detail.Events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(detail.Events))
	}
	if detail.Labels["app"] != "myapp" {
		t.Errorf("expected label app=myapp, got %s", detail.Labels["app"])
	}
	if stripped != "" {
		t.Errorf("expected empty stripped, got %q", stripped)
	}
}

func TestExtractDetailMarker_ResourceDetail(t *testing.T) {
	input := `some prefix text
__KUBEWISE_DETAIL:resource__
{"kind":"deployment","name":"my-deploy","namespace":"prod","status":{"replicas":"3","ready_replicas":"3"}}
__END__
some suffix text`

	detail, stripped, ok := extractDetailMarker(input)
	if !ok {
		t.Fatal("expected ok=true")
	}
	if detail.Kind != "deployment" {
		t.Errorf("expected kind=deployment, got %s", detail.Kind)
	}
	if detail.Status["replicas"] != "3" {
		t.Errorf("expected replicas=3, got %s", detail.Status["replicas"])
	}
	if stripped != "some prefix text\n\nsome suffix text" {
		t.Errorf("unexpected stripped: %q", stripped)
	}
}

func TestExtractDetailMarker_NoMarker(t *testing.T) {
	input := "this is plain text with no marker"
	_, _, ok := extractDetailMarker(input)
	if ok {
		t.Fatal("expected ok=false")
	}
}

func TestExtractDetailMarker_InvalidJSON(t *testing.T) {
	input := `__KUBEWISE_DETAIL:pod__
{not valid json}
__END__`
	_, _, ok := extractDetailMarker(input)
	if ok {
		t.Fatal("expected ok=false for invalid JSON")
	}
}

func TestExtractDetailMarker_NoEndMarker(t *testing.T) {
	input := `__KUBEWISE_DETAIL:pod__
{"kind":"pod","name":"x"}`
	_, _, ok := extractDetailMarker(input)
	if ok {
		t.Fatal("expected ok=false when __END__ is missing")
	}
}

func TestExtractDetailMarker_EmptyStatus(t *testing.T) {
	input := `__KUBEWISE_DETAIL:resource__
{"kind":"configmap","name":"cfg","namespace":"default","status":{}}
__END__`
	detail, _, ok := extractDetailMarker(input)
	if !ok {
		t.Fatal("expected ok=true")
	}
	if len(detail.Status) != 0 {
		t.Errorf("expected empty status, got %v", detail.Status)
	}
}

func TestEmitRenderEvent_DetailMarkerEmitsDetailEvent(t *testing.T) {
	var emitted []stream.Event
	emit := func(e stream.Event) {
		emitted = append(emitted, e)
	}

	input := `__KUBEWISE_DETAIL:pod__
{"kind":"pod","name":"test","namespace":"default","status":{"phase":"Running"}}
__END__`

	emitRenderEvent(emit, "q-test", input)

	if len(emitted) < 1 {
		t.Fatal("expected at least 1 event")
	}
	detailEv, ok := emitted[0].(stream.RenderDetail)
	if !ok {
		t.Fatalf("expected RenderDetail, got %T", emitted[0])
	}
	if detailEv.Detail.Kind != "pod" {
		t.Errorf("expected kind=pod, got %s", detailEv.Detail.Kind)
	}
}

func TestEmitRenderEvent_DetailMarkerWithTrailingText(t *testing.T) {
	var emitted []stream.Event
	emit := func(e stream.Event) {
		emitted = append(emitted, e)
	}

	input := `__KUBEWISE_DETAIL:resource__
{"kind":"deployment","name":"d","namespace":"ns","status":{}}
__END__
The deployment is running normally.`

	emitRenderEvent(emit, "q-test", input)

	if len(emitted) < 2 {
		t.Fatalf("expected 2 events (detail + text), got %d", len(emitted))
	}
	if _, ok := emitted[0].(stream.RenderDetail); !ok {
		t.Errorf("expected first event to be RenderDetail, got %T", emitted[0])
	}
	if _, ok := emitted[1].(stream.RenderText); !ok {
		t.Errorf("expected second event to be RenderText, got %T", emitted[1])
	}
}

func TestSkipLeadingNewline(t *testing.T) {
	tests := []struct {
		input string
		pos   int
		want  int
	}{
		{"abc\nxyz", 3, 4},
		{"abc\r\nxyz", 3, 5},
		{"abcxyz", 3, 3},
		{"abc", 0, 0},
	}
	for _, tt := range tests {
		got := skipLeadingNewline(tt.input, tt.pos)
		if got != tt.want {
			t.Errorf("skipLeadingNewline(%q, %d) = %d, want %d", tt.input, tt.pos, got, tt.want)
		}
	}
}
