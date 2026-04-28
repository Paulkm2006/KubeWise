# Operation Agent Design

**Date:** 2026-04-28  
**Status:** Approved  
**Author:** Paul Liu

---

## Overview

Implement the `operation` task type in KubeWise, which is currently a stub in the router agent. The Operation Agent allows users to describe cluster mutations in natural language (scale, restart, delete, apply, cordon/drain, label/annotate), and executes them only after explicit per-step user confirmation.

---

## Architecture

### Two-Phase: Plan then Execute

```
User Input
  │
  ▼
[Planning Phase] OperationAgent → LLM (with read tools)
  │  Uses existing read tools to inspect current cluster state
  │  Outputs []OperationStep via submit_operation_plan virtual tool
  │
  ▼
[Execution Phase] Iterate over each OperationStep
  │  Display step summary (structured or YAML diff)
  │  User confirms [y/N] or provides correction instruction
  │    y        → execute write tool → proceed to next step
  │    n + text → LLM replans single step → re-confirm (max 2 retries)
  │    n + <CR> → skip step → proceed to next step
  │
  ▼
Return aggregated result summary
```

### New Files

```
pkg/
├── agent/
│   └── operation/
│       ├── agent.go       # OperationAgent: plan + execute loop
│       └── confirm.go     # ConfirmationHandler interface + implementations
├── k8s/
│   └── operations.go      # K8s write methods (new file)
└── tools/v1/operation/    # Six operation tools
    ├── scale.go
    ├── restart.go
    ├── delete.go
    ├── apply.go
    ├── cordon_drain.go
    └── label_annotate.go
```

### Integration Points

- `pkg/agent/router/agent.go`: replace `operation` stub with `operationAgent.HandleQuery(ctx, query, entities)`
- `pkg/agent/router/agent.go` `New()`: add `operation.New(k8sClient, llmClient)` alongside the existing three agents
- `pkg/types/types.go`: `TaskTypeOperation = "operation"` already exists, no change needed

---

## Data Model

### OperationStep

The planning phase LLM returns a JSON array of steps via the `submit_operation_plan` virtual tool:

```go
type OperationStep struct {
    StepIndex     int               // 1-based step number
    OperationType string            // scale | restart | delete | apply | cordon_drain | label_annotate
    ResourceKind  string            // Deployment | StatefulSet | Pod | Node | etc.
    ResourceName  string
    Namespace     string            // empty for cluster-scoped resources
    Replicas      *int32            // scale only
    Labels        map[string]string // label_annotate only
    Annotations   map[string]string // label_annotate only
    GeneratedYAML string            // apply/create only; LLM-generated full YAML
    Description   string            // human-readable summary of the operation
}
```

---

## Confirmation UX (Mixed Mode)

### Simple operations (scale / restart / delete / cordon_drain / label_annotate)

Structured summary display:

```
步骤 1/2：Scale
  资源：Deployment/nginx (namespace: default)
  变更：replicas 3 → 5
确认执行？[y/N]：
```

### Apply/Create operations

Full YAML display:

```
步骤 1/1：Apply
  资源：Deployment/web (namespace: default)
  以下 YAML 将被 Apply：
---
apiVersion: apps/v1
kind: Deployment
...
确认执行？[y/N]：
```

### Rejection with correction

```
确认执行？[y/N]：n
请输入修正指令（直接回车跳过该步骤）：把副本数改为 10
[LLM replans single step → re-display → re-confirm]
```

- Blank input skips the step and records it as `skipped`
- Correction triggers single-step replan (max 2 retries), does not affect other steps

---

## ConfirmationHandler Interface

To support future TUI and API modes without changing agent core logic:

```go
// pkg/agent/operation/confirm.go

type ConfirmationHandler interface {
    // Confirm presents a step to the user and waits for their decision.
    // Returns: confirmed=true to execute, correction non-empty to replan,
    //          both zero values to skip the step.
    Confirm(ctx context.Context, step OperationStep) (confirmed bool, correction string, err error)
}
```

Two built-in implementations:

| Implementation | Usage |
|---|---|
| `StdinConfirmationHandler` | Default CLI mode; reads `os.Stdin`, writes to `os.Stdout` |
| `ChannelConfirmationHandler` | TUI/API mode; exposes channels for step display and decision input |

Agent constructor uses functional options:

```go
func New(k8sClient *k8s.Client, llmClient *llm.Client, opts ...Option) *Agent

// Option to inject a custom handler (defaults to StdinConfirmationHandler):
func WithConfirmationHandler(h ConfirmationHandler) Option
```

