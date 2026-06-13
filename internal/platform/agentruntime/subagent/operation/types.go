package operation

import "fmt"

// OperationStep represents a single planned cluster mutation.
type OperationStep struct {
	StepIndex     int               `json:"step_index"`
	OperationType string            `json:"operation_type"`
	ResourceKind  string            `json:"resource_kind"`
	ResourceName  string            `json:"resource_name"`
	Namespace     string            `json:"namespace,omitempty"`
	Group         string            `json:"group,omitempty"`
	Version       string            `json:"version,omitempty"`
	Resource      string            `json:"resource,omitempty"`
	Replicas      *int32            `json:"replicas,omitempty"`
	Action        string            `json:"action,omitempty"`
	Labels        map[string]string `json:"labels,omitempty"`
	Annotations   map[string]string `json:"annotations,omitempty"`
	GeneratedYAML string            `json:"generated_yaml,omitempty"`
	Description   string            `json:"description"`
}

// stepToToolCall maps an OperationStep to a write tool name and its args map.
func stepToToolCall(step OperationStep) (toolName string, args map[string]any, err error) {
	switch step.OperationType {
	case "scale":
		if step.Replicas == nil {
			return "", nil, fmt.Errorf("scale operation requires replicas field")
		}
		return "scale_resource", map[string]any{
			"namespace": step.Namespace,
			"kind":      step.ResourceKind,
			"name":      step.ResourceName,
			"replicas":  float64(*step.Replicas),
		}, nil
	case "restart":
		return "restart_resource", map[string]any{
			"namespace": step.Namespace,
			"kind":      step.ResourceKind,
			"name":      step.ResourceName,
		}, nil
	case "delete":
		return "delete_resource", map[string]any{
			"namespace": step.Namespace,
			"group":     step.Group,
			"version":   step.Version,
			"resource":  step.Resource,
			"name":      step.ResourceName,
		}, nil
	case "apply":
		return "apply_resource", map[string]any{
			"yaml_content": step.GeneratedYAML,
		}, nil
	case "cordon_drain":
		return "cordon_drain_node", map[string]any{
			"node_name": step.ResourceName,
			"action":    step.Action,
		}, nil
	case "label_annotate":
		return "label_annotate_resource", map[string]any{
			"namespace":   step.Namespace,
			"group":       step.Group,
			"version":     step.Version,
			"resource":    step.Resource,
			"name":        step.ResourceName,
			"labels":      step.Labels,
			"annotations": step.Annotations,
		}, nil
	default:
		return "", nil, fmt.Errorf("unknown operation type: %s", step.OperationType)
	}
}

// operationTypeDisplay returns a human-readable Chinese display name for an operation type.
func operationTypeDisplay(opType string) string {
	switch opType {
	case "scale":
		return "Scale（扩缩容）"
	case "restart":
		return "Restart（重启）"
	case "delete":
		return "Delete（删除）"
	case "apply":
		return "Apply（创建/更新）"
	case "cordon_drain":
		return "Cordon/Drain（节点封锁/驱逐）"
	case "label_annotate":
		return "Label/Annotate（标签/注解）"
	default:
		return opType
	}
}
