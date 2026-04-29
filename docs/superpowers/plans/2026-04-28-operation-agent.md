# Operation Agent Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement the `operation` task type — a two-phase plan-then-execute agent that handles K8s cluster mutations with per-step user confirmation.

**Architecture:** Planning phase runs a ReAct loop with read-only tools to produce a `[]OperationStep`; execution phase iterates steps, presenting each for user confirmation via an injectable `ConfirmationHandler` interface before calling the write tool. Default handler reads stdin; `ChannelConfirmationHandler` enables future TUI/API integration.

**Tech Stack:** Go 1.26, k8s.io/client-go v0.35.4, k8s.io/apimachinery v0.35.4, sigs.k8s.io/yaml v1.6.0, github.com/kubewise/kubewise internal packages.

---

## File Map

| Action | Path | Responsibility |
|---|---|---|
| Modify | `pkg/tool/interface.go` | Add `Category string` to `ToolMetadata` |
| Modify | `pkg/tool/registry.go` | Add `LoadGlobalRegistryByCategory` |
| Create | `pkg/tool/registry_test.go` | Test category-filtered loading |
| Create | `pkg/k8s/operations.go` | All K8s write methods |
| Create | `pkg/tools/v1/operation/scale.go` | `scale_resource` tool |
| Create | `pkg/tools/v1/operation/restart.go` | `restart_resource` tool |
| Create | `pkg/tools/v1/operation/delete.go` | `delete_resource` tool |
| Create | `pkg/tools/v1/operation/apply.go` | `apply_resource` tool |
| Create | `pkg/tools/v1/operation/cordon_drain.go` | `cordon_drain_node` tool |
| Create | `pkg/tools/v1/operation/label_annotate.go` | `label_annotate_resource` tool |
| Create | `pkg/agent/operation/types.go` | `OperationStep`, `stepToToolCall` |
| Create | `pkg/agent/operation/confirm.go` | `ConfirmationHandler` interface + implementations |
| Create | `pkg/agent/operation/agent.go` | `OperationAgent` (plan + execute) |
| Create | `pkg/agent/operation/agent_test.go` | Execution loop unit tests |
| Modify | `pkg/agent/router/agent.go` | Wire `OperationAgent` into router |

---

## Task 1: Add `Category` to `ToolMetadata` and add `LoadGlobalRegistryByCategory`

**Files:**
- Modify: `pkg/tool/interface.go`
- Modify: `pkg/tool/registry.go`
- Create: `pkg/tool/registry_test.go`

- [ ] **Step 1: Write the failing test**

Create `pkg/tool/registry_test.go`:

```go
package tool

import (
	"context"
	"testing"
)

func TestLoadGlobalRegistryByCategory(t *testing.T) {
	// Save and restore global registry to avoid polluting other tests.
	saved := globalRegistryEntries
	defer func() { globalRegistryEntries = saved }()

	noop := func(dep any) (Tool, error) {
		return &noopTool{}, nil
	}

	globalRegistryEntries = []ToolMetadata{
		{Name: "read_tool", Category: "", Factory: noop},
		{Name: "op_tool", Category: "operation", Factory: noop},
	}

	t.Run("loads only operation tools", func(t *testing.T) {
		reg, err := LoadGlobalRegistryByCategory(ToolDependency{}, "operation")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if _, ok := reg.GetTool("op_tool"); !ok {
			t.Error("expected op_tool to be present")
		}
		if _, ok := reg.GetTool("read_tool"); ok {
			t.Error("expected read_tool to be absent")
		}
	})

	t.Run("loads only read tools (empty category)", func(t *testing.T) {
		reg, err := LoadGlobalRegistryByCategory(ToolDependency{}, "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if _, ok := reg.GetTool("read_tool"); !ok {
			t.Error("expected read_tool to be present")
		}
		if _, ok := reg.GetTool("op_tool"); ok {
			t.Error("expected op_tool to be absent")
		}
	})
}

// noopTool satisfies the Tool interface for testing.
type noopTool struct{}

func (n *noopTool) Name() string                                                   { return "noop" }
func (n *noopTool) Description() string                                            { return "" }
func (n *noopTool) Parameters() map[string]any                                     { return nil }
func (n *noopTool) Execute(_ context.Context, _ map[string]any) (string, error)    { return "", nil }
```

- [ ] **Step 2: Run test to verify it fails**

```
cd d:/KubeWise && go test ./pkg/tool/... -run TestLoadGlobalRegistryByCategory -v
```

Expected: FAIL — `LoadGlobalRegistryByCategory undefined`

- [ ] **Step 3: Add `Category` field to `ToolMetadata` in `pkg/tool/interface.go`**

Replace the `ToolMetadata` struct (lines 26-31):

```go
// ToolMetadata 工具元数据，用于注册和发现
type ToolMetadata struct {
	Name        string
	Description string
	Parameters  map[string]any
	Category    string            // "operation" for write tools; "" for read/query tools
	Factory     func(dep any) (Tool, error)
}
```

- [ ] **Step 4: Add `LoadGlobalRegistryByCategory` to `pkg/tool/registry.go`**

Append after `LoadGlobalRegistry` (after line 110):

```go
// LoadGlobalRegistryByCategory loads only tools whose Category matches the given value.
// Use "" to load all tools without an explicit category (backward-compatible read tools).
// Use "operation" to load only write/operation tools.
func LoadGlobalRegistryByCategory(dep ToolDependency, category string) (*Registry, error) {
	reg := NewRegistry(dep)
	for _, meta := range globalRegistryEntries {
		if meta.Category != category {
			continue
		}
		if err := reg.Register(meta); err != nil {
			return nil, err
		}
	}
	if err := reg.LoadAll(); err != nil {
		return nil, err
	}
	return reg, nil
}
```

- [ ] **Step 5: Run test to verify it passes**

```
cd d:/KubeWise && go test ./pkg/tool/... -run TestLoadGlobalRegistryByCategory -v
```

Expected: PASS

- [ ] **Step 6: Verify existing code still compiles**

```
cd d:/KubeWise && go build ./...
```

Expected: no errors (existing tools have `Category: ""` by default — zero value is correct)

- [ ] **Step 7: Commit**

```bash
git add pkg/tool/interface.go pkg/tool/registry.go pkg/tool/registry_test.go
git commit -m "feat(tool): add Category field to ToolMetadata and LoadGlobalRegistryByCategory"
```

---

## Task 2: K8s Write Methods (`pkg/k8s/operations.go`)

**Files:**
- Create: `pkg/k8s/operations.go`

These methods require a live cluster to test fully. Each method is verified to compile; integration testing is manual.

- [ ] **Step 1: Create `pkg/k8s/operations.go`**

```go
package k8s

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	k8stypes "k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/yaml"
)

// ScaleResource sets the replica count for a Deployment or StatefulSet.
func (c *Client) ScaleResource(ctx context.Context, namespace, kind, name string, replicas int32) error {
	switch kind {
	case "Deployment":
		deploy, err := c.clientset.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return fmt.Errorf("获取 Deployment %s/%s 失败: %w", namespace, name, err)
		}
		deploy.Spec.Replicas = &replicas
		_, err = c.clientset.AppsV1().Deployments(namespace).Update(ctx, deploy, metav1.UpdateOptions{})
		return err
	case "StatefulSet":
		sts, err := c.clientset.AppsV1().StatefulSets(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return fmt.Errorf("获取 StatefulSet %s/%s 失败: %w", namespace, name, err)
		}
		sts.Spec.Replicas = &replicas
		_, err = c.clientset.AppsV1().StatefulSets(namespace).Update(ctx, sts, metav1.UpdateOptions{})
		return err
	default:
		return fmt.Errorf("ScaleResource 不支持资源类型: %s（仅支持 Deployment, StatefulSet）", kind)
	}
}

// RestartResource triggers a rolling restart by patching the restartedAt annotation.
func (c *Client) RestartResource(ctx context.Context, namespace, kind, name string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	patch := fmt.Sprintf(
		`{"spec":{"template":{"metadata":{"annotations":{"kubectl.kubernetes.io/restartedAt":%q}}}}}`,
		now,
	)
	data := []byte(patch)

	switch kind {
	case "Deployment":
		_, err := c.clientset.AppsV1().Deployments(namespace).Patch(
			ctx, name, k8stypes.MergePatchType, data, metav1.PatchOptions{},
		)
		return err
	case "StatefulSet":
		_, err := c.clientset.AppsV1().StatefulSets(namespace).Patch(
			ctx, name, k8stypes.MergePatchType, data, metav1.PatchOptions{},
		)
		return err
	case "DaemonSet":
		_, err := c.clientset.AppsV1().DaemonSets(namespace).Patch(
			ctx, name, k8stypes.MergePatchType, data, metav1.PatchOptions{},
		)
		return err
	default:
		return fmt.Errorf("RestartResource 不支持资源类型: %s（仅支持 Deployment, StatefulSet, DaemonSet）", kind)
	}
}

// DeleteResource deletes any K8s resource via the dynamic client.
func (c *Client) DeleteResource(ctx context.Context, namespace string, gvr schema.GroupVersionResource, name string) error {
	if namespace == "" {
		return c.dynamicClient.Resource(gvr).Delete(ctx, name, metav1.DeleteOptions{})
	}
	return c.dynamicClient.Resource(gvr).Namespace(namespace).Delete(ctx, name, metav1.DeleteOptions{})
}

// ApplyResource parses a YAML manifest and applies it via Server-Side Apply.
// GVR is derived from the object's apiVersion + kind using a best-effort heuristic
// (lowercase Kind + "s"). Works correctly for all standard K8s resources.
func (c *Client) ApplyResource(ctx context.Context, yamlContent string) error {
	jsonBytes, err := yaml.YAMLToJSON([]byte(yamlContent))
	if err != nil {
		return fmt.Errorf("YAML 解析失败: %w", err)
	}

	obj := &unstructured.Unstructured{}
	if err := json.Unmarshal(jsonBytes, obj); err != nil {
		return fmt.Errorf("JSON 反序列化失败: %w", err)
	}

	gvr := gvrFromUnstructured(obj)
	namespace := obj.GetNamespace()
	name := obj.GetName()
	force := true

	if namespace == "" {
		_, err = c.dynamicClient.Resource(gvr).Patch(
			ctx, name, k8stypes.ApplyPatchType, jsonBytes,
			metav1.PatchOptions{FieldManager: "kubewise", Force: &force},
		)
	} else {
		_, err = c.dynamicClient.Resource(gvr).Namespace(namespace).Patch(
			ctx, name, k8stypes.ApplyPatchType, jsonBytes,
			metav1.PatchOptions{FieldManager: "kubewise", Force: &force},
		)
	}
	return err
}

// gvrFromUnstructured derives a best-effort GVR from an Unstructured object's
// apiVersion and kind. Covers all standard K8s resources (Kind + "s" pluralisation).
func gvrFromUnstructured(obj *unstructured.Unstructured) schema.GroupVersionResource {
	gvk := obj.GroupVersionKind()
	resource := strings.ToLower(gvk.Kind) + "s"
	return schema.GroupVersionResource{
		Group:    gvk.Group,
		Version:  gvk.Version,
		Resource: resource,
	}
}

// CordonNode sets or clears the node's Unschedulable flag.
func (c *Client) CordonNode(ctx context.Context, nodeName string, cordon bool) error {
	patch := fmt.Sprintf(`{"spec":{"unschedulable":%v}}`, cordon)
	_, err := c.clientset.CoreV1().Nodes().Patch(
		ctx, nodeName, k8stypes.MergePatchType, []byte(patch), metav1.PatchOptions{},
	)
	return err
}

// DrainNode evicts all non-DaemonSet, non-mirror Pods from the node.
// The node should be cordoned first. Respects ctx for timeout.
func (c *Client) DrainNode(ctx context.Context, nodeName string) (evicted []string, remaining []string, err error) {
	pods, err := c.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{
		FieldSelector: "spec.nodeName=" + nodeName,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("列出节点 %s 上的 Pod 失败: %w", nodeName, err)
	}

	for _, pod := range pods.Items {
		if isDaemonSetPod(&pod) || isMirrorPod(&pod) {
			continue
		}
		eviction := &policyv1.Eviction{
			ObjectMeta: metav1.ObjectMeta{
				Name:      pod.Name,
				Namespace: pod.Namespace,
			},
		}
		if evictErr := c.clientset.PolicyV1().Evictions(pod.Namespace).Evict(ctx, eviction); evictErr != nil {
			remaining = append(remaining, pod.Namespace+"/"+pod.Name)
		} else {
			evicted = append(evicted, pod.Namespace+"/"+pod.Name)
		}
	}
	return evicted, remaining, nil
}

// LabelResource applies a strategic merge patch on labels and annotations.
func (c *Client) LabelResource(
	ctx context.Context,
	namespace string,
	gvr schema.GroupVersionResource,
	name string,
	labels map[string]string,
	annotations map[string]string,
) error {
	patch := map[string]any{
		"metadata": map[string]any{
			"labels":      labels,
			"annotations": annotations,
		},
	}
	data, err := json.Marshal(patch)
	if err != nil {
		return err
	}

	if namespace == "" {
		_, err = c.dynamicClient.Resource(gvr).Patch(
			ctx, name, k8stypes.MergePatchType, data, metav1.PatchOptions{},
		)
	} else {
		_, err = c.dynamicClient.Resource(gvr).Namespace(namespace).Patch(
			ctx, name, k8stypes.MergePatchType, data, metav1.PatchOptions{},
		)
	}
	return err
}

func isDaemonSetPod(pod *corev1.Pod) bool {
	for _, ref := range pod.OwnerReferences {
		if ref.Kind == "DaemonSet" {
			return true
		}
	}
	return false
}

func isMirrorPod(pod *corev1.Pod) bool {
	_, ok := pod.Annotations["kubernetes.io/config.mirror"]
	return ok
}

```

- [ ] **Step 2: Verify compilation**

```
cd d:/KubeWise && go build ./pkg/k8s/...
```

Expected: no errors. Fix any import issues reported.

- [ ] **Step 3: Commit**

```bash
git add pkg/k8s/operations.go
git commit -m "feat(k8s): add write methods for scale, restart, delete, apply, cordon/drain, label"
```

---

## Task 3: `scale_resource` Tool

**Files:**
- Create: `pkg/tools/v1/operation/scale.go`

- [ ] **Step 1: Create `pkg/tools/v1/operation/scale.go`**

```go
package operation

import (
	"context"
	"fmt"

	"github.com/kubewise/kubewise/pkg/k8s"
	"github.com/kubewise/kubewise/pkg/tool"
)

type ScaleResourceTool struct {
	k8sClient *k8s.Client
}

func NewScaleResourceTool(k8sClient *k8s.Client) *ScaleResourceTool {
	return &ScaleResourceTool{k8sClient: k8sClient}
}

func (t *ScaleResourceTool) Name() string { return "scale_resource" }

func (t *ScaleResourceTool) Description() string {
	return "调整 Deployment 或 StatefulSet 的副本数"
}

func (t *ScaleResourceTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"namespace": map[string]any{"type": "string", "description": "命名空间"},
			"kind":      map[string]any{"type": "string", "description": "资源类型：Deployment 或 StatefulSet"},
			"name":      map[string]any{"type": "string", "description": "资源名称"},
			"replicas":  map[string]any{"type": "integer", "description": "目标副本数"},
		},
		"required": []string{"namespace", "kind", "name", "replicas"},
	}
}

func (t *ScaleResourceTool) Execute(ctx context.Context, args map[string]any) (string, error) {
	namespace, _ := args["namespace"].(string)
	kind, _ := args["kind"].(string)
	name, _ := args["name"].(string)

	replicasRaw, ok := args["replicas"]
	if !ok {
		return "", fmt.Errorf("参数 replicas 不能为空")
	}
	// JSON numbers unmarshal as float64.
	var replicas int32
	switch v := replicasRaw.(type) {
	case float64:
		replicas = int32(v)
	case int32:
		replicas = v
	case int:
		replicas = int32(v)
	default:
		return "", fmt.Errorf("参数 replicas 类型错误: %T", replicasRaw)
	}

	if err := t.k8sClient.ScaleResource(ctx, namespace, kind, name, replicas); err != nil {
		return "", err
	}
	return fmt.Sprintf("已将 %s/%s (namespace: %s) 的副本数设置为 %d", kind, name, namespace, replicas), nil
}

func init() {
	tool.RegisterGlobal(tool.ToolMetadata{
		Name:        "scale_resource",
		Description: "调整 Deployment 或 StatefulSet 的副本数",
		Category:    "operation",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"namespace": map[string]any{"type": "string", "description": "命名空间"},
				"kind":      map[string]any{"type": "string", "description": "资源类型：Deployment 或 StatefulSet"},
				"name":      map[string]any{"type": "string", "description": "资源名称"},
				"replicas":  map[string]any{"type": "integer", "description": "目标副本数"},
			},
			"required": []string{"namespace", "kind", "name", "replicas"},
		},
		Factory: func(dep any) (tool.Tool, error) {
			d, ok := dep.(tool.ToolDependency)
			if !ok {
				return nil, fmt.Errorf("invalid dependency type")
			}
			return NewScaleResourceTool(d.K8sClient), nil
		},
	})
}
```

- [ ] **Step 2: Verify compilation**

```
cd d:/KubeWise && go build ./pkg/tools/v1/operation/...
```

Expected: no errors.

---

## Task 4: `restart_resource` Tool

**Files:**
- Create: `pkg/tools/v1/operation/restart.go`

- [ ] **Step 1: Create `pkg/tools/v1/operation/restart.go`**

```go
package operation

import (
	"context"
	"fmt"

	"github.com/kubewise/kubewise/pkg/k8s"
	"github.com/kubewise/kubewise/pkg/tool"
)

type RestartResourceTool struct {
	k8sClient *k8s.Client
}

func NewRestartResourceTool(k8sClient *k8s.Client) *RestartResourceTool {
	return &RestartResourceTool{k8sClient: k8sClient}
}

func (t *RestartResourceTool) Name() string { return "restart_resource" }

func (t *RestartResourceTool) Description() string {
	return "触发 Deployment、StatefulSet 或 DaemonSet 的滚动重启"
}

func (t *RestartResourceTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"namespace": map[string]any{"type": "string", "description": "命名空间"},
			"kind":      map[string]any{"type": "string", "description": "资源类型：Deployment、StatefulSet 或 DaemonSet"},
			"name":      map[string]any{"type": "string", "description": "资源名称"},
		},
		"required": []string{"namespace", "kind", "name"},
	}
}

func (t *RestartResourceTool) Execute(ctx context.Context, args map[string]any) (string, error) {
	namespace, _ := args["namespace"].(string)
	kind, _ := args["kind"].(string)
	name, _ := args["name"].(string)

	if err := t.k8sClient.RestartResource(ctx, namespace, kind, name); err != nil {
		return "", err
	}
	return fmt.Sprintf("已触发 %s/%s (namespace: %s) 的滚动重启", kind, name, namespace), nil
}

func init() {
	tool.RegisterGlobal(tool.ToolMetadata{
		Name:        "restart_resource",
		Description: "触发 Deployment、StatefulSet 或 DaemonSet 的滚动重启",
		Category:    "operation",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"namespace": map[string]any{"type": "string", "description": "命名空间"},
				"kind":      map[string]any{"type": "string", "description": "资源类型：Deployment、StatefulSet 或 DaemonSet"},
				"name":      map[string]any{"type": "string", "description": "资源名称"},
			},
			"required": []string{"namespace", "kind", "name"},
		},
		Factory: func(dep any) (tool.Tool, error) {
			d, ok := dep.(tool.ToolDependency)
			if !ok {
				return nil, fmt.Errorf("invalid dependency type")
			}
			return NewRestartResourceTool(d.K8sClient), nil
		},
	})
}
```

---

## Task 5: `delete_resource` Tool

**Files:**
- Create: `pkg/tools/v1/operation/delete.go`

- [ ] **Step 1: Create `pkg/tools/v1/operation/delete.go`**

```go
package operation

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/kubewise/kubewise/pkg/k8s"
	"github.com/kubewise/kubewise/pkg/tool"
)

type DeleteResourceTool struct {
	k8sClient *k8s.Client
}

func NewDeleteResourceTool(k8sClient *k8s.Client) *DeleteResourceTool {
	return &DeleteResourceTool{k8sClient: k8sClient}
}

func (t *DeleteResourceTool) Name() string { return "delete_resource" }

func (t *DeleteResourceTool) Description() string {
	return "删除任意 Kubernetes 资源（通过 GVR 指定资源类型）"
}

func (t *DeleteResourceTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"namespace": map[string]any{"type": "string", "description": "命名空间（集群级资源留空）"},
			"group":     map[string]any{"type": "string", "description": "API 组，如 \"\"（核心组）、\"apps\""},
			"version":   map[string]any{"type": "string", "description": "API 版本，如 \"v1\""},
			"resource":  map[string]any{"type": "string", "description": "资源类型复数名，如 \"pods\"、\"deployments\""},
			"name":      map[string]any{"type": "string", "description": "资源名称"},
		},
		"required": []string{"group", "version", "resource", "name"},
	}
}

func (t *DeleteResourceTool) Execute(ctx context.Context, args map[string]any) (string, error) {
	namespace, _ := args["namespace"].(string)
	group, _ := args["group"].(string)
	version, _ := args["version"].(string)
	resource, _ := args["resource"].(string)
	name, _ := args["name"].(string)

	if version == "" || resource == "" || name == "" {
		return "", fmt.Errorf("参数 version、resource、name 不能为空")
	}

	gvr := schema.GroupVersionResource{Group: group, Version: version, Resource: resource}
	if err := t.k8sClient.DeleteResource(ctx, namespace, gvr, name); err != nil {
		return "", err
	}
	return fmt.Sprintf("已删除 %s/%s/%s/%s", namespace, group, resource, name), nil
}

func init() {
	tool.RegisterGlobal(tool.ToolMetadata{
		Name:        "delete_resource",
		Description: "删除任意 Kubernetes 资源（通过 GVR 指定资源类型）",
		Category:    "operation",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"namespace": map[string]any{"type": "string", "description": "命名空间（集群级资源留空）"},
				"group":     map[string]any{"type": "string", "description": "API 组，如 \"\"（核心组）、\"apps\""},
				"version":   map[string]any{"type": "string", "description": "API 版本，如 \"v1\""},
				"resource":  map[string]any{"type": "string", "description": "资源类型复数名，如 \"pods\"、\"deployments\""},
				"name":      map[string]any{"type": "string", "description": "资源名称"},
			},
			"required": []string{"group", "version", "resource", "name"},
		},
		Factory: func(dep any) (tool.Tool, error) {
			d, ok := dep.(tool.ToolDependency)
			if !ok {
				return nil, fmt.Errorf("invalid dependency type")
			}
			return NewDeleteResourceTool(d.K8sClient), nil
		},
	})
}
```

---

## Task 6: `apply_resource` Tool

**Files:**
- Create: `pkg/tools/v1/operation/apply.go`

- [ ] **Step 1: Create `pkg/tools/v1/operation/apply.go`**

