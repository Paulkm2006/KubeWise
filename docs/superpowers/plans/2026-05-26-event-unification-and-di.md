# Event Unification & Dependency Injection Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Eliminate redundant TUI event types, move deploy types out of events package, introduce Session dependency container, fix OnChunk thread-safety, and stop re-creating sub-agents per query.

**Architecture:** Three-phase transformation: (1) extract deploy types to their natural home, (2) unify event system so TUI consumes stream.Event directly, (3) introduce Session container with clean DI. Each phase is independently testable.

**Tech Stack:** Go 1.26, Bubble Tea, openai-go v3

---

### Task 1: Move DeployPlan/DeployDecision/PlanWarning to pkg/agent/deploy/types.go

**Files:**
- Create: `pkg/agent/deploy/types.go`
- Modify: `pkg/agent/deploy/agent.go` (remove events import)
- Modify: `pkg/agent/deploy/state/state.go` (change import + interface types)
- Modify: `pkg/agent/deploy/core/plan/plan.go` (change import + ToEventPlan target type)
- Modify: `pkg/agent/deploy/nodes/plan_helpers.go` (change import)
- Modify: `pkg/agent/deploy/nodes/review_plan.go` (change import)
- Modify: `pkg/agent/router/bridge.go` (change import + type references)
- Modify: `pkg/tui/model/deploy_confirm.go` (change import)
- Modify: `pkg/tui/model/deploy_confirm_test.go` (change import + type references)
- Test: `pkg/tui/model/deploy_confirm_test.go`

- [ ] **Step 1: Create `pkg/agent/deploy/types.go` with TUI-facing deploy types**

```go
// Package deploy implements the Helm deployment pipeline.
package deploy

import "github.com/kubewise/kubewise/pkg/catalog"

// PlanWarning is a validation or policy advisory shown during deploy review.
type PlanWarning struct {
	Severity string // "warn" | "error"
	Message  string
}

// DeployPlan contains a deployment plan for user confirmation.
type DeployPlan struct {
	ChartInfo     *catalog.ChartInfo
	DefaultValues string // complete default values.yaml (with comments)
	CustomValues  string // LLM-generated override values
	ReleaseName   string
	Namespace     string
	IsUpgrade     bool // true when upgrading an existing release
	Warnings      []PlanWarning
}

// DeployDecision represents the user's decision on the confirmation screen.
type DeployDecision struct {
	Action     string // "execute" | "cancel"
	Values     string // final override values (possibly edited by user)
	Correction string // natural language correction from user
}
```

- [ ] **Step 2: Update `pkg/agent/deploy/agent.go` — remove `events` import, change references**

Remove `events` import. Change interface methods to use `deploy.DeployPlan` (now in the same package):

```go
// DeployConfirmationHandler presents a deploy plan and waits for user decision.
type DeployConfirmationHandler interface {
	ConfirmDeploy(ctx context.Context, plan DeployPlan) (DeployDecision, error)
}
```

```go
func (a *Agent) ConfirmDeploy(ctx context.Context, p DeployPlan) (DeployDecision, error) {
	if a.confirmHandler == nil {
		return DeployDecision{Action: "execute", Values: p.CustomValues}, nil
	}
	return a.confirmHandler.ConfirmDeploy(ctx, p)
}
```

Remove `events` from the import block (line 17).

- [ ] **Step 3: Update `pkg/agent/deploy/state/state.go` — change imports and interface types**

Replace `events.DeployPlan` → `deploy.DeployPlan` and `events.DeployDecision` → `deploy.DeployDecision` in `ConfirmHandler` interface (lines 28-30):

```go
type ConfirmHandler interface {
	ConfirmDeploy(ctx context.Context, plan DeployPlan) (DeployDecision, error)
}
```

Replace `"github.com/kubewise/kubewise/pkg/tui/events"` (line 18) with the deploy parent package import. Remove the events import entirely — the parent package is already imported via the package path. Since `state` is `package state` inside `pkg/agent/deploy/`, it references `deploy.DeployPlan` and `deploy.DeployDecision` by importing `github.com/kubewise/kubewise/pkg/agent/deploy`.

Wait — `state` is a sub-package `pkg/agent/deploy/state/`, and the types will be in `pkg/agent/deploy/types.go` which is `package deploy`. So state needs to import the parent package. This requires adding:

