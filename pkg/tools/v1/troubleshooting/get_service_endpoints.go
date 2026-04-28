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
