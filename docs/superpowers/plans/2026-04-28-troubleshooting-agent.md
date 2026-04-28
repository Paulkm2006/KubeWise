# Troubleshooting Agent Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement the `troubleshooting` intent path — an LLM agent that diagnoses Kubernetes failures across pods, custom resources, services, and storage, then outputs a structured Markdown report with root cause and remediation steps.

**Architecture:** A `TroubleshootingAgent` (mirroring `QueryAgent`) loads both query tools and four new troubleshooting-specific tools from the global registry. The agent loops up to 10 rounds letting the LLM call tools to gather evidence, then outputs a fixed-format Markdown report. The router is updated to route `TaskTypeTroubleshooting` intents to this agent instead of the stub.

**Tech Stack:** Go, `k8s.io/client-go` (Kubernetes API), `github.com/openai/openai-go/v3` (LLM), Cobra CLI.

---

## File Map

| Action | Path | Responsibility |
| ------ | ---- | -------------- |
| Modify | `pkg/k8s/client.go` | Add `tailLines` param to `GetPodLogs`; add `GetEvents`, `GetEndpoints`, `GetNodeList` |
| Create | `pkg/tools/v1/troubleshooting/get_pod_logs.go` | Tool: fetch pod container logs |
| Create | `pkg/tools/v1/troubleshooting/get_resource_events.go` | Tool: fetch K8s events for any resource by name |
| Create | `pkg/tools/v1/troubleshooting/get_node_status.go` | Tool: fetch all node conditions and allocatable resources |
| Create | `pkg/tools/v1/troubleshooting/get_service_endpoints.go` | Tool: fetch service endpoints, flag empty subsets |
| Create | `pkg/agent/troubleshooting/agent.go` | TroubleshootingAgent struct, New(), HandleQuery() |
| Modify | `pkg/agent/router/agent.go` | Wire troubleshootingAgent, replace stub |

---

## Task 1: Extend K8s Client

**Files:**
- Modify: `pkg/k8s/client.go`

Update the existing `GetPodLogs` to accept a `tailLines` parameter, and add three new methods: `GetEvents`, `GetEndpoints`, `GetNodeList`.

- [ ] **Step 1: Update `GetPodLogs` signature and add new methods**

Replace the existing `GetPodLogs` function (lines 98–110) and add the three new methods at the end of the file, before the `ptr` helper:

```go
// GetPodLogs 获取Pod日志，tailLines为0时使用默认100行
func (c *Client) GetPodLogs(ctx context.Context, namespace, podName, containerName string, tailLines int64) (string, error) {
	if tailLines <= 0 {
		tailLines = 100
	}
	req := c.clientset.CoreV1().Pods(namespace).GetLogs(podName, &corev1.PodLogOptions{
		Container: containerName,
		TailLines: ptr(tailLines),
	})
	logs, err := req.DoRaw(ctx)
	if err != nil {
		return "", err
	}
	return string(logs), nil
}

// GetEvents 获取指定资源相关的K8s事件
func (c *Client) GetEvents(ctx context.Context, namespace, involvedObjectName string) ([]corev1.Event, error) {
	fieldSelector := fmt.Sprintf("involvedObject.name=%s", involvedObjectName)
	eventList, err := c.clientset.CoreV1().Events(namespace).List(ctx, metav1.ListOptions{
		FieldSelector: fieldSelector,
	})
	if err != nil {
		return nil, err
	}
	return eventList.Items, nil
}

// GetEndpoints 获取Service对应的Endpoints
func (c *Client) GetEndpoints(ctx context.Context, namespace, serviceName string) (*corev1.Endpoints, error) {
	ep, err := c.clientset.CoreV1().Endpoints(namespace).Get(ctx, serviceName, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	return ep, nil
}

// GetNodeList 获取所有节点列表
func (c *Client) GetNodeList(ctx context.Context) ([]corev1.Node, error) {
	nodeList, err := c.clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return nodeList.Items, nil
}
```

- [ ] **Step 2: Verify it compiles**

```bash
make build
```

Expected: binary builds with no errors.

- [ ] **Step 3: Commit**

```bash
git add pkg/k8s/client.go
git commit -m "feat(k8s): add tailLines to GetPodLogs, add GetEvents/GetEndpoints/GetNodeList"
```

---

## Task 2: Tool — `get_pod_logs`

**Files:**
- Create: `pkg/tools/v1/troubleshooting/get_pod_logs.go`

- [ ] **Step 1: Create the file**

```go
package troubleshooting

import (
	"context"
	"fmt"

	"github.com/kubewise/kubewise/pkg/k8s"
	"github.com/kubewise/kubewise/pkg/tool"
)

type GetPodLogsTool struct {
	k8sClient *k8s.Client
}

func NewGetPodLogsTool(k8sClient *k8s.Client) *GetPodLogsTool {
	return &GetPodLogsTool{k8sClient: k8sClient}
}

func (t *GetPodLogsTool) Name() string { return "get_pod_logs" }

func (t *GetPodLogsTool) Description() string {
	return "获取Pod中指定容器的日志，用于分析崩溃原因、错误信息等"
}

func (t *GetPodLogsTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"namespace": map[string]any{
				"type":        "string",
				"description": "Pod所在的命名空间",
			},
			"pod_name": map[string]any{
				"type":        "string",
				"description": "Pod名称",
			},
			"container": map[string]any{
				"type":        "string",
				"description": "容器名称，可选，不指定则使用第一个容器",
			},
			"tail_lines": map[string]any{
				"type":        "integer",
				"description": "返回最后N行日志，默认100行",
			},
		},
		"required": []string{"namespace", "pod_name"},
	}
}

func (t *GetPodLogsTool) Execute(ctx context.Context, args map[string]any) (string, error) {
	namespace, ok := args["namespace"].(string)
	if !ok || namespace == "" {
		return "", fmt.Errorf("参数namespace不能为空")
	}
	podName, ok := args["pod_name"].(string)
	if !ok || podName == "" {
		return "", fmt.Errorf("参数pod_name不能为空")
	}

	container, _ := args["container"].(string)

	var tailLines int64
	switch v := args["tail_lines"].(type) {
	case float64:
		tailLines = int64(v)
	case int64:
		tailLines = v
	case int:
		tailLines = int64(v)
	}

	logs, err := t.k8sClient.GetPodLogs(ctx, namespace, podName, container, tailLines)
	if err != nil {
		return "", fmt.Errorf("获取Pod日志失败: %w", err)
	}

	return fmt.Sprintf("Pod %s/%s 的日志 (容器: %s):\n%s", namespace, podName, container, logs), nil
}

func init() {
	tool.RegisterGlobal(tool.ToolMetadata{
		Name:        "get_pod_logs",
		Description: "获取Pod中指定容器的日志，用于分析崩溃原因、错误信息等",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"namespace": map[string]any{
					"type":        "string",
					"description": "Pod所在的命名空间",
				},
				"pod_name": map[string]any{
					"type":        "string",
					"description": "Pod名称",
				},
				"container": map[string]any{
					"type":        "string",
					"description": "容器名称，可选，不指定则使用第一个容器",
				},
				"tail_lines": map[string]any{
					"type":        "integer",
					"description": "返回最后N行日志，默认100行",
				},
			},
			"required": []string{"namespace", "pod_name"},
		},
		Factory: func(dep any) (tool.Tool, error) {
			toolDep, ok := dep.(tool.ToolDependency)
			if !ok {
				return nil, fmt.Errorf("invalid dependency type")
			}
			return NewGetPodLogsTool(toolDep.K8sClient), nil
		},
	})
}
```

- [ ] **Step 2: Verify it compiles**

```bash
make build
```

Expected: binary builds with no errors.

- [ ] **Step 3: Commit**

```bash
git add pkg/tools/v1/troubleshooting/get_pod_logs.go
git commit -m "feat(tools): add get_pod_logs troubleshooting tool"
```

---

## Task 3: Tool — `get_resource_events`

**Files:**
- Create: `pkg/tools/v1/troubleshooting/get_resource_events.go`

- [ ] **Step 1: Create the file**

```go
package troubleshooting

import (
	"context"
	"fmt"
	"strings"

	"github.com/kubewise/kubewise/pkg/k8s"
	"github.com/kubewise/kubewise/pkg/tool"
)

type GetResourceEventsTool struct {
	k8sClient *k8s.Client
}

func NewGetResourceEventsTool(k8sClient *k8s.Client) *GetResourceEventsTool {
	return &GetResourceEventsTool{k8sClient: k8sClient}
}

func (t *GetResourceEventsTool) Name() string { return "get_resource_events" }

func (t *GetResourceEventsTool) Description() string {
	return "获取指定Kubernetes资源的事件列表，适用于Pod、PVC、IngressRoute等任意资源类型"
}

func (t *GetResourceEventsTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"namespace": map[string]any{
				"type":        "string",
				"description": "资源所在的命名空间",
			},
			"resource_name": map[string]any{
				"type":        "string",
				"description": "资源名称",
			},
		},
		"required": []string{"namespace", "resource_name"},
	}
}

func (t *GetResourceEventsTool) Execute(ctx context.Context, args map[string]any) (string, error) {
	namespace, ok := args["namespace"].(string)
	if !ok || namespace == "" {
		return "", fmt.Errorf("参数namespace不能为空")
	}
	resourceName, ok := args["resource_name"].(string)
	if !ok || resourceName == "" {
		return "", fmt.Errorf("参数resource_name不能为空")
	}

	events, err := t.k8sClient.GetEvents(ctx, namespace, resourceName)
	if err != nil {
		return "", fmt.Errorf("获取事件失败: %w", err)
	}

	if len(events) == 0 {
		return fmt.Sprintf("资源 %s/%s 没有相关事件", namespace, resourceName), nil
	}

	var result strings.Builder
	result.WriteString(fmt.Sprintf("资源 %s/%s 的事件列表:\n", namespace, resourceName))
	result.WriteString("时间\t类型\t原因\t消息\n")
	result.WriteString("----------------------------------------\n")

	for _, e := range events {
		t := e.LastTimestamp.Format("2006-01-02 15:04:05")
		if e.LastTimestamp.IsZero() {
			t = e.EventTime.Format("2006-01-02 15:04:05")
		}
		result.WriteString(fmt.Sprintf("%s\t%s\t%s\t%s\n", t, e.Type, e.Reason, e.Message))
	}

	result.WriteString(fmt.Sprintf("\n总计: %d条事件", len(events)))
	return result.String(), nil
}

func init() {
	tool.RegisterGlobal(tool.ToolMetadata{
		Name:        "get_resource_events",
		Description: "获取指定Kubernetes资源的事件列表，适用于Pod、PVC、IngressRoute等任意资源类型",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"namespace": map[string]any{
					"type":        "string",
					"description": "资源所在的命名空间",
				},
				"resource_name": map[string]any{
					"type":        "string",
					"description": "资源名称",
				},
			},
			"required": []string{"namespace", "resource_name"},
		},
		Factory: func(dep any) (tool.Tool, error) {
			toolDep, ok := dep.(tool.ToolDependency)
			if !ok {
				return nil, fmt.Errorf("invalid dependency type")
			}
			return NewGetResourceEventsTool(toolDep.K8sClient), nil
		},
	})
}
```

- [ ] **Step 2: Verify it compiles**

```bash
make build
```

Expected: binary builds with no errors.

- [ ] **Step 3: Commit**

```bash
git add pkg/tools/v1/troubleshooting/get_resource_events.go
git commit -m "feat(tools): add get_resource_events troubleshooting tool"
```

---

## Task 4: Tool — `get_node_status`

**Files:**
- Create: `pkg/tools/v1/troubleshooting/get_node_status.go`

- [ ] **Step 1: Create the file**

```go
package troubleshooting

import (
	"context"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"

	"github.com/kubewise/kubewise/pkg/k8s"
	"github.com/kubewise/kubewise/pkg/tool"
)

type GetNodeStatusTool struct {
	k8sClient *k8s.Client
}

func NewGetNodeStatusTool(k8sClient *k8s.Client) *GetNodeStatusTool {
	return &GetNodeStatusTool{k8sClient: k8sClient}
}

func (t *GetNodeStatusTool) Name() string { return "get_node_status" }

func (t *GetNodeStatusTool) Description() string {
	return "获取集群所有节点的状态、资源压力和可分配资源，用于排查Pod调度失败、节点资源不足等问题"
}

func (t *GetNodeStatusTool) Parameters() map[string]any {
	return map[string]any{
		"type":       "object",
		"properties": map[string]any{},
	}
}

func (t *GetNodeStatusTool) Execute(ctx context.Context, args map[string]any) (string, error) {
	nodes, err := t.k8sClient.GetNodeList(ctx)
	if err != nil {
		return "", fmt.Errorf("获取节点列表失败: %w", err)
	}

	var result strings.Builder
	result.WriteString("集群节点状态:\n")
	result.WriteString("节点名称\tReady\tMemoryPressure\tDiskPressure\tPIDPressure\t可分配CPU\t可分配内存\n")
	result.WriteString("----------------------------------------\n")

	for _, node := range nodes {
		conditions := map[corev1.NodeConditionType]string{}
		for _, c := range node.Status.Conditions {
			conditions[c.Type] = string(c.Status)
		}

		ready := conditions[corev1.NodeReady]
		memPressure := conditions[corev1.NodeMemoryPressure]
		diskPressure := conditions[corev1.NodeDiskPressure]
		pidPressure := conditions[corev1.NodePIDPressure]

		allocCPU := node.Status.Allocatable.Cpu().String()
		allocMem := node.Status.Allocatable.Memory().String()

		result.WriteString(fmt.Sprintf("%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			node.Name, ready, memPressure, diskPressure, pidPressure, allocCPU, allocMem))
	}

	result.WriteString(fmt.Sprintf("\n总计: %d个节点", len(nodes)))
	return result.String(), nil
}

func init() {
	tool.RegisterGlobal(tool.ToolMetadata{
		Name:        "get_node_status",
		Description: "获取集群所有节点的状态、资源压力和可分配资源，用于排查Pod调度失败、节点资源不足等问题",
		Parameters: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
		Factory: func(dep any) (tool.Tool, error) {
			toolDep, ok := dep.(tool.ToolDependency)
			if !ok {
				return nil, fmt.Errorf("invalid dependency type")
			}
			return NewGetNodeStatusTool(toolDep.K8sClient), nil
		},
	})
}
```

- [ ] **Step 2: Verify it compiles**

```bash
make build
```

Expected: binary builds with no errors.

- [ ] **Step 3: Commit**

```bash
git add pkg/tools/v1/troubleshooting/get_node_status.go
git commit -m "feat(tools): add get_node_status troubleshooting tool"
```

---

## Task 5: Tool — `get_service_endpoints`

**Files:**
- Create: `pkg/tools/v1/troubleshooting/get_service_endpoints.go`

- [ ] **Step 1: Create the file**