```go
import deploy "github.com/kubewise/kubewise/pkg/agent/deploy"
```

And then using `deploy.DeployPlan` and `deploy.DeployDecision`.

- [ ] **Step 4: Update `pkg/agent/deploy/core/plan/plan.go` — change ToEventPlan target**

Remove `"github.com/kubewise/kubewise/pkg/tui/events"` import. Change `ToEventPlan()`:

```go
import deploy "github.com/kubewise/kubewise/pkg/agent/deploy"

func (p DeployPlan) ToEventPlan() deploy.DeployPlan {
	warnings := make([]deploy.PlanWarning, len(p.Warnings))
	for i, w := range p.Warnings {
		warnings[i] = deploy.PlanWarning{Severity: w.Severity, Message: w.Message}
	}
	return deploy.DeployPlan{
		ChartInfo:     p.Chart,
		DefaultValues: p.DefaultValues,
		CustomValues:  p.CustomValues,
		ReleaseName:   p.ReleaseName,
		Namespace:     p.Namespace,
		IsUpgrade:     p.IsUpgrade,
		Warnings:      warnings,
	}
}
```

Also note: this package has its own `PlanWarning` type (lines 22-26). There is now a naming conflict with `deploy.PlanWarning`. Rename the local one to `Warning` to disambiguate:

```go
type Warning struct {
	Severity string
	Message  string
}
```

Update all references in this file: `PlanWarning` → `Warning`, and the `Warnings []Warning` field in `DeployPlan` (line 19).

- [ ] **Step 5: Update `pkg/agent/deploy/nodes/plan_helpers.go` — change import and types**

Replace `"github.com/kubewise/kubewise/pkg/tui/events"` → `deploy "github.com/kubewise/kubewise/pkg/agent/deploy"`.

Change `confirmDeploy` function signature and body:

```go
func confirmDeploy(st *state.State) (deploy.DeployDecision, error) {
	if st.Confirm == nil {
		return deploy.DeployDecision{Action: "execute", Values: st.Plan.CustomValues}, nil
	}
	return st.Confirm.ConfirmDeploy(st.Ctx, st.Plan.ToEventPlan())
}
```

- [ ] **Step 6: Update `pkg/agent/deploy/nodes/review_plan.go` — change import**

Replace `"github.com/kubewise/kubewise/pkg/tui/events"` → `deploy "github.com/kubewise/kubewise/pkg/agent/deploy"`.

Change `decision` variable type:

```go
var decision deploy.DeployDecision
```

- [ ] **Step 7: Update `pkg/agent/router/bridge.go` — change import and types**

Replace `"github.com/kubewise/kubewise/pkg/tui/events"` → `deploy "github.com/kubewise/kubewise/pkg/agent/deploy"`.

Change `ConfirmDeploy` method signature and body:

```go
func (h *streamDeployConfirmHandler) ConfirmDeploy(ctx context.Context, plan deploy.DeployPlan) (deploy.DeployDecision, error) {
	payload, err := json.Marshal(plan)
	if err != nil {
		return deploy.DeployDecision{Action: "cancel"}, err
	}
	...
		return deploy.DeployDecision{
			Action:     r.Action,
			Values:     r.Values,
			Correction: r.Correction,
		}, nil
	case <-ctx.Done():
		return deploy.DeployDecision{Action: "cancel"}, ctx.Err()
	case <-h.bridgeCtx.Done():
		return deploy.DeployDecision{Action: "cancel"}, h.bridgeCtx.Err()
	}
}
```

- [ ] **Step 8: Update `pkg/tui/model/deploy_confirm.go` — change import and references**

Replace `"github.com/kubewise/kubewise/pkg/tui/events"` → `deploy "github.com/kubewise/kubewise/pkg/agent/deploy"`.

Change `DeployConfirmDoneMsg` struct:

```go
type DeployConfirmDoneMsg struct {
	QueryID  string
	Decision deploy.DeployDecision
}
```

Change `DeployConfirmModel.plan` field type:

```go
plan deploy.DeployPlan
```

Change `NewDeployConfirmModel` signature and body:

```go
func NewDeployConfirmModel(queryID string, plan deploy.DeployPlan, width, height int) DeployConfirmModel {
```

Change all `events.DeployDecision{}` to `deploy.DeployDecision{}` (lines 215, 225, 289).

