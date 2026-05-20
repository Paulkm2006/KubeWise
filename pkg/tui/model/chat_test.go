package model_test

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/kubewise/kubewise/pkg/tui/events"
	"github.com/kubewise/kubewise/pkg/tui/model"
)

func TestPhaseEventUpdatesPhase(t *testing.T) {
	m := model.NewChatModel(80, 40)

	// Start an agent so a progress card exists
	m.Update(events.AgentStartEvent{QueryID: "q-1", AgentName: "Query Agent"})

	// Send a PhaseEvent
	updated, _ := m.Update(events.PhaseEvent{QueryID: "q-1", Phase: "thinking"})

	if updated.Phase() != "thinking" {
		t.Errorf("expected phase 'thinking', got %q", updated.Phase())
	}
}

func TestPhaseEventIgnoredForUnknownQuery(t *testing.T) {
	m := model.NewChatModel(80, 40)

	// No AgentStartEvent for q-1, so PhaseEvent should be ignored
	updated, _ := m.Update(events.PhaseEvent{QueryID: "q-1", Phase: "thinking"})

	if updated.Phase() != "" {
		t.Errorf("expected empty phase for unknown query, got %q", updated.Phase())
	}
}

func TestStreamDoneStopsSpinner(t *testing.T) {
	m := model.NewChatModel(80, 40)

	m.Update(events.AgentStartEvent{QueryID: "q-1", AgentName: "Query Agent"})

	updated, _ := m.Update(events.StreamDoneEvent{QueryID: "q-1", Result: "done"})

	if updated.IsSpinning() {
		t.Error("expected spinner to stop after StreamDoneEvent")
	}
}

func TestToolFailMarksToolDoneWithoutErrorText(t *testing.T) {
	m := model.NewChatModel(80, 40)
	m, _ = m.Update(events.AgentStartEvent{QueryID: "q-1", AgentName: "Deploy Agent"})
	m, _ = m.Update(events.ToolCallEvent{QueryID: "q-1", ToolName: "helm preflight", Step: 5})
	m, _ = m.Update(events.ToolFailEvent{
		QueryID: "q-1", ToolName: "helm preflight", Step: 5, Elapsed: time.Second,
	})

	view := m.View()
	if !strings.Contains(view, "✗") || !strings.Contains(view, "helm preflight") {
		t.Fatalf("expected failed tool in card, got:\n%s", view)
	}
	if strings.Contains(view, "Helm 预检") || strings.Contains(view, "⟳ helm preflight") {
		t.Fatalf("should not show error body or running state, got:\n%s", view)
	}
}

func TestStreamErrStopsSpinner(t *testing.T) {
	m := model.NewChatModel(80, 40)

	m.Update(events.AgentStartEvent{QueryID: "q-1", AgentName: "Query Agent"})

	updated, _ := m.Update(events.StreamErrEvent{QueryID: "q-1", Err: fmt.Errorf("boom")})

	if updated.IsSpinning() {
		t.Error("expected spinner to stop after StreamErrEvent")
	}
}
