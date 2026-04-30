# Thinking Indicator Design

Add a context-aware animated spinner with phase labels and elapsed time to the TUI chat, so users see feedback when the model is processing their input.

## Problem

When the user submits a message, the LLM `ChatCompletion` call is synchronous and blocking — it can take many seconds with zero visual feedback. The existing progress cards only update when agent events arrive (tool calls, etc.), but during LLM thinking time the display is frozen.

## Design

### PhaseEvent

New event type in `pkg/tui/events/events.go`:

```go
type PhaseEvent struct {
    QueryID string
    Phase   string // e.g. "classifying intent", "thinking", "running tool: get_pods"
}
```

### Phase emission points

**Router agent** (`pkg/agent/router/agent.go`):
- Before `classifyIntent()`: emit `PhaseEvent{Phase: "classifying intent"}`
- After classification, before routing: emit `PhaseEvent{Phase: "routing to <agent_name>"}`

**Sub-agents** (query, troubleshooting, security):
- Before each `ChatCompletion()` call: emit `PhaseEvent{Phase: "thinking"}`
- Before tool execution: emit `PhaseEvent{Phase: "running tool: <name>"}`

### Spinner in ChatModel

Add `bubbles/spinner` to `ChatModel` (`pkg/tui/model/chat.go`):
- New fields: `spinner spinner.Model`, `phase string`, `phaseStart time.Time`
- On `AgentStartEvent`: start the spinner (emit `spinner.Tick`)
- On `PhaseEvent`: update `phase` label, reset `phaseStart`
- On `spinner.TickMsg`: advance spinner frame, return `spinner.Tick` cmd
- On `StreamDoneEvent`/`StreamErrEvent`: stop the spinner

### Rendering

Replace the static gear icon in active progress cards with the animated spinner + phase + elapsed time:

- Before: `⚙ Query Agent`
- After: `⠙ Thinking... 3s` / `⠙ Classifying intent... 1s` / `⠙ Running tool: get_pods... 0.5s`

Elapsed time counts from the last `PhaseEvent`.

### App.Update wiring

In `pkg/tui/app.go`:
- Route `PhaseEvent` alongside existing agent events to `chat.Update()`
- Route `spinner.TickMsg` to `chat.Update()` to drive animation

## Files changed

| File | Change |
|---|---|
| `pkg/tui/events/events.go` | Add `PhaseEvent` type |
| `pkg/tui/model/chat.go` | Add spinner, phase label, timer; handle `PhaseEvent` and `spinner.TickMsg`; update `renderCard` |
| `pkg/tui/app.go` | Route `PhaseEvent` and `spinner.TickMsg` to chat |
| `pkg/agent/router/agent.go` | Emit `PhaseEvent` before classification and routing |
| `pkg/agent/query/agent.go` | Emit `PhaseEvent` before `ChatCompletion` and tool execution |
| `pkg/agent/troubleshooting/agent.go` | Same as query |
| `pkg/agent/security/agent.go` | Same as query |
