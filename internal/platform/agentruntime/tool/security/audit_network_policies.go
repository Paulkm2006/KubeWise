package security

import (
	"context"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"

	toolv2 "github.com/kubewise/kubewise/internal/platform/agentruntime/tool/v2"
	"github.com/kubewise/kubewise/internal/platform/cluster"
)

// AuditNetworkPoliciesTool 网络策略审计工具
type AuditNetworkPoliciesTool struct {
	k8sClient *cluster.Client
}

// NewAuditNetworkPoliciesTool 创建网络策略审计工具实例
func NewAuditNetworkPoliciesTool(k8sClient *cluster.Client) *AuditNetworkPoliciesTool {
	return &AuditNetworkPoliciesTool{k8sClient: k8sClient}
}

// Name 返回工具唯一标识
func (t *AuditNetworkPoliciesTool) Name() string { return "audit_network_policies" }

// Description 返回工具功能描述
func (t *AuditNetworkPoliciesTool) Description() string {
	return "审计Kubernetes网络策略，检查缺少NetworkPolicy的命名空间以及未被任何NetworkPolicy覆盖的Pod"
}

// Parameters 返回工具参数定义（JSON Schema格式）
func (t *AuditNetworkPoliciesTool) Parameters() map[string]any {
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

// Meta returns native toolv2 metadata.
func (t *AuditNetworkPoliciesTool) Meta() toolv2.ToolMeta {
	return toolv2.TextMeta(
		t.Name(),
		"v1",
		t.Description(),
		t.Parameters(),
		toolv2.CapabilityAudit,
		toolv2.RiskLow,
		toolv2.ConfirmNever,
		"security",
		"audit",
	)
}

// Execute 执行工具调用
func (t *AuditNetworkPoliciesTool) Execute(ctx context.Context, args map[string]any) (toolv2.ToolResult, error) {
	meta := t.Meta()
	text, err := t.executeText(ctx, args)
	if err != nil {
		return toolv2.ToolResult{}, err
	}
	return toolv2.TextResult(meta, text), nil
}

func (t *AuditNetworkPoliciesTool) executeText(ctx context.Context, args map[string]any) (string, error) {
	namespace, _ := args["namespace"].(string)

	var namespaces []corev1.Namespace
	if namespace == "" {
		var err error
		namespaces, err = t.k8sClient.ListNamespaces(ctx)
		if err != nil {
			return "", fmt.Errorf("获取命名空间列表失败: %w", err)
		}
	} else {
		namespaces = []corev1.Namespace{{ObjectMeta: metav1.ObjectMeta{Name: namespace}}}
	}

	networkPolicies, err := t.k8sClient.ListNetworkPolicies(ctx, namespace)
	if err != nil {
		return "", fmt.Errorf("获取NetworkPolicy列表失败: %w", err)
	}
	pods, err := t.k8sClient.ListPods(ctx, namespace)
	if err != nil {
		return "", fmt.Errorf("获取Pod列表失败: %w", err)
	}

	return analyzeNetworkPolicies(namespaces, networkPolicies, pods), nil
}

func analyzeNetworkPolicies(
	namespaces []corev1.Namespace,
	networkPolicies []networkingv1.NetworkPolicy,
	pods []corev1.Pod,
) string {
	systemNamespaces := map[string]struct{}{
		"kube-system": {}, "kube-public": {}, "kube-node-lease": {},
	}
	var findings []string

	nsHasPolicy := make(map[string]struct{})
	for _, np := range networkPolicies {
		nsHasPolicy[np.Namespace] = struct{}{}
	}

	// [HIGH] 命名空间无 NetworkPolicy
	for _, ns := range namespaces {
		if _, inSystem := systemNamespaces[ns.Name]; inSystem {
			continue
		}
		if _, hasPolicy := nsHasPolicy[ns.Name]; !hasPolicy {
			findings = append(findings, fmt.Sprintf(
				"[HIGH] 命名空间 %q 没有任何 NetworkPolicy", ns.Name,
			))
		}
	}

	// [MEDIUM] Pod 未被任何 NetworkPolicy 选中
	for _, pod := range pods {
		if _, inSystem := systemNamespaces[pod.Namespace]; inSystem {
			continue
		}
		if !podSelectedByAnyPolicy(pod, networkPolicies) {
			findings = append(findings, fmt.Sprintf(
				"[MEDIUM] Pod %s/%s 未被任何 NetworkPolicy 覆盖", pod.Namespace, pod.Name,
			))
		}
	}

	if len(findings) == 0 {
		return "=== 网络策略审计结果 ===\n\n未发现安全问题"
	}
	return fmt.Sprintf("=== 网络策略审计结果 ===\n\n%s\n\n共发现 %d 个安全问题",
		strings.Join(findings, "\n"), len(findings))
}

func podSelectedByAnyPolicy(pod corev1.Pod, policies []networkingv1.NetworkPolicy) bool {
	for _, policy := range policies {
		if policy.Namespace != pod.Namespace {
			continue
		}
		selector, err := metav1.LabelSelectorAsSelector(&policy.Spec.PodSelector)
		if err != nil {
			continue
		}
		if selector.Matches(labels.Set(pod.Labels)) {
			return true
		}
	}
	return false
}
