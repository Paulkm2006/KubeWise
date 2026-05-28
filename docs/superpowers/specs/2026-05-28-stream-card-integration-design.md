# Stream-Card Integration Design

**Date:** 2026-05-28
**Status:** Draft
**Product:** KubeWise TUI

## Problem

LLM streaming text (`LLMTextDelta` events) and progress cards are rendered as separate visual elements, causing two problems:

1. **Layout disruption** — Streaming text appears *above* the progress card, pushing it down as text accumulates. The card position is unstable.
2. **Visual separation** — The user sees two unrelated UI elements (text + card) instead of one coherent execution unit.

## Design

### Overview

Each agent execution produces one **progress card** that serves as the complete unit of display. The card contains:

- **Header** — Agent name, duration, token counts
- **Phase list** — Each phase is a group containing its tools and reasoning text
- **Final report** — The agent's concluding output, always visible

No separate free-standing `chatEntry` is created from `StreamDone.Result` — the card *is* the display unit. For session persistence, the completed card is encoded into a `chatEntry` (see Persistence section).

### Data Model

#### progressCard (modified)

```go
type phaseGroup struct {
    label         string
    start         time.Time
    done          bool
    elapsed       time.Duration
    tools         []toolLine
    reasoningText strings.Builder
}

type progressCard struct {
    queryID       string
    agentName     string
    phases        []phaseGroup  // replaces flat phases + tools lists
    done          bool
    failed        bool
    errMsg        string
    duration      time.Duration
    inTokens      int
    outTokens     int
    finalReport   string         // from AgentDone.Result
}
```

#### ChatModel (modifications)

**Remove:**
- `streamingText strings.Builder` — moved into `phaseGroup.reasoningText`
- `streamingID string` — no longer needed
- `pending map[string]*pendingMsg` — dead code, removed

**Add:**
- `func (m *ChatModel) TogglePhaseReasoning() bool` — toggle latest phase's reasoning expansion

### Rendering

```
┌───────────────────────────────────────────────┐
│ ✓ Query Agent  4.2s | ↑120 ↓350 tok          │ ← Header
├───────────────────────────────────────────────┤
│ ✓ classify intent       0.5s                 │ ← Phase (done)
│ ✓ thinking              3.2s                 │ ← Phase (done)
│   ▶ 推理过程                                   │ ← collapsed reasoning preview
│   ✓ list_resources      1.2s                 │ ← Tool (nested in phase)
│   ✓ get_resource        0.8s                 │
│ ⟳ generating answer     1.5s                 │ ← Phase (active)
│   ▶ 推理过程                                   │ ← 3 line tail preview, live scroll
│   ⟳ final_response      0.3s                 │
├───────────────────────────────────────────────┤
│ 在 default 命名空间发现 2 个异常 Pod:             │ ← Final report (always visible)
│ - nginx (CrashLoopBackOff)                    │
│ - redis (ImagePullBackOff)                    │
└───────────────────────────────────────────────┘
```

#### Three display concerns (not full states — they compose):

1. **Phase reasoning** — Default collapsed. Shows 3-line tail preview. Non-intrusive "still working" signal. Ctrl+O toggles expand/collapse for the latest active phase that has reasoning text.
2. **Phase without reasoning** — No `▶` indicator shown. Just phase label + tools.
3. **Completed card** — Stays in chat history with all phases visible. Reasoning sections auto-collapsed on completion. Final report always visible.

### Event Data Changes

Add `Result string` field to `stream.AgentDone`:

```go
type AgentDone struct {
    QueryID   string
    Result    string        // final report text (was previously carried by StreamDone)
    Duration  time.Duration
    InTokens  int
    OutTokens int
}
```

Remove `Result string` field from `stream.StreamDone` — it no longer carries payload:

```go
type StreamDone struct {
    QueryID string
}
```

### Event Handling

| Event | Behavior |
|-------|----------|
| `AgentStart` | Create `progressCard`, append initial `phaseGroup` from `ev.AgentName` |
| `Phase` | Mark current `phaseGroup.done = true`, append new `phaseGroup` |
| `LLMTextDelta` | Write to `currentPhaseGroup.reasoningText` |
| `ToolCall` | Append `toolLine` to `currentPhaseGroup.tools` |
| `ToolDone` | Mark tool as done in `currentPhaseGroup.tools` |
| `ToolFail` | Mark tool as failed in `currentPhaseGroup.tools` |
| `AgentDone` | Mark current phase done. Set `card.finalReport = ev.Result`, `card.done = true`, collapse all phase reasoning sections; also create a `chatEntry{role:"assistant", content: ev.Result}` with the card's phases/tools encoded as session Blocks for persistence |
| `StreamDone` | No card-level effect. Only app-level cleanup (`running=false`, `input.SetEnabled(true)`). |
| `StreamErr` | Mark card as failed, remove from `cards` map |

### Persistence

Completed cards are stored as standard `chatEntry` in `m.messages` so session persistence (`CompletedMessages()`/`AllMessages()`) works unchanged. The card-structured view is the *rendering* of that entry — when loading a session via `SetMessages()`, completed cards render in the same compact card format rather than plain text. The card metadata (phases, tools, timings) is encoded into `session.Block` payloads and decoded on restore.

For the initial implementation, completed cards render as:

```
✓ Query Agent  4.2s | ↑120 ↓350 tok
在 default 命名空间发现 2 个异常 Pod:
- nginx (CrashLoopBackOff)
- redis (ImagePullBackOff)
```

