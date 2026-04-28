package security

import (
	"context"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"

	"github.com/kubewise/kubewise/pkg/k8s"
	"github.com/kubewise/kubewise/pkg/tool"
)

// AuditPodSecurityTool Pod安全审计工具
type AuditPodSecurityTool struct {
	k8sClient *k8s.Client
}

// NewAuditPodSecurityTool 创建Pod安全审计工具实例
func NewAuditPodSecurityTool(k8sClient *k8s.Client) *AuditPodSecurityTool {
	return &AuditPodSecurityTool{k8sClient: k8sClient}
}

// Name 返回工具唯一标识
func (t *AuditPodSecurityTool) Name() string { return "audit_pod_security" }

// Description 返回工具功能描述
func (t *AuditPodSecurityTool) Description() string {
	return "审计Pod安全配置，检查privileged容器、hostNetwork/hostPID/hostIPC、allowPrivilegeEscalation、root用户运行、hostPath挂载等安全风险"
}

// Parameters 返回工具参数定义（JSON Schema格式）
func (t *AuditPodSecurityTool) Parameters() map[string]any {
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
func (t *AuditPodSecurityTool) Execute(ctx context.Context, args map[string]any) (string, error) {
	namespace, _ := args["namespace"].(string)
	pods, err := t.k8sClient.ListPods(ctx, namespace)
	if err != nil {
		return "", fmt.Errorf("获取Pod列表失败: %w", err)
	}
	return analyzePodSecurity(pods), nil
}

func analyzePodSecurity(pods []corev1.Pod) string {
	var findings []string

	for _, pod := range pods {
		podRef := fmt.Sprintf("Pod %s/%s", pod.Namespace, pod.Name)

		// [HIGH] hostNetwork / hostPID / hostIPC
		if pod.Spec.HostNetwork {
			findings = append(findings, fmt.Sprintf("[HIGH] %s: 使用 hostNetwork", podRef))
		}
		if pod.Spec.HostPID {
			findings = append(findings, fmt.Sprintf("[HIGH] %s: 使用 hostPID", podRef))
		}
		if pod.Spec.HostIPC {
			findings = append(findings, fmt.Sprintf("[HIGH] %s: 使用 hostIPC", podRef))
		}

		// [MEDIUM] hostPath 卷
		for _, vol := range pod.Spec.Volumes {
			if vol.HostPath != nil {
				findings = append(findings, fmt.Sprintf(
					"[MEDIUM] %s: 卷 %q 使用 hostPath (%s)", podRef, vol.Name, vol.HostPath.Path,
				))
			}
		}

		// 容器级检查（包含 initContainers）
		allContainers := append(pod.Spec.InitContainers, pod.Spec.Containers...)
		for _, c := range allContainers {
			cRef := fmt.Sprintf("%s 容器 %q", podRef, c.Name)

			// [CRITICAL] privileged
			if c.SecurityContext != nil && c.SecurityContext.Privileged != nil && *c.SecurityContext.Privileged {
				findings = append(findings, fmt.Sprintf("[CRITICAL] %s: 以 privileged 模式运行", cRef))
			}

			// [HIGH] allowPrivilegeEscalation 为 true 或未设置
			ape := true // default: vulnerable
			if c.SecurityContext != nil && c.SecurityContext.AllowPrivilegeEscalation != nil {
				ape = *c.SecurityContext.AllowPrivilegeEscalation
			}
			if ape {
				findings = append(findings, fmt.Sprintf("[HIGH] %s: allowPrivilegeEscalation 为 true 或未设置", cRef))
			}

			// [HIGH] 可能以 root 用户运行
			mayRunAsRoot := true
			if c.SecurityContext != nil {
				if c.SecurityContext.RunAsNonRoot != nil && *c.SecurityContext.RunAsNonRoot {
					mayRunAsRoot = false
				}
				if c.SecurityContext.RunAsUser != nil && *c.SecurityContext.RunAsUser != 0 {
					mayRunAsRoot = false
				}
				if c.SecurityContext.RunAsUser != nil && *c.SecurityContext.RunAsUser == 0 {
					mayRunAsRoot = true
				}
			}
			if mayRunAsRoot {
				findings = append(findings, fmt.Sprintf("[HIGH] %s: 可能以 root 用户运行（未设置 runAsNonRoot 或 runAsUser）", cRef))
			}
		}
	}

	if len(findings) == 0 {
		return "=== Pod安全审计结果 ===\n\n未发现安全问题"
	}
	return fmt.Sprintf("=== Pod安全审计结果 ===\n\n%s\n\n共发现 %d 个安全问题",
		strings.Join(findings, "\n"), len(findings))
}

func init() {
	tool.RegisterGlobal(tool.ToolMetadata{
		Name:        "audit_pod_security",
		Description: "审计Pod安全配置，检查privileged容器、hostNetwork/hostPID/hostIPC、allowPrivilegeEscalation、root用户运行、hostPath挂载等安全风险",
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
			return NewAuditPodSecurityTool(toolDep.K8sClient), nil
		},
	})
}
