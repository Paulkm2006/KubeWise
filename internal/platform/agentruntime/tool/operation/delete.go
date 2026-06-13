package operation

import (
	"context"
	"fmt"

	toolv2 "github.com/kubewise/kubewise/internal/platform/agentruntime/tool/v2"
	"github.com/kubewise/kubewise/internal/platform/cluster"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// DeleteResourceTool deletes a Kubernetes resource by GVR and name.
type DeleteResourceTool struct {
	k8sClient *cluster.Client
}

// NewDeleteResourceTool creates a DeleteResourceTool with the given K8s client.
func NewDeleteResourceTool(k8sClient *cluster.Client) *DeleteResourceTool {
	return &DeleteResourceTool{k8sClient: k8sClient}
}

func (t *DeleteResourceTool) Name() string { return "delete_resource" }
func (t *DeleteResourceTool) Description() string {
	return "Delete a Kubernetes resource by GVR and name"
}
func (t *DeleteResourceTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"namespace": map[string]any{"type": "string", "description": "Kubernetes namespace (empty for cluster-scoped resources)"},
			"group":     map[string]any{"type": "string", "description": "API group (e.g. apps, empty string for core resources)"},
			"version":   map[string]any{"type": "string", "description": "API version (e.g. v1, apps/v1)"},
			"resource":  map[string]any{"type": "string", "description": "Resource plural name (e.g. deployments, pods)"},
			"name":      map[string]any{"type": "string", "description": "Name of the resource to delete"},
		},
		"required": []string{"namespace", "group", "version", "resource", "name"},
	}
}

func (t *DeleteResourceTool) Meta() toolv2.ToolMeta {
	return operationWriteMeta(t.Name(), t.Description(), t.Parameters())
}

func (t *DeleteResourceTool) Execute(ctx context.Context, args map[string]any) (toolv2.ToolResult, error) {
	meta := t.Meta()
	text, err := t.executeText(ctx, args)
	if err != nil {
		return toolv2.ToolResult{}, err
	}
	return toolv2.TextResult(meta, text), nil
}

// executeText deletes the target Kubernetes resource.
func (t *DeleteResourceTool) executeText(ctx context.Context, args map[string]any) (string, error) {
	namespace, _ := args["namespace"].(string)

	// group may be empty for core API resources (e.g. pods, services)
	group, ok := args["group"].(string)
	if !ok {
		return "", fmt.Errorf("delete_resource: missing or invalid 'group' argument")
	}
	version, ok := args["version"].(string)
	if !ok || version == "" {
		return "", fmt.Errorf("delete_resource: missing or invalid 'version' argument")
	}
	resource, ok := args["resource"].(string)
	if !ok || resource == "" {
		return "", fmt.Errorf("delete_resource: missing or invalid 'resource' argument")
	}
	name, ok := args["name"].(string)
	if !ok || name == "" {
		return "", fmt.Errorf("delete_resource: missing or invalid 'name' argument")
	}

	gvr := schema.GroupVersionResource{Group: group, Version: version, Resource: resource}
	if err := t.k8sClient.DeleteResource(ctx, namespace, gvr, name); err != nil {
		return "", err
	}
	return fmt.Sprintf("Successfully deleted %s/%s", resource, name), nil
}
