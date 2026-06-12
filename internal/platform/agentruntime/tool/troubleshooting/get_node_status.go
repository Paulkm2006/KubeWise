package troubleshooting

import (
	"context"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"

	toolv2 "github.com/kubewise/kubewise/internal/platform/agentruntime/tool/v2"
	"github.com/kubewise/kubewise/internal/platform/cluster"
)

// GetNodeStatusTool 获取节点状态工具
type GetNodeStatusTool struct {
	k8sClient *cluster.Client
}

// NewGetNodeStatusTool 创建获取节点状态工具实例
func NewGetNodeStatusTool(k8sClient *cluster.Client) *GetNodeStatusTool {
	return &GetNodeStatusTool{k8sClient: k8sClient}
}

// Name 返回工具唯一标识
func (t *GetNodeStatusTool) Name() string { return "get_node_status" }

// Description 返回工具功能描述
func (t *GetNodeStatusTool) Description() string {
	return "获取集群所有节点的状态、资源压力和可分配资源，用于排查Pod调度失败、节点资源不足等问题"
}

// Parameters 返回工具参数定义（JSON Schema格式）
func (t *GetNodeStatusTool) Parameters() map[string]any {
	return map[string]any{
		"type":       "object",
		"properties": map[string]any{},
	}
}

func (t *GetNodeStatusTool) Meta() toolv2.ToolMeta {
	return toolv2.TextMeta(
		t.Name(),
		"v2",
		t.Description(),
		t.Parameters(),
		toolv2.CapabilityRead,
		toolv2.RiskNone,
		toolv2.ConfirmNever,
		"troubleshooting",
		"recovery",
	)
}

// Execute 执行工具调用
func (t *GetNodeStatusTool) Execute(ctx context.Context, args map[string]any) (toolv2.ToolResult, error) {
	meta := t.Meta()
	text, err := t.executeText(ctx, args)
	if err != nil {
		return toolv2.ToolResult{}, err
	}
	return toolv2.TextResult(meta, text), nil
}

func (t *GetNodeStatusTool) executeText(ctx context.Context, args map[string]any) (string, error) {
	nodes, err := t.k8sClient.ListNodes(ctx)
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

		condVal := func(t corev1.NodeConditionType) string {
			if v, ok := conditions[t]; ok {
				return v
			}
			return "Unknown"
		}

		allocCPU := node.Status.Allocatable.Cpu().String()
		allocMem := node.Status.Allocatable.Memory().String()

		result.WriteString(fmt.Sprintf("%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			node.Name,
			condVal(corev1.NodeReady),
			condVal(corev1.NodeMemoryPressure),
			condVal(corev1.NodeDiskPressure),
			condVal(corev1.NodePIDPressure),
			allocCPU, allocMem))
	}

	result.WriteString(fmt.Sprintf("\n总计: %d个节点", len(nodes)))
	return result.String(), nil
}
