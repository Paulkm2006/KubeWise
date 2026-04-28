# Security Feature Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement the Kubernetes security audit feature — four scanner tools, a SecurityAgent, and router wiring — replacing the current "coming soon" stub.

**Architecture:** A new `SecurityAgent` (mirrors `TroubleshootingAgent`) owns four scanner tools registered via `init()`. Each tool makes direct k8s API calls, applies rule-based analysis via a pure helper function, and returns a structured findings string. The LLM synthesizes findings into natural language.

**Tech Stack:** Go 1.26, `k8s.io/api v0.35`, `k8s.io/client-go v0.35`, `k8s.io/apimachinery v0.35`

---

## File Map

| Path | Action | Responsibility |
| --- | --- | --- |
| `pkg/k8s/client.go` | Modify | Add `ListNetworkPolicies` method |
| `pkg/tools/v1/security/audit_rbac.go` | Create | RBAC scanner tool + `init()` registration |
| `pkg/tools/v1/security/audit_rbac_test.go` | Create | Unit tests for RBAC analysis helper |
| `pkg/tools/v1/security/audit_pod_security.go` | Create | Pod security scanner tool + `init()` registration |
| `pkg/tools/v1/security/audit_pod_security_test.go` | Create | Unit tests for pod security analysis helper |
| `pkg/tools/v1/security/audit_network_policies.go` | Create | Network policy scanner tool + `init()` registration |
| `pkg/tools/v1/security/audit_network_policies_test.go` | Create | Unit tests for network policy analysis helper |
| `pkg/tools/v1/security/audit_image_security.go` | Create | Image security scanner tool + `init()` registration |
| `pkg/tools/v1/security/audit_image_security_test.go` | Create | Unit tests for image security analysis helper |
| `pkg/agent/security/agent.go` | Create | SecurityAgent with 10-step tool-calling loop |
| `pkg/agent/router/agent.go` | Modify | Add `securityAgent` field, replace stub with agent call |

---

### Task 1: Add `ListNetworkPolicies` to the k8s client

**Files:**
- Modify: `pkg/k8s/client.go`

- [ ] **Step 1: Add the method**

Append to `pkg/k8s/client.go`. Add `networkingv1 "k8s.io/api/networking/v1"` to the import block alongside the existing `corev1` import.

```go
// ListNetworkPolicies 获取指定命名空间下的NetworkPolicy（空命名空间表示所有）
func (c *Client) ListNetworkPolicies(ctx context.Context, namespace string) ([]networkingv1.NetworkPolicy, error) {
	npList, err := c.clientset.NetworkingV1().NetworkPolicies(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return npList.Items, nil
}
```

- [ ] **Step 2: Verify it compiles**

```bash
make build
```

Expected: binary built with no errors.

- [ ] **Step 3: Commit**

```bash
git add pkg/k8s/client.go
git commit -m "feat(k8s): add ListNetworkPolicies method"
```

---

### Task 2: Implement `audit_rbac` tool

**Files:**
- Create: `pkg/tools/v1/security/audit_rbac.go`
- Create: `pkg/tools/v1/security/audit_rbac_test.go`

- [ ] **Step 1: Write the failing tests**

Create `pkg/tools/v1/security/audit_rbac_test.go`:

