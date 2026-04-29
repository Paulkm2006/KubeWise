package operation

import (
	"context"
	"fmt"

	"github.com/kubewise/kubewise/pkg/k8s"
	"github.com/kubewise/kubewise/pkg/tool"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func init() {
	t := NewDeleteResourceTool(nil)
	tool.RegisterGlobal(tool.ToolMetadata{
		Name:        t.Name(),
		Description: t.Description(),
		Parameters:  t.Parameters(),
		Category:    "operation",
		Factory: func(dep any) (tool.Tool, error) {
			d, ok := dep.(tool.ToolDependency)
			if !ok {
				return nil, fmt.Errorf("delete_resource: invalid dependency type")
			}
			return NewDeleteResourceTool(d.K8sClient), nil
		},
	})
}

// DeleteResourceTool deletes a Kubernetes resource by GVR and name.
type DeleteResourceTool struct {
	k8sClient *k8s.Client
}

// NewDeleteResourceTool creates a DeleteResourceTool with the given K8s client.
func NewDeleteResourceTool(k8sClient *k8s.Client) *DeleteResourceTool {
	return &DeleteResourceTool{k8sClient: k8sClient}
}

func (t *DeleteResourceTool) Name() string        { return "delete_resource" }
func (t *DeleteResourceTool) Description() string { return "Delete a Kubernetes resource by GVR and name" }
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

// Execute deletes the target Kubernetes resource.
func (t *DeleteResourceTool) Execute(ctx context.Context, args map[string]any) (string, error) {
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
