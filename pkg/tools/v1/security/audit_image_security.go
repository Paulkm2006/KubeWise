package security

import (
	"context"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"

	"github.com/kubewise/kubewise/pkg/k8s"
	"github.com/kubewise/kubewise/pkg/tool"
)

// AuditImageSecurityTool 镜像安全审计工具
type AuditImageSecurityTool struct {
	k8sClient *k8s.Client
}

// NewAuditImageSecurityTool 创建镜像安全审计工具实例
func NewAuditImageSecurityTool(k8sClient *k8s.Client) *AuditImageSecurityTool {
	return &AuditImageSecurityTool{k8sClient: k8sClient}
}

// Name 返回工具唯一标识
func (t *AuditImageSecurityTool) Name() string { return "audit_image_security" }

// Description 返回工具功能描述
func (t *AuditImageSecurityTool) Description() string {
	return "审计Pod镜像安全，检查latest标签使用、imagePullPolicy:Never配置、缺少imagePullSecrets等风险"
}

// Parameters 返回工具参数定义（JSON Schema格式）
func (t *AuditImageSecurityTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"namespace": map[string]any{
				"type":        "string",
				"description": "审计范围的命名空间，不填则审计所有命名空间",
			},
		},
	}
}

// Execute 执行工具调用
func (t *AuditImageSecurityTool) Execute(ctx context.Context, args map[string]any) (string, error) {
	namespace, _ := args["namespace"].(string)
	pods, err := t.k8sClient.ListPods(ctx, namespace)
	if err != nil {
		return "", fmt.Errorf("获取Pod列表失败: %w", err)
	}
	return analyzeImageSecurity(pods), nil
}

func analyzeImageSecurity(pods []corev1.Pod) string {
	systemNamespaces := map[string]struct{}{
		"kube-system": {}, "kube-public": {}, "kube-node-lease": {},
	}
	var findings []string

	for _, pod := range pods {
		podRef := fmt.Sprintf("Pod %s/%s", pod.Namespace, pod.Name)
		_, inSystem := systemNamespaces[pod.Namespace]

		// [LOW] 非系统命名空间中无 imagePullSecrets
		if !inSystem && len(pod.Spec.ImagePullSecrets) == 0 {
			findings = append(findings, fmt.Sprintf(
				"[LOW] %s: 未配置 imagePullSecrets", podRef,
			))
		}

		allContainers := make([]corev1.Container, 0, len(pod.Spec.InitContainers)+len(pod.Spec.Containers))
		allContainers = append(allContainers, pod.Spec.InitContainers...)
		allContainers = append(allContainers, pod.Spec.Containers...)
		for _, c := range allContainers {
			cRef := fmt.Sprintf("%s 容器 %q", podRef, c.Name)

			// [MEDIUM] latest 标签或无标签（摘要固定的除外）
			if imageHasUnsafeTag(c.Image) {
				findings = append(findings, fmt.Sprintf(
					"[MEDIUM] %s: 镜像 %q 使用 latest 标签或未指定标签", cRef, c.Image,
				))
			}

			// [LOW] imagePullPolicy: Never
			if c.ImagePullPolicy == corev1.PullNever {
				findings = append(findings, fmt.Sprintf(
					"[LOW] %s: imagePullPolicy 设置为 Never", cRef,
				))
			}
		}
	}

	if len(findings) == 0 {
		return "=== 镜像安全审计结果 ===\n\n未发现安全问题"
	}
	return fmt.Sprintf("=== 镜像安全审计结果 ===\n\n%s\n\n共发现 %d 个安全问题",
		strings.Join(findings, "\n"), len(findings))
}

// imageHasUnsafeTag returns true for images without a pinned tag or using "latest".
// Digest-pinned images (containing "@") are considered safe.
func imageHasUnsafeTag(image string) bool {
	if strings.Contains(image, "@") {
		return false
	}
	parts := strings.Split(image, "/")
	lastPart := parts[len(parts)-1]
	colonIdx := strings.LastIndex(lastPart, ":")
	if colonIdx == -1 {
		return true
	}
	tag := lastPart[colonIdx+1:]
	return tag == "latest" || tag == ""
}

func init() {
	tool.RegisterGlobal(tool.ToolMetadata{
		Name:        "audit_image_security",
		Description: "审计Pod镜像安全，检查latest标签使用、imagePullPolicy:Never配置、缺少imagePullSecrets等风险",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"namespace": map[string]any{
					"type":        "string",
					"description": "审计范围的命名空间，不填则审计所有命名空间",
				},
			},
		},
		Factory: func(dep any) (tool.Tool, error) {
			toolDep, ok := dep.(tool.ToolDependency)
			if !ok {
				return nil, fmt.Errorf("invalid dependency type")
			}
			return NewAuditImageSecurityTool(toolDep.K8sClient), nil
		},
	})
}
