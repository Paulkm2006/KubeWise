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

func TestAnalyzeRBAC_WildcardResourceInRole(t *testing.T) {
	roles := []rbacv1.Role{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "wild-resource-role", Namespace: "default"},
			Rules: []rbacv1.PolicyRule{
				{Verbs: []string{"get"}, Resources: []string{"*"}},
			},
		},
	}
	result := analyzeRBAC(roles, nil, nil, nil, nil)
	if !strings.Contains(result, "[HIGH]") {
		t.Errorf("expected HIGH finding for wildcard resource in Role, got: %s", result)
	}
}

func TestAnalyzeRBAC_PortforwardClusterRole(t *testing.T) {
	crs := []rbacv1.ClusterRole{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "portforward-cr"},
			Rules: []rbacv1.PolicyRule{
				{Verbs: []string{"create"}, Resources: []string{"pods/portforward"}},
			},
		},
	}
	result := analyzeRBAC(nil, crs, nil, nil, nil)
	if !strings.Contains(result, "[MEDIUM]") {
		t.Errorf("expected MEDIUM finding for portforward ClusterRole, got: %s", result)
	}
	if !strings.Contains(result, "portforward-cr") {
		t.Errorf("expected ClusterRole name in result, got: %s", result)
	}
}

func TestAnalyzeRBAC_DefaultServiceAccountSkipped(t *testing.T) {
	sas := []corev1.ServiceAccount{
		{ObjectMeta: metav1.ObjectMeta{Name: "default", Namespace: "production"}},
	}
	result := analyzeRBAC(nil, nil, nil, nil, sas)
	if strings.Contains(result, "[LOW]") {
		t.Errorf("expected default SA to be skipped, got: %s", result)
	}
}

func TestAnalyzeRBAC_SystemNamespaceSASkipped(t *testing.T) {
	sas := []corev1.ServiceAccount{
		{ObjectMeta: metav1.ObjectMeta{Name: "custom-sa", Namespace: "kube-system"}},
		{ObjectMeta: metav1.ObjectMeta{Name: "custom-sa", Namespace: "kube-public"}},
		{ObjectMeta: metav1.ObjectMeta{Name: "custom-sa", Namespace: "kube-node-lease"}},
	}
	result := analyzeRBAC(nil, nil, nil, nil, sas)
	if strings.Contains(result, "[LOW]") {
		t.Errorf("expected system namespace SAs to be skipped, got: %s", result)
	}
}

func TestAnalyzeRBAC_WildcardVerbTriggersExecMedium(t *testing.T) {
	roles := []rbacv1.Role{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "wildverb-exec-role", Namespace: "default"},
			Rules: []rbacv1.PolicyRule{
				{Verbs: []string{"*"}, Resources: []string{"pods/exec"}},
			},
		},
	}
	result := analyzeRBAC(roles, nil, nil, nil, nil)
	if !strings.Contains(result, "[MEDIUM]") {
		t.Errorf("expected MEDIUM finding when wildcard verb grants exec, got: %s", result)
	}
}
