package query

import (
	"context"
	"fmt"
	"sort"

	"github.com/kubewise/kubewise/pkg/k8s"
	"github.com/kubewise/kubewise/pkg/tool"
)

// GetLargestPVTool 获取最大PV工具
type GetLargestPVTool struct {
	k8sClient *k8s.Client
}

// NewGetLargestPVTool 创建获取最大PV工具实例
func NewGetLargestPVTool(k8sClient *k8s.Client) *GetLargestPVTool {
	return &GetLargestPVTool{k8sClient: k8sClient}
}

// Name 返回工具唯一标识
func (t *GetLargestPVTool) Name() string {
	return "get_largest_pv"
}

// Description 返回工具功能描述
func (t *GetLargestPVTool) Description() string {
	return "获取集群中容量最大的PV信息"
}

// Parameters 返回工具参数定义（JSON Schema格式）
func (t *GetLargestPVTool) Parameters() map[string]any {
	return map[string]any{
		"type":       "object",
		"properties": map[string]any{},
	}
}

// Execute 执行工具调用
func (t *GetLargestPVTool) Execute(ctx context.Context, args map[string]any) (string, error) {
	pvs, err := t.k8sClient.ListPersistentVolumes(ctx)
	if err != nil {
		return "", fmt.Errorf("获取PV列表失败: %w", err)
	}

	if len(pvs) == 0 {
		return "集群中没有PV", nil
	}

	// 按容量排序
	sort.Slice(pvs, func(i, j int) bool {
		capI := pvs[i].Spec.Capacity.Storage().Value()
		capJ := pvs[j].Spec.Capacity.Storage().Value()
		return capI > capJ
	})

	largestPV := pvs[0]
	capacity := largestPV.Spec.Capacity.Storage().String()
	status := string(largestPV.Status.Phase)
	claimRef := ""
	if largestPV.Spec.ClaimRef != nil {
		claimRef = fmt.Sprintf("%s/%s", largestPV.Spec.ClaimRef.Namespace, largestPV.Spec.ClaimRef.Name)
	}

	return fmt.Sprintf("最大的PV是: %s\n容量: %s\n状态: %s\n绑定PVC: %s",
		largestPV.Name, capacity, status, claimRef), nil
}

// 注册工具到全局注册中心
func init() {
	tool.RegisterGlobal(tool.ToolMetadata{
		Name:        "get_largest_pv",
		Description: "获取集群中容量最大的PV信息",
		Parameters: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
		Factory: func(dep any) (tool.Tool, error) {
			toolDep, ok := dep.(tool.ToolDependency)
			if !ok {
				return nil, fmt.Errorf("invalid dependency type")
			}
			return NewGetLargestPVTool(toolDep.K8sClient), nil
		},
	})
}
