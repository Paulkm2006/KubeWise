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
