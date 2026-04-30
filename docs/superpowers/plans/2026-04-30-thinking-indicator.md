# Thinking Indicator Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add an animated spinner with context-aware phase labels and elapsed time to the TUI chat, so users see feedback when the model is processing their input.

**Architecture:** Add a `PhaseEvent` to the existing event channel system. Agents emit phase events at key moments (before LLM calls, before tool execution, during classification). `ChatModel` integrates a `bubbles/spinner` that animates between events and updates its label on `PhaseEvent`. The active progress card renders the spinner + phase + elapsed time instead of a static gear icon.

**Tech Stack:** Go, bubbletea, bubbles/spinner, lipgloss

---

### Task 1: Add PhaseEvent to events package

**Files:**
- Modify: `pkg/tui/events/events.go`

- [ ] **Step 1: Add PhaseEvent type**

Add after the `StreamErrEvent` definition (after line 125):

```go
// PhaseEvent carries a human-readable phase label for the thinking indicator.
type PhaseEvent struct {
	QueryID string
	Phase   string // e.g. "classifying intent", "thinking", "running tool: get_pods"
}

func (PhaseEvent) isTUIEvent() {}
```

- [ ] **Step 2: Run tests to verify compilation**

Run: `cd d:/KubeWise && go build ./pkg/tui/events/`
Expected: no errors

- [ ] **Step 3: Commit**

```bash
git add pkg/tui/events/events.go
git commit -m "feat(events): add PhaseEvent for thinking indicator"
```

---

### Task 2: Integrate spinner into ChatModel

**Files:**
- Modify: `pkg/tui/model/chat.go`

- [ ] **Step 1: Add spinner imports and fields**

Add `"github.com/charmbracelet/bubbles/spinner"` to imports.

Add these fields to `ChatModel` struct (after line 62):

```go
	spinner    spinner.Model
	phase      string
	phaseStart time.Time
	spinning   bool
```

- [ ] **Step 2: Initialize spinner in NewChatModel**

In `NewChatModel`, after the return statement's struct literal, add spinner initialization. Replace the current `return` with:

```go
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = styles.CardRunning

	return ChatModel{
		messages:   make([]chatEntry, 0),
		pending:    make(map[string]*pendingMsg),
		cards:      make(map[string]*progressCard),
		renderer:   NewRenderer(width - 2),
		width:      width,
		height:     height,
		spinner:    sp,
	}
```

- [ ] **Step 3: Add PhaseEvent and spinner.TickMsg handling in Update**

Add these cases to the `switch ev := msg.(type)` in `ChatModel.Update`, before the closing `return m, nil`:

```go
	case events.PhaseEvent:
		if _, ok := m.cards[ev.QueryID]; ok {
			m.phase = ev.Phase
			m.phaseStart = time.Now()
		}

	case spinner.TickMsg:
		if m.spinning {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			return m, cmd
		}
```

Also, in the `events.AgentStartEvent` case, add spinner start after creating the card:

```go
	case events.AgentStartEvent:
		m.cards[ev.QueryID] = &progressCard{
			queryID:   ev.QueryID,
			agentName: ev.AgentName,
		}
		m.phase = ev.AgentName
		m.phaseStart = time.Now()
		m.spinning = true
		return m, m.spinner.Tick
```

And in the `events.StreamDoneEvent` case, add spinner stop before deleting the card:

```go
		m.spinning = false
		m.phase = ""
		delete(m.pending, ev.QueryID)
		delete(m.cards, ev.QueryID)
```

And in the `events.StreamErrEvent` case, add spinner stop:

```go
		m.spinning = false
		m.phase = ""
		delete(m.pending, ev.QueryID)
		delete(m.cards, ev.QueryID)
```

- [ ] **Step 4: Update renderCard to show spinner + phase + elapsed time**

Replace the `renderCard` method entirely:

