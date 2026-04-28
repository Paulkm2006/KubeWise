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