```go
package troubleshooting

import (
	"context"
	"fmt"
	"strings"

	"github.com/kubewise/kubewise/pkg/k8s"
	"github.com/kubewise/kubewise/pkg/tool"
)

type GetServiceEndpointsTool struct {
	k8sClient *k8s.Client
}

func NewGetServiceEndpointsTool(k8sClient *k8s.Client) *GetServiceEndpointsTool {
	return &GetServiceEndpointsTool{k8sClient: k8sClient}
}

func (t *GetServiceEndpointsTool) Name() string { return "get_service_endpoints" }

func (t *GetServiceEndpointsTool) Description() string {
	return "获取Service对应的Endpoints，检查是否有就绪的后端Pod，用于排查服务不可达、流量不通等问题"
}

func (t *GetServiceEndpointsTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"namespace": map[string]any{
				"type":        "string",
				"description": "Service所在的命名空间",
			},
			"service_name": map[string]any{
				"type":        "string",
				"description": "Service名称",
			},
		},
		"required": []string{"namespace", "service_name"},
	}
}

func (t *GetServiceEndpointsTool) Execute(ctx context.Context, args map[string]any) (string, error) {
	namespace, ok := args["namespace"].(string)
	if !ok || namespace == "" {
		return "", fmt.Errorf("参数namespace不能为空")
	}
	serviceName, ok := args["service_name"].(string)
	if !ok || serviceName == "" {
		return "", fmt.Errorf("参数service_name不能为空")
	}

	ep, err := t.k8sClient.GetEndpoints(ctx, namespace, serviceName)
	if err != nil {
		return "", fmt.Errorf("获取Endpoints失败: %w", err)
	}

	var result strings.Builder
	result.WriteString(fmt.Sprintf("Service %s/%s 的Endpoints:\n", namespace, serviceName))

	if len(ep.Subsets) == 0 {
		result.WriteString("【警告】Endpoints为空！Service没有任何后端Pod，这通常意味着：\n")
		result.WriteString("  1. 没有Pod与Service的selector匹配\n")
		result.WriteString("  2. 匹配的Pod都处于非Ready状态\n")
		result.WriteString("  3. selector标签配置错误\n")
		return result.String(), nil
	}

	totalReady := 0
	totalNotReady := 0

	for i, subset := range ep.Subsets {
		result.WriteString(fmt.Sprintf("\nSubset %d:\n", i+1))

		if len(subset.Addresses) > 0 {
			result.WriteString(fmt.Sprintf("  就绪地址 (%d):\n", len(subset.Addresses)))
			for _, addr := range subset.Addresses {
				nodeName := ""
				if addr.NodeName != nil {
					nodeName = *addr.NodeName
				}
				targetRef := ""
				if addr.TargetRef != nil {
					targetRef = fmt.Sprintf(" -> %s/%s", addr.TargetRef.Kind, addr.TargetRef.Name)
				}
				result.WriteString(fmt.Sprintf("    %s (节点: %s%s)\n", addr.IP, nodeName, targetRef))
			}
			totalReady += len(subset.Addresses)
		}

		if len(subset.NotReadyAddresses) > 0 {
			result.WriteString(fmt.Sprintf("  未就绪地址 (%d):\n", len(subset.NotReadyAddresses)))
			for _, addr := range subset.NotReadyAddresses {
				targetRef := ""
				if addr.TargetRef != nil {
					targetRef = fmt.Sprintf(" -> %s/%s", addr.TargetRef.Kind, addr.TargetRef.Name)
				}
				result.WriteString(fmt.Sprintf("    %s【未就绪%s】\n", addr.IP, targetRef))
			}
			totalNotReady += len(subset.NotReadyAddresses)
		}

		if len(subset.Ports) > 0 {
			ports := make([]string, 0, len(subset.Ports))
			for _, p := range subset.Ports {
				ports = append(ports, fmt.Sprintf("%s:%d/%s", p.Name, p.Port, p.Protocol))
			}
			result.WriteString(fmt.Sprintf("  端口: %s\n", strings.Join(ports, ", ")))
		}
	}

	result.WriteString(fmt.Sprintf("\n就绪后端: %d, 未就绪后端: %d", totalReady, totalNotReady))
	return result.String(), nil
}

func init() {
	tool.RegisterGlobal(tool.ToolMetadata{
		Name:        "get_service_endpoints",
		Description: "获取Service对应的Endpoints，检查是否有就绪的后端Pod，用于排查服务不可达、流量不通等问题",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"namespace": map[string]any{
					"type":        "string",
					"description": "Service所在的命名空间",
				},
				"service_name": map[string]any{
					"type":        "string",
					"description": "Service名称",
				},
			},
			"required": []string{"namespace", "service_name"},
		},
		Factory: func(dep any) (tool.Tool, error) {
			toolDep, ok := dep.(tool.ToolDependency)
			if !ok {
				return nil, fmt.Errorf("invalid dependency type")
			}
			return NewGetServiceEndpointsTool(toolDep.K8sClient), nil
		},
	})
}
```

