package query

import (
	"context"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"

	"github.com/kubewise/kubewise/pkg/k8s"
	"github.com/kubewise/kubewise/pkg/tool"
)

// ListPersistentVolumeClaimsTool 列出PVC工具
type ListPersistentVolumeClaimsTool struct {
	k8sClient *k8s.Client
}

// NewListPersistentVolumeClaimsTool 创建列出PVC工具实例
func NewListPersistentVolumeClaimsTool(k8sClient *k8s.Client) *ListPersistentVolumeClaimsTool {
	return &ListPersistentVolumeClaimsTool{k8sClient: k8sClient}
}

// Name 返回工具唯一标识
func (t *ListPersistentVolumeClaimsTool) Name() string {
	return "list_persistent_volume_claims"
}

// Description 返回工具功能描述
func (t *ListPersistentVolumeClaimsTool) Description() string {
	return "列出指定命名空间下的所有PVC（持久卷声明），不指定命名空间则列出所有命名空间的PVC"
}

// Parameters 返回工具参数定义（JSON Schema格式）
func (t *ListPersistentVolumeClaimsTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"namespace": map[string]any{
				"type":        "string",
				"description": "命名空间，可选，不指定则列出所有命名空间的PVC",
			},
		},
	}
}

// Execute 执行工具调用
func (t *ListPersistentVolumeClaimsTool) Execute(ctx context.Context, args map[string]any) (string, error) {
	namespace := ""
	if ns, ok := args["namespace"].(string); ok {
		namespace = ns
	}

	var pvcs []corev1.PersistentVolumeClaim
	var err error
	pvcs, err = t.k8sClient.ListPersistentVolumeClaims(ctx, namespace)
	if err != nil {
		return "", fmt.Errorf("获取PVC列表失败: %w", err)
	}

	var result strings.Builder
	if namespace == "" {
		result.WriteString("所有命名空间的PVC列表:\n")
	} else {
		result.WriteString(fmt.Sprintf("命名空间 %s 的PVC列表:\n", namespace))
	}
	result.WriteString("命名空间\t名称\t状态\t容量\t存储类\t接入模式\t创建时间\n")
	result.WriteString("----------------------------------------------------------------------------------------------------\n")

	for _, pvc := range pvcs {
		capacity := pvc.Status.Capacity.Storage().String()
		storageClass := ""
		if pvc.Spec.StorageClassName != nil {
			storageClass = *pvc.Spec.StorageClassName
		}
		accessModes := make([]string, len(pvc.Status.AccessModes))
		for i, am := range pvc.Status.AccessModes {
			accessModes[i] = string(am)
		}
		accessModesStr := strings.Join(accessModes, ",")

		result.WriteString(fmt.Sprintf("%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			pvc.Namespace, pvc.Name, pvc.Status.Phase, capacity, storageClass, accessModesStr,
			pvc.CreationTimestamp.Format("2006-01-02 15:04:05")))
	}

	result.WriteString(fmt.Sprintf("\n总计: %d个PVC", len(pvcs)))
	return result.String(), nil
}

// 注册工具到全局注册中心
func init() {
	tool.RegisterGlobal(tool.ToolMetadata{
		Name:        "list_persistent_volume_claims",
		Description: "列出指定命名空间下的所有PVC（持久卷声明），不指定命名空间则列出所有命名空间的PVC",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"namespace": map[string]any{
					"type":        "string",
					"description": "命名空间，可选，不指定则列出所有命名空间的PVC",
				},
			},
		},
		Factory: func(dep any) (tool.Tool, error) {
			toolDep, ok := dep.(tool.ToolDependency)
			if !ok {
				return nil, fmt.Errorf("invalid dependency type")
			}
			return NewListPersistentVolumeClaimsTool(toolDep.K8sClient), nil
		},
	})
}