Future enhancement: fully expandable historical cards (with phases/tools visible on Ctrl+O).

### Keybindings

| Key | Behavior |
|-----|----------|
| `Ctrl+O` | Toggle reasoning expansion for the *latest phase with reasoning text* across all running cards |
| `↑↓` (input focused) | Navigate input history (shell-style) |
| `PgUp/PgDown` | Scroll chat view (unchanged) |
| Mouse wheel | Scroll chat view (unchanged) |

### Input History (new InputModel feature)

```go
type InputModel struct {
    // existing fields
    history    []string
    historyIdx int  // -1 = fresh input, 0..len-1 = browsing history
}
```

- On submit: append current value to `history`, reset `historyIdx = -1`
- `↑`: if at fresh input and non-empty, save current buffer to provisional slot; cycle back through `history`
- `↓`: cycle forward through `history`; when past end, restore fresh buffer (or empty)

### File Changes

| File | Change Summary |
|------|---------------|
| `pkg/stream/event.go` | Add `Result string` field to `AgentDone`, remove `Result string` from `StreamDone` |
| `pkg/api/types.go` | Update `AgentDoneData` (add Result), `StreamDoneData` (remove Result) |
| `pkg/api/handler_stream.go` | Update `bridgeStreamEvent` to pass `Result` to agent_done SSE event instead of stream_done |
| `pkg/agent/query/agent.go` | Capture result via named return in `HandleQuery`, pass to `AgentDone.Result` |
| `pkg/agent/operation/agent.go` | Same |
| `pkg/agent/troubleshooting/agent.go` | Same |
| `pkg/agent/security/agent.go` | Same |
| `pkg/agent/deploy/agent.go` | Same |
| `pkg/tui/model/chat.go` | Core: replace flat phases+tools with `[]phaseGroup`, remove `streamingText`/`streamingID`/`pending`, modify `Update()` event handling, modify `View()` rendering, modify `renderCard()` for three-zone layout |
| `pkg/tui/model/input.go` | Add `history []string` + `historyIdx int`, intercept `KeyUp`/`KeyDown` for history navigation |
| `pkg/tui/app.go` | Add `Ctrl+O` shortcut, reroute `↑↓` from chat scroll to input history |

### Not Affected

- `pkg/llm/` — LLM client unchanged
- `pkg/agent/*` — Agents still emit the same events (AgentDone with additional Result field)
- Session persistence — ChatModel's `CompletedMessages()` / `AllMessages()` only store completed entries; cards remain transient in-memory state

## Edge Cases

- **Phase with no reasoning text** (`reasoningText.Len() == 0`): No `▶` / `▼` indicator, no reasoning area rendered
- **Ctrl+O with no active phase that has reasoning**: No-op
- **Multiple cards running simultaneously** (e.g., supervisor routing to new agent while previous still in progress): Ctrl+O targets the *latest* phase across all cards (most recently appended phaseGroup)
- **Very long reasoning text**: Default tail is exactly 3 lines; full text is scrollable within the card when expanded (card height itself is capped by chat area; user scrolls the overall chat with mouse wheel/PgUpPgDown)
- **Empty final report**: If `StreamDone.Result` is empty, omit the final report section entirely

## SSE / Frontend Considerations

While this design focuses on TUI, the same event model is exposed via SSE to web frontends. The event model changes must be validated from a frontend perspective.

### Current SSE Mapping

| Stream Event | SSE Event Name | Key Data |
|-------------|---------------|----------|
| `AgentStart` | `agent_start` | query_id, agent_name |
| `AgentDone` | `agent_done` | query_id, duration, in_tokens, out_tokens — **no result** |
| `Phase` | `phase` | query_id, phase |
| `LLMTextDelta` | `llm_text_delta` | query_id, delta |
| `ToolCall` | `tool_call` | query_id, tool_name, step |
| `ToolDone` | `tool_done` | query_id, tool_name, step, elapsed |
| `ToolFail` | `tool_fail` | query_id, tool_name, step, elapsed, error |
| `StreamDone` | `stream_done` | query_id, **result** |
| `StreamErr` | `stream_err` | query_id, error |
| `InteractionRequest` | `interaction_request` | interaction_id, kind, payload |

### Changes Required

Two payload type changes in `pkg/api/types.go`:

1. **`AgentDoneData`** — add `Result string`
2. **`StreamDoneData`** — remove `Result string`
3. **`api/bridgeStreamEvent`** in `handler_stream.go` — include `e.Result` in the `agent_done` SSE event

### Frontend UX Parallel

A web frontend consuming these SSE events would render the same card structure:

```
agent_start  →  create card component
phase        →  add phase row
tool_call    →  append tool under current phase
tool_done    →  mark tool as done
llm_text_delta → update phase reasoning block (collapsible)
agent_done   →  set finalReport, mark card complete, reasoning collapsed
               card stays visible in conversation with phases + report
stream_done  →  stop listening (no payload to render)
```

Key design takeaway: the event stream IS the card protocol. Both TUI and web frontend consume the same events to build the same visual structure. The card is not a TUI-specific concept — it's the natural presentation of the event stream.

This validates that:
- Moving `Result` from `StreamDone` to `AgentDone` is correct for both TUI and web
- The phase-grouped data model (`phaseGroup` with nested tools + reasoning) matches how any frontend would structure this information
- The event stream naturally describes a card lifecycle, not just tool progress