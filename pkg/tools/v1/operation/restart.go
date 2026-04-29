package operation

import (
	"context"
	"fmt"

	"github.com/kubewise/kubewise/pkg/k8s"
	"github.com/kubewise/kubewise/pkg/tool"
)

func init() {
	t := NewRestartResourceTool(nil)
	tool.RegisterGlobal(tool.ToolMetadata{
		Name:        t.Name(),
		Description: t.Description(),
		Parameters:  t.Parameters(),
		Category:    "operation",
		Factory: func(dep any) (tool.Tool, error) {
			d, ok := dep.(tool.ToolDependency)
			if !ok {
				return nil, fmt.Errorf("restart_resource: invalid dependency type")
			}
			return NewRestartResourceTool(d.K8sClient), nil
		},
	})
}

// RestartResourceTool triggers a rolling restart of a Deployment, StatefulSet, or DaemonSet.
type RestartResourceTool struct {
	k8sClient *k8s.Client
}

// NewRestartResourceTool creates a RestartResourceTool with the given K8s client.
func NewRestartResourceTool(k8sClient *k8s.Client) *RestartResourceTool {
	return &RestartResourceTool{k8sClient: k8sClient}
}

func (t *RestartResourceTool) Name() string        { return "restart_resource" }
func (t *RestartResourceTool) Description() string { return "Trigger a rolling restart of a Deployment, StatefulSet, or DaemonSet" }
func (t *RestartResourceTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"namespace": map[string]any{"type": "string", "description": "Kubernetes namespace of the resource"},
			"kind":      map[string]any{"type": "string", "description": "Resource kind: Deployment, StatefulSet, or DaemonSet"},
			"name":      map[string]any{"type": "string", "description": "Name of the resource"},
		},
		"required": []string{"namespace", "kind", "name"},
	}
}

// Execute triggers a rolling restart of the target resource.
func (t *RestartResourceTool) Execute(ctx context.Context, args map[string]any) (string, error) {
	namespace, ok := args["namespace"].(string)
	if !ok || namespace == "" {
		return "", fmt.Errorf("restart_resource: missing or invalid 'namespace' argument")
	}
	kind, ok := args["kind"].(string)
	if !ok || kind == "" {
		return "", fmt.Errorf("restart_resource: missing or invalid 'kind' argument")
	}
	name, ok := args["name"].(string)
	if !ok || name == "" {
		return "", fmt.Errorf("restart_resource: missing or invalid 'name' argument")
	}

	if err := t.k8sClient.RestartResource(ctx, namespace, kind, name); err != nil {
		return "", err
	}
	return fmt.Sprintf("Successfully triggered rolling restart of %s/%s in namespace %s", kind, name, namespace), nil
}