- [ ] **Step 2: Verify it compiles**

```bash
make build
```

Expected: binary builds with no errors.

- [ ] **Step 3: Commit**

```bash
git add pkg/tools/v1/troubleshooting/get_service_endpoints.go
git commit -m "feat(tools): add get_service_endpoints troubleshooting tool"
```

---

## Task 6: TroubleshootingAgent

**Files:**
- Create: `pkg/agent/troubleshooting/agent.go`

- [ ] **Step 1: Create the agent file**

```go
package troubleshooting

import (
	"context"
	"fmt"
	"strings"

	"github.com/kubewise/kubewise/pkg/k8s"
	"github.com/kubewise/kubewise/pkg/llm"
	"github.com/kubewise/kubewise/pkg/tool"
	"github.com/kubewise/kubewise/pkg/types"

	// 加载查询工具和故障排查工具，触发init函数注册
	_ "github.com/kubewise/kubewise/pkg/tools/v1/query"
	_ "github.com/kubewise/kubewise/pkg/tools/v1/troubleshooting"
)

type Agent struct {
	k8sClient    *k8s.Client
	llmClient    *llm.Client
	toolRegistry *tool.Registry
}

func New(k8sClient *k8s.Client, llmClient *llm.Client) (*Agent, error) {
	toolDep := tool.ToolDependency{
		K8sClient: k8sClient,
	}
	registry, err := tool.LoadGlobalRegistry(toolDep)
	if err != nil {
		return nil, fmt.Errorf("加载工具注册中心失败: %w", err)
	}
	return &Agent{
		k8sClient:    k8sClient,
		llmClient:    llmClient,
		toolRegistry: registry,
	}, nil
}

func (a *Agent) buildSystemPrompt() string {
	return `你是Kubernetes智能故障排查助手。当用户描述集群异常时，你需要系统性地收集信息并诊断根因。

## 信息收集顺序
1. 先获取目标资源的状态（使用 get_resource_by_gvr_and_name 或 list_resources_by_gvr）
2. 获取该资源的事件（使用 get_resource_events）
3. 如果是Pod问题，获取日志（使用 get_pod_logs）
4. 如果涉及调度失败，检查节点状态（使用 get_node_status）
5. 如果涉及Service不可达，检查Endpoints（使用 get_service_endpoints）

## 常见资源GVR参照表
- Pod: group="", version="v1", resource="pods"
- Service: group="", version="v1", resource="services"
- PersistentVolumeClaim: group="", version="v1", resource="persistentvolumeclaims"
- PersistentVolume: group="", version="v1", resource="persistentvolumes"
- Deployment: group="apps", version="v1", resource="deployments"
- StatefulSet: group="apps", version="v1", resource="statefulsets"
- Node: group="", version="v1", resource="nodes"
- IngressRoute (Traefik): group="traefik.io", version="v1alpha1", resource="ingressroutes"

对于不确定的CRD，可以先用 list_resources_by_gvr 尝试，或向用户确认GVR信息。

## 输出格式
收集到足够信息后，必须输出以下固定Markdown格式的报告，不要调用更多工具：

## 故障摘要
（一段话描述故障现象和受影响的资源）

## 根因分析
（结合工具返回的具体数据，解释故障原因，引用关键错误信息）