```go
package security

import (
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestAnalyzeRBAC_ClusterAdminNonSystemSubject(t *testing.T) {
	crbs := []rbacv1.ClusterRoleBinding{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "admin-binding"},
			RoleRef:    rbacv1.RoleRef{Name: "cluster-admin"},
			Subjects:   []rbacv1.Subject{{Kind: "User", Name: "alice"}},
		},
	}
	result := analyzeRBAC(nil, nil, nil, crbs, nil)
	if !strings.Contains(result, "[CRITICAL]") {
		t.Errorf("expected CRITICAL finding, got: %s", result)
	}
	if !strings.Contains(result, "alice") {
		t.Errorf("expected subject name 'alice' in result, got: %s", result)
	}
}

func TestAnalyzeRBAC_ClusterAdminSystemSubjectIgnored(t *testing.T) {
	crbs := []rbacv1.ClusterRoleBinding{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "system-binding"},
			RoleRef:    rbacv1.RoleRef{Name: "cluster-admin"},
			Subjects:   []rbacv1.Subject{{Kind: "User", Name: "system:kube-scheduler"}},
		},
	}
	result := analyzeRBAC(nil, nil, nil, crbs, nil)
	if strings.Contains(result, "[CRITICAL]") {
		t.Errorf("expected no CRITICAL finding for system: subject, got: %s", result)
	}
}

func TestAnalyzeRBAC_WildcardVerbInRole(t *testing.T) {
	roles := []rbacv1.Role{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "wild-role", Namespace: "default"},
			Rules: []rbacv1.PolicyRule{
				{Verbs: []string{"*"}, Resources: []string{"pods"}},
			},
		},
	}
	result := analyzeRBAC(roles, nil, nil, nil, nil)
	if !strings.Contains(result, "[HIGH]") {
		t.Errorf("expected HIGH finding for wildcard verb, got: %s", result)
	}
}

func TestAnalyzeRBAC_WildcardResourceInClusterRole(t *testing.T) {
	crs := []rbacv1.ClusterRole{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "wild-cr"},
			Rules: []rbacv1.PolicyRule{
				{Verbs: []string{"get"}, Resources: []string{"*"}},
			},
		},
	}
	result := analyzeRBAC(nil, crs, nil, nil, nil)
	if !strings.Contains(result, "[HIGH]") {
		t.Errorf("expected HIGH finding for wildcard resource, got: %s", result)
	}
}

func TestAnalyzeRBAC_SystemClusterRoleIgnored(t *testing.T) {
	crs := []rbacv1.ClusterRole{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "system:node"},
			Rules: []rbacv1.PolicyRule{
				{Verbs: []string{"*"}, Resources: []string{"*"}},
			},
		},
	}
	result := analyzeRBAC(nil, crs, nil, nil, nil)
	if strings.Contains(result, "[HIGH]") {
		t.Errorf("expected system: clusterrole to be ignored, got: %s", result)
	}
}

func TestAnalyzeRBAC_ExecRole(t *testing.T) {
	roles := []rbacv1.Role{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "exec-role", Namespace: "default"},
			Rules: []rbacv1.PolicyRule{
				{Verbs: []string{"create"}, Resources: []string{"pods/exec"}},
			},
		},
	}
	result := analyzeRBAC(roles, nil, nil, nil, nil)
	if !strings.Contains(result, "[MEDIUM]") {
		t.Errorf("expected MEDIUM finding for exec role, got: %s", result)
	}
}

func TestAnalyzeRBAC_OrphanedServiceAccount(t *testing.T) {
	sas := []corev1.ServiceAccount{
		{ObjectMeta: metav1.ObjectMeta{Name: "unused-sa", Namespace: "default"}},
	}
	result := analyzeRBAC(nil, nil, nil, nil, sas)
	if !strings.Contains(result, "[LOW]") {
		t.Errorf("expected LOW finding for orphaned SA, got: %s", result)
	}
	if !strings.Contains(result, "unused-sa") {
		t.Errorf("expected SA name in result, got: %s", result)
	}
}

func TestAnalyzeRBAC_BoundServiceAccountNotFlagged(t *testing.T) {
	sas := []corev1.ServiceAccount{
		{ObjectMeta: metav1.ObjectMeta{Name: "bound-sa", Namespace: "default"}},
	}
	rbs := []rbacv1.RoleBinding{
		{
			Subjects: []rbacv1.Subject{{Kind: "ServiceAccount", Name: "bound-sa", Namespace: "default"}},
		},
	}
	result := analyzeRBAC(nil, nil, rbs, nil, sas)
	if strings.Contains(result, "bound-sa") {
		t.Errorf("expected bound SA to not be flagged, got: %s", result)
	}
}

func TestAnalyzeRBAC_NoFindings(t *testing.T) {
	result := analyzeRBAC(nil, nil, nil, nil, nil)
	if !strings.Contains(result, "未发现") {
		t.Errorf("expected no-findings message, got: %s", result)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
cd d:/KubeWise && go test ./pkg/tools/v1/security/... -run TestAnalyzeRBAC -v
```

Expected: compilation error — `analyzeRBAC` undefined.

- [ ] **Step 3: Create the implementation**

Create `pkg/tools/v1/security/audit_rbac.go`:

```go
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

func (t *AuditRBACTool) Name() string { return "audit_rbac" }

func (t *AuditRBACTool) Description() string {
	return "审计Kubernetes RBAC配置，检查cluster-admin滥用、通配符权限、exec/portforward授权、孤立ServiceAccount等安全风险"
}

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
	boundSAs := make(map[string]bool)
	for _, rb := range roleBindings {
		for _, subject := range rb.Subjects {
			if subject.Kind == "ServiceAccount" {
				boundSAs[subject.Namespace+"/"+subject.Name] = true
			}
		}
	}
	for _, crb := range clusterRoleBindings {
		for _, subject := range crb.Subjects {
			if subject.Kind == "ServiceAccount" {
				boundSAs[subject.Namespace+"/"+subject.Name] = true
			}
		}
	}
	systemNamespaces := map[string]bool{
		"kube-system": true, "kube-public": true, "kube-node-lease": true,
	}
	for _, sa := range serviceAccounts {
		if systemNamespaces[sa.Namespace] || sa.Name == "default" {
			continue
		}
		if !boundSAs[sa.Namespace+"/"+sa.Name] {
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
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
cd d:/KubeWise && go test ./pkg/tools/v1/security/... -run TestAnalyzeRBAC -v
```