```go
package operation

import (
	"context"
	"fmt"

	"sigs.k8s.io/yaml"

	"github.com/kubewise/kubewise/pkg/k8s"
	"github.com/kubewise/kubewise/pkg/tool"
)

type ApplyResourceTool struct {
	k8sClient *k8s.Client
}

func NewApplyResourceTool(k8sClient *k8s.Client) *ApplyResourceTool {
	return &ApplyResourceTool{k8sClient: k8sClient}
}

func (t *ApplyResourceTool) Name() string { return "apply_resource" }

func (t *ApplyResourceTool) Description() string {
	return "通过 Server-Side Apply 创建或更新 Kubernetes 资源（传入完整 YAML 内容）"
}

func (t *ApplyResourceTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"yaml_content": map[string]any{
				"type":        "string",
				"description": "完整的 Kubernetes 资源 YAML 内容",
			},
		},
		"required": []string{"yaml_content"},
	}
}

func (t *ApplyResourceTool) Execute(ctx context.Context, args map[string]any) (string, error) {
	yamlContent, _ := args["yaml_content"].(string)
	if yamlContent == "" {
		return "", fmt.Errorf("参数 yaml_content 不能为空")
	}

	// Validate YAML is parseable before sending to server.
	if err := validateYAML(yamlContent); err != nil {
		return "", fmt.Errorf("YAML 格式校验失败: %w", err)
	}

	if err := t.k8sClient.ApplyResource(ctx, yamlContent); err != nil {
		return "", err
	}
	return "资源 Apply 成功", nil
}

// validateYAML checks that the YAML is syntactically valid.
func validateYAML(content string) error {
	var out map[string]any
	return yaml.Unmarshal([]byte(content), &out)
}

func init() {
	tool.RegisterGlobal(tool.ToolMetadata{
		Name:        "apply_resource",
		Description: "通过 Server-Side Apply 创建或更新 Kubernetes 资源（传入完整 YAML 内容）",
		Category:    "operation",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"yaml_content": map[string]any{
					"type":        "string",
					"description": "完整的 Kubernetes Kubernetes 资源 YAML 内容",
				},
			},
			"required": []string{"yaml_content"},
		},
		Factory: func(dep any) (tool.Tool, error) {
			d, ok := dep.(tool.ToolDependency)
			if !ok {
				return nil, fmt.Errorf("invalid dependency type")
			}
			return NewApplyResourceTool(d.K8sClient), nil
		},
	})
}
```

---

## Task 7: `cordon_drain_node` Tool

**Files:**
- Create: `pkg/tools/v1/operation/cordon_drain.go`

- [ ] **Step 1: Create `pkg/tools/v1/operation/cordon_drain.go`**

```go
package operation

import (
	"context"
	"fmt"
	"strings"

	"github.com/kubewise/kubewise/pkg/k8s"
	"github.com/kubewise/kubewise/pkg/tool"
)

type CordonDrainNodeTool struct {
	k8sClient *k8s.Client
}

func NewCordonDrainNodeTool(k8sClient *k8s.Client) *CordonDrainNodeTool {
	return &CordonDrainNodeTool{k8sClient: k8sClient}
}

func (t *CordonDrainNodeTool) Name() string { return "cordon_drain_node" }

func (t *CordonDrainNodeTool) Description() string {
	return "封锁节点（cordon）、解封节点（uncordon）或驱逐节点上所有 Pod（drain）"
}

func (t *CordonDrainNodeTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"node_name": map[string]any{"type": "string", "description": "节点名称"},
			"action": map[string]any{
				"type":        "string",
				"enum":        []string{"cordon", "uncordon", "drain"},
				"description": "操作：cordon（封锁）、uncordon（解封）、drain（驱逐所有 Pod）",
			},
		},
		"required": []string{"node_name", "action"},
	}
}

func (t *CordonDrainNodeTool) Execute(ctx context.Context, args map[string]any) (string, error) {
	nodeName, _ := args["node_name"].(string)
	action, _ := args["action"].(string)

	if nodeName == "" || action == "" {
		return "", fmt.Errorf("参数 node_name 和 action 不能为空")
	}

	switch action {
	case "cordon":
		if err := t.k8sClient.CordonNode(ctx, nodeName, true); err != nil {
			return "", err
		}
		return fmt.Sprintf("节点 %s 已封锁（Unschedulable=true）", nodeName), nil

	case "uncordon":
		if err := t.k8sClient.CordonNode(ctx, nodeName, false); err != nil {
			return "", err
		}
		return fmt.Sprintf("节点 %s 已解封（Unschedulable=false）", nodeName), nil

	case "drain":
		evicted, remaining, err := t.k8sClient.DrainNode(ctx, nodeName)
		if err != nil {
			return "", err
		}
		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("节点 %s 驱逐完成\n", nodeName))
		sb.WriteString(fmt.Sprintf("  已驱逐：%d 个 Pod\n", len(evicted)))
		if len(remaining) > 0 {
			sb.WriteString(fmt.Sprintf("  驱逐失败：%s\n", strings.Join(remaining, ", ")))
		}
		return sb.String(), nil

	default:
		return "", fmt.Errorf("未知 action: %s，支持 cordon/uncordon/drain", action)
	}
}

func init() {
	tool.RegisterGlobal(tool.ToolMetadata{
		Name:        "cordon_drain_node",
		Description: "封锁节点（cordon）、解封节点（uncordon）或驱逐节点上所有 Pod（drain）",
		Category:    "operation",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"node_name": map[string]any{"type": "string", "description": "节点名称"},
				"action": map[string]any{
					"type":        "string",
					"enum":        []string{"cordon", "uncordon", "drain"},
					"description": "操作：cordon（封锁）、uncordon（解封）、drain（驱逐所有 Pod）",
				},
			},
			"required": []string{"node_name", "action"},
		},
		Factory: func(dep any) (tool.Tool, error) {
			d, ok := dep.(tool.ToolDependency)
			if !ok {
				return nil, fmt.Errorf("invalid dependency type")
			}
			return NewCordonDrainNodeTool(d.K8sClient), nil
		},
	})
}
```

---

## Task 8: `label_annotate_resource` Tool

**Files:**
- Create: `pkg/tools/v1/operation/label_annotate.go`

- [ ] **Step 1: Create `pkg/tools/v1/operation/label_annotate.go`**

```go
package operation

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/kubewise/kubewise/pkg/k8s"
	"github.com/kubewise/kubewise/pkg/tool"
)

type LabelAnnotateResourceTool struct {
	k8sClient *k8s.Client
}

func NewLabelAnnotateResourceTool(k8sClient *k8s.Client) *LabelAnnotateResourceTool {
	return &LabelAnnotateResourceTool{k8sClient: k8sClient}
}

func (t *LabelAnnotateResourceTool) Name() string { return "label_annotate_resource" }

func (t *LabelAnnotateResourceTool) Description() string {
	return "为任意 Kubernetes 资源添加或修改 labels 和 annotations"
}

func (t *LabelAnnotateResourceTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"namespace":   map[string]any{"type": "string", "description": "命名空间（集群级资源留空）"},
			"group":       map[string]any{"type": "string", "description": "API 组"},
			"version":     map[string]any{"type": "string", "description": "API 版本"},
			"resource":    map[string]any{"type": "string", "description": "资源类型复数名"},
			"name":        map[string]any{"type": "string", "description": "资源名称"},
			"labels":      map[string]any{"type": "object", "description": "要设置的 labels（键值对）"},
			"annotations": map[string]any{"type": "object", "description": "要设置的 annotations（键值对）"},
		},
		"required": []string{"group", "version", "resource", "name"},
	}
}

func (t *LabelAnnotateResourceTool) Execute(ctx context.Context, args map[string]any) (string, error) {
	namespace, _ := args["namespace"].(string)
	group, _ := args["group"].(string)
	version, _ := args["version"].(string)
	resource, _ := args["resource"].(string)
	name, _ := args["name"].(string)

	if version == "" || resource == "" || name == "" {
		return "", fmt.Errorf("参数 version、resource、name 不能为空")
	}

	labels := toStringMap(args["labels"])
	annotations := toStringMap(args["annotations"])

	gvr := schema.GroupVersionResource{Group: group, Version: version, Resource: resource}
	if err := t.k8sClient.LabelResource(ctx, namespace, gvr, name, labels, annotations); err != nil {
		return "", err
	}
	return fmt.Sprintf("已更新 %s/%s 的 labels/annotations", resource, name), nil
}

func toStringMap(v any) map[string]string {
	if v == nil {
		return nil
	}
	m, ok := v.(map[string]any)
	if !ok {
		return nil
	}
	result := make(map[string]string, len(m))
	for k, val := range m {
		if s, ok := val.(string); ok {
			result[k] = s
		}
	}
	return result
}

func init() {
	tool.RegisterGlobal(tool.ToolMetadata{
		Name:        "label_annotate_resource",
		Description: "为任意 Kubernetes 资源添加或修改 labels 和 annotations",
		Category:    "operation",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"namespace":   map[string]any{"type": "string", "description": "命名空间（集群级资源留空）"},
				"group":       map[string]any{"type": "string", "description": "API 组"},
				"version":     map[string]any{"type": "string", "description": "API 版本"},
				"resource":    map[string]any{"type": "string", "description": "资源类型复数名"},
				"name":        map[string]any{"type": "string", "description": "资源名称"},
				"labels":      map[string]any{"type": "object", "description": "要设置的 labels（键值对）"},
				"annotations": map[string]any{"type": "object", "description": "要设置的 annotations（键值对）"},
			},
			"required": []string{"group", "version", "resource", "name"},
		},
		Factory: func(dep any) (tool.Tool, error) {
			d, ok := dep.(tool.ToolDependency)
			if !ok {
				return nil, fmt.Errorf("invalid dependency type")
			}
			return NewLabelAnnotateResourceTool(d.K8sClient), nil
		},
	})
}
```

