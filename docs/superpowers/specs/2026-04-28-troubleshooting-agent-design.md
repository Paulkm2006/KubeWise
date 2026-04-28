# Troubleshooting Agent Design

**Date:** 2026-04-28  
**Status:** Approved

## Overview

Implement the `troubleshooting` intent path in KubeWise. The agent diagnoses Kubernetes failures across pods, services, custom resources (e.g., IngressRoute), and storage (PVC/PV), then outputs a structured Markdown report with root cause and remediation steps.

## Architecture

The TroubleshootingAgent follows the same pattern as the existing QueryAgent: a single agent with a tool registry, calling the LLM in a loop (max 10 rounds) until the LLM produces a final answer without tool calls.

```text
Router Agent
  └── TroubleshootingAgent  (pkg/agent/troubleshooting/agent.go)
        ├── [Query Tools]          loaded from pkg/tools/v1/query
        │     ├── list_resources_by_gvr
        │     ├── get_resource_by_gvr_and_name
        │     └── ... (all existing query tools)
        └── [Troubleshooting Tools]  loaded from pkg/tools/v1/troubleshooting
              ├── get_pod_logs
              ├── get_resource_events
              ├── get_node_status
              └── get_service_endpoints
```

Loading both tool sets gives the LLM the ability to fetch any Kubernetes resource (including CRDs) by GVR, and additionally retrieve logs, events, node pressure, and service endpoints — covering all three failure categories.

## Components

### 1. New K8s Client Methods (`pkg/k8s/client.go`)

| Method | Signature | Notes |
| ------ | --------- | ----- |
| `GetPodLogs` | `(ctx, namespace, podName, container string, tailLines int64) (string, error)` | Uses `CoreV1().Pods().GetLogs()`. If container is empty, uses first container. |
| `GetEvents` | `(ctx, namespace, involvedObjectName string) ([]corev1.Event, error)` | Filters by `involvedObject.name` field selector. Returns all event types (Normal + Warning). |
| `GetEndpoints` | `(ctx, namespace, serviceName string) (*corev1.Endpoints, error)` | Returns Endpoints object so tool can check if subsets are empty. |
| `GetNodeList` | `(ctx) ([]corev1.Node, error)` | Returns all nodes with their Conditions. |

### 2. Troubleshooting Tools (`pkg/tools/v1/troubleshooting/`)

Each tool is a separate file, registered via `init()`.

**`get_pod_logs.go`**

- Parameters: `namespace` (string), `pod_name` (string), `container` (string, optional), `tail_lines` (integer, optional, default 100)
- Calls `k8sClient.GetPodLogs()`
- Returns raw log text prefixed with metadata line

**`get_resource_events.go`**

- Parameters: `namespace` (string), `resource_name` (string)
- Calls `k8sClient.GetEvents()`
- Returns formatted table: `Time | Type | Reason | Message`
- Works for any resource (Pod, PVC, IngressRoute, etc.) since K8s events are indexed by `involvedObject.name`

**`get_node_status.go`**

- Parameters: none
- Calls `k8sClient.GetNodeList()`
- Returns table of nodes with: `Name | Ready | MemoryPressure | DiskPressure | PIDPressure | Allocatable CPU | Allocatable Memory`

**`get_service_endpoints.go`**

- Parameters: `namespace` (string), `service_name` (string)
- Calls `k8sClient.GetEndpoints()`
- Returns whether Endpoints has ready addresses, and lists them; flags empty endpoints explicitly

### 3. TroubleshootingAgent (`pkg/agent/troubleshooting/agent.go`)

```go
type Agent struct {
    k8sClient    *k8s.Client
    llmClient    *llm.Client
    toolRegistry *tool.Registry
}
```

`New()` loads the global registry (which includes both query and troubleshooting tools via their `init()` imports) using the same `tool.LoadGlobalRegistry()` call as QueryAgent.

**System Prompt** instructs the LLM to:

1. Identify the resource type and name from the user query
2. Gather information in order: resource spec/status → events → logs (if pod) → related resources
3. For unknown resource types, use `list_resources_by_gvr` / `get_resource_by_gvr_and_name` with appropriate GVR
4. Output a Markdown report with exactly these sections:
   - `## 故障摘要` — one-paragraph summary of what is wrong
   - `## 根因分析` — detailed root cause with evidence from tool results
   - `## 修复建议` — numbered list of concrete remediation steps (kubectl commands or config changes)

`HandleQuery()` is identical in structure to `query.Agent.HandleQuery()`: message loop, tool dispatch, error returned to LLM for self-correction.

### 4. Router Integration (`pkg/agent/router/agent.go`)

- Add `troubleshootingAgent *troubleshooting.Agent` field to `Agent` struct
- Initialize it in `New()` alongside `queryAgent`
- Replace the stub in the `TaskTypeTroubleshooting` switch case with `a.troubleshootingAgent.HandleQuery(ctx, userQuery, intent.Entities)`

### 5. Tool Package Imports (`pkg/agent/troubleshooting/agent.go`)

Import both tool packages at the top of the troubleshooting agent file to trigger their `init()` registrations:

```go
import (
    _ "github.com/kubewise/kubewise/pkg/tools/v1/query"
    _ "github.com/kubewise/kubewise/pkg/tools/v1/troubleshooting"
)
```

This ensures `tool.LoadGlobalRegistry()` sees all tools — same pattern as the query agent which imports `_ ".../query"`. The global registry is shared, so both tool sets are available when the troubleshooting agent instantiates its registry.

## Failure Scenarios Covered

| Scenario | Tools Used |
| -------- | ---------- |
| Pod CrashLoopBackOff | `get_resource_by_gvr_and_name` (pod status) + `get_pod_logs` + `get_resource_events` |
| Pod OOMKilled | `get_resource_by_gvr_and_name` (pod status/limits) + `get_pod_logs` |
| Pod Pending / scheduling failure | `get_resource_events` + `get_node_status` |
| Service not reachable | `get_resource_by_gvr_and_name` (service) + `get_service_endpoints` + `get_resource_events` |
| IngressRoute not working | `get_resource_by_gvr_and_name` (IngressRoute via GVR) + `get_resource_events` + `get_resource_by_gvr_and_name` (backend service) + `get_service_endpoints` |
| PVC mount failure | `get_resource_by_gvr_and_name` (PVC status) + `get_resource_by_gvr_and_name` (PV) + `get_resource_events` (PVC + Pod) |
| Node resource pressure | `get_node_status` + `list_resources_by_gvr` (pods on node) |

## Data Flow

```text
User: "为什么 my-app pod 一直 CrashLoopBackOff"
  │
  ▼
Router classifies → TaskTypeTroubleshooting
  │
  ▼
TroubleshootingAgent.HandleQuery()
  │
  ├─ LLM Round 1: calls get_resource_by_gvr_and_name(pods, my-app)
  │    → pod status shows restartCount=15, last state OOMKilled
  │
  ├─ LLM Round 2: calls get_pod_logs(my-app, tail=200)
  │    → logs show "java.lang.OutOfMemoryError"
  │
  ├─ LLM Round 3: calls get_resource_events(namespace, my-app)
  │    → events show "OOMKilling" warnings
  │
  └─ LLM Final: no tool call → returns Markdown report
       ## 故障摘要 ...
       ## 根因分析 ...
       ## 修复建议 ...
```

## Out of Scope

- Automatic remediation (applying fixes) — that is the `operation` agent
- Metrics/Prometheus integration — only K8s API data used
- Multi-cluster support — single kubeconfig only
