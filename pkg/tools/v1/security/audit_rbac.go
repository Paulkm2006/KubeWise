package security

import (
	"context"
	"fmt"
	"slices"
	"strings"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"

	"github.com/kubewise/kubewise/pkg/k8s"
	"github.com/kubewise/kubewise/pkg/tool"
)

// AuditRBACTool RBAC安全审计工具
type AuditRBACTool struct {
	k8sClient *k8s.Client
}

// NewAuditRBACTool 创建RBAC审计工具实例
func NewAuditRBACTool(k8sClient *k8s.Client) *AuditRBACTool {
	return &AuditRBACTool{k8sClient: k8sClient}
}

// Name 返回工具唯一标识
func (t *AuditRBACTool) Name() string { return "audit_rbac" }

// Description 返回工具功能描述
func (t *AuditRBACTool) Description() string {
	return "审计Kubernetes RBAC配置，检查cluster-admin滥用、通配符权限、exec/portforward授权、孤立ServiceAccount等安全风险"
}

// Parameters 返回工具参数定义（JSON Schema格式）
func (t *AuditRBACTool) Parameters() map[string]any {
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
func (t *AuditRBACTool) Execute(ctx context.Context, args map[string]any) (string, error) {
	namespace, _ := args["namespace"].(string)

	roles, err := t.k8sClient.ListRoles(ctx, namespace)
	if err != nil {
		return "", fmt.Errorf("获取Role失败: %w", err)
	}
	clusterRoles, err := t.k8sClient.ListClusterRoles(ctx)
	if err != nil {
		return "", fmt.Errorf("获取ClusterRole失败: %w", err)
	}
	roleBindings, err := t.k8sClient.ListRoleBindings(ctx, namespace)
	if err != nil {
		return "", fmt.Errorf("获取RoleBinding失败: %w", err)
	}
	clusterRoleBindings, err := t.k8sClient.ListClusterRoleBindings(ctx)
	if err != nil {
		return "", fmt.Errorf("获取ClusterRoleBinding失败: %w", err)
	}
	serviceAccounts, err := t.k8sClient.ListServiceAccounts(ctx, namespace)
	if err != nil {
		return "", fmt.Errorf("获取ServiceAccount失败: %w", err)
	}

	return analyzeRBAC(roles, clusterRoles, roleBindings, clusterRoleBindings, serviceAccounts), nil
}

func analyzeRBAC(
	roles []rbacv1.Role,
	clusterRoles []rbacv1.ClusterRole,
	roleBindings []rbacv1.RoleBinding,
	clusterRoleBindings []rbacv1.ClusterRoleBinding,
	serviceAccounts []corev1.ServiceAccount,
) string {
	var findings []string

	// [CRITICAL] cluster-admin 绑定到非系统主体
	for _, crb := range clusterRoleBindings {
		if crb.RoleRef.Name != "cluster-admin" {
			continue
		}
		for _, subject := range crb.Subjects {
			if !strings.HasPrefix(subject.Name, "system:") {
				findings = append(findings, fmt.Sprintf(
					"[CRITICAL] ClusterRoleBinding %q: 主体 %s %q 拥有 cluster-admin 权限",
					crb.Name, subject.Kind, subject.Name,
				))
			}
		}
	}

	// [HIGH] Role 包含通配符权限
	for _, role := range roles {
		for _, rule := range role.Rules {
			hasWildcardVerb := slices.Contains(rule.Verbs, "*")
			hasWildcardResource := slices.Contains(rule.Resources, "*")
			if hasWildcardVerb || hasWildcardResource {
				findings = append(findings, fmt.Sprintf(
					"[HIGH] Role %s/%s: 包含通配符%s",
					role.Namespace, role.Name, wildcardDesc(hasWildcardVerb, hasWildcardResource),
				))
			}
		}
	}

	// [HIGH] ClusterRole 包含通配符权限（跳过系统 ClusterRole）
	for _, cr := range clusterRoles {
		if strings.HasPrefix(cr.Name, "system:") {
			continue
		}
		for _, rule := range cr.Rules {
			hasWildcardVerb := slices.Contains(rule.Verbs, "*")
			hasWildcardResource := slices.Contains(rule.Resources, "*")
			if hasWildcardVerb || hasWildcardResource {
				findings = append(findings, fmt.Sprintf(
					"[HIGH] ClusterRole %q: 包含通配符%s",
					cr.Name, wildcardDesc(hasWildcardVerb, hasWildcardResource),
				))
			}
		}
	}

	// [MEDIUM] 允许 exec/portforward 的 Role
	for _, role := range roles {
		for _, rule := range role.Rules {
			canCreate := slices.Contains(rule.Verbs, "create") || slices.Contains(rule.Verbs, "*")
			if !canCreate {
				continue
			}
			for _, resource := range rule.Resources {
				if resource == "pods/exec" || resource == "pods/portforward" {
					findings = append(findings, fmt.Sprintf(
						"[MEDIUM] Role %s/%s: 允许对 %s 执行 create 操作",
						role.Namespace, role.Name, resource,
					))
				}
			}
		}
	}

	// [MEDIUM] 允许 exec/portforward 的 ClusterRole（跳过系统）
	for _, cr := range clusterRoles {
		if strings.HasPrefix(cr.Name, "system:") {
			continue
		}
		for _, rule := range cr.Rules {
			canCreate := slices.Contains(rule.Verbs, "create") || slices.Contains(rule.Verbs, "*")
			if !canCreate {
				continue
			}
			for _, resource := range rule.Resources {
				if resource == "pods/exec" || resource == "pods/portforward" {
					findings = append(findings, fmt.Sprintf(
						"[MEDIUM] ClusterRole %q: 允许对 %s 执行 create 操作",
						cr.Name, resource,
					))
				}
			}
		}
	}

	// [LOW] 未绑定任何角色的 ServiceAccount（跳过系统命名空间和 default SA）
	boundSAs := make(map[string]struct{})
	for _, rb := range roleBindings {
		for _, subject := range rb.Subjects {
			if subject.Kind == "ServiceAccount" {
				boundSAs[subject.Namespace+"/"+subject.Name] = struct{}{}
			}
		}
	}
	for _, crb := range clusterRoleBindings {
		for _, subject := range crb.Subjects {
			if subject.Kind == "ServiceAccount" {
				boundSAs[subject.Namespace+"/"+subject.Name] = struct{}{}
			}
		}
	}
	systemNamespaces := map[string]struct{}{
		"kube-system": {}, "kube-public": {}, "kube-node-lease": {},
	}
	for _, sa := range serviceAccounts {
		_, inSystem := systemNamespaces[sa.Namespace]
		if inSystem || sa.Name == "default" {
			continue
		}
		if _, ok := boundSAs[sa.Namespace+"/"+sa.Name]; !ok {
			findings = append(findings, fmt.Sprintf(
				"[LOW] ServiceAccount %s/%s: 未绑定任何角色",
				sa.Namespace, sa.Name,
			))
		}
	}

	if len(findings) == 0 {
		return "=== RBAC安全审计结果 ===\n\n未发现安全问题"
	}
	return fmt.Sprintf("=== RBAC安全审计结果 ===\n\n%s\n\n共发现 %d 个安全问题",
		strings.Join(findings, "\n"), len(findings))
}

func wildcardDesc(verb, resource bool) string {
	switch {
	case verb && resource:
		return "动词(*) 和 资源(*)"
	case verb:
		return "动词(*)"
	default:
		return "资源(*)"
	}
}

func init() {
	tool.RegisterGlobal(tool.ToolMetadata{
		Name:        "audit_rbac",
		Description: "审计Kubernetes RBAC配置，检查cluster-admin滥用、通配符权限、exec/portforward授权、孤立ServiceAccount等安全风险",
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
			return NewAuditRBACTool(toolDep.K8sClient), nil
		},
	})
}
