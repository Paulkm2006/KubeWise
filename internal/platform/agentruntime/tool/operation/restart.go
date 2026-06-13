package operation

import (
	"context"
	"fmt"

	toolv2 "github.com/kubewise/kubewise/internal/platform/agentruntime/tool/v2"
	"github.com/kubewise/kubewise/internal/platform/cluster"
)

// RestartResourceTool triggers a rolling restart of a Deployment, StatefulSet, or DaemonSet.
type RestartResourceTool struct {
	k8sClient *cluster.Client
}

// NewRestartResourceTool creates a RestartResourceTool with the given K8s client.
func NewRestartResourceTool(k8sClient *cluster.Client) *RestartResourceTool {
	return &RestartResourceTool{k8sClient: k8sClient}
}

func (t *RestartResourceTool) Name() string { return "restart_resource" }
func (t *RestartResourceTool) Description() string {
	return "Trigger a rolling restart of a Deployment, StatefulSet, or DaemonSet"
}
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

func (t *RestartResourceTool) Meta() toolv2.ToolMeta {
	return operationWriteMeta(t.Name(), t.Description(), t.Parameters())
}

func (t *RestartResourceTool) Execute(ctx context.Context, args map[string]any) (toolv2.ToolResult, error) {
	meta := t.Meta()
	text, err := t.executeText(ctx, args)
	if err != nil {
		return toolv2.ToolResult{}, err
	}
	return toolv2.TextResult(meta, text), nil
}

// executeText triggers a rolling restart of the target resource.
func (t *RestartResourceTool) executeText(ctx context.Context, args map[string]any) (string, error) {
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
