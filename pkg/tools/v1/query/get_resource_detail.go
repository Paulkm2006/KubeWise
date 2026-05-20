package query

import (
	"context"
	"encoding/json"
	"fmt"

	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/kubewise/kubewise/pkg/k8s"
	"github.com/kubewise/kubewise/pkg/tool"
)

type GetResourceDetailTool struct {
	k8sClient *k8s.Client
}

func NewGetResourceDetailTool(k8sClient *k8s.Client) *GetResourceDetailTool {
	return &GetResourceDetailTool{k8sClient: k8sClient}
}

func (t *GetResourceDetailTool) Name() string {
	return "get_resource_detail"
}

func (t *GetResourceDetailTool) Description() string {
	return "获取任意Kubernetes资源的详细信息（包括自定义资源），聚合资源详情和相关事件。适用于需要全面了解某个资源的状态和活动情况。"
}

func (t *GetResourceDetailTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"group": map[string]any{
				"type":        "string",
				"description": "资源的API组，核心API组为空字符串",
			},
			"version": map[string]any{
				"type":        "string",
				"description": "资源的API版本，例如：\"v1\"",
			},
			"resource": map[string]any{
				"type":        "string",
				"description": "资源类型的复数名称，例如：\"deployments\"、\"services\"",
			},
			"name": map[string]any{
				"type":        "string",
				"description": "资源名称",
			},
			"namespace": map[string]any{
				"type":        "string",
				"description": "命名空间，集群级资源不需要提供",
			},
		},
		"required": []string{"group", "version", "resource", "name"},
	}
}

func (t *GetResourceDetailTool) Execute(ctx context.Context, args map[string]any) (string, error) {
	group, _ := args["group"].(string)
	version, ok := args["version"].(string)
	if !ok || version == "" {
		return "", fmt.Errorf("参数version不能为空")
	}
	resource, ok := args["resource"].(string)
	if !ok || resource == "" {
		return "", fmt.Errorf("参数resource不能为空")
	}
	name, ok := args["name"].(string)
	if !ok || name == "" {
		return "", fmt.Errorf("参数name不能为空")
	}
	namespace, _ := args["namespace"].(string)

	gvr := schema.GroupVersionResource{
		Group:    group,
		Version:  version,
		Resource: resource,
	}

	cr, err := t.k8sClient.GetCustomResource(ctx, gvr, namespace, name)
	if err != nil {
		return "", fmt.Errorf("获取资源详情失败: %w", err)
	}

	// Extract structured fields from unstructured object
	obj, ok := cr.(map[string]interface{})
	if !ok {
		// Fallback: return raw JSON as detail
		jsonBytes, _ := json.Marshal(cr)
		return fmt.Sprintf("__KUBEWISE_DETAIL:resource__\n%s\n__END__", string(jsonBytes)), nil
	}

	// Extract kind
	kind, _ := obj["kind"].(string)

	// Extract metadata
	metadata, _ := obj["metadata"].(map[string]interface{})
	var labels map[string]string
	if metadata != nil {
		if l, ok := metadata["labels"].(map[string]interface{}); ok {
			labels = make(map[string]string)
			for k, v := range l {
				labels[k] = fmt.Sprintf("%v", v)
			}
		}
	}

	// Extract status
	status := make(map[string]string)
	if s, ok := obj["status"].(map[string]interface{}); ok {
		if phase, ok := s["phase"].(string); ok {
			status["phase"] = phase
		}
		if replicas, ok := s["replicas"]; ok {
			status["replicas"] = fmt.Sprintf("%v", replicas)
		}
		if ready, ok := s["readyReplicas"]; ok {
			status["ready_replicas"] = fmt.Sprintf("%v", ready)
		}
		if available, ok := s["availableReplicas"]; ok {
			status["available_replicas"] = fmt.Sprintf("%v", available)
		}
		if updated, ok := s["updatedReplicas"]; ok {
			status["updated_replicas"] = fmt.Sprintf("%v", updated)
		}
	}

	// Extract conditions
	var conditions []eventCondition
	if s, ok := obj["status"].(map[string]interface{}); ok {
		if conds, ok := s["conditions"].([]interface{}); ok {
			for _, c := range conds {
				if cm, ok := c.(map[string]interface{}); ok {
					cond := eventCondition{
						Type:    fmt.Sprintf("%v", cm["type"]),
						Status:  fmt.Sprintf("%v", cm["status"]),
						Reason:  fmt.Sprintf("%v", cm["reason"]),
						Message: fmt.Sprintf("%v", cm["message"]),
					}
					conditions = append(conditions, cond)
				}
			}
		}
	}

	// Fetch events
	var eventList []eventItem
	events, err := t.k8sClient.GetEvents(ctx, namespace, name)
	if err == nil {
		for i, e := range events {
			if i >= 10 {
				break
			}
			ts := e.LastTimestamp.Time
			if ts.IsZero() {
				ts = e.EventTime.Time
			}
			eventList = append(eventList, eventItem{
				Type:      e.Type,
				Reason:    e.Reason,
				Message:   e.Message,
				Timestamp: ts.Format("2006-01-02 15:04:05"),
			})
		}
	}

	detail := resourceDetailJSON{
		Kind:       kind,
		Name:       name,
		Namespace:  namespace,
		Status:     status,
		Conditions: conditions,
		Events:     eventList,
		Labels:     labels,
	}

	jsonBytes, err := json.Marshal(detail)
	if err != nil {
		return "", fmt.Errorf("序列化资源详情失败: %w", err)
	}

	return fmt.Sprintf("__KUBEWISE_DETAIL:resource__\n%s\n__END__", string(jsonBytes)), nil
}

type resourceDetailJSON struct {
	Kind       string            `json:"kind"`
	Name       string            `json:"name"`
	Namespace  string            `json:"namespace"`
	Status     map[string]string `json:"status"`
	Conditions []eventCondition  `json:"conditions,omitempty"`
	Events     []eventItem       `json:"events,omitempty"`
	Labels     map[string]string `json:"labels,omitempty"`
}

type eventCondition struct {
	Type    string `json:"type"`
	Status  string `json:"status"`
	Reason  string `json:"reason"`
	Message string `json:"message"`
}

type eventItem struct {
	Type      string `json:"type"`
	Reason    string `json:"reason"`
	Message   string `json:"message"`
	Timestamp string `json:"timestamp"`
}

func init() {
	tool.RegisterGlobal(tool.ToolMetadata{
		Name:        "get_resource_detail",
		Description: "获取任意Kubernetes资源的详细信息（包括自定义资源），聚合资源详情和相关事件。适用于需要全面了解某个资源的状态和活动情况。",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"group": map[string]any{
					"type":        "string",
					"description": "资源的API组，核心API组为空字符串",
				},
				"version": map[string]any{
					"type":        "string",
					"description": "资源的API版本，例如：\"v1\"",
				},
				"resource": map[string]any{
					"type":        "string",
					"description": "资源类型的复数名称，例如：\"deployments\"、\"services\"",
				},
				"name": map[string]any{
					"type":        "string",
					"description": "资源名称",
				},
				"namespace": map[string]any{
					"type":        "string",
					"description": "命名空间，集群级资源不需要提供",
				},
			},
			"required": []string{"group", "version", "resource", "name"},
		},
		Factory: func(dep any) (tool.Tool, error) {
			toolDep, ok := dep.(tool.ToolDependency)
			if !ok {
				return nil, fmt.Errorf("invalid dependency type")
			}
			return NewGetResourceDetailTool(toolDep.K8sClient), nil
		},
	})
}