Expected: all 8 tests PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/tools/v1/security/audit_rbac.go pkg/tools/v1/security/audit_rbac_test.go
git commit -m "feat(security): add audit_rbac scanner tool"
```

---

### Task 3: Implement `audit_pod_security` tool

**Files:**
- Create: `pkg/tools/v1/security/audit_pod_security.go`
- Create: `pkg/tools/v1/security/audit_pod_security_test.go`

- [ ] **Step 1: Write the failing tests**

Create `pkg/tools/v1/security/audit_pod_security_test.go`:

```go
package security

import (
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func podWithContainer(namespace, name string, sc *corev1.SecurityContext) corev1.Pod {
	return corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{Name: "app", Image: "nginx:1.25", SecurityContext: sc},
			},
		},
	}
}

func boolPtr(b bool) *bool { return &b }
func int64Ptr(i int64) *int64 { return &i }

func TestAnalyzePodSecurity_Privileged(t *testing.T) {
	pods := []corev1.Pod{
		podWithContainer("default", "priv-pod", &corev1.SecurityContext{
			Privileged: boolPtr(true),
		}),
	}
	result := analyzePodSecurity(pods)
	if !strings.Contains(result, "[CRITICAL]") {
		t.Errorf("expected CRITICAL finding for privileged pod, got: %s", result)
	}
}

func TestAnalyzePodSecurity_HostNetwork(t *testing.T) {
	pods := []corev1.Pod{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "hostnet-pod", Namespace: "default"},
			Spec: corev1.PodSpec{
				HostNetwork: true,
				Containers:  []corev1.Container{{Name: "app", SecurityContext: &corev1.SecurityContext{RunAsNonRoot: boolPtr(true), AllowPrivilegeEscalation: boolPtr(false)}}},
			},
		},
	}
	result := analyzePodSecurity(pods)
	if !strings.Contains(result, "[HIGH]") || !strings.Contains(result, "hostNetwork") {
		t.Errorf("expected HIGH finding for hostNetwork, got: %s", result)
	}
}

func TestAnalyzePodSecurity_HostPath(t *testing.T) {
	pods := []corev1.Pod{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "hostpath-pod", Namespace: "default"},
			Spec: corev1.PodSpec{
				Volumes: []corev1.Volume{
					{Name: "host-vol", VolumeSource: corev1.VolumeSource{HostPath: &corev1.HostPathVolumeSource{Path: "/etc"}}},
				},
				Containers: []corev1.Container{{Name: "app", SecurityContext: &corev1.SecurityContext{RunAsNonRoot: boolPtr(true), AllowPrivilegeEscalation: boolPtr(false)}}},
			},
		},
	}
	result := analyzePodSecurity(pods)
	if !strings.Contains(result, "[MEDIUM]") || !strings.Contains(result, "hostPath") {
		t.Errorf("expected MEDIUM finding for hostPath volume, got: %s", result)
	}
}

func TestAnalyzePodSecurity_AllowPrivilegeEscalationAbsent(t *testing.T) {
	pods := []corev1.Pod{
		podWithContainer("default", "no-ape-pod", &corev1.SecurityContext{
			RunAsNonRoot: boolPtr(true),
			// AllowPrivilegeEscalation not set
		}),
	}
	result := analyzePodSecurity(pods)
	if !strings.Contains(result, "[HIGH]") {
		t.Errorf("expected HIGH finding when allowPrivilegeEscalation is absent, got: %s", result)
	}
}

func TestAnalyzePodSecurity_RunAsRoot(t *testing.T) {
	pods := []corev1.Pod{
		podWithContainer("default", "root-pod", &corev1.SecurityContext{
			RunAsUser:                int64Ptr(0),
			AllowPrivilegeEscalation: boolPtr(false),
		}),
	}
	result := analyzePodSecurity(pods)
	if !strings.Contains(result, "[HIGH]") {
		t.Errorf("expected HIGH finding for runAsUser=0, got: %s", result)
	}
}