```go
func (m ChatModel) renderCard(c *progressCard) string {
	if c.done {
		summary := fmt.Sprintf("✓ %s  %.1fs | ↑ %d ↓ %d tok",
			c.agentName, c.duration.Seconds(), c.inTokens, c.outTokens)
		return styles.CardDone.Render(summary)
	}
	if c.failed {
		summary := fmt.Sprintf("✗ %s: %s", c.agentName, c.errMsg)
		return styles.CardFailed.Render(summary)
	}

	// Phase line with spinner and elapsed time
	phaseLabel := m.phase
	if phaseLabel == "" {
		phaseLabel = c.agentName
	}
	elapsed := time.Since(m.phaseStart).Round(time.Second)
	header := fmt.Sprintf("%s %s... %s", m.spinner.View(), phaseLabel, elapsed)
	lines := []string{styles.CardRunning.Render(header)}

	for _, t := range c.tools {
		if t.done {
			line := fmt.Sprintf("  ✓ %-30s %s", t.name, t.elapsed.Round(time.Millisecond).String())
			lines = append(lines, styles.CardDone.Render(line))
		} else {
			line := fmt.Sprintf("  ⟳ %s", t.name)
			lines = append(lines, styles.CardRunning.Render(line))
		}
	}

	return styles.CardStyle.Width(m.width - 4).Render(strings.Join(lines, "\n"))
}
```

- [ ] **Step 5: Run build to verify compilation**

Run: `cd d:/KubeWise && go build ./pkg/tui/model/`
Expected: no errors

- [ ] **Step 6: Commit**

```bash
git add pkg/tui/model/chat.go
git commit -m "feat(tui): integrate spinner with phase labels into ChatModel"
```

---

### Task 3: Route PhaseEvent and spinner.TickMsg in App

**Files:**
- Modify: `pkg/tui/app.go`

- [ ] **Step 1: Add PhaseEvent to the event routing case**

In `App.Update`, add `events.PhaseEvent` to the existing event routing case (line 108-118). Change:

```go
		case events.AgentStartEvent,
			events.AgentDoneEvent,
			events.ToolCallEvent,
			events.ToolDoneEvent,
			events.RenderTextEvent,
			events.RenderTableEvent,
			events.RenderCodeEvent,
			events.RenderKVEvent,
			events.RenderListEvent:
			a.chat, _ = a.chat.Update(msg)
			return a, listenForEvents(a.eventCh)
```

to:

```go
		case events.AgentStartEvent,
			events.AgentDoneEvent,
			events.ToolCallEvent,
			events.ToolDoneEvent,
			events.RenderTextEvent,
			events.RenderTableEvent,
			events.RenderCodeEvent,
			events.RenderKVEvent,
			events.RenderListEvent,
			events.PhaseEvent:
			var chatCmd tea.Cmd
			a.chat, chatCmd = a.chat.Update(msg)
			return a, tea.Batch(listenForEvents(a.eventCh), chatCmd)
```

Note: `cmd` from `chat.Update` carries the spinner tick (if any). `tea.Batch` lets both the event listener and spinner tick run concurrently.

- [ ] **Step 2: Route spinner.TickMsg to ChatModel**

Add a new case in `App.Update` before the `model.SubmitMsg` case:

```go
		case spinner.TickMsg:
			var cmd tea.Cmd
			a.chat, cmd = a.chat.Update(msg)
			if cmd != nil {
				return a, cmd
			}
			return a, nil
```

Add `"github.com/charmbracelet/bubbles/spinner"` to the imports.

- [ ] **Step 3: Run build to verify compilation**

Run: `cd d:/KubeWise && go build ./pkg/tui/`
Expected: no errors

- [ ] **Step 4: Commit**

```bash
git add pkg/tui/app.go
git commit -m "feat(tui): route PhaseEvent and spinner.TickMsg in App"
```

---

### Task 4: Emit PhaseEvents from router agent

**Files:**
- Modify: `pkg/agent/router/agent.go`

- [ ] **Step 1: Add phase emissions in HandleQueryStream**

In `HandleQueryStream`, emit `PhaseEvent` before `classifyIntent` and before routing. Add these lines after the `emit` closure definition (after line 99):

Before the `classifyIntent` call (around line 102), add:

```go
		emit(events.PhaseEvent{QueryID: queryID, Phase: "classifying intent"})
```

After classification succeeds and before the `switch intent.TaskType` (around line 111), add:

```go
		phaseLabel := fmt.Sprintf("routing to %s agent", intent.TaskTypeDescription)
		emit(events.PhaseEvent{QueryID: queryID, Phase: phaseLabel})
```

- [ ] **Step 2: Run build to verify compilation**

Run: `cd d:/KubeWise && go build ./pkg/agent/router/`
Expected: no errors

- [ ] **Step 3: Commit**