---

## K8s Write Methods (`pkg/k8s/operations.go`)

| Method | Description |
|---|---|
| `ScaleResource(ctx, namespace, kind, name string, replicas int32)` | Set replica count; supports Deployment and StatefulSet |
| `RestartResource(ctx, namespace, kind, name string)` | Patch `kubectl.kubernetes.io/restartedAt` annotation to trigger rolling restart |
| `DeleteResource(ctx, namespace string, gvr schema.GroupVersionResource, name string)` | Delete any resource via dynamic client |
| `ApplyResource(ctx context.Context, yamlContent string)` | Parse YAML, apply via Server-Side Apply (force=true, field manager="kubewise") |
| `CordonNode(ctx context.Context, nodeName string, cordon bool)` | Set/unset `spec.unschedulable` |
| `DrainNode(ctx context.Context, nodeName string)` | Evict all non-DaemonSet, non-mirror Pods; respects `ctx` timeout |
| `LabelResource(ctx, namespace string, gvr schema.GroupVersionResource, name string, labels, annotations map[string]string)` | Strategic merge patch labels and annotations |

---

## Operation Tools (`pkg/tools/v1/operation/`)

Each tool follows the existing pattern: implements `tool.Tool`, registers via `tool.RegisterGlobal()` in `init()`.

| Tool Name | K8s Method | Key Parameters |
|---|---|---|
| `scale_resource` | `ScaleResource` | namespace, kind, name, replicas |
| `restart_resource` | `RestartResource` | namespace, kind, name |
| `delete_resource` | `DeleteResource` | namespace, group, version, resource, name |
| `apply_resource` | `ApplyResource` | yaml_content |
| `cordon_drain_node` | `CordonNode` + `DrainNode` | node_name, action (cordon/uncordon/drain) |
| `label_annotate_resource` | `LabelResource` | namespace, group, version, resource, name, labels, annotations |

---

## OperationAgent Implementation

### Planning Phase (`plan` method)

- Runs a ReAct loop with **read-only tools only** (filtered from global registry by tool name prefix / category)
- Registers `submit_operation_plan` as a virtual tool; loop terminates when LLM calls it
- `submit_operation_plan` parameter: `{ "steps": [ ...OperationStep JSON... ] }`
- Maximum 10 rounds before returning an error asking the user to rephrase
- System prompt instructs LLM to query current state before planning (e.g., get current replica count for scale ops, verify resource exists before delete)

### Execution Phase (`execute` method)

```
for each step:
  attempts := 0
  for {
    display step via confirmHandler
    confirmed, correction, err = confirmHandler.Confirm(ctx, step)
    if confirmed:
      result = executeTool(step)
      record result; break
    if correction == "":
      record as skipped; break
    if attempts >= 2:
      record as failed (replan limit exceeded); break
    step = replan(step, correction)  // single-step LLM call: sends original step JSON + correction text, returns updated OperationStep
    attempts++
  }
return aggregated summary
```

### Registry Split

The agent maintains two registries loaded from the global registry:
- `readRegistry`: query-category tools (for planning phase)
- `writeRegistry`: operation-category tools (for execution phase)

Tool category is determined by a `Category` field added to `ToolMetadata` (`"query"` or `"operation"`). All existing tools keep `Category: "query"`; the six new operation tools use `Category: "operation"`.

---

## Error Handling

| Scenario | Behavior |
|---|---|
| Planning LLM never calls `submit_operation_plan` | After max rounds, return error: "无法生成操作计划，请重新描述您的需求" |
| Write tool execution fails | Show error to user, ask whether to continue remaining steps |
| Replan produces invalid step after 2 retries | Skip step, include in summary as `failed (replan limit)` |
| `DrainNode` context timeout | Return list of evicted Pods and remaining Pods |
| `ApplyResource` YAML parse failure | Validate YAML locally before confirmation display; trigger replan if invalid |
| Resource not found during execution | Show error, skip step |

---

## Testing Strategy

| Test Type | Coverage |
|---|---|
| Unit | `OperationStep` JSON marshal/unmarshal; `StdinConfirmationHandler` output formatting; YAML validation logic |
| Agent logic | Use `ChannelConfirmationHandler` + mock LLM responses to test: confirm path, correction loop, skip path, replan retry limit |
| Integration (real cluster) | All methods in `operations.go`; consistent with existing K8s integration test patterns |

---

## Out of Scope (Future Iterations)

- Operation audit log (who executed what and when)
- Pre-flight RBAC permission check (verify kubeconfig has required permissions before planning)
- Operation rollback