func TestAnalyzePodSecurity_SecurePod_NoFindings(t *testing.T) {
	pods := []corev1.Pod{
		podWithContainer("default", "secure-pod", &corev1.SecurityContext{
			Privileged:               boolPtr(false),
			RunAsNonRoot:             boolPtr(true),
			AllowPrivilegeEscalation: boolPtr(false),
		}),
	}
	result := analyzePodSecurity(pods)
	if strings.Contains(result, "[CRITICAL]") || strings.Contains(result, "[HIGH]") {
		t.Errorf("expected no CRITICAL/HIGH findings for secure pod, got: %s", result)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
cd d:/KubeWise && go test ./pkg/tools/v1/security/... -run TestAnalyzePodSecurity -v
```

Expected: compilation error — `analyzePodSecurity` undefined.

- [ ] **Step 3: Create the implementation**

Create `pkg/tools/v1/security/audit_pod_security.go`:

```go
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

func (t *AuditPodSecurityTool) Name() string { return "audit_pod_security" }

func (t *AuditPodSecurityTool) Description() string {
	return "审计Pod安全配置，检查privileged容器、hostNetwork/hostPID/hostIPC、allowPrivilegeEscalation、root用户运行、hostPath挂载等安全风险"
}

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
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
cd d:/KubeWise && go test ./pkg/tools/v1/security/... -run TestAnalyzePodSecurity -v
```

Expected: all 6 tests PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/tools/v1/security/audit_pod_security.go pkg/tools/v1/security/audit_pod_security_test.go
git commit -m "feat(security): add audit_pod_security scanner tool"
```

---

### Task 4: Implement `audit_network_policies` tool

**Files:**
- Create: `pkg/tools/v1/security/audit_network_policies.go`
- Create: `pkg/tools/v1/security/audit_network_policies_test.go`

- [ ] **Step 1: Write the failing tests**

Create `pkg/tools/v1/security/audit_network_policies_test.go`:

```go
package security

import (
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestAnalyzeNetworkPolicies_NamespaceWithNoPolicy(t *testing.T) {
	namespaces := []corev1.Namespace{
		{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
	}
	result := analyzeNetworkPolicies(namespaces, nil, nil)
	if !strings.Contains(result, "[HIGH]") {
		t.Errorf("expected HIGH finding for namespace with no NetworkPolicy, got: %s", result)
	}
	if !strings.Contains(result, "default") {
		t.Errorf("expected namespace name in result, got: %s", result)
	}
}

func TestAnalyzeNetworkPolicies_SystemNamespaceIgnored(t *testing.T) {
	namespaces := []corev1.Namespace{
		{ObjectMeta: metav1.ObjectMeta{Name: "kube-system"}},
	}
	result := analyzeNetworkPolicies(namespaces, nil, nil)
	if strings.Contains(result, "[HIGH]") {
		t.Errorf("expected system namespace to be ignored, got: %s", result)
	}
}

func TestAnalyzeNetworkPolicies_NamespaceWithPolicy_NoHighFinding(t *testing.T) {
	namespaces := []corev1.Namespace{
		{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
	}
	policies := []networkingv1.NetworkPolicy{
		{ObjectMeta: metav1.ObjectMeta{Name: "default-deny", Namespace: "default"}},
	}
	result := analyzeNetworkPolicies(namespaces, policies, nil)
	if strings.Contains(result, "[HIGH]") && strings.Contains(result, "default") {
		t.Errorf("expected no HIGH finding for namespace with a NetworkPolicy, got: %s", result)
	}
}

func TestAnalyzeNetworkPolicies_PodNotSelectedByAnyPolicy(t *testing.T) {
	namespaces := []corev1.Namespace{
		{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
	}
	policies := []networkingv1.NetworkPolicy{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "app-policy", Namespace: "default"},
			Spec: networkingv1.NetworkPolicySpec{
				PodSelector: metav1.LabelSelector{
					MatchLabels: map[string]string{"app": "web"},
				},
			},
		},
	}
	pods := []corev1.Pod{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "other-pod",
				Namespace: "default",
				Labels:    map[string]string{"app": "other"},
			},
		},
	}
	result := analyzeNetworkPolicies(namespaces, policies, pods)
	if !strings.Contains(result, "[MEDIUM]") {
		t.Errorf("expected MEDIUM finding for pod not selected by any policy, got: %s", result)
	}
	if !strings.Contains(result, "other-pod") {
		t.Errorf("expected pod name in result, got: %s", result)
	}
}

func TestAnalyzeNetworkPolicies_PodSelectedByPolicy_NoMediumFinding(t *testing.T) {
	namespaces := []corev1.Namespace{
		{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
	}
	policies := []networkingv1.NetworkPolicy{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "app-policy", Namespace: "default"},
			Spec: networkingv1.NetworkPolicySpec{
				PodSelector: metav1.LabelSelector{
					MatchLabels: map[string]string{"app": "web"},
				},
			},
		},
	}
	pods := []corev1.Pod{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "web-pod",
				Namespace: "default",
				Labels:    map[string]string{"app": "web"},
			},
		},
	}
	result := analyzeNetworkPolicies(namespaces, policies, pods)
	if strings.Contains(result, "web-pod") {
		t.Errorf("expected selected pod to not be flagged, got: %s", result)
	}
}

func TestAnalyzeNetworkPolicies_NoFindings(t *testing.T) {
	result := analyzeNetworkPolicies(nil, nil, nil)
	if !strings.Contains(result, "未发现") {
		t.Errorf("expected no-findings message, got: %s", result)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
cd d:/KubeWise && go test ./pkg/tools/v1/security/... -run TestAnalyzeNetworkPolicies -v
```

Expected: compilation error — `analyzeNetworkPolicies` undefined.

- [ ] **Step 3: Create the implementation**

Create `pkg/tools/v1/security/audit_network_policies.go`:

```go
package security

import (
	"context"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"

	"github.com/kubewise/kubewise/pkg/k8s"
	"github.com/kubewise/kubewise/pkg/tool"
)

// AuditNetworkPoliciesTool 网络策略审计工具
type AuditNetworkPoliciesTool struct {
	k8sClient *k8s.Client
}

// NewAuditNetworkPoliciesTool 创建网络策略审计工具实例
func NewAuditNetworkPoliciesTool(k8sClient *k8s.Client) *AuditNetworkPoliciesTool {
	return &AuditNetworkPoliciesTool{k8sClient: k8sClient}
}

func (t *AuditNetworkPoliciesTool) Name() string { return "audit_network_policies" }

func (t *AuditNetworkPoliciesTool) Description() string {
	return "审计Kubernetes网络策略，检查缺少NetworkPolicy的命名空间以及未被任何NetworkPolicy覆盖的Pod"
}

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

func (t *AuditNetworkPoliciesTool) Execute(ctx context.Context, args map[string]any) (string, error) {
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
	systemNamespaces := map[string]bool{
		"kube-system": true, "kube-public": true, "kube-node-lease": true,
	}
	var findings []string

	// 哪些命名空间有 NetworkPolicy
	nsHasPolicy := make(map[string]bool)
	for _, np := range networkPolicies {
		nsHasPolicy[np.Namespace] = true
	}

	// [HIGH] 命名空间无 NetworkPolicy
	for _, ns := range namespaces {
		if systemNamespaces[ns.Name] {
			continue
		}
		if !nsHasPolicy[ns.Name] {
			findings = append(findings, fmt.Sprintf(
				"[HIGH] 命名空间 %q 没有任何 NetworkPolicy", ns.Name,
			))
		}
	}

	// [MEDIUM] Pod 未被任何 NetworkPolicy 选中
	for _, pod := range pods {
		if systemNamespaces[pod.Namespace] {
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

func init() {
	tool.RegisterGlobal(tool.ToolMetadata{
		Name:        "audit_network_policies",
		Description: "审计Kubernetes网络策略，检查缺少NetworkPolicy的命名空间以及未被任何NetworkPolicy覆盖的Pod",
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
			return NewAuditNetworkPoliciesTool(toolDep.K8sClient), nil
		},
	})
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
cd d:/KubeWise && go test ./pkg/tools/v1/security/... -run TestAnalyzeNetworkPolicies -v
```

Expected: all 6 tests PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/tools/v1/security/audit_network_policies.go pkg/tools/v1/security/audit_network_policies_test.go
git commit -m "feat(security): add audit_network_policies scanner tool"
```

---

### Task 5: Implement `audit_image_security` tool

**Files:**
- Create: `pkg/tools/v1/security/audit_image_security.go`
- Create: `pkg/tools/v1/security/audit_image_security_test.go`

- [ ] **Step 1: Write the failing tests**

Create `pkg/tools/v1/security/audit_image_security_test.go`:

```go
package security

import (
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func podWithImage(namespace, podName, image string, pullPolicy corev1.PullPolicy) corev1.Pod {
	return corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: podName, Namespace: namespace},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{Name: "app", Image: image, ImagePullPolicy: pullPolicy},
			},
		},
	}
}

