package operation

import (
	"context"
	"fmt"

	toolv2 "github.com/kubewise/kubewise/internal/platform/agentruntime/tool/v2"
	"github.com/kubewise/kubewise/internal/platform/cluster"
)

// ScaleResourceTool scales a Deployment or StatefulSet to a specified replica count.
type ScaleResourceTool struct {
	k8sClient *cluster.Client
}

// NewScaleResourceTool creates a ScaleResourceTool with the given K8s client.
func NewScaleResourceTool(k8sClient *cluster.Client) *ScaleResourceTool {
	return &ScaleResourceTool{k8sClient: k8sClient}
}

func (t *ScaleResourceTool) Name() string { return "scale_resource" }
func (t *ScaleResourceTool) Description() string {
	return "Scale a Deployment or StatefulSet to the specified number of replicas"
}
func (t *ScaleResourceTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"namespace": map[string]any{"type": "string", "description": "Kubernetes namespace of the resource"},
			"kind":      map[string]any{"type": "string", "description": "Resource kind: Deployment or StatefulSet"},
			"name":      map[string]any{"type": "string", "description": "Name of the resource"},
			"replicas":  map[string]any{"type": "integer", "description": "Desired number of replicas"},
		},
		"required": []string{"namespace", "kind", "name", "replicas"},
	}
}

func (t *ScaleResourceTool) Meta() toolv2.ToolMeta {
	return operationWriteMeta(t.Name(), t.Description(), t.Parameters())
}

func (t *ScaleResourceTool) Execute(ctx context.Context, args map[string]any) (toolv2.ToolResult, error) {
	meta := t.Meta()
	text, err := t.executeText(ctx, args)
	if err != nil {
		return toolv2.ToolResult{}, err
	}
	return toolv2.TextResult(meta, text), nil
}

// executeText scales the target resource to the requested replica count.
func (t *ScaleResourceTool) executeText(ctx context.Context, args map[string]any) (string, error) {
	namespace, ok := args["namespace"].(string)
	if !ok || namespace == "" {
		return "", fmt.Errorf("scale_resource: missing or invalid 'namespace' argument")
	}
	kind, ok := args["kind"].(string)
	if !ok || kind == "" {
		return "", fmt.Errorf("scale_resource: missing or invalid 'kind' argument")
	}
	name, ok := args["name"].(string)
	if !ok || name == "" {
		return "", fmt.Errorf("scale_resource: missing or invalid 'name' argument")
	}

	// replicas arrives as float64 from JSON decoding; also handle int32/int
	var replicas int32
	switch v := args["replicas"].(type) {
	case float64:
		replicas = int32(v)
	case int32:
		replicas = v
	case int:
		replicas = int32(v)
	default:
		return "", fmt.Errorf("scale_resource: invalid replicas type %T", args["replicas"])
	}

	if err := t.k8sClient.ScaleResource(ctx, namespace, kind, name, replicas); err != nil {
		return "", err
	}
	return fmt.Sprintf("Successfully scaled %s/%s in namespace %s to %d replicas", kind, name, namespace, replicas), nil
}
