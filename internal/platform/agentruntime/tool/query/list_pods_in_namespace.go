package query

import (
	"context"
	"fmt"
	"strings"

	toolv2 "github.com/kubewise/kubewise/internal/platform/agentruntime/tool/v2"
	"github.com/kubewise/kubewise/internal/platform/cluster"
)

// ListPodsInNamespaceTool 列出指定命名空间下的Pod工具
type ListPodsInNamespaceTool struct {
	k8sClient *cluster.Client
}

// NewListPodsInNamespaceTool 创建列出Pod工具实例
func NewListPodsInNamespaceTool(k8sClient *cluster.Client) *ListPodsInNamespaceTool {
	return &ListPodsInNamespaceTool{k8sClient: k8sClient}
}

// Name 返回工具唯一标识
func (t *ListPodsInNamespaceTool) Name() string {
	return "list_pods_in_namespace"
}

// Description 返回工具功能描述
func (t *ListPodsInNamespaceTool) Description() string {
	return "列出指定命名空间下的所有Pod，不指定命名空间则列出所有命名空间的Pod"
}

// Parameters 返回工具参数定义（JSON Schema格式）
func (t *ListPodsInNamespaceTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"namespace": map[string]any{
				"type":        "string",
				"description": "命名空间，可选，不指定则列出所有命名空间的Pod",
			},
		},
	}
}

func (t *ListPodsInNamespaceTool) Meta() toolv2.ToolMeta {
	return toolv2.TextMeta(t.Name(), "v1", t.Description(), t.Parameters(), toolv2.CapabilityRead, toolv2.RiskNone, toolv2.ConfirmNever, "query")
}

func (t *ListPodsInNamespaceTool) Execute(ctx context.Context, args map[string]any) (toolv2.ToolResult, error) {
	meta := t.Meta()
	text, err := t.executeText(ctx, args)
	if err != nil {
		return toolv2.ToolResult{}, err
	}
	return toolv2.TextResult(meta, text), nil
}

func (t *ListPodsInNamespaceTool) executeText(ctx context.Context, args map[string]any) (string, error) {
	namespace := ""
	if ns, ok := args["namespace"].(string); ok {
		namespace = ns
	}

	pods, err := t.k8sClient.ListPods(ctx, namespace)
	if err != nil {
		return "", fmt.Errorf("获取Pod列表失败: %w", err)
	}

	var result strings.Builder
	if namespace == "" {
		result.WriteString("所有命名空间的Pod列表:\n")
	} else {
		result.WriteString(fmt.Sprintf("命名空间 %s 的Pod列表:\n", namespace))
	}
	result.WriteString("命名空间\t名称\t状态\tIP\t节点\n")
	result.WriteString("----------------------------------------\n")

	for _, pod := range pods {
		podIP := pod.Status.PodIP
		nodeName := pod.Spec.NodeName
		result.WriteString(fmt.Sprintf("%s\t%s\t%s\t%s\t%s\n", pod.Namespace, pod.Name, pod.Status.Phase, podIP, nodeName))
	}

	result.WriteString(fmt.Sprintf("\n总计: %d个Pod", len(pods)))
	return result.String(), nil
}