func TestAnalyzeImageSecurity_LatestTag(t *testing.T) {
	pods := []corev1.Pod{
		podWithImage("default", "pod1", "nginx:latest", corev1.PullAlways),
	}
	result := analyzeImageSecurity(pods)
	if !strings.Contains(result, "[MEDIUM]") {
		t.Errorf("expected MEDIUM finding for latest tag, got: %s", result)
	}
}

func TestAnalyzeImageSecurity_NoTag(t *testing.T) {
	pods := []corev1.Pod{
		podWithImage("default", "pod1", "nginx", corev1.PullAlways),
	}
	result := analyzeImageSecurity(pods)
	if !strings.Contains(result, "[MEDIUM]") {
		t.Errorf("expected MEDIUM finding for image with no tag, got: %s", result)
	}
}

func TestAnalyzeImageSecurity_DigestPinned_NoFinding(t *testing.T) {
	pods := []corev1.Pod{
		podWithImage("default", "pod1", "nginx@sha256:abc123", corev1.PullAlways),
	}
	result := analyzeImageSecurity(pods)
	if strings.Contains(result, "[MEDIUM]") {
		t.Errorf("expected no MEDIUM finding for digest-pinned image, got: %s", result)
	}
}

func TestAnalyzeImageSecurity_PullNever(t *testing.T) {
	pods := []corev1.Pod{
		podWithImage("default", "pod1", "nginx:1.25", corev1.PullNever),
	}
	result := analyzeImageSecurity(pods)
	if !strings.Contains(result, "[LOW]") {
		t.Errorf("expected LOW finding for imagePullPolicy Never, got: %s", result)
	}
}

