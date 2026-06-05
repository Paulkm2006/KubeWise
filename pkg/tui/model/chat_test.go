package model_test

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/kubewise/kubewise/pkg/stream"
	"github.com/kubewise/kubewise/pkg/tui/model"
)

func TestPhaseEventUpdatesPhase(t *testing.T) {
	m := model.NewChatModel(80, 40)

	// Start an agent so a progress card exists
	m.Update(stream.AgentStart{QueryID: "q-1", AgentName: "Query Agent"})

	// Send a PhaseEvent
	updated, _ := m.Update(stream.Phase{QueryID: "q-1", Phase: "thinking"})

	if updated.Phase() != "thinking" {
		t.Errorf("expected phase 'thinking', got %q", updated.Phase())
	}
}

func TestPhaseEventIgnoredForUnknownQuery(t *testing.T) {
	m := model.NewChatModel(80, 40)

	// No AgentStartEvent for q-1, so PhaseEvent should be ignored
	updated, _ := m.Update(stream.Phase{QueryID: "q-1", Phase: "thinking"})

	if updated.Phase() != "" {
		t.Errorf("expected empty phase for unknown query, got %q", updated.Phase())
	}
}

func TestStreamDoneStopsSpinner(t *testing.T) {
	m := model.NewChatModel(80, 40)

	m.Update(stream.AgentStart{QueryID: "q-1", AgentName: "Query Agent"})

	updated, _ := m.Update(stream.StreamDone{QueryID: "q-1"})

	if updated.IsSpinning() {
		t.Error("expected spinner to stop after StreamDoneEvent")
	}
}

func TestToolFailMarksToolDoneWithoutErrorText(t *testing.T) {
	m := model.NewChatModel(80, 40)
	m, _ = m.Update(stream.AgentStart{QueryID: "q-1", AgentName: "Deploy Agent"})
	m, _ = m.Update(stream.ToolCall{QueryID: "q-1", ToolName: "helm preflight", Step: 5})
	m, _ = m.Update(stream.ToolFail{
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

	m.Update(stream.AgentStart{QueryID: "q-1", AgentName: "Query Agent"})

	updated, _ := m.Update(stream.StreamErr{QueryID: "q-1", Err: fmt.Errorf("boom")})

	if updated.IsSpinning() {
		t.Error("expected spinner to stop after StreamErrEvent")
	}
}

func TestAgentDoneSetsFinalReportStreamDoneCreatesEntry(t *testing.T) {
	m := model.NewChatModel(80, 40)

	m.Update(stream.AgentStart{QueryID: "q-1", AgentName: "Query Agent"})

	// AgentDone sets finalReport on the card
	m.Update(stream.AgentDone{
		QueryID: "q-1", Result: "found 2 pods", Duration: time.Second,
		InTokens: 100, OutTokens: 50,
	})

	// Card should still be in m.cards (not yet consumed)
	// StreamDone creates the chatEntry from the card's finalReport
	updated, _ := m.Update(stream.StreamDone{QueryID: "q-1"})

	view := updated.View()
	if !strings.Contains(view, "found 2 pods") {
		t.Errorf("expected final report in chat entry after StreamDone, got:\n%s", view)
	}
	if strings.Contains(view, "⟳") {
		t.Errorf("expected no spinner in view after StreamDone, got:\n%s", view)
	}
}

func TestLLMTextDeltaWritesToLatestPhase(t *testing.T) {
	m := model.NewChatModel(80, 40)
	m.Update(stream.AgentStart{QueryID: "q-1", AgentName: "Query Agent"})
	m.Update(stream.Phase{QueryID: "q-1", Phase: "thinking"})

	// Phase emits LLMTextDelta
	m.Update(stream.LLMTextDelta{QueryID: "q-1", Delta: "checking pod status..."})
	m.Update(stream.LLMTextDelta{QueryID: "q-1", Delta: " found 3 pods"})

	view := m.View()
	if !strings.Contains(view, "checking pod status...") || !strings.Contains(view, "found 3 pods") {
		t.Errorf("expected reasoning text in card view, got:\n%s", view)
	}
}

func TestToolCallNestedUnderPhase(t *testing.T) {
	m := model.NewChatModel(80, 40)
	m.Update(stream.AgentStart{QueryID: "q-1", AgentName: "Query Agent"})

	// Phase 1: thinking, with tool call
	m.Update(stream.Phase{QueryID: "q-1", Phase: "thinking"})
	m.Update(stream.ToolCall{QueryID: "q-1", ToolName: "list_resources", Step: 1})

	// Phase 2: analyzing, with different tool call
	m.Update(stream.Phase{QueryID: "q-1", Phase: "analyzing"})
	m.Update(stream.ToolCall{QueryID: "q-1", ToolName: "get_resource", Step: 1})

	// Mark analyzing phase's tool done
	m.Update(stream.ToolDone{QueryID: "q-1", ToolName: "get_resource", Step: 1, Elapsed: time.Second})

	view := m.View()
	if !strings.Contains(view, "list_resources") {
		t.Errorf("expected list_resources tool under thinking phase, got:\n%s", view)
	}
	if !strings.Contains(view, "get_resource") {
		t.Errorf("expected get_resource tool under analyzing phase, got:\n%s", view)
	}
}

func TestTogglePhaseReasoning(t *testing.T) {
	m := model.NewChatModel(80, 40)
	m.Update(stream.AgentStart{QueryID: "q-1", AgentName: "Query Agent"})
	m.Update(stream.Phase{QueryID: "q-1", Phase: "thinking"})
	m.Update(stream.LLMTextDelta{QueryID: "q-1", Delta: "some reasoning text"})

	// Default: reasoning is collapsed — check that expanded marker is absent
	view := m.View()
	if strings.Contains(view, "▼") {
		t.Errorf("expected reasoning collapsed by default, got expanded:\n%s", view)
	}

	// After toggle: reasoning should be expanded
	m.TogglePhaseReasoning()
	view = m.View()
	if !strings.Contains(view, "▼") || !strings.Contains(view, "some reasoning text") {
		t.Errorf("expected expanded reasoning after toggle, got:\n%s", view)
	}
}