- [ ] **Step 2: Verify all 6 tools compile together**

```
cd d:/KubeWise && go build ./pkg/tools/v1/operation/...
```

Expected: no errors.

- [ ] **Step 3: Commit all 6 tools**

```bash
git add pkg/tools/v1/operation/
git commit -m "feat(tools): add six operation tools (scale, restart, delete, apply, cordon_drain, label_annotate)"
```

---

## Task 9: `OperationStep` Types + `ConfirmationHandler`

**Files:**
- Create: `pkg/agent/operation/types.go`
- Create: `pkg/agent/operation/confirm.go`

- [ ] **Step 1: Write failing tests for `stepToToolCall` and `formatStep`**

Create `pkg/agent/operation/agent_test.go`:

```go
package operation

import (
	"context"
	"testing"
)

func TestStepToToolCall(t *testing.T) {
	t.Run("scale", func(t *testing.T) {
		replicas := int32(5)
		step := OperationStep{
			StepIndex:     1,
			OperationType: "scale",
			ResourceKind:  "Deployment",
			ResourceName:  "nginx",
			Namespace:     "default",
			Replicas:      &replicas,
			Description:   "扩容 nginx",
		}
		toolName, args, err := stepToToolCall(step)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if toolName != "scale_resource" {
			t.Errorf("expected scale_resource, got %s", toolName)
		}
		if args["namespace"] != "default" {
			t.Errorf("wrong namespace: %v", args["namespace"])
		}
		if args["replicas"] != float64(5) {
			t.Errorf("wrong replicas: %v", args["replicas"])
		}
	})

	t.Run("restart", func(t *testing.T) {
		step := OperationStep{
			StepIndex:     1,
			OperationType: "restart",
			ResourceKind:  "Deployment",
			ResourceName:  "api",
			Namespace:     "prod",
			Description:   "重启 api",
		}
		toolName, _, err := stepToToolCall(step)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if toolName != "restart_resource" {
			t.Errorf("expected restart_resource, got %s", toolName)
		}
	})

	t.Run("delete", func(t *testing.T) {
		step := OperationStep{
			StepIndex:     1,
			OperationType: "delete",
			ResourceKind:  "Pod",
			ResourceName:  "bad-pod",
			Namespace:     "default",
			Group:         "",
			Version:       "v1",
			Resource:      "pods",
			Description:   "删除 bad-pod",
		}
		toolName, args, err := stepToToolCall(step)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if toolName != "delete_resource" {
			t.Errorf("expected delete_resource, got %s", toolName)
		}
		if args["resource"] != "pods" {
			t.Errorf("wrong resource: %v", args["resource"])
		}
	})

	t.Run("cordon_drain", func(t *testing.T) {
		step := OperationStep{
			StepIndex:     1,
			OperationType: "cordon_drain",
			ResourceKind:  "Node",
			ResourceName:  "node-1",
			Action:        "drain",
			Description:   "驱逐 node-1",
		}
		toolName, args, err := stepToToolCall(step)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if toolName != "cordon_drain_node" {
			t.Errorf("expected cordon_drain_node, got %s", toolName)
		}
		if args["action"] != "drain" {
			t.Errorf("wrong action: %v", args["action"])
		}
	})

	t.Run("unknown operation returns error", func(t *testing.T) {
		step := OperationStep{OperationType: "unknown"}
		_, _, err := stepToToolCall(step)
		if err == nil {
			t.Error("expected error for unknown operation type")
		}
	})
}

func TestChannelConfirmationHandlerConfirm(t *testing.T) {
	handler := NewChannelConfirmationHandler()
	ctx := context.Background()

	step := OperationStep{
		StepIndex:     1,
		OperationType: "scale",
		ResourceKind:  "Deployment",
		ResourceName:  "nginx",
		Namespace:     "default",
		Description:   "扩容 nginx",
	}

	go func() {
		req := <-handler.Requests
		if req.Step.ResourceName != "nginx" {
			t.Errorf("expected nginx, got %s", req.Step.ResourceName)
		}
		handler.Responses <- ConfirmResponse{Confirmed: true}
	}()

	confirmed, correction, err := handler.Confirm(ctx, step, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !confirmed {
		t.Error("expected confirmed=true")
	}
	if correction != "" {
		t.Errorf("expected empty correction, got %q", correction)
	}
}

func TestChannelConfirmationHandlerCorrection(t *testing.T) {
	handler := NewChannelConfirmationHandler()
	ctx := context.Background()

	step := OperationStep{StepIndex: 1, OperationType: "scale", ResourceName: "nginx"}

	go func() {
		<-handler.Requests
		handler.Responses <- ConfirmResponse{Confirmed: false, Correction: "改为 10 个副本"}
	}()

	confirmed, correction, err := handler.Confirm(ctx, step, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if confirmed {
		t.Error("expected confirmed=false")
	}
	if correction != "改为 10 个副本" {
		t.Errorf("expected correction text, got %q", correction)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```
cd d:/KubeWise && go test ./pkg/agent/operation/... -v
```

Expected: FAIL — `OperationStep undefined`, `stepToToolCall undefined`, `ChannelConfirmationHandler undefined`

- [ ] **Step 3: Create `pkg/agent/operation/types.go`**

```go
package operation

import "fmt"

// OperationStep represents a single planned cluster mutation.
type OperationStep struct {
	StepIndex     int               `json:"step_index"`
	OperationType string            `json:"operation_type"`
	ResourceKind  string            `json:"resource_kind"`
	ResourceName  string            `json:"resource_name"`
	Namespace     string            `json:"namespace,omitempty"`
	Group         string            `json:"group,omitempty"`
	Version       string            `json:"version,omitempty"`
	Resource      string            `json:"resource,omitempty"`
	Replicas      *int32            `json:"replicas,omitempty"`
	Action        string            `json:"action,omitempty"`
	Labels        map[string]string `json:"labels,omitempty"`
	Annotations   map[string]string `json:"annotations,omitempty"`
	GeneratedYAML string            `json:"generated_yaml,omitempty"`
	Description   string            `json:"description"`
}

// stepToToolCall maps an OperationStep to a write tool name and its args map.
func stepToToolCall(step OperationStep) (toolName string, args map[string]any, err error) {
	switch step.OperationType {
	case "scale":
		if step.Replicas == nil {
			return "", nil, fmt.Errorf("scale 操作需要 replicas 字段")
		}
		return "scale_resource", map[string]any{
			"namespace": step.Namespace,
			"kind":      step.ResourceKind,
			"name":      step.ResourceName,
			"replicas":  float64(*step.Replicas),
		}, nil

	case "restart":
		return "restart_resource", map[string]any{
			"namespace": step.Namespace,
			"kind":      step.ResourceKind,
			"name":      step.ResourceName,
		}, nil

	case "delete":
		return "delete_resource", map[string]any{
			"namespace": step.Namespace,
			"group":     step.Group,
			"version":   step.Version,
			"resource":  step.Resource,
			"name":      step.ResourceName,
		}, nil

	case "apply":
		return "apply_resource", map[string]any{
			"yaml_content": step.GeneratedYAML,
		}, nil

	case "cordon_drain":
		return "cordon_drain_node", map[string]any{
			"node_name": step.ResourceName,
			"action":    step.Action,
		}, nil

	case "label_annotate":
		return "label_annotate_resource", map[string]any{
			"namespace":   step.Namespace,
			"group":       step.Group,
			"version":     step.Version,
			"resource":    step.Resource,
			"name":        step.ResourceName,
			"labels":      step.Labels,
			"annotations": step.Annotations,
		}, nil

	default:
		return "", nil, fmt.Errorf("未知操作类型: %s", step.OperationType)
	}
}