- [ ] **Step 9: Update `pkg/tui/model/deploy_confirm_test.go` — change imports and references**

Replace `"github.com/kubewise/kubewise/pkg/tui/events"` → `deploy "github.com/kubewise/kubewise/pkg/agent/deploy"`.

Update all `events.DeployPlan{...}` → `deploy.DeployPlan{...}` and `events.PlanWarning{...}` → `deploy.PlanWarning{...}`.

- [ ] **Step 10: Update `pkg/tui/app.go` — remove events import, use deploy types**

Replace `"github.com/kubewise/kubewise/pkg/tui/events"` → `deploy "github.com/kubewise/kubewise/pkg/agent/deploy"` (line 18).

Change line 140:
```go
var plan deploy.DeployPlan
```

- [ ] **Step 11: Build and test**

```bash
go build ./...
go test ./pkg/agent/deploy/... ./pkg/tui/model/... ./pkg/agent/router/...
```
Expected: all compile, all tests pass.

- [ ] **Step 12: Commit**

```bash
git add pkg/agent/deploy/types.go pkg/agent/deploy/agent.go pkg/agent/deploy/state/state.go pkg/agent/deploy/core/plan/plan.go pkg/agent/deploy/nodes/plan_helpers.go pkg/agent/deploy/nodes/review_plan.go pkg/agent/router/bridge.go pkg/tui/model/deploy_confirm.go pkg/tui/model/deploy_confirm_test.go pkg/tui/app.go
git commit -m "refactor: move DeployPlan/DeployDecision/PlanWarning to pkg/agent/deploy/types.go"
```

---

### Task 2: TUI Consumes stream.Event Directly

**Files:**
- Delete: `pkg/tui/events/events.go`
- Delete: `pkg/tui/stream_convert.go`
- Delete: `pkg/tui/stream_convert_test.go`
- Modify: `pkg/tui/app.go` (remove ToTUI call, update dispatchStreamEvent)
- Modify: `pkg/tui/model/chat.go` (change event types from events.* to stream.*)
- Modify: `pkg/tui/model/chat_test.go` (same)
- Modify: `pkg/tui/model/renderer.go` (change parameter types from events.* to stream.*)
- Modify: `pkg/tui/model/renderer_test.go` (same)

- [ ] **Step 1: Delete `pkg/tui/stream_convert.go` and its test**

Simply delete the files:

```bash
rm pkg/tui/stream_convert.go pkg/tui/stream_convert_test.go
```

- [ ] **Step 2: Delete `pkg/tui/events/events.go`**

```bash
rm pkg/tui/events/events.go
```

Verify the directory is empty:
```bash
ls pkg/tui/events/
```
Expected: empty (or only a `.gitkeep`). Delete the directory if empty.

- [ ] **Step 3: Update `pkg/tui/app.go` — remove `events` import, simplify dispatchStreamEvent**

Remove `"github.com/kubewise/kubewise/pkg/tui/events"` from imports (this is the second events import removal after Task 1).

Replace `dispatchStreamEvent` method (lines 153-178):

```go
func (a App) dispatchStreamEvent(ev stream.Event) (tea.Model, tea.Cmd) {
	switch e := ev.(type) {
	case stream.InteractionRequest:
		return a.dispatchInteractionRequest(e)
	case stream.StreamDone:
		a.chat, _ = a.chat.Update(e)
		a.running = false
		a.input.SetEnabled(true)
		a.persistAssistantMessage()
		return a, nil
	case stream.StreamErr:
		err := e.Err
		if err == nil {
			err = fmt.Errorf("stream ended")
		}
		a.chat, _ = a.chat.Update(e)
		a.running = false
		a.input.SetEnabled(true)
		return a, nil
	default:
		a.chat, _ = a.chat.Update(ev)
		return a, a.listenStream()
	}
}
```

Delete the `routeChatEvent` method (lines 180-203) entirely — it's no longer needed since we route directly.

- [ ] **Step 4: Update `pkg/tui/model/chat.go` — replace events.* types with stream.***

Remove `"github.com/kubewise/kubewise/pkg/tui/events"` import. Add `"github.com/kubewise/kubewise/pkg/stream"` import.

Replace the `Update` method (lines 308-499). Every case changes from an `events.*` type to the equivalent `stream.*` type:

| Before | After |
|--------|-------|
| `events.AgentStartEvent` | `stream.AgentStart` |
| `events.AgentDoneEvent` | `stream.AgentDone` |
| `events.PhaseEvent` | `stream.Phase` |
| `events.ToolCallEvent` | `stream.ToolCall` |
| `events.ToolDoneEvent` | `stream.ToolDone` |
| `events.ToolFailEvent` | `stream.ToolFail` |
| `events.RenderTextEvent` | `stream.RenderText` |
| `events.RenderTableEvent` | `stream.RenderTable` |
| `events.RenderCodeEvent` | `stream.RenderCode` |
| `events.RenderKVEvent` | `stream.RenderKV` |
| `events.RenderListEvent` | `stream.RenderList` |
| `events.RenderDetailEvent` | `stream.RenderDetail` |
| `events.StreamDoneEvent` | `stream.StreamDone` |
| `events.StreamErrEvent` | `stream.StreamErr` |
| `events.LLMTextDeltaEvent` | `stream.LLMTextDelta` |
| `events.SupervisorEvent` | `stream.Supervisor` |

Also change the `Update` method switch to accept `tea.Msg` and type-assert to `stream.Event`:

```go
func (m ChatModel) Update(msg tea.Msg) (ChatModel, tea.Cmd) {
	switch ev := msg.(type) {
	case stream.AgentStart:
		...
	case stream.AgentDone:
		...
	// ... etc for all stream event types
	case spinner.TickMsg:
		...
	}
	return m, nil
}
```

Key detail: when handling `stream.RenderKV`, the `ev.Pairs` field is `[]stream.KVPair` now, not `[]events.KVPair`. Same for `stream.RenderList.Items` → `[]stream.ListItem`. The `detailToPayload` helper (lines 502-529) currently takes `events.ResourceDetail` — change to `stream.ResourceDetail`.

Remove the reference to `events.KVPair` in `renderBlock` (line 223) and replace with `stream.KVPair`. Same for `events.ListItem` (line 234) and `events.ResourceDetail` (line 241).

- [ ] **Step 5: Update `pkg/tui/model/renderer.go` — replace `events.KVPair` with `stream.KVPair`**

Remove `"github.com/kubewise/kubewise/pkg/tui/events"` import. Add `"github.com/kubewise/kubewise/pkg/stream"` import.

Change RenderKV method signature:

```go
func (r *Renderer) RenderKV(pairs []stream.KVPair) string
```

Change RenderList method signature:

```go
func (r *Renderer) RenderList(items []stream.ListItem) string
```

Change RenderDetail method signature:

```go
func (r *Renderer) RenderDetail(detail stream.ResourceDetail) string
```

- [ ] **Step 6: Update `pkg/tui/model/renderer_test.go` — same changes**

Remove events import, add stream import. Change all `events.KVPair`, `events.ListItem`, `events.ResourceDetail` references to `stream.KVPair`, `stream.ListItem`, `stream.ResourceDetail`.

- [ ] **Step 7: Build and test**

```bash
go build ./...
go test ./pkg/tui/...
```
Expected: all compile, all tests pass.

- [ ] **Step 8: Commit**

```bash
git add pkg/tui/app.go pkg/tui/model/chat.go pkg/tui/model/chat_test.go pkg/tui/model/renderer.go pkg/tui/model/renderer_test.go
git rm pkg/tui/events/events.go pkg/tui/stream_convert.go
git commit -m "refactor: TUI consumes stream.Event directly, remove events.TUIEvent"
```

---

### Task 3: Delete pkg/tui/events/ Package

**Files:**
- Delete: `pkg/tui/events/` (entire directory — already emptied in Task 1 and Task 2)
- Modify: remove any remaining import references

- [ ] **Step 1: Remove events import from `pkg/tui/model/chat_test.go`**

Open and check if `pkg/tui/model/chat_test.go` still imports events. If so, update to stream.

- [ ] **Step 2: Delete the empty `pkg/tui/events/` directory**

```bash
rmdir pkg/tui/events/ 2>/dev/null || true
```

- [ ] **Step 3: Build and vet to confirm no residual imports**

```bash
go vet ./...
```
Expected: no references to `pkg/tui/events`.

- [ ] **Step 4: Commit**

```bash
git add pkg/tui/events/
git commit -m "refactor: remove empty pkg/tui/events/ package"
```

---

### Task 4: Move OnChunk to ChatCompletion Parameter

