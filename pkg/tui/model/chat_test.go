package model_test

import (
	"fmt"
	"testing"

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

func TestStreamErrStopsSpinner(t *testing.T) {
	m := model.NewChatModel(80, 40)

	m.Update(events.AgentStartEvent{QueryID: "q-1", AgentName: "Query Agent"})

	updated, _ := m.Update(events.StreamErrEvent{QueryID: "q-1", Err: fmt.Errorf("boom")})

	if updated.IsSpinning() {
		t.Error("expected spinner to stop after StreamErrEvent")
	}
}
