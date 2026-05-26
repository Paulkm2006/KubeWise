# Event System Unification & Dependency Injection Architecture

## Status

Design proposal for the KubeWise architecture refactoring. Covers two related problems:
event system redundancy and dependency injection chaos.

## 1. Problem Summary

### 1.1 Three Event Layers, One Too Many

```
stream.Event (canonical, 17 types)
  → events.TUIEvent (1:1 copy, 16 types)    ← redundant
  → SSE bridge (no intermediate type)        ← correct pattern
```

The TUI maintains its own `events.TUIEvent` sealed interface with 16 concrete types
that are near-identical copies of `stream.Event` types. A `ToTUI()` conversion
function (`pkg/tui/stream_convert.go`) mechanically maps between them — zero
transformation logic, just field copies.

Meanwhile the SSE bridge (`pkg/api/handler_stream.go`) consumes `stream.Event`
directly without an intermediate type, proving the extra layer is unnecessary.

Additionally, data types (`KVPair`, `ListItem`, `ResourceDetail`, `ContainerInfo`,
`ConditionInfo`, `EventInfo`) are duplicated across `stream`, `events`, and `api`
packages with identical structure.

### 1.2 Deploy Types in the Wrong Place

`DeployPlan`, `DeployDecision`, and `PlanWarning` live in `pkg/tui/events/events.go`
but are **not events** — they do not implement `isTUIEvent()`. They were placed
there solely to avoid a circular dependency that no longer exists in the current
package graph.

### 1.3 Dependency Injection (Anti-)Pattern

Current initialization flow exposes several problems:

**Double initialization:** `router.New()` creates all 5 sub-agents (lines 63-81),
then `HandleQueryStream` creates fresh instances again per query (lines 161, 170,
179, 204-211, 263-269). The constructor's work is wasted.

**Scattered dependencies:** `*llm.Client` is stored as a field on router, each
sub-agent, supervisor instances, deploy State, deploy pipeline nodes — at least
10 structs store the same pointer, manually threaded through constructors.

**Mutable singleton:** `OnChunk` callback is a mutable field on the shared
`*llm.Client` struct. Streaming calls mutate it before each use with no mutex
protection. Safe only because TUI processes one query at a time, but technically
unsafe under concurrent API usage.

**No session concept:** There is no lifecycle boundary. All dependencies are
process-level singletons. A per-session LLM client (with per-session rate limits,
logging, tracing) cannot be created without touching every file.

## 2. Design: Event Unification

### 2.1 TUI Consumes stream.Event Directly

```
BEFORE:
  stream.Event → ToTUI() → events.TUIEvent → ChatModel.Update()

AFTER:
  stream.Event → ChatModel.Update()  (type-switch on stream.Event directly)
```

Removed:
- `pkg/tui/events/events.go` — entire file (TUIEvent interface + 16 types + data copies)
- `pkg/tui/stream_convert.go` — entire file
- `pkg/tui/stream_convert_test.go` — entire file

Changed:
- `chatEventMsg` wrapping type: `events.TUIEvent` → `stream.Event`
- `ChatModel.Update()` cases: `events.PhaseEvent` → `stream.Phase`, etc.
- Renderer parameters: `events.RenderTableEvent` → `stream.RenderTable`, etc.
- `App.dispatchStreamEvent()`: remove `ToTUI()` call, type-switch on `stream.Event`

Preserved:
- `TeaMsg` wrapper (`pkg/stream/tea_listen.go`) — already wraps `stream.Event`
- `dispatchStreamEvent` special handling for `InteractionRequest`/`StreamDone`/`StreamErr`
- Bubble Tea message types (`chatEventMsg`, `confirmMsg`, etc.) — they route events,
  they don't define event types

### 2.2 Deploy Types Move to pkg/agent/deploy/types.go

`DeployPlan`, `DeployDecision`, `PlanWarning` move from `pkg/tui/events/events.go`
to `pkg/agent/deploy/types.go`.

Reference updates:

| File | Change |
|------|--------|
| `pkg/agent/deploy/agent.go` | Same package, no import change |
| `pkg/agent/deploy/state/state.go` | Same package |
| `pkg/agent/deploy/core/plan/plan.go` | `events.PlanWarning` → `deploy-types` |
| `pkg/agent/deploy/nodes/plan_helpers.go` | `events.DeployDecision` → `deploy-types` |
| `pkg/agent/deploy/nodes/review_plan.go` | `events.DeployDecision` → `deploy-types` |
| `pkg/agent/router/bridge.go` | `events.DeployPlan/Decision` → `deploy-types` |
| `pkg/tui/model/deploy_confirm.go` | Add `agent/deploy` import, drop `events` |
| `pkg/tui/model/deploy_confirm_test.go` | Same |

