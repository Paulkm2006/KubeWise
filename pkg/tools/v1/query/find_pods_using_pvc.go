package query

import (
	"context"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"

	"github.com/kubewise/kubewise/pkg/k8s"
	"github.com/kubewise/kubewise/pkg/tool"
)

// FindPodsUsingPVCTool 查找使用指定PVC的Pod工具
type FindPodsUsingPVCTool struct {
	k8sClient *k8s.Client
}

// NewFindPodsUsingPVCTool 创建查找Pod工具实例
func NewFindPodsUsingPVCTool(k8sClient *k8s.Client) *FindPodsUsingPVCTool {
	return &FindPodsUsingPVCTool{k8sClient: k8sClient}
}

// Name 返回工具唯一标识
func (t *FindPodsUsingPVCTool) Name() string {
	return "find_pods_using_pvc"
}

// Description 返回工具功能描述
func (t *FindPodsUsingPVCTool) Description() string {
	return "查找使用指定PVC的所有Pod"
}

// Parameters 返回工具参数定义（JSON Schema格式）
func (t *FindPodsUsingPVCTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"pvcName": map[string]any{
				"type":        "string",
				"description": "PVC名称",
			},
			"namespace": map[string]any{
				"type":        "string",
				"description": "PVC所在的命名空间，可选，不指定则在所有命名空间中查找",
			},
		},
		"required": []string{"pvcName"},
	}
}

// Execute 执行工具调用
func (t *FindPodsUsingPVCTool) Execute(ctx context.Context, args map[string]any) (string, error) {
	pvcName, ok := args["pvcName"].(string)
	if !ok || pvcName == "" {
		return "", fmt.Errorf("参数pvcName不能为空")
	}

	namespace := ""
	if ns, ok := args["namespace"].(string); ok {
		namespace = ns
	}

	pods, err := t.k8sClient.ListPods(ctx, namespace)
	if err != nil {
		return "", fmt.Errorf("获取Pod列表失败: %w", err)
	}

	var usingPods []corev1.Pod
	for _, pod := range pods {
		for _, volume := range pod.Spec.Volumes {
			if volume.PersistentVolumeClaim != nil && volume.PersistentVolumeClaim.ClaimName == pvcName {
				usingPods = append(usingPods, pod)
				break
			}
		}
	}

	if len(usingPods) == 0 {
		if namespace == "" {
			return fmt.Sprintf("没有找到使用PVC名称为 %s 的Pod（在所有命名空间中查找）", pvcName), nil
		}
		return fmt.Sprintf("没有找到使用PVC %s/%s 的Pod", namespace, pvcName), nil
	}

	var result strings.Builder
	if namespace == "" {
		result.WriteString(fmt.Sprintf("在所有命名空间中找到使用PVC名称为 %s 的Pod:\n", pvcName))
	} else {
		result.WriteString(fmt.Sprintf("使用PVC %s/%s 的Pod:\n", namespace, pvcName))
	}
	for _, pod := range usingPods {
		result.WriteString(fmt.Sprintf("- %s/%s (状态: %s)\n", pod.Namespace, pod.Name, pod.Status.Phase))
	}

	return result.String(), nil
}

// 注册工具到全局注册中心
func init() {
	tool.RegisterGlobal(tool.ToolMetadata{
		Name:        "find_pods_using_pvc",
		Description: "查找使用指定PVC的所有Pod，不指定命名空间则在所有命名空间中查找",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"pvcName": map[string]any{
					"type":        "string",
					"description": "PVC名称",
				},
				"namespace": map[string]any{
					"type":        "string",
					"description": "PVC所在的命名空间，可选，不指定则在所有命名空间中查找",
				},
			},
			"required": []string{"pvcName"},
		},
		Factory: func(dep any) (tool.Tool, error) {
			toolDep, ok := dep.(tool.ToolDependency)
			if !ok {
				return nil, fmt.Errorf("invalid dependency type")
			}
			return NewFindPodsUsingPVCTool(toolDep.K8sClient), nil
		},
	})
}
