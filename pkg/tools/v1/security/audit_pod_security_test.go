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