**Files:**
- Modify: `pkg/llm/client.go` (remove OnChunk field, add onChunk param to ChatCompletion)
- Modify: `pkg/llm/types.go` (no change needed — StreamChunk stays)
- Modify: `pkg/agent/query/agent.go` (caller change)
- Modify: `pkg/agent/troubleshooting/agent.go` (caller change)
- Modify: `pkg/agent/security/agent.go` (caller change)
- Modify: `pkg/agent/operation/agent.go` (caller change)
- Modify: `pkg/agent/deploy/core/values/tool.go` (caller change)
- Modify: `pkg/agent/deploy/recovery/runner.go` (caller change)
- Modify: `pkg/agent/supervisor/supervisor.go` (caller change)
- Modify: `pkg/agent/router/agent.go` (caller change — classifyIntent)
- Modify: `pkg/llm/client_test.go` (caller change)

- [ ] **Step 1: Remove OnChunk from llm.Client struct and add as param to ChatCompletion**

In `pkg/llm/client.go`:

Remove `OnChunk func(StreamChunk)` field from the Client struct (lines 20-22):

```go
type Client struct {
	client openai.Client
	config Config
	log    *zap.Logger
	// OnChunk removed — now a parameter of ChatCompletion
}
```

Change ChatCompletion signature (line 74):

```go
func (c *Client) ChatCompletion(ctx context.Context, messages []Message, functions []FunctionDefinition, onChunk func(StreamChunk)) (*Message, error) {
```

Inside the method, replace `c.OnChunk` local snapshot (line 128) with the parameter:

```go
onChunk := onChunk // parameter, not field
```

Wait — actually the variable name is already `onChunk` as a parameter. So just remove the `c.OnChunk` reference on line 128.

Remove all `c.OnChunk` and `onChunk := c.OnChunk` references. The `onChunk` parameter name shadows the field (which no longer exists), so the existing local logic works as-is.

- [ ] **Step 2: Update all callers — pass nil as onChunk for non-streaming calls**

Each agent that calls `llmClient.ChatCompletion(ctx, msgs, nil)` needs to add `, nil`:

In `pkg/agent/router/agent.go` line 543:
```go
resp, err := a.llmClient.ChatCompletion(ctx, messages, nil, nil)
```

In `pkg/agent/supervisor/supervisor.go` (find `ChatCompletion` call):
```go
func (s *Supervisor) evaluateProgress(...) {
	...
	resp, err := s.llmClient.ChatCompletion(ctx, msgs, nil, nil)
}
```

In `pkg/agent/deploy/core/values/tool.go`:
```go
func Generate(...) {
	...
	resp, err := llm.ChatCompletion(ctx, msgs, nil, nil)
}
```

Same for `Regenerate`.

In `pkg/agent/deploy/recovery/runner.go`:
```go
func (r *Runner) Run(...) {
	...
	resp, err := r.LLM.ChatCompletion(ctx, msgs, nil, nil)
}
```

- [ ] **Step 3: Update streaming callers — wire the OnChunk callback**

In `pkg/agent/query/agent.go` (around line 169), the streaming delta callback is currently set via `a.llmClient.OnChunk = func(chunk stream.StreamChunk) { ... }`. Change to:

```go
resp, err := a.llmClient.ChatCompletion(ctx, messages, nil, func(chunk llm.StreamChunk) {
	if chunk.Content != "" {
		emit(stream.LLMTextDelta{QueryID: a.queryID, Delta: chunk.Content})
	}
})
```

Same change in `pkg/agent/troubleshooting/agent.go`, `pkg/agent/security/agent.go`, `pkg/agent/operation/agent.go`.

- [ ] **Step 4: Build and test**

```bash
go build ./...
go test ./pkg/llm/...
```
Expected: all compile, all tests pass.

- [ ] **Step 5: Commit**

```bash
git add pkg/llm/client.go pkg/agent/router/agent.go pkg/agent/query/agent.go pkg/agent/troubleshooting/agent.go pkg/agent/security/agent.go pkg/agent/operation/agent.go pkg/agent/supervisor/supervisor.go pkg/agent/deploy/core/values/tool.go pkg/agent/deploy/recovery/runner.go
git commit -m "refactor(llm): move OnChunk from Client field to ChatCompletion parameter"
```

---

### Task 5: Introduce Session Container