```bash
git add pkg/agent/router/agent.go
git commit -m "feat(router): emit PhaseEvent before classification and routing"
```

---

### Task 5: Emit PhaseEvents from query agent

**Files:**
- Modify: `pkg/agent/query/agent.go`

- [ ] **Step 1: Add phase emissions in HandleQuery**

Before each `ChatCompletion` call (line 127), add:

```go
			a.emit(events.PhaseEvent{QueryID: a.queryID, Phase: "thinking"})
```

Before tool execution (before line 161), add:

```go
			a.emit(events.PhaseEvent{QueryID: a.queryID, Phase: fmt.Sprintf("running tool: %s", funcCall.Name)})
```

- [ ] **Step 2: Run build to verify compilation**

Run: `cd d:/KubeWise && go build ./pkg/agent/query/`
Expected: no errors

- [ ] **Step 3: Commit**

```bash
git add pkg/agent/query/agent.go
git commit -m "feat(query): emit PhaseEvent before LLM calls and tool execution"
```

---

### Task 6: Emit PhaseEvents from troubleshooting agent

**Files:**
- Modify: `pkg/agent/troubleshooting/agent.go`

- [ ] **Step 1: Add phase emissions in HandleQuery**

Before each `ChatCompletion` call (line 130), add:

```go
			a.emit(events.PhaseEvent{QueryID: a.queryID, Phase: "thinking"})
```

Before tool execution (before line 160), add:

```go
			a.emit(events.PhaseEvent{QueryID: a.queryID, Phase: fmt.Sprintf("running tool: %s", funcCall.Name)})
```

- [ ] **Step 2: Run build to verify compilation**

Run: `cd d:/KubeWise && go build ./pkg/agent/troubleshooting/`
Expected: no errors

- [ ] **Step 3: Commit**

```bash
git add pkg/agent/troubleshooting/agent.go
git commit -m "feat(troubleshooting): emit PhaseEvent before LLM calls and tool execution"
```

---

### Task 7: Emit PhaseEvents from security agent

**Files:**
- Modify: `pkg/agent/security/agent.go`

- [ ] **Step 1: Add phase emissions in HandleQuery**

Before each `ChatCompletion` call (line 121), add:

```go
			a.emit(events.PhaseEvent{QueryID: a.queryID, Phase: "thinking"})
```

Before tool execution (before line 150), add:

```go
			a.emit(events.PhaseEvent{QueryID: a.queryID, Phase: fmt.Sprintf("running tool: %s", funcCall.Name)})
```

- [ ] **Step 2: Run build to verify compilation**

Run: `cd d:/KubeWise && go build ./pkg/agent/security/`
Expected: no errors

- [ ] **Step 3: Commit**

```bash
git add pkg/agent/security/agent.go
git commit -m "feat(security): emit PhaseEvent before LLM calls and tool execution"
```

---

### Task 8: Write ChatModel spinner tests

**Files:**
- Create: `pkg/tui/model/chat_test.go`

- [ ] **Step 1: Write tests for PhaseEvent handling and spinner lifecycle**

```go
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
```

This requires exposing `Phase()` and `IsSpinning()` accessor methods on ChatModel, and adding `"fmt"` to imports.

- [ ] **Step 2: Add accessor methods to ChatModel**

Add to `pkg/tui/model/chat.go`:

```go
// Phase returns the current phase label for the thinking indicator.
func (m ChatModel) Phase() string {
	return m.phase
}

// IsSpinning reports whether the spinner is currently active.
func (m ChatModel) IsSpinning() bool {
	return m.spinning
}
```

- [ ] **Step 3: Run tests to verify they pass**

Run: `cd d:/KubeWise && go test ./pkg/tui/model/ -v`
Expected: all PASS

- [ ] **Step 4: Commit**

```bash
git add pkg/tui/model/chat_test.go pkg/tui/model/chat.go
git commit -m "test(tui): add ChatModel spinner lifecycle tests"
```

---

### Task 9: Full build and test

**Files:** None (verification only)

- [ ] **Step 1: Run full build**

Run: `cd d:/KubeWise && go build ./...`
Expected: no errors

- [ ] **Step 2: Run all tests**

Run: `cd d:/KubeWise && go test ./...`
Expected: all PASS

- [ ] **Step 3: Final commit (if any adjustments were needed)**

Only if fixes were needed during the build/test step.