### 2.3 Package Cleanup

After both moves, `pkg/tui/events/` has zero content and is deleted.

The duplicated data types in `pkg/api/` (`api.KVPair`, `api.ResourceDetailData`, etc.)
are kept for now — they carry JSON tags for SSE serialization which the stream types
lack. A follow-up could add JSON tags to stream types and remove the API copies, but
that is out of scope for this change.

## 3. Design: Session & Dependency Injection

### 3.1 Session Object

A `Session` struct acts as the dependency container. All shared resources are
created once in its constructor and injected into the component tree.

```go
// pkg/session/session.go
type Session struct {
    LLM    *llm.Client
    K8s    *k8s.Client
    Helm   *helm.Client
    Router *router.Agent
}

func New(cfg Config) (*Session, error)
```

`New` is the single place where all dependencies are wired:

1. Create `llm.Client` from config
2. Create `k8s.Client` from config
3. Create `helm.Client`
4. Create `router.Agent` — passing all three
5. `router.New` creates all 5 sub-agents once, stores them as fields

No more scattered `New()` calls in entry points.

### 3.2 Sub-Agent Lifecycle

Sub-agents are created **once** in `router.New()` and reused across all queries
on the same session. `HandleQueryStream` does not re-create sub-agents.

For sub-agents that need per-query state (query text, entities, event channel),
the pattern is:

```go
type QueryAgent struct {
    llmClient *llm.Client    // immutable, set once in New()
    k8sClient *k8s.Client    // immutable
    // ...
}

func (a *QueryAgent) HandleQuery(ctx, query, eventCh) {
    // use a.llmClient, a.k8sClient — not re-created
    // per-query state is local variables or ephemeral objects
}
```

### 3.3 OnChunk as Parameter, Not Field

Remove `OnChunk` from the `llm.Client` struct. Add it as a parameter to
`ChatCompletion`.

```go
// BEFORE
type Client struct {
    OnChunk func([]byte)  // mutable field, shared across goroutines
}
func (c *Client) ChatCompletion(ctx, msgs, tools) (*Response, error)

// AFTER
type Client struct { /* no OnChunk */ }
func (c *Client) ChatCompletion(ctx, msgs, tools, onChunk func([]byte)) (*Response, error)
```

This makes `llm.Client` safe for concurrent use and removes the implicit
sequencing requirement.

### 3.4 Entry Point Simplification

```go
// BEFORE — cmd/main.go TUI path
llmClient := llm.NewClient(config)
k8sClient := k8s.New(config)
tui.Run(k8sClient, llmClient, maxSteps, supervisorCfg)

// AFTER
session := session.New(session.Config{
    LLM:        config,
    K8s:        kubeConfig,
    MaxSteps:   10,
    Supervisor: supervisor.DefaultConfig(),
})
tui.Run(session)
```

## 4. Package Dependency Graph (After)

```
cmd/
  └─ session.New() creates everything

pkg/session/
  └─ depends on: llm, k8s, helm, agent/router

pkg/agent/router/
  └─ depends on: llm, k8s, agent/query, agent/operation, ...

pkg/agent/query/
  └─ depends on: llm, k8s, tool (NOT tui/events — removed)

pkg/agent/deploy/
  └─ defines: DeployPlan, DeployDecision, PlanWarning
  └─ depends on: llm, k8s, helm (NOT tui/events — removed)

pkg/tui/
  └─ depends on: stream (direct event consumption), session, agent (for types)

pkg/api/
  └─ depends on: stream (direct SSE bridging), session
```

Circular dependencies eliminated. Each layer depends on abstractions or
lower-level packages, never upward.

## 5. Migration Order

| Step | Description | Files Touched |
|------|-------------|---------------|
| 1 | Move DeployPlan/Decision/Warning → deploy/types.go | ~10 files |
| 2 | Remove TUIEvent, TUI consumes stream.Event directly | ~12 files |
| 3 | Delete pkg/tui/events/ | 0 (empty) |
| 4 | Add Session container, wire in cmd/main.go | ~6 files |
| 5 | Move OnChunk to ChatCompletion parameter | ~8 files |
| 6 | Stop re-creating sub-agents in HandleQueryStream | 1 file (router/agent.go) |

Steps 1-3 are an isolated event refactor. Steps 4-6 are a separate DI refactor.
They can be done in a single pass or split across PRs.