func TestAnalyzeImageSecurity_NoImagePullSecrets_NonSystem(t *testing.T) {
	pods := []corev1.Pod{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "pod1", Namespace: "production"},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{{Name: "app", Image: "nginx:1.25"}},
				// ImagePullSecrets not set
			},
		},
	}
	result := analyzeImageSecurity(pods)
	if !strings.Contains(result, "[LOW]") {
		t.Errorf("expected LOW finding for pod without imagePullSecrets in non-system ns, got: %s", result)
	}
}

func TestAnalyzeImageSecurity_NoImagePullSecrets_SystemNamespace_Ignored(t *testing.T) {
	pods := []corev1.Pod{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "coredns", Namespace: "kube-system"},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{{Name: "app", Image: "coredns:1.10"}},
			},
		},
	}
	result := analyzeImageSecurity(pods)
	if strings.Contains(result, "kube-system") {
		t.Errorf("expected kube-system pod to be ignored, got: %s", result)
	}
}

func TestAnalyzeImageSecurity_NoFindings(t *testing.T) {
	result := analyzeImageSecurity(nil)
	if !strings.Contains(result, "未发现") {
		t.Errorf("expected no-findings message, got: %s", result)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
cd d:/KubeWise && go test ./pkg/tools/v1/security/... -run TestAnalyzeImageSecurity -v
```

Expected: compilation error — `analyzeImageSecurity` undefined.

- [ ] **Step 3: Create the implementation**

Create `pkg/tools/v1/security/audit_image_security.go`:

```go
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

func (t *AuditImageSecurityTool) Name() string { return "audit_image_security" }

func (t *AuditImageSecurityTool) Description() string {
	return "审计Pod镜像安全，检查latest标签使用、imagePullPolicy:Never配置、缺少imagePullSecrets等风险"
}

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

func (t *AuditImageSecurityTool) Execute(ctx context.Context, args map[string]any) (string, error) {
	namespace, _ := args["namespace"].(string)
	pods, err := t.k8sClient.ListPods(ctx, namespace)
	if err != nil {
		return "", fmt.Errorf("获取Pod列表失败: %w", err)
	}
	return analyzeImageSecurity(pods), nil
}

func analyzeImageSecurity(pods []corev1.Pod) string {
	systemNamespaces := map[string]bool{
		"kube-system": true, "kube-public": true, "kube-node-lease": true,
	}
	var findings []string

	for _, pod := range pods {
		podRef := fmt.Sprintf("Pod %s/%s", pod.Namespace, pod.Name)

		// [LOW] 非系统命名空间中无 imagePullSecrets
		if !systemNamespaces[pod.Namespace] && len(pod.Spec.ImagePullSecrets) == 0 {
			findings = append(findings, fmt.Sprintf(
				"[LOW] %s: 未配置 imagePullSecrets", podRef,
			))
		}

		allContainers := append(pod.Spec.InitContainers, pod.Spec.Containers...)
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

// imageHasUnsafeTag 检查镜像是否使用 latest 标签或未指定标签（摘要固定的视为安全）
func imageHasUnsafeTag(image string) bool {
	// digest-pinned (@sha256:...) is safe
	if strings.Contains(image, "@") {
		return false
	}
	// find tag in the last path segment
	parts := strings.Split(image, "/")
	lastPart := parts[len(parts)-1]
	colonIdx := strings.LastIndex(lastPart, ":")
	if colonIdx == -1 {
		return true // no tag = effectively latest
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
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
cd d:/KubeWise && go test ./pkg/tools/v1/security/... -run TestAnalyzeImageSecurity -v
```

Expected: all 7 tests PASS.

- [ ] **Step 5: Run full security package tests**

```bash
cd d:/KubeWise && go test ./pkg/tools/v1/security/... -v
```

Expected: all tests in the package PASS.

- [ ] **Step 6: Commit**

```bash
git add pkg/tools/v1/security/audit_image_security.go pkg/tools/v1/security/audit_image_security_test.go
git commit -m "feat(security): add audit_image_security scanner tool"
```

---

### Task 6: Implement SecurityAgent

**Files:**
- Create: `pkg/agent/security/agent.go`

- [ ] **Step 1: Create the agent**

Create `pkg/agent/security/agent.go`:

```go
package security

import (
	"context"
	"fmt"
	"strings"

	"github.com/kubewise/kubewise/pkg/k8s"
	"github.com/kubewise/kubewise/pkg/llm"
	"github.com/kubewise/kubewise/pkg/tool"
	"github.com/kubewise/kubewise/pkg/types"

	// 加载安全审计工具，触发init函数注册
	_ "github.com/kubewise/kubewise/pkg/tools/v1/security"
)

// Agent 安全审计Agent
type Agent struct {
	k8sClient    *k8s.Client
	llmClient    *llm.Client
	toolRegistry *tool.Registry
}

// New 创建安全审计Agent
func New(k8sClient *k8s.Client, llmClient *llm.Client) (*Agent, error) {
	toolDep := tool.ToolDependency{
		K8sClient: k8sClient,
	}
	registry, err := tool.LoadGlobalRegistry(toolDep)
	if err != nil {
		return nil, fmt.Errorf("加载工具注册中心失败: %w", err)
	}
	return &Agent{
		k8sClient:    k8sClient,
		llmClient:    llmClient,
		toolRegistry: registry,
	}, nil
}

// buildSystemPrompt 生成系统提示词
func (a *Agent) buildSystemPrompt() string {
	return `你是Kubernetes安全审计助手。你有四个审计工具可用：
- audit_rbac：审计RBAC配置（cluster-admin滥用、通配符权限、exec/portforward授权、孤立ServiceAccount）
- audit_pod_security：审计Pod安全配置（privileged容器、hostNetwork/hostPID/hostIPC、allowPrivilegeEscalation、root用户、hostPath）
- audit_network_policies：审计网络策略（无NetworkPolicy的命名空间、未覆盖的Pod）
- audit_image_security：审计镜像安全（latest标签、imagePullPolicy:Never、缺少imagePullSecrets）

## 响应策略

**针对具体问题的查询**（如"列出所有privileged pod"、"检查default命名空间的RBAC"）：
- 调用最相关的单个工具，使用用户指定的命名空间范围
- 直接返回工具结果，无需添加严重程度分组或修复建议

**针对全面审计的查询**（如"审计集群安全"、"检查所有安全问题"、"做一次安全扫描"）：
- 依次调用全部四个工具
- 将结果整合为按严重程度分组的报告：Critical → High → Medium → Low
- 每类问题附上简要的修复建议

## 命名空间范围
如果用户提到了特定命名空间，在工具调用时传入 namespace 参数。否则留空（审计所有命名空间）。`
}

// HandleQuery 处理安全审计请求
func (a *Agent) HandleQuery(ctx context.Context, userQuery string, entities types.Entities) (string, error) {
	functions := a.toolRegistry.GetAllFunctionDefinitions()

	userMsg := userQuery
	if entities.Namespace != "" {
		userMsg = fmt.Sprintf("%s\n\n（目标命名空间：%s）", userQuery, entities.Namespace)
	}

	messages := []llm.Message{
		{Role: "system", Content: a.buildSystemPrompt()},
		{Role: "user", Content: userMsg},
	}

	maxSteps := 10
	for step := range maxSteps {
		resp, err := a.llmClient.ChatCompletion(ctx, messages, functions)
		if err != nil {
			return "", fmt.Errorf("LLM调用失败: %w", err)
		}

		if len(resp.ToolCalls) == 0 {
			return resp.Content, nil
		}

		funcCall := &resp.ToolCalls[0].Function

		fmt.Printf("第%d步：调用工具 %s\n", step+1, funcCall.Name)
		if len(funcCall.Arguments) > 0 {
			args := make([]string, 0, len(funcCall.Arguments))
			for k, v := range funcCall.Arguments {
				args = append(args, fmt.Sprintf("%s=%v", k, v))
			}
			fmt.Printf("参数：%s\n", strings.Join(args, ", "))
		}

		t, exists := a.toolRegistry.GetTool(funcCall.Name)
		if !exists {
			return "", fmt.Errorf("未知工具: %s", funcCall.Name)
		}
		result, err := t.Execute(ctx, funcCall.Arguments)
		if err != nil {
			fmt.Printf("工具调用失败：%v\n", err)
			result = fmt.Sprintf("工具调用失败：%v\n请修正参数后重新调用工具。", err)
		} else {
			fmt.Printf("工具返回结果长度：%d 字节\n", len(result))
		}

		messages = append(messages, *resp)
		toolMsg := llm.Message{
			Role:    "tool",
			Content: fmt.Sprintf("工具返回结果：\n%s", result),
		}
		if len(resp.ToolCalls) > 0 {
			toolMsg.ToolCallID = resp.ToolCalls[0].ID
		}
		messages = append(messages, toolMsg)
	}

	return "", fmt.Errorf("超过最大调用轮次，无法完成安全审计")
}
```

- [ ] **Step 2: Build to verify**

```bash
make build
```

Expected: binary built with no errors.

- [ ] **Step 3: Commit**

```bash
git add pkg/agent/security/agent.go
git commit -m "feat(agent): add SecurityAgent"
```

---

### Task 7: Wire SecurityAgent into the router

**Files:**
- Modify: `pkg/agent/router/agent.go`

- [ ] **Step 1: Add the security import and field**

In `pkg/agent/router/agent.go`, add the security package import alongside the existing agent imports:

```go
import (
    "context"
    "encoding/json"
    "fmt"
    "strings"

    "github.com/kubewise/kubewise/pkg/agent/query"
    "github.com/kubewise/kubewise/pkg/agent/security"
    "github.com/kubewise/kubewise/pkg/agent/troubleshooting"
    "github.com/kubewise/kubewise/pkg/k8s"
    "github.com/kubewise/kubewise/pkg/llm"
    "github.com/kubewise/kubewise/pkg/types"
)
```

- [ ] **Step 2: Add the securityAgent field to the struct**

Replace the `Agent` struct:

```go
// Agent 路由Agent
type Agent struct {
	k8sClient            *k8s.Client
	llmClient            *llm.Client
	queryAgent           *query.Agent
	troubleshootingAgent *troubleshooting.Agent
	securityAgent        *security.Agent
}
```

- [ ] **Step 3: Instantiate securityAgent in New()**

Replace the `New` function:

```go
// New 创建路由Agent
func New(k8sClient *k8s.Client, llmClient *llm.Client) (*Agent, error) {
	queryAgent, err := query.New(k8sClient, llmClient)
	if err != nil {
		return nil, fmt.Errorf("初始化查询Agent失败: %w", err)
	}
	troubleshootingAgent, err := troubleshooting.New(k8sClient, llmClient)
	if err != nil {
		return nil, fmt.Errorf("初始化故障排查Agent失败: %w", err)
	}
	securityAgent, err := security.New(k8sClient, llmClient)
	if err != nil {
		return nil, fmt.Errorf("初始化安全审计Agent失败: %w", err)
	}
	return &Agent{
		k8sClient:            k8sClient,
		llmClient:            llmClient,
		queryAgent:           queryAgent,
		troubleshootingAgent: troubleshootingAgent,
		securityAgent:        securityAgent,
	}, nil
}
```

- [ ] **Step 4: Replace the security stub in HandleQuery**

In the `switch intent.TaskType` block, replace:

```go
case types.TaskTypeSecurity:
    return "安全审计功能正在开发中，敬请期待", nil
```

With:

```go
case types.TaskTypeSecurity:
    return a.securityAgent.HandleQuery(ctx, userQuery, intent.Entities)
```

- [ ] **Step 5: Build to verify**

```bash
make build
```

Expected: binary built with no errors.

- [ ] **Step 6: Run all tests**

```bash
make test
```

Expected: all tests PASS.

- [ ] **Step 7: Commit**

```bash
git add pkg/agent/router/agent.go
git commit -m "feat(router): wire SecurityAgent into router"
```