## 修复建议
1. （具体操作步骤，优先给出kubectl命令或配置修改方案）
2. ...`
}

func (a *Agent) HandleQuery(ctx context.Context, userQuery string, entities types.Entities) (string, error) {
	functions := a.toolRegistry.GetAllFunctionDefinitions()
	messages := []llm.Message{
		{Role: "system", Content: a.buildSystemPrompt()},
		{Role: "user", Content: userQuery},
	}

	maxSteps := 10
	for step := range maxSteps {
		resp, err := a.llmClient.ChatCompletion(ctx, messages, functions)
		if err != nil {
			return "", fmt.Errorf("LLM调用失败: %w", err)
		}

		if len(resp.ToolCalls) == 0 {
			return resp.Content, nil
		}

		funcCall := &resp.ToolCalls[0].Function

		fmt.Printf("第%d步：调用工具 %s\n", step+1, funcCall.Name)
		if len(funcCall.Arguments) > 0 {
			args := make([]string, 0, len(funcCall.Arguments))
			for k, v := range funcCall.Arguments {
				args = append(args, fmt.Sprintf("%s=%v", k, v))
			}
			fmt.Printf("参数：%s\n", strings.Join(args, ", "))
		}

		t, exists := a.toolRegistry.GetTool(funcCall.Name)
		if !exists {
			return "", fmt.Errorf("未知工具: %s", funcCall.Name)
		}
		result, err := t.Execute(ctx, funcCall.Arguments)
		if err != nil {
			fmt.Printf("工具调用失败：%v\n", err)
			result = fmt.Sprintf("工具调用失败：%v\n请修正参数后重新调用工具。", err)
		} else {
			fmt.Printf("工具返回结果长度：%d 字节\n", len(result))
		}

		messages = append(messages, *resp)
		toolMsg := llm.Message{
			Role:    "tool",
			Content: fmt.Sprintf("工具返回结果：\n%s", result),
		}
		if len(resp.ToolCalls) > 0 {
			toolMsg.ToolCallID = resp.ToolCalls[0].ID
		}
		messages = append(messages, toolMsg)
	}

	return "", fmt.Errorf("超过最大调用轮次，无法完成故障排查")
}
```

- [ ] **Step 2: Verify it compiles**

```bash
make build
```

Expected: binary builds with no errors.

- [ ] **Step 3: Commit**

```bash
git add pkg/agent/troubleshooting/agent.go
git commit -m "feat(agent): add TroubleshootingAgent"
```

---

## Task 7: Wire Troubleshooting Agent into Router

**Files:**
- Modify: `pkg/agent/router/agent.go`

The router currently returns a stub string for `TaskTypeTroubleshooting`. Replace it with the real agent.

- [ ] **Step 1: Update `pkg/agent/router/agent.go`**

Replace the entire file content:

```go
package router

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/kubewise/kubewise/pkg/agent/query"
	"github.com/kubewise/kubewise/pkg/agent/troubleshooting"
	"github.com/kubewise/kubewise/pkg/k8s"
	"github.com/kubewise/kubewise/pkg/llm"
	"github.com/kubewise/kubewise/pkg/types"
)

type Agent struct {
	k8sClient            *k8s.Client
	llmClient            *llm.Client
	queryAgent           *query.Agent
	troubleshootingAgent *troubleshooting.Agent
}

func New(k8sClient *k8s.Client, llmClient *llm.Client) (*Agent, error) {
	queryAgent, err := query.New(k8sClient, llmClient)
	if err != nil {
		return nil, fmt.Errorf("初始化查询Agent失败: %w", err)
	}

	troubleshootingAgent, err := troubleshooting.New(k8sClient, llmClient)
	if err != nil {
		return nil, fmt.Errorf("初始化故障排查Agent失败: %w", err)
	}

	return &Agent{
		k8sClient:            k8sClient,
		llmClient:            llmClient,
		queryAgent:           queryAgent,
		troubleshootingAgent: troubleshootingAgent,
	}, nil
}

func (a *Agent) HandleQuery(userQuery string) (string, error) {
	ctx := context.Background()

	intent, err := a.classifyIntent(ctx, userQuery)
	if err != nil {
		return "", fmt.Errorf("意图分类失败: %w", err)
	}

	fmt.Printf("识别到任务类型：%s，置信度：%.2f\n", intent.TaskTypeDescription, intent.Confidence)
	if intent.Entities.Namespace != "" {
		fmt.Printf("目标命名空间：%s\n", intent.Entities.Namespace)
	}
	if intent.Entities.ResourceName != "" && len(intent.Entities.ResourceType) > 0 {
		fmt.Printf("目标资源：%s/%s\n", strings.Join([]string(intent.Entities.ResourceType), ","), intent.Entities.ResourceName)
	}

	switch intent.TaskType {
	case types.TaskTypeQuery:
		return a.queryAgent.HandleQuery(ctx, userQuery, intent.Entities)
	case types.TaskTypeOperation:
		return "操作类功能正在开发中，敬请期待", nil
	case types.TaskTypeTroubleshooting:
		return a.troubleshootingAgent.HandleQuery(ctx, userQuery, intent.Entities)
	case types.TaskTypeSecurity:
		return "安全审计功能正在开发中，敬请期待", nil
	default:
		return "", fmt.Errorf("不支持的任务类型: %s", intent.TaskType)
	}
}

func (a *Agent) classifyIntent(ctx context.Context, userQuery string) (*types.IntentClassification, error) {
	systemPrompt := `你是Kubernetes智能运维系统的路由分析器，负责将用户的自然语言查询分类到以下四种任务类型之一：
