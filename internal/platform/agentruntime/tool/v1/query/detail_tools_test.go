package query

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

func TestGetResourceDetailTool_Name(t *testing.T) {
	tool := &GetResourceDetailTool{}
	if tool.Name() != "get_resource_detail" {
		t.Errorf("expected name get_resource_detail, got %s", tool.Name())
	}
}

func TestGetResourceDetailTool_Parameters(t *testing.T) {
	tool := &GetResourceDetailTool{}
	params := tool.Parameters()
	props, ok := params["properties"].(map[string]any)
	if !ok {
		t.Fatal("expected properties map")
	}
	required, ok := params["required"].([]string)
	if !ok {
		t.Fatal("expected required slice")
	}
	for _, r := range []string{"group", "version", "resource", "name"} {
		if _, ok := props[r]; !ok {
			t.Errorf("missing property: %s", r)
		}
		found := false
		for _, req := range required {
			if req == r {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("missing required: %s", r)
		}
	}
}

func TestGetResourceDetailTool_Execute_MissingVersion(t *testing.T) {
	tool := &GetResourceDetailTool{}
	_, err := tool.Execute(context.Background(), map[string]any{
		"group":    "",
		"resource": "pods",
		"name":     "test",
	})
	if err == nil {
		t.Fatal("expected error for missing version")
	}
}

func TestGetResourceDetailTool_Execute_MissingResource(t *testing.T) {
	tool := &GetResourceDetailTool{}
	_, err := tool.Execute(context.Background(), map[string]any{
		"group":   "",
		"version": "v1",
		"name":    "test",
	})
	if err == nil {
		t.Fatal("expected error for missing resource")
	}
}

func TestGetResourceDetailTool_Execute_MissingName(t *testing.T) {
	tool := &GetResourceDetailTool{}
	_, err := tool.Execute(context.Background(), map[string]any{
		"group":    "",
		"version":  "v1",
		"resource": "pods",
	})
	if err == nil {
		t.Fatal("expected error for missing name")
	}
}

func TestGetPodDetailTool_Name(t *testing.T) {
	tool := &GetPodDetailTool{}
	if tool.Name() != "get_pod_detail" {
		t.Errorf("expected name get_pod_detail, got %s", tool.Name())
	}
}

func TestGetPodDetailTool_Parameters(t *testing.T) {
	tool := &GetPodDetailTool{}
	params := tool.Parameters()
	required, ok := params["required"].([]string)
	if !ok {
		t.Fatal("expected required slice")
	}
	for _, r := range []string{"podName", "namespace"} {
		found := false
		for _, req := range required {
			if req == r {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("missing required: %s", r)
		}
	}
}

func TestGetPodDetailTool_Execute_MissingPodName(t *testing.T) {
	tool := &GetPodDetailTool{}
	_, err := tool.Execute(context.Background(), map[string]any{
		"namespace": "default",
	})
	if err == nil {
		t.Fatal("expected error for missing podName")
	}
}

func TestGetPodDetailTool_Execute_MissingNamespace(t *testing.T) {
	tool := &GetPodDetailTool{}
	_, err := tool.Execute(context.Background(), map[string]any{
		"podName": "test",
	})
	if err == nil {
		t.Fatal("expected error for missing namespace")
	}
}

func TestContainerStateString_Running(t *testing.T) {
	state := corev1.ContainerState{Running: &corev1.ContainerStateRunning{}}
	got := containerStateString(state)
	if got != "Running" {
		t.Errorf("expected Running, got %s", got)
	}
}

func TestContainerStateString_Waiting(t *testing.T) {
	state := corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "ImagePullBackOff"}}
	got := containerStateString(state)
	if got != "Waiting: ImagePullBackOff" {
		t.Errorf("expected Waiting: ImagePullBackOff, got %s", got)
	}
}

func TestContainerStateString_Terminated(t *testing.T) {
	state := corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{Reason: "OOMKilled"}}
	got := containerStateString(state)
	if got != "Terminated: OOMKilled" {
		t.Errorf("expected Terminated: OOMKilled, got %s", got)
	}
}

func TestContainerStateString_Unknown(t *testing.T) {
	state := corev1.ContainerState{}
	got := containerStateString(state)
	if got != "Unknown" {
		t.Errorf("expected Unknown, got %s", got)
	}
}

func TestResourceMap_Empty(t *testing.T) {
	m := resourceMap(corev1.ResourceRequirements{})
	if m == nil {
		t.Fatal("expected non-nil map")
	}
	if len(m) != 0 {
		t.Errorf("expected empty map, got %v", m)
	}
}

func TestResourceMap_WithRequestsAndLimits(t *testing.T) {
	rr := corev1.ResourceRequirements{
		Requests: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("100m"),
			corev1.ResourceMemory: resource.MustParse("128Mi"),
		},
		Limits: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("500m"),
			corev1.ResourceMemory: resource.MustParse("512Mi"),
		},
	}
	m := resourceMap(rr)
	if m["cpu_request"] != "100m" {
		t.Errorf("expected cpu_request=100m, got %s", m["cpu_request"])
	}
	if m["memory_request"] != "128Mi" {
		t.Errorf("expected memory_request=128Mi, got %s", m["memory_request"])
	}
	if m["cpu_limit"] != "500m" {
		t.Errorf("expected cpu_limit=500m, got %s", m["cpu_limit"])
	}
	if m["memory_limit"] != "512Mi" {
		t.Errorf("expected memory_limit=512Mi, got %s", m["memory_limit"])
	}
}
