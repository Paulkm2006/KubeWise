package operation

import (
	"context"
	"fmt"

	"github.com/kubewise/kubewise/pkg/k8s"
	"github.com/kubewise/kubewise/pkg/tool"
)

func init() {
	tool.RegisterGlobal(tool.ToolMetadata{
		Name:        "scale_resource",
		Description: "Scale a Deployment or StatefulSet to the specified number of replicas",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"namespace": map[string]any{
					"type":        "string",
					"description": "Kubernetes namespace of the resource",
				},
				"kind": map[string]any{
					"type":        "string",
					"description": "Resource kind: Deployment or StatefulSet",
				},
				"name": map[string]any{
					"type":        "string",
					"description": "Name of the resource",
				},
				"replicas": map[string]any{
					"type":        "integer",
					"description": "Desired number of replicas",
				},
			},
			"required": []string{"namespace", "kind", "name", "replicas"},
		},
		Category: "operation",
		Factory: func(dep any) (tool.Tool, error) {
			d, ok := dep.(tool.ToolDependency)
			if !ok {
				return nil, fmt.Errorf("scale_resource: invalid dependency type")
			}
			return &ScaleResourceTool{k8sClient: d.K8sClient}, nil
		},
	})
}

type ScaleResourceTool struct {
	k8sClient *k8s.Client
}

func (t *ScaleResourceTool) Name() string        { return "scale_resource" }
func (t *ScaleResourceTool) Description() string { return "Scale a Deployment or StatefulSet to the specified number of replicas" }
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

func (t *ScaleResourceTool) Execute(ctx context.Context, args map[string]any) (string, error) {
	namespace, _ := args["namespace"].(string)
	kind, _ := args["kind"].(string)
	name, _ := args["name"].(string)

	// replicas comes from LLM JSON as float64; also handle int32/int
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
