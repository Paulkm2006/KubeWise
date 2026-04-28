# Security Agent Design

**Date:** 2026-04-28  
**Status:** Approved  

## Overview

Implement the `security` intent handler in KubeWise. When a user asks a security-related question, the Router classifies it as `security` and dispatches to a new `SecurityAgent`. The agent uses four Go-based scanner tools to audit RBAC, pod security, network policies, and image security. It currently returns a hardcoded "coming soon" stub.

## Architecture

### New Files

```
pkg/agent/security/agent.go
pkg/tools/v1/security/audit_rbac.go
pkg/tools/v1/security/audit_pod_security.go
pkg/tools/v1/security/audit_network_policies.go
pkg/tools/v1/security/audit_image_security.go
```

### Modified Files

```
pkg/agent/router/agent.go    — add SecurityAgent field, wire in case branch
```

### SecurityAgent

Structurally identical to `TroubleshootingAgent`:

- Struct fields: `k8sClient *k8s.Client`, `llmClient *llm.Client`, `toolRegistry *tool.Registry`
- Constructor `New(k8sClient, llmClient)` blank-imports `pkg/tools/v1/security` to trigger `init()` registration, then calls `tool.LoadGlobalRegistry(toolDep)`
- `HandleQuery(ctx, userQuery string, entities types.Entities) (string, error)` — 10-step tool-calling loop, same logic as other agents
- Tool errors are returned to the LLM as tool response messages (self-healing, no abort)

### Router Wiring

In `pkg/agent/router/agent.go`:
- Add `securityAgent *security.Agent` field to the router struct
- Instantiate it in `router.New()`
- Replace the `case types.TaskTypeSecurity` stub with a call to `a.securityAgent.HandleQuery(ctx, userQuery, entities)`

## Scanner Tools

All four tools implement `tool.Tool`, register via `init()`, and accept `namespace string` (optional — empty string means all namespaces). Each makes direct k8s API calls with no LLM involvement and returns a structured text findings report.

### `audit_rbac`

**Checks:**

| Finding | Severity |
| --- | --- |
| Cluster-admin binding to non-system subject | Critical |
| Role/ClusterRole with wildcard verb (`*`) or resource (`*`) | High |
| Role granting `exec` or `portforward` on pods | Medium |
| Service account with no role bindings (orphaned) | Low |

> **System subjects** (excluded from cluster-admin check): subjects whose `name` starts with `system:`

**k8s methods used:** `ListRoles`, `ListClusterRoles`, `ListRoleBindings`, `ListClusterRoleBindings`, `ListServiceAccounts` (all exist in `pkg/k8s/rbac.go`)

### `audit_pod_security`

**Checks:**

| Finding | Severity |
| --- | --- |
| Container with `privileged: true` | Critical |
| Pod using `hostNetwork`, `hostPID`, or `hostIPC` | High |
| Container with `allowPrivilegeEscalation: true` or field absent | High |
| Container running as root (`runAsUser: 0` or no `runAsNonRoot: true`) | High |
| Volume of type `hostPath` | Medium |

**k8s methods used:** `ListPods(ctx, namespace string)` — new method to add to `pkg/k8s/client.go`

### `audit_network_policies`

**Checks:**

| Finding | Severity |
| --- | --- |
| Namespace with no NetworkPolicy | High |
| Pod not selected by any NetworkPolicy | Medium |

**k8s methods used:** `ListNamespaces` (already exists on `k8s.Client`), `ListNetworkPolicies` (new), `ListPods` (new)

### `audit_image_security`

**Checks:**

| Finding | Severity |
| --- | --- |
| Container image using `latest` tag or no tag | Medium |
| Container with `imagePullPolicy: Never` | Low |
| Pod without `imagePullSecrets` in non-system namespace | Low |

> **System namespaces** (excluded from imagePullSecrets check): `kube-system`, `kube-public`, `kube-node-lease`

**k8s methods used:** `ListPods`

## k8s Client Extensions

The following methods need to be added to `pkg/k8s/client.go` (or a new file in `pkg/k8s/`):

- `ListPods(ctx, namespace string) ([]corev1.Pod, error)` — list pods across all namespaces or scoped
- `ListNetworkPolicies(ctx, namespace string) ([]networkingv1.NetworkPolicy, error)` — list network policies

`ListNamespaces` already exists on `k8s.Client` (used by the `list_namespaces` query tool). RBAC methods already exist in `pkg/k8s/rbac.go`.

## Hybrid Response Behavior

The SecurityAgent system prompt instructs the LLM:

- **Narrow query** (specific resource/namespace asked): call the relevant scanner scoped to the requested namespace/resource, return direct findings without severity framing
- **Broad audit query** (e.g. "audit my cluster security", "check all security issues"): call all four scanners, synthesize a consolidated report grouped by severity (Critical → High → Medium → Low), include a brief remediation hint per finding

The `namespace` entity extracted by the router is passed via `entities` and included in the user message context so the LLM can scope tool calls appropriately.

## Error Handling

Follows the existing pattern:
- Tool execution errors are returned to the LLM as a `role: "tool"` message with the error text
- The LLM may retry with corrected parameters
- After 10 steps with no terminal (non-tool-call) response, `HandleQuery` returns an error

## Out of Scope

- Sub-agent architecture (each scanner is a plain Go tool, no LLM involvement inside tools)
- CIS Benchmark / NSA compliance scoring
- Mutation/remediation actions (read-only audit only)
- Custom severity thresholds or rule configuration