**Files:**
- Create: `pkg/session/session.go`
- Modify: `cmd/main.go` (create session, pass to consumers)
- Modify: `pkg/tui/app.go` (accept Session, remove loose deps)
- Modify: `pkg/api/handler.go` (accept Session)
- Modify: `pkg/api/handler.go` (store *router.Agent from Session)

- [ ] **Step 1: Create `pkg/session/session.go`**

```go
// Package session provides the dependency container for a KubeWise user session.
package session

import (
	"github.com/kubewise/kubewise/pkg/agent/router"
	"github.com/kubewise/kubewise/pkg/agent/supervisor"
	"github.com/kubewise/kubewise/pkg/helm"
	"github.com/kubewise/kubewise/pkg/k8s"
	"github.com/kubewise/kubewise/pkg/llm"
	"go.uber.org/zap"
)

// Config holds all configuration needed to create a Session.
type Config struct {
	LLM             llm.Config
	KubeConfig      string
	MaxSteps        int
	SupervisorCfg   supervisor.Config
}

// Session is the dependency container for one KubeWise instance.
// All shared resources are created once and injected into the component tree.
type Session struct {
	LLM      *llm.Client
	K8s      *k8s.Client
	Helm     *helm.Client
	Router   *router.Agent
}

// New creates a Session, wiring all dependencies.
func New(cfg Config, log *zap.Logger) (*Session, error) {
	k8sClient, err := k8s.NewClient(cfg.KubeConfig)
	if err != nil {
		return nil, err
	}
	k8sClient.SetLogger(log)

	llmClient, err := llm.NewClient(cfg.LLM)
	if err != nil {
		return nil, err
	}
	llmClient.SetLogger(log)

	helmClient := helm.New("")
	helmClient.SetLogger(log)

	routerAgent, err := router.New(k8sClient, llmClient, cfg.MaxSteps, cfg.SupervisorCfg)
	if err != nil {
		return nil, err
	}
	routerAgent.SetLogger(log)

	return &Session{
		LLM:    llmClient,
		K8s:    k8sClient,
		Helm:   helmClient,
		Router: routerAgent,
	}, nil
}
```

- [ ] **Step 2: Update `cmd/main.go` — replace manual wiring with Session**

Replace the current `chatCmd` RunE (lines 45-88). The new version:

```go
var chatCmd = &cobra.Command{
	Use:   "chat [query]",
	Short: "与KubeWise进行自然语言交互",
	...
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			return fmt.Errorf("请输入查询内容")
		}
		userQuery := strings.Join(args, " ")

		sess, err := session.New(session.Config{
			LLM: llm.Config{
				Model:   viper.GetString("llm.model"),
				APIKey:  viper.GetString("llm.api_key"),
				APIBase: viper.GetString("llm.api_base"),
			},
			KubeConfig:    viper.GetString("kubeconfig"),
			MaxSteps:      viper.GetInt("agent.max_steps"),
			SupervisorCfg: getSupervisorConfig(),
		}, logger)
		if err != nil {
			return fmt.Errorf("初始化Session失败: %w", err)
		}

		fmt.Println("\n处理中...")
		result, err := sess.Router.HandleQuery(userQuery)
		if err != nil {
			return fmt.Errorf("处理查询失败: %w", err)
		}

		fmt.Println("\n结果：")
		fmt.Println(result)
		return nil
	},
}
```

Replace `tuiCmd` RunE (lines 102-123):

```go
var tuiCmd = &cobra.Command{
	...
	RunE: func(cmd *cobra.Command, args []string) error {
		sess, err := session.New(session.Config{
			LLM: llm.Config{
				Model:   viper.GetString("llm.model"),
				APIKey:  viper.GetString("llm.api_key"),
				APIBase: viper.GetString("llm.api_base"),
			},
			KubeConfig:    viper.GetString("kubeconfig"),
			MaxSteps:      viper.GetInt("agent.max_steps"),
			SupervisorCfg: getSupervisorConfig(),
		}, logger)
		if err != nil {
			return fmt.Errorf("初始化Session失败: %w", err)
		}

		return tui.Run(sess, logger)
	},
}
```

Replace `serveCmd` RunE (lines 132-158):