1. operation（操作类）：用户需要执行创建、修改、删除、部署等主动操作
2. query（查询类）：用户需要查询集群的状态、信息、统计等
3. troubleshooting（故障排查类）：用户需要排查异常、错误、崩溃等问题
4. security（安全审计类）：用户需要进行安全检查、权限审计、合规扫描等

请分析用户查询，返回JSON格式的结果，包含：
- task_type: 任务类型枚举值（operation/query/troubleshooting/security）
- task_type_description: 任务类型中文描述
- entities: 提取的关键实体，包含：
  - namespace: 提到的命名空间（如果有）
  - resource_name: 提到的资源名称（如果有）
  - resource_type: 资源类型（Pod/Deployment/Service/PV/PVC等，如果有）
  - app_name: 应用名称（如果有）
  - operation: 操作类型（如果有）
- confidence: 分类置信度（0-1之间的浮点数）

注意：只返回JSON，不要有其他解释性文字。`

	messages := []llm.Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userQuery},
	}

	resp, err := a.llmClient.ChatCompletion(ctx, messages, nil)
	if err != nil {
		return nil, err
	}

	var intent types.IntentClassification
	content := strings.TrimSpace(resp.Content)
	content = strings.TrimPrefix(content, "```json")
	content = strings.TrimSuffix(content, "```")
	content = strings.TrimSpace(content)

	if err := json.Unmarshal([]byte(content), &intent); err != nil {
		return nil, fmt.Errorf("解析意图结果失败: %w，原始内容: %s", err, content)
	}

	return &intent, nil
}
```

- [ ] **Step 2: Verify it compiles**

```bash
make build
```

Expected: binary builds with no errors.

- [ ] **Step 3: Commit**

```bash
git add pkg/agent/router/agent.go
git commit -m "feat(router): wire TroubleshootingAgent, replace stub"
```

---

## Task 8: End-to-End Smoke Test

Verify the full flow works with a live cluster (or skip if no cluster is available).

- [ ] **Step 1: Build the binary**

```bash
make build
```

Expected: `./kubewise` binary created.

- [ ] **Step 2: Run a troubleshooting query**

```bash
./kubewise chat "为什么 default 命名空间下的 <your-pod-name> pod 一直重启" \
  --api-key <your-key> \
  --api-base <your-base-url> \
  --model <your-model>
```

Expected output:
```
使用配置文件: ...
处理中...
识别到任务类型：故障排查类，置信度：0.9x
第1步：调用工具 get_resource_by_gvr_and_name
...
结果：

## 故障摘要
...
## 根因分析
...
## 修复建议
...
```

- [ ] **Step 3: Verify router still handles query intent correctly**

```bash
./kubewise chat "列出所有命名空间" --api-key <your-key> ...
```

Expected: router classifies as `query`, returns namespace list (not a troubleshooting report).