func operationTypeDisplay(opType string) string {
	switch opType {
	case "scale":
		return "Scale（扩缩容）"
	case "restart":
		return "Restart（重启）"
	case "delete":
		return "Delete（删除）"
	case "apply":
		return "Apply（创建/更新）"
	case "cordon_drain":
		return "Cordon/Drain（节点封锁/驱逐）"
	case "label_annotate":
		return "Label/Annotate（标签/注解）"
	default:
		return opType
	}
}
```

- [ ] **Step 4: Create `pkg/agent/operation/confirm.go`**

```go
package operation

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strings"
)

// ConfirmationHandler abstracts the per-step confirmation interaction.
// It is injected into the agent, allowing CLI, TUI, and API implementations.
type ConfirmationHandler interface {
	// Confirm presents a step to the user and waits for their decision.
	// Returns confirmed=true to execute, non-empty correction to replan,
	// or both false/empty to skip the step.
	Confirm(ctx context.Context, step OperationStep, totalSteps int) (confirmed bool, correction string, err error)
}

// StdinConfirmationHandler is the default CLI implementation.
type StdinConfirmationHandler struct {
	reader *bufio.Reader
	writer io.Writer
}

func NewStdinConfirmationHandler() *StdinConfirmationHandler {
	return &StdinConfirmationHandler{
		reader: bufio.NewReader(os.Stdin),
		writer: os.Stdout,
	}
}

func (h *StdinConfirmationHandler) Confirm(ctx context.Context, step OperationStep, totalSteps int) (bool, string, error) {
	h.formatStep(step, totalSteps)
	fmt.Fprint(h.writer, "确认执行？[y/N]：")

	line, err := h.reader.ReadString('\n')
	if err != nil {
		return false, "", err
	}
	if strings.ToLower(strings.TrimSpace(line)) == "y" {
		return true, "", nil
	}

	fmt.Fprint(h.writer, "请输入修正指令（直接回车跳过该步骤）：")
	line, err = h.reader.ReadString('\n')
	if err != nil {
		return false, "", err
	}
	return false, strings.TrimSpace(line), nil
}

func (h *StdinConfirmationHandler) formatStep(step OperationStep, totalSteps int) {
	fmt.Fprintf(h.writer, "\n步骤 %d/%d：%s\n", step.StepIndex, totalSteps, operationTypeDisplay(step.OperationType))

	if step.Namespace != "" {
		fmt.Fprintf(h.writer, "  资源：%s/%s (namespace: %s)\n", step.ResourceKind, step.ResourceName, step.Namespace)
	} else {
		fmt.Fprintf(h.writer, "  资源：%s/%s\n", step.ResourceKind, step.ResourceName)
	}

	switch step.OperationType {
	case "scale":
		fmt.Fprintf(h.writer, "  变更：replicas → %d\n", *step.Replicas)
	case "restart":
		fmt.Fprintln(h.writer, "  操作：触发滚动重启")
	case "delete":
		fmt.Fprintln(h.writer, "  操作：删除资源（不可撤销）")
	case "apply":
		fmt.Fprintf(h.writer, "  以下 YAML 将被 Apply：\n---\n%s\n---\n", step.GeneratedYAML)
	case "cordon_drain":
		fmt.Fprintf(h.writer, "  操作：%s\n", step.Action)
	case "label_annotate":
		if len(step.Labels) > 0 {
			fmt.Fprintf(h.writer, "  Labels：%v\n", step.Labels)
		}
		if len(step.Annotations) > 0 {
			fmt.Fprintf(h.writer, "  Annotations：%v\n", step.Annotations)
		}
	}
	fmt.Fprintf(h.writer, "  说明：%s\n", step.Description)
}

// ConfirmRequest is sent by the agent to the TUI/API layer.
type ConfirmRequest struct {
	Step       OperationStep
	TotalSteps int
}

// ConfirmResponse is sent back by the TUI/API layer to the agent.
type ConfirmResponse struct {
	Confirmed  bool
	Correction string
	Err        error
}

// ChannelConfirmationHandler enables TUI/API-driven confirmation via channels.
type ChannelConfirmationHandler struct {
	Requests  chan ConfirmRequest
	Responses chan ConfirmResponse
}

func NewChannelConfirmationHandler() *ChannelConfirmationHandler {
	return &ChannelConfirmationHandler{
		Requests:  make(chan ConfirmRequest),
		Responses: make(chan ConfirmResponse),
	}
}

func (h *ChannelConfirmationHandler) Confirm(ctx context.Context, step OperationStep, totalSteps int) (bool, string, error) {
	select {
	case h.Requests <- ConfirmRequest{Step: step, TotalSteps: totalSteps}:
	case <-ctx.Done():
		return false, "", ctx.Err()
	}
	select {
	case resp := <-h.Responses:
		return resp.Confirmed, resp.Correction, resp.Err
	case <-ctx.Done():
		return false, "", ctx.Err()
	}
}
```

- [ ] **Step 5: Run tests to verify they pass**

```
cd d:/KubeWise && go test ./pkg/agent/operation/... -run "TestStepToToolCall|TestChannelConfirmation" -v
```

Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add pkg/agent/operation/types.go pkg/agent/operation/confirm.go pkg/agent/operation/agent_test.go
git commit -m "feat(operation): add OperationStep types, ConfirmationHandler, and unit tests"
```

---

## Task 10: `OperationAgent` Implementation

**Files:**
- Create: `pkg/agent/operation/agent.go`

- [ ] **Step 1: Write failing tests for the execute loop**

Add the following test functions to `pkg/agent/operation/agent_test.go`:

```go
func TestExecuteConfirmPath(t *testing.T) {
	handler := NewChannelConfirmationHandler()
	called := false
	writeReg := &mockRegistry{
		executeFn: func(name string, args map[string]any) (string, error) {
			called = true
			if name != "scale_resource" {
				t.Errorf("expected scale_resource, got %s", name)
			}
			return "ok", nil
		},
	}

	replicas := int32(5)
	steps := []OperationStep{{
		StepIndex:     1,
		OperationType: "scale",
		ResourceKind:  "Deployment",
		ResourceName:  "nginx",
		Namespace:     "default",
		Replicas:      &replicas,
		Description:   "扩容",
	}}

	ctx := context.Background()
	agent := &Agent{confirmHandler: handler, writeRegistry: writeReg}

	go func() {
		<-handler.Requests
		handler.Responses <- ConfirmResponse{Confirmed: true}
	}()

	summary, err := agent.execute(ctx, steps)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Error("expected write tool to be called")
	}
	if summary == "" {
		t.Error("expected non-empty summary")
	}
}

func TestExecuteSkipPath(t *testing.T) {
	handler := NewChannelConfirmationHandler()
	called := false
	writeReg := &mockRegistry{
		executeFn: func(name string, args map[string]any) (string, error) {
			called = true
			return "ok", nil
		},
	}

	replicas := int32(3)
	steps := []OperationStep{{
		StepIndex: 1, OperationType: "scale", ResourceKind: "Deployment",
		ResourceName: "nginx", Namespace: "default", Replicas: &replicas,
	}}

	ctx := context.Background()
	agent := &Agent{confirmHandler: handler, writeRegistry: writeReg}

	go func() {
		<-handler.Requests
		// Empty correction = skip
		handler.Responses <- ConfirmResponse{Confirmed: false, Correction: ""}
	}()

	_, err := agent.execute(ctx, steps)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if called {
		t.Error("expected write tool NOT to be called for skipped step")
	}
}

// mockRegistry satisfies the writeRegistry interface used by agent.execute.
type mockRegistry struct {
	executeFn func(name string, args map[string]any) (string, error)
}

func (m *mockRegistry) GetTool(name string) (toolExecutor, bool) {
	return &mockTool{name: name, fn: m.executeFn}, true
}

type mockTool struct {
	name string
	fn   func(name string, args map[string]any) (string, error)
}

func (m *mockTool) Execute(ctx context.Context, args map[string]any) (string, error) {
	return m.fn(m.name, args)
}
```

- [ ] **Step 2: Run to confirm failure**

```
cd d:/KubeWise && go test ./pkg/agent/operation/... -v
```

Expected: FAIL — `Agent undefined`, `toolExecutor undefined`, `writeRegistry interface not found`

- [ ] **Step 3: Create `pkg/agent/operation/agent.go`**