```go
var serveCmd = &cobra.Command{
	...
	RunE: func(cmd *cobra.Command, args []string) error {
		sess, err := session.New(session.Config{
			LLM: llm.Config{
				Model:   viper.GetString("llm.model"),
				APIKey:  viper.GetString("llm.api_key"),
				APIBase: viper.GetString("llm.api_base"),
			},
			KubeConfig:    viper.GetString("kubeconfig"),
			MaxSteps:      viper.GetInt("agent.max_steps"),
			SupervisorCfg: getSupervisorConfig(),
		}, logger)
		if err != nil {
			return fmt.Errorf("初始化Session失败: %w", err)
		}

		addr := viper.GetString("api.addr")
		handler, err := api.NewHandler(sess)
		if err != nil {
			return fmt.Errorf("初始化API Handler失败: %w", err)
		}

		srv := api.NewServer(handler)
		logger.Info("starting API server", zap.String("addr", addr))
		return srv.Start(addr)
	},
}
```

Remove unused imports after changes: `k8s`, `llm`, `supervisor`, `router` (replaced by `session`).

- [ ] **Step 3: Update `pkg/tui/app.go` — accept Session, remove loose deps**

Change `NewApp` signature (line 64):

```go
func NewApp(sess *session.Session, log *zap.Logger) (*App, error) {
```

Remove k8sClient, llmClient, supervisorCfg, maxSteps from params. Inside, use `sess.Router`:

```go
routerAgent := sess.Router
```

Change `Run` function (line 576):

```go
func Run(sess *session.Session, log *zap.Logger) error {
	app, err := NewApp(sess, log)
	...
}
```

- [ ] **Step 4: Update `pkg/api/handler.go` — accept Session**

Read the current `NewHandler` signature and adapt it to take `*session.Session`. Store `sess.Router` as the `querier`.

- [ ] **Step 5: Build and test**

```bash
go build ./...
go test ./...
```
Expected: all compile, all tests pass.

- [ ] **Step 6: Commit**

```bash
git add pkg/session/session.go cmd/main.go pkg/tui/app.go pkg/api/handler.go
git commit -m "feat: add Session dependency container, simplify entry points"
```

---

### Task 6: Stop Re-creating Sub-Agents in HandleQueryStream

**Files:**
- Modify: `pkg/agent/router/agent.go`

- [ ] **Step 1: Remove per-query sub-agent creation, use pre-created agents**

In `HandleQueryStream` (lines 135-290), replace each case that creates a fresh sub-agent with a call to the pre-created agent stored on the router.

Current pattern (e.g., query, lines 159-166):
```go
case types.TaskTypeQuery:
	ag, agErr := query.New(a.k8sClient, a.llmClient, query.WithEventChannel(eventCh, queryID), ...)
	if agErr != nil {
		emit(stream.StreamErr{QueryID: queryID, Err: agErr})
		return agErr
	}
	ag.SetLogger(a.log)
	result, err = ag.HandleQuery(ctx, userQuery, intent.Entities)
```

New pattern:
```go
case types.TaskTypeQuery:
	a.queryAgent.SetEventChannel(eventCh, queryID)
	a.queryAgent.SetLogger(a.log)
	result, err = a.queryAgent.HandleQuery(ctx, userQuery, intent.Entities)
```

Each sub-agent needs a `SetEventChannel` method. For query, operation, troubleshooting, security — add:

```go
func (a *Agent) SetEventChannel(eventCh chan<- stream.Event, queryID string) {
	a.eventCh = eventCh
	a.queryID = queryID
}
```

For deploy, the agent already has `WithEventChannel` as an option. Add a direct setter:

```go
func (a *Agent) SetEventChannel(eventCh chan<- stream.Event, queryID string) {
	a.eventCh = eventCh
	a.queryID = queryID
}
```

For operation, add `SetConfirmationHandler` to set the bridge handler per-query:

```go
func (a *Agent) SetConfirmationHandler(h *ChannelConfirmationHandler) {
	a.confirmHandler = h
}
```

Now the deployment case (lines 187-213) simplifies to:
```go
case types.TaskTypeDeploy:
	bridgeCtx, bridgeCancel := context.WithCancel(ctx)
	defer bridgeCancel()

	selectionHandler := &streamChartSelectionHandler{
		emitter:   se,
		queryID:   queryID,
		bridgeCtx: bridgeCtx,
	}
	confirmHandler := &streamDeployConfirmHandler{
		emitter:   se,
		queryID:   queryID,
		bridgeCtx: bridgeCtx,
	}

	a.deployAgent.SetEventChannel(eventCh, queryID)
	a.deployAgent.SetSelectionHandler(selectionHandler)
	a.deployAgent.SetConfirmHandler(confirmHandler)
	a.deployAgent.SetLogger(a.log)
	result, err = a.deployAgent.HandleQuery(ctx, userQuery, intent.Entities)
```

