package query

import (
	"context"
	"encoding/json"
	"fmt"

	corev1 "k8s.io/api/core/v1"

	"github.com/kubewise/kubewise/internal/utils/k8s"
	"github.com/kubewise/kubewise/internal/agent/tool"
)

type GetPodDetailTool struct {
	k8sClient *k8s.Client
}

func NewGetPodDetailTool(k8sClient *k8s.Client) *GetPodDetailTool {
	return &GetPodDetailTool{k8sClient: k8sClient}
}

func (t *GetPodDetailTool) Name() string {
	return "get_pod_detail"
}

func (t *GetPodDetailTool) Description() string {
	return "获取指定Pod的完整详细信息，包括状态、容器、条件、事件和最近日志。适用于需要全面了解某个Pod的情况。"
}

func (t *GetPodDetailTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"podName": map[string]any{
				"type":        "string",
				"description": "Pod名称",
			},
			"namespace": map[string]any{
				"type":        "string",
				"description": "Pod所在的命名空间",
			},
		},
		"required": []string{"podName", "namespace"},
	}
}

func (t *GetPodDetailTool) Execute(ctx context.Context, args map[string]any) (string, error) {
	podName, ok := args["podName"].(string)
	if !ok || podName == "" {
		return "", fmt.Errorf("参数podName不能为空")
	}
	namespace, ok := args["namespace"].(string)
	if !ok || namespace == "" {
		return "", fmt.Errorf("参数namespace不能为空")
	}

	pod, err := t.k8sClient.GetPod(ctx, namespace, podName)
	if err != nil {
		return "", fmt.Errorf("获取Pod信息失败: %w", err)
	}

	// Build status map
	status := map[string]string{
		"phase": string(pod.Status.Phase),
		"ip":    pod.Status.PodIP,
		"node":  pod.Status.HostIP,
	}
	if pod.Status.StartTime != nil {
		status["start_time"] = pod.Status.StartTime.Time.Format("2006-01-02 15:04:05")
	}

	// Build containers
	var containers []containerInfo
	for _, cs := range pod.Status.ContainerStatuses {
		ci := containerInfo{
			Name:         cs.Name,
			Image:        cs.Image,
			Ready:        cs.Ready,
			RestartCount: cs.RestartCount,
			State:        containerStateString(cs.State),
		}
		// Find matching spec container for resources
		for _, spec := range pod.Spec.Containers {
			if spec.Name == cs.Name {
				ci.Resources = resourceMap(spec.Resources)
				break
			}
		}
		containers = append(containers, ci)
	}

	// Build conditions
	var conditions []conditionInfo
	for _, c := range pod.Status.Conditions {
		conditions = append(conditions, conditionInfo{
			Type:    string(c.Type),
			Status:  string(c.Status),
			Reason:  c.Reason,
			Message: c.Message,
		})
	}

	// Fetch events
	events, err := t.k8sClient.GetEvents(ctx, namespace, podName)
	var eventInfos []eventInfo
	if err == nil {
		for i, e := range events {
			if i >= 10 {
				break
			}
			ts := e.LastTimestamp.Time
			if ts.IsZero() {
				ts = e.EventTime.Time
			}
			eventInfos = append(eventInfos, eventInfo{
				Type:      e.Type,
				Reason:    e.Reason,
				Message:   e.Message,
				Timestamp: ts.Format("2006-01-02 15:04:05"),
			})
		}
	}

	// Fetch recent logs from first container
	recentLogs := ""
	if len(pod.Spec.Containers) > 0 {
		logs, err := t.k8sClient.GetPodLogs(ctx, namespace, podName, pod.Spec.Containers[0].Name, 20)
		if err == nil {
			recentLogs = logs
		}
	}

	// Build labels
	labels := make(map[string]string)
	for k, v := range pod.Labels {
		labels[k] = v
	}

	detail := podDetailJSON{
		Kind:       "pod",
		Name:       podName,
		Namespace:  namespace,
		Status:     status,
		Containers: containers,
		Conditions: conditions,
		Events:     eventInfos,
		RecentLogs: recentLogs,
		Labels:     labels,
	}

	jsonBytes, err := json.Marshal(detail)
	if err != nil {
		return "", fmt.Errorf("序列化Pod详情失败: %w", err)
	}

	return fmt.Sprintf("__KUBEWISE_DETAIL:pod__\n%s\n__END__", string(jsonBytes)), nil
}

// Internal JSON-friendly types (not exported, used only for serialization)

type podDetailJSON struct {
	Kind       string            `json:"kind"`
	Name       string            `json:"name"`
	Namespace  string            `json:"namespace"`
	Status     map[string]string `json:"status"`
	Containers []containerInfo   `json:"containers,omitempty"`
	Conditions []conditionInfo   `json:"conditions,omitempty"`
	Events     []eventInfo       `json:"events,omitempty"`
	RecentLogs string            `json:"recent_logs,omitempty"`
	Labels     map[string]string `json:"labels,omitempty"`
}

type containerInfo struct {
	Name         string            `json:"name"`
	Image        string            `json:"image"`
	Ready        bool              `json:"ready"`
	RestartCount int32             `json:"restart_count"`
	State        string            `json:"state"`
	Resources    map[string]string `json:"resources,omitempty"`
}

type conditionInfo struct {
	Type    string `json:"type"`
	Status  string `json:"status"`
	Reason  string `json:"reason"`
	Message string `json:"message"`
}

type eventInfo struct {
	Type      string `json:"type"`
	Reason    string `json:"reason"`
	Message   string `json:"message"`
	Timestamp string `json:"timestamp"`
}

func containerStateString(state corev1.ContainerState) string {
	if state.Running != nil {
		return "Running"
	}
	if state.Waiting != nil {
		return "Waiting: " + state.Waiting.Reason
	}
	if state.Terminated != nil {
		return "Terminated: " + state.Terminated.Reason
	}
	return "Unknown"
}

func resourceMap(resources corev1.ResourceRequirements) map[string]string {
	m := make(map[string]string)
	if req := resources.Requests; req != nil {
		if cpu, ok := req[corev1.ResourceCPU]; ok {
			m["cpu_request"] = cpu.String()
		}
		if mem, ok := req[corev1.ResourceMemory]; ok {
			m["memory_request"] = mem.String()
		}
	}
	if lim := resources.Limits; lim != nil {
		if cpu, ok := lim[corev1.ResourceCPU]; ok {
			m["cpu_limit"] = cpu.String()
		}
		if mem, ok := lim[corev1.ResourceMemory]; ok {
			m["memory_limit"] = mem.String()
		}
	}
	return m
}

func init() {
	tool.RegisterGlobal(tool.ToolMetadata{
		Name:        "get_pod_detail",
		Description: "获取指定Pod的完整详细信息，包括状态、容器、条件、事件和最近日志。适用于需要全面了解某个Pod的情况。",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"podName": map[string]any{
					"type":        "string",
					"description": "Pod名称",
				},
				"namespace": map[string]any{
					"type":        "string",
					"description": "Pod所在的命名空间",
				},
			},
			"required": []string{"podName", "namespace"},
		},
		Factory: func(dep any) (tool.Tool, error) {
			toolDep, ok := dep.(tool.ToolDependency)
			if !ok {
				return nil, fmt.Errorf("invalid dependency type")
			}
			return NewGetPodDetailTool(toolDep.K8sClient), nil
		},
	})
}