```go
package operation

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/kubewise/kubewise/pkg/k8s"
	"github.com/kubewise/kubewise/pkg/llm"
	"github.com/kubewise/kubewise/pkg/tool"
	"github.com/kubewise/kubewise/pkg/types"

	// Imports query tools for planning phase read access.
	_ "github.com/kubewise/kubewise/pkg/tools/v1/query"
	// Imports operation tools for execution phase write access.
	_ "github.com/kubewise/kubewise/pkg/tools/v1/operation"
)

// toolExecutor is the minimal interface needed from a registry tool.
type toolExecutor interface {
	Execute(ctx context.Context, args map[string]any) (string, error)
}

// writeRegistryI is satisfied by *tool.Registry and by mockRegistry in tests.
type writeRegistryI interface {
	GetTool(name string) (toolExecutor, bool)
}

// toolRegistryAdapter wraps *tool.Registry to satisfy writeRegistryI.
type toolRegistryAdapter struct{ reg *tool.Registry }

func (a *toolRegistryAdapter) GetTool(name string) (toolExecutor, bool) {
	t, ok := a.reg.GetTool(name)
	return t, ok
}

// Option configures the Agent.
type Option func(*Agent)

// WithConfirmationHandler injects a custom confirmation handler (for TUI/API use).
func WithConfirmationHandler(h ConfirmationHandler) Option {
	return func(a *Agent) { a.confirmHandler = h }
}

// Agent is the operation agent. It plans via LLM + read tools, then executes
// each step only after receiving user confirmation.
type Agent struct {
	k8sClient      *k8s.Client
	llmClient      *llm.Client
	readRegistry   *tool.Registry
	writeRegistry  writeRegistryI
	confirmHandler ConfirmationHandler
}

// New creates a new OperationAgent. Defaults to StdinConfirmationHandler.
func New(k8sClient *k8s.Client, llmClient *llm.Client, opts ...Option) (*Agent, error) {
	dep := tool.ToolDependency{K8sClient: k8sClient}

	readReg, err := tool.LoadGlobalRegistryByCategory(dep, "")
	if err != nil {
		return nil, fmt.Errorf("加载读工具注册中心失败: %w", err)
	}

	writeReg, err := tool.LoadGlobalRegistryByCategory(dep, "operation")
	if err != nil {
		return nil, fmt.Errorf("加载写工具注册中心失败: %w", err)
	}

	a := &Agent{
		k8sClient:      k8sClient,
		llmClient:      llmClient,
		readRegistry:   readReg,
		writeRegistry:  &toolRegistryAdapter{reg: writeReg},
		confirmHandler: NewStdinConfirmationHandler(),
	}
	for _, opt := range opts {
		opt(a)
	}
	return a, nil
}

// HandleQuery is the entry point called by the router.
func (a *Agent) HandleQuery(ctx context.Context, userQuery string, entities types.Entities) (string, error) {
	fmt.Println("正在分析操作意图并规划执行步骤...")

	steps, err := a.plan(ctx, userQuery, entities)
	if err != nil {
		return "", fmt.Errorf("规划阶段失败: %w", err)
	}
	if len(steps) == 0 {
		return "未生成任何操作步骤", nil
	}

	return a.execute(ctx, steps)
}

// plan runs a ReAct loop with read-only tools to produce []OperationStep.
func (a *Agent) plan(ctx context.Context, userQuery string, entities types.Entities) ([]OperationStep, error) {
	submitToolDef := llm.FunctionDefinition{
		Name:        "submit_operation_plan",
		Description: "提交操作计划。在分析集群状态并确定操作步骤后，调用此工具提交计划列表。",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"steps": map[string]any{
					"type":        "array",
					"description": "操作步骤列表，按执行顺序排列",
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"step_index":     map[string]any{"type": "integer"},
							"operation_type": map[string]any{"type": "string", "enum": []string{"scale", "restart", "delete", "apply", "cordon_drain", "label_annotate"}},
							"resource_kind":  map[string]any{"type": "string"},
							"resource_name":  map[string]any{"type": "string"},
							"namespace":      map[string]any{"type": "string"},
							"group":          map[string]any{"type": "string"},
							"version":        map[string]any{"type": "string"},
							"resource":       map[string]any{"type": "string"},
							"replicas":       map[string]any{"type": "integer"},
							"action":         map[string]any{"type": "string", "enum": []string{"cordon", "uncordon", "drain"}},
							"labels":         map[string]any{"type": "object"},
							"annotations":    map[string]any{"type": "object"},
							"generated_yaml": map[string]any{"type": "string"},
							"description":    map[string]any{"type": "string"},
						},
						"required": []string{"step_index", "operation_type", "resource_name", "description"},
					},
				},
			},
			"required": []string{"steps"},
		},
	}

	functions := a.readRegistry.GetAllFunctionDefinitions()
	functions = append(functions, submitToolDef)

	messages := []llm.Message{
		{Role: "system", Content: a.buildPlanningSystemPrompt()},
		{Role: "user", Content: userQuery},
	}

	const maxSteps = 10
	for step := range maxSteps {
		resp, err := a.llmClient.ChatCompletion(ctx, messages, functions)
		if err != nil {
			return nil, fmt.Errorf("LLM 调用失败: %w", err)
		}

		if len(resp.ToolCalls) == 0 {
			return nil, fmt.Errorf("规划未完成（LLM 未调用 submit_operation_plan），请重新描述您的操作需求")
		}

		funcCall := &resp.ToolCalls[0].Function

		if funcCall.Name == "submit_operation_plan" {
			return parseOperationPlan(funcCall.Arguments)
		}

		fmt.Printf("规划第%d步：调用工具 %s\n", step+1, funcCall.Name)

		t, exists := a.readRegistry.GetTool(funcCall.Name)
		if !exists {
			result := fmt.Sprintf("工具 %s 不存在，请选择可用工具", funcCall.Name)
			messages = append(messages, *resp, llm.Message{
				Role: "tool", Content: result, ToolCallID: resp.ToolCalls[0].ID,
			})
			continue
		}

		result, toolErr := t.Execute(ctx, funcCall.Arguments)
		if toolErr != nil {
			result = fmt.Sprintf("工具调用失败：%v\n请修正参数后重新调用。", toolErr)
		}

		messages = append(messages, *resp, llm.Message{
			Role:       "tool",
			Content:    fmt.Sprintf("工具返回结果：\n%s", result),
			ToolCallID: resp.ToolCalls[0].ID,
		})
	}

	return nil, fmt.Errorf("超过最大规划轮次（%d），无法生成操作计划，请重新描述您的需求", maxSteps)
}

// execute iterates steps with per-step confirmation. Supports correction-based replan.
func (a *Agent) execute(ctx context.Context, steps []OperationStep) (string, error) {
	type stepResult struct {
		step   OperationStep
		status string // "executed", "skipped", "failed"
		detail string
	}
	results := make([]stepResult, 0, len(steps))

	for _, step := range steps {
		const maxReplanAttempts = 2
		attempts := 0

		for {
			confirmed, correction, err := a.confirmHandler.Confirm(ctx, step, len(steps))
			if err != nil {
				return "", fmt.Errorf("确认交互失败: %w", err)
			}

			if confirmed {
				toolName, args, mappingErr := stepToToolCall(step)
				if mappingErr != nil {
					results = append(results, stepResult{step: step, status: "failed", detail: mappingErr.Error()})
					break
				}
				t, exists := a.writeRegistry.GetTool(toolName)
				if !exists {
					results = append(results, stepResult{step: step, status: "failed", detail: fmt.Sprintf("写工具 %s 未注册", toolName)})
					break
				}
				execResult, execErr := t.Execute(ctx, args)
				if execErr != nil {
					fmt.Printf("执行失败：%v\n", execErr)
					results = append(results, stepResult{step: step, status: "failed", detail: execErr.Error()})
				} else {
					results = append(results, stepResult{step: step, status: "executed", detail: execResult})
				}
				break
			}

			if correction == "" {
				results = append(results, stepResult{step: step, status: "skipped"})
				break
			}

			if attempts >= maxReplanAttempts {
				fmt.Printf("已达最大修正次数（%d），跳过该步骤\n", maxReplanAttempts)
				results = append(results, stepResult{step: step, status: "skipped", detail: "超过最大修正次数"})
				break
			}

			replanned, replanErr := a.replan(ctx, step, correction)
			if replanErr != nil {
				fmt.Printf("修正规划失败：%v，跳过该步骤\n", replanErr)
				results = append(results, stepResult{step: step, status: "skipped", detail: replanErr.Error()})
				break
			}
			step = replanned
			attempts++
		}
	}

	return buildSummary(results), nil
}

// replan asks the LLM to revise a single step given the user's correction.
func (a *Agent) replan(ctx context.Context, original OperationStep, correction string) (OperationStep, error) {
	originalJSON, _ := json.Marshal(original)

	messages := []llm.Message{
		{Role: "system", Content: "你是 Kubernetes 操作规划助手。用户对某个操作步骤有修正意见，请根据用户的修正指令返回修改后的操作步骤 JSON，只返回一个 JSON 对象，不要有任何额外说明。"},
		{Role: "user", Content: fmt.Sprintf("原始操作步骤：\n%s\n\n用户修正指令：%s", string(originalJSON), correction)},
	}

	resp, err := a.llmClient.ChatCompletion(ctx, messages, nil)
	if err != nil {
		return OperationStep{}, err
	}

	content := strings.TrimSpace(resp.Content)
	content = strings.TrimPrefix(content, "```json")
	content = strings.TrimPrefix(content, "```")
	content = strings.TrimSuffix(content, "```")
	content = strings.TrimSpace(content)

	var revised OperationStep
	if err := json.Unmarshal([]byte(content), &revised); err != nil {
		return OperationStep{}, fmt.Errorf("修正结果 JSON 解析失败: %w", err)
	}
	return revised, nil
}