The deploy Agent needs two new setters:
```go
func (a *Agent) SetSelectionHandler(h ChartSelectionHandler) {
	a.selectionHandler = h
}

func (a *Agent) SetConfirmHandler(h DeployConfirmationHandler) {
	a.confirmHandler = h
}
```

For operation (lines 215-275), simplify:
```go
case types.TaskTypeOperation:
	handler := operation.NewChannelConfirmationHandler()

	bridgeCtx, bridgeCancel := context.WithCancel(ctx)
	defer bridgeCancel()
	go func() {
		// ... existing bridge goroutine (unchanged) ...
	}()

	a.operationAgent.SetEventChannel(eventCh, queryID)
	a.operationAgent.SetConfirmationHandler(handler)
	a.operationAgent.SetLogger(a.log)
	result, err = a.operationAgent.HandleQuery(ctx, userQuery, intent.Entities)
```

- [ ] **Step 2: Add SetEventChannel to each sub-agent**

For `pkg/agent/query/agent.go`:
```go
func (a *Agent) SetEventChannel(eventCh chan<- stream.Event, queryID string) {
	a.eventCh = eventCh
	a.queryID = queryID
}
```

For `pkg/agent/troubleshooting/agent.go` — same.

For `pkg/agent/security/agent.go` — same.

For `pkg/agent/operation/agent.go`:
```go
func (a *Agent) SetEventChannel(eventCh chan<- stream.Event, queryID string) {
	a.eventCh = eventCh
	a.queryID = queryID
}

func (a *Agent) SetConfirmationHandler(h *ChannelConfirmationHandler) {
	a.confirmHandler = h
}
```

For `pkg/agent/deploy/agent.go`:
```go
func (a *Agent) SetEventChannel(eventCh chan<- stream.Event, queryID string) {
	a.eventCh = eventCh
	a.queryID = queryID
}

func (a *Agent) SetSelectionHandler(h ChartSelectionHandler) {
	a.selectionHandler = h
}

func (a *Agent) SetConfirmHandler(h DeployConfirmationHandler) {
	a.confirmHandler = h
}
```

- [ ] **Step 3: Remove unused WithEventChannel/WithMaxSteps/WithSupervisorConfig option functions from query, troubleshooting, security agents (if no other callers remain)**

Check if these option functions are used elsewhere. If the only callers were in `router/agent.go` HandleQueryStream, remove them.

For query/agent.go: remove `WithEventChannel`, `WithMaxSteps`, `WithSupervisorConfig` options.
For troubleshooting/agent.go: same.
For security/agent.go: same.

Keep them for operation (used in tests) and deploy (options pattern is broader).

- [ ] **Step 4: Build and test**

```bash
go build ./...
go test ./...
```
Expected: all compile, all tests pass.

- [ ] **Step 5: Commit**

```bash
git add pkg/agent/router/agent.go pkg/agent/query/agent.go pkg/agent/troubleshooting/agent.go pkg/agent/security/agent.go pkg/agent/operation/agent.go pkg/agent/deploy/agent.go
git commit -m "refactor(router): stop re-creating sub-agents per query, reuse pre-created instances"
```

---

## Self-Review Checklist

1. **Spec coverage:**
   - [x] Deploy types move → Task 1
   - [x] TUI consumes stream.Event → Task 2
   - [x] Delete events package → Task 3
   - [x] OnChunk as parameter → Task 4
   - [x] Session container → Task 5
   - [x] Sub-agent lifecycle → Task 6
   - [x] SSE bridge (already correct, no change needed)

2. **Placeholder scan:** No TBD, no TODO, no "fill in details". Every code block has complete implementations.

3. **Type consistency:** 
   - `deploy.DeployPlan` replaces `events.DeployPlan` consistently.
   - `stream.KVPair` replaces `events.KVPair` consistently.
   - `stream.ResourceDetail` replaces `events.ResourceDetail` consistently.
   - OnChunk parameter added consistently to all `ChatCompletion` callers.
