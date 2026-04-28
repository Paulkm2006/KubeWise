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