func parseOperationPlan(args map[string]any) ([]OperationStep, error) {
	stepsRaw, ok := args["steps"]
	if !ok {
		return nil, fmt.Errorf("submit_operation_plan 缺少 steps 参数")
	}
	data, err := json.Marshal(stepsRaw)
	if err != nil {
		return nil, err
	}
	var steps []OperationStep
	if err := json.Unmarshal(data, &steps); err != nil {
		return nil, fmt.Errorf("操作计划 JSON 解析失败: %w", err)
	}
	return steps, nil
}

func buildSummary(results []struct {
	step   OperationStep
	status string
	detail string
}) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("\n操作执行完成，共 %d 步：\n", len(results)))
	for _, r := range results {
		icon := map[string]string{"executed": "✓", "skipped": "○", "failed": "✗"}[r.status]
		sb.WriteString(fmt.Sprintf("  %s 步骤%d [%s] %s\n", icon, r.step.StepIndex, r.status, r.step.Description))
		if r.detail != "" && r.status != "executed" {
			sb.WriteString(fmt.Sprintf("      → %s\n", r.detail))
		}
	}
	return sb.String()
}

func (a *Agent) buildPlanningSystemPrompt() string {
	return `你是 Kubernetes 集群操作规划专家。你的任务是：
1. 使用查询工具了解集群当前状态（如确认资源存在、查询当前副本数等）
2. 规划出精确的操作步骤列表
3. 调用 submit_operation_plan 工具提交操作计划

支持的操作类型：
- scale: 调整副本数（支持 Deployment, StatefulSet），需填写 replicas 字段
- restart: 触发滚动重启（支持 Deployment, StatefulSet, DaemonSet）
- delete: 删除资源，需填写 group/version/resource（GVR）字段
- apply: 创建或更新资源，需在 generated_yaml 中填写完整的 YAML
- cordon_drain: 节点封锁/解封/驱逐，需填写 action（cordon/uncordon/drain）
- label_annotate: 修改标签/注解，需填写 group/version/resource 和 labels/annotations

注意事项：
- scale 操作前，请先查询当前副本数并在 description 中注明变化（如"3 → 5"）
- delete 操作前，请先确认资源存在
- apply 操作，generated_yaml 必须是完整合法的 Kubernetes YAML

常见 GVR 对照：
- Pod: group="", version="v1", resource="pods"
- Deployment: group="apps", version="v1", resource="deployments"
- StatefulSet: group="apps", version="v1", resource="statefulsets"
- DaemonSet: group="apps", version="v1", resource="daemonsets"
- Service: group="", version="v1", resource="services"
- ConfigMap: group="", version="v1", resource="configmaps"
- Node: group="", version="v1", resource="nodes"`
}
```

> **Note:** The `buildSummary` function receives an anonymous struct slice. To avoid a compile error, define a named type `stepResult` inside the `execute` method and update `buildSummary` to accept `[]stepResult`. The code above shows the intent — adjust types so `buildSummary` receives `[]stepResult`.

- [ ] **Step 4: Fix the `buildSummary` signature to use the named `stepResult` type**

In `agent.go`, change `buildSummary` to:

```go
type stepResult struct {
	step   OperationStep
	status string
	detail string
}

func buildSummary(results []stepResult) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("\n操作执行完成，共 %d 步：\n", len(results)))
	for _, r := range results {
		icon := map[string]string{"executed": "✓", "skipped": "○", "failed": "✗"}[r.status]
		sb.WriteString(fmt.Sprintf("  %s 步骤%d [%s] %s\n", icon, r.step.StepIndex, r.status, r.step.Description))
		if r.detail != "" && r.status != "executed" {
			sb.WriteString(fmt.Sprintf("      → %s\n", r.detail))
		}
	}
	return sb.String()
}
```

And remove the local `type stepResult` declaration from inside `execute`, keeping it as a package-level type.

- [ ] **Step 5: Run all operation agent tests**

```
cd d:/KubeWise && go test ./pkg/agent/operation/... -v
```

Expected: PASS for all tests (`TestStepToToolCall`, `TestChannelConfirmation*`, `TestExecute*`).

- [ ] **Step 6: Verify full compilation**

```
cd d:/KubeWise && go build ./...
```

Expected: no errors.

- [ ] **Step 7: Commit**

```bash
git add pkg/agent/operation/agent.go pkg/agent/operation/agent_test.go
git commit -m "feat(agent): implement OperationAgent with plan/execute loop and ConfirmationHandler"
```

---

## Task 11: Wire `OperationAgent` into Router

**Files:**
- Modify: `pkg/agent/router/agent.go`

- [ ] **Step 1: Add `operationAgent` to the `Agent` struct and `New()` in `pkg/agent/router/agent.go`**

Add import:
```go
"github.com/kubewise/kubewise/pkg/agent/operation"
```

Add field to `Agent` struct (after `securityAgent *security.Agent`):
```go
operationAgent *operation.Agent
```

Add initialization in `New()` (after `securityAgent` init):
```go
operationAgent, err := operation.New(k8sClient, llmClient)
if err != nil {
    return nil, fmt.Errorf("初始化操作Agent失败: %w", err)
}
```

Add `operationAgent: operationAgent` to the returned struct literal.

- [ ] **Step 2: Replace the stub in `HandleQuery`**

Find (line 72-73):
```go
case types.TaskTypeOperation:
    return "操作类功能正在开发中，敬请期待", nil
```

Replace with:
```go
case types.TaskTypeOperation:
    return a.operationAgent.HandleQuery(ctx, userQuery, intent.Entities)
```

- [ ] **Step 3: Verify full compilation**

```
cd d:/KubeWise && go build ./...
```

Expected: no errors.

- [ ] **Step 4: Run all tests**

```
cd d:/KubeWise && go test ./... -v 2>&1 | tail -40
```

Expected: all existing tests pass; new operation agent tests pass.

- [ ] **Step 5: Commit**

```bash
git add pkg/agent/router/agent.go
git commit -m "feat(router): wire OperationAgent into router, replacing operation stub"
```

---

## Verification Checklist

After all tasks are complete, run the following:

```bash
# All tests pass
cd d:/KubeWise && go test ./... 2>&1 | grep -E "FAIL|ok"

# Binary builds clean
cd d:/KubeWise && go build ./...

# Linter clean
cd d:/KubeWise && make lint
```

Expected output (tests):
```
ok      github.com/kubewise/kubewise/pkg/tool
ok      github.com/kubewise/kubewise/pkg/agent/operation
ok      github.com/kubewise/kubewise/pkg/...
```
