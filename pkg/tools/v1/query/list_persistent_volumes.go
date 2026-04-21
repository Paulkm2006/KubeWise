package query

import (
	"context"
	"fmt"
	"strings"

	"github.com/kubewise/kubewise/pkg/k8s"
	"github.com/kubewise/kubewise/pkg/tool"
)

// ListPersistentVolumesTool 列出PV工具
type ListPersistentVolumesTool struct {
	k8sClient *k8s.Client
}

// NewListPersistentVolumesTool 创建列出PV工具实例
func NewListPersistentVolumesTool(k8sClient *k8s.Client) *ListPersistentVolumesTool {
	return &ListPersistentVolumesTool{k8sClient: k8sClient}
}

// Name 返回工具唯一标识
func (t *ListPersistentVolumesTool) Name() string {
	return "list_persistent_volumes"
}

// Description 返回工具功能描述
func (t *ListPersistentVolumesTool) Description() string {
	return "获取集群中所有PV的列表信息"
}

// Parameters 返回工具参数定义（JSON Schema格式）
func (t *ListPersistentVolumesTool) Parameters() map[string]any {
	return map[string]any{
		"type":       "object",
		"properties": map[string]any{},
	}
}

// Execute 执行工具调用
func (t *ListPersistentVolumesTool) Execute(ctx context.Context, args map[string]any) (string, error) {
	pvs, err := t.k8sClient.ListPersistentVolumes(ctx)
	if err != nil {
		return "", fmt.Errorf("获取PV列表失败: %w", err)
	}

	var result strings.Builder
	result.WriteString("PV列表:\n")
	result.WriteString("名称\t\t容量\t状态\t绑定PVC\t存储类\n")
	result.WriteString("----------------------------------------\n")

	for _, pv := range pvs {
		capacity := pv.Spec.Capacity.Storage().String()
		status := string(pv.Status.Phase)
		claimRef := ""
		if pv.Spec.ClaimRef != nil {
			claimRef = fmt.Sprintf("%s/%s", pv.Spec.ClaimRef.Namespace, pv.Spec.ClaimRef.Name)
		}
		storageClass := pv.Spec.StorageClassName
		result.WriteString(fmt.Sprintf("%s\t%s\t%s\t%s\t%s\n", pv.Name, capacity, status, claimRef, storageClass))
	}

	return result.String(), nil
}

// 注册工具到全局注册中心
func init() {
	tool.RegisterGlobal(tool.ToolMetadata{
		Name:        "list_persistent_volumes",
		Description: "获取集群中所有PV的列表信息",
		Parameters: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
		Factory: func(dep any) (tool.Tool, error) {
			toolDep, ok := dep.(tool.ToolDependency)
			if !ok {
				return nil, fmt.Errorf("invalid dependency type")
			}
			return NewListPersistentVolumesTool(toolDep.K8sClient), nil
		},
	})
}
