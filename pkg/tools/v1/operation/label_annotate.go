package operation

import (
	"context"
	"fmt"

	"github.com/kubewise/kubewise/pkg/k8s"
	"github.com/kubewise/kubewise/pkg/tool"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func init() {
	t := NewLabelAnnotateResourceTool(nil)
	tool.RegisterGlobal(tool.ToolMetadata{
		Name:        t.Name(),
		Description: t.Description(),
		Parameters:  t.Parameters(),
		Category:    "operation",
		Factory: func(dep any) (tool.Tool, error) {
			d, ok := dep.(tool.ToolDependency)
			if !ok {
				return nil, fmt.Errorf("label_annotate_resource: invalid dependency type")
			}
			return NewLabelAnnotateResourceTool(d.K8sClient), nil
		},
	})
}

// LabelAnnotateResourceTool adds or updates labels and/or annotations on a Kubernetes resource.
type LabelAnnotateResourceTool struct {
	k8sClient *k8s.Client
}

// NewLabelAnnotateResourceTool creates a LabelAnnotateResourceTool with the given K8s client.
func NewLabelAnnotateResourceTool(k8sClient *k8s.Client) *LabelAnnotateResourceTool {
	return &LabelAnnotateResourceTool{k8sClient: k8sClient}
}

func (t *LabelAnnotateResourceTool) Name() string        { return "label_annotate_resource" }
func (t *LabelAnnotateResourceTool) Description() string { return "Add or update labels and/or annotations on a Kubernetes resource" }
func (t *LabelAnnotateResourceTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"namespace": map[string]any{"type": "string", "description": "Kubernetes namespace (empty for cluster-scoped resources)"},
			"group":     map[string]any{"type": "string", "description": "API group (empty string for core resources)"},
			"version":   map[string]any{"type": "string", "description": "API version (e.g. v1)"},
			"resource":  map[string]any{"type": "string", "description": "Resource plural name (e.g. deployments, pods)"},
			"name":      map[string]any{"type": "string", "description": "Name of the resource"},
			"labels": map[string]any{
				"type":                 "object",
				"additionalProperties": map[string]any{"type": "string"},
				"description":          "Map of label key-value pairs to set",
			},
			"annotations": map[string]any{
				"type":                 "object",
				"additionalProperties": map[string]any{"type": "string"},
				"description":          "Map of annotation key-value pairs to set",
			},
		},
		"required": []string{"namespace", "group", "version", "resource", "name"},
	}
}

// Execute applies the given labels and/or annotations to the target resource.
func (t *LabelAnnotateResourceTool) Execute(ctx context.Context, args map[string]any) (string, error) {
	namespace, _ := args["namespace"].(string)

	group, ok := args["group"].(string)
	if !ok {
		return "", fmt.Errorf("label_annotate_resource: missing or invalid 'group' argument")
	}
	version, ok := args["version"].(string)
	if !ok || version == "" {
		return "", fmt.Errorf("label_annotate_resource: missing or invalid 'version' argument")
	}
	resource, ok := args["resource"].(string)
	if !ok || resource == "" {
		return "", fmt.Errorf("label_annotate_resource: missing or invalid 'resource' argument")
	}
	name, ok := args["name"].(string)
	if !ok || name == "" {
		return "", fmt.Errorf("label_annotate_resource: missing or invalid 'name' argument")
	}

	labels := toStringMap(args["labels"])
	annotations := toStringMap(args["annotations"])

	gvr := schema.GroupVersionResource{Group: group, Version: version, Resource: resource}
	if err := t.k8sClient.LabelResource(ctx, namespace, gvr, name, labels, annotations); err != nil {
		return "", err
	}
	return fmt.Sprintf("Successfully updated labels/annotations on %s/%s", resource, name), nil
}

// toStringMap converts a map[string]any to map[string]string, skipping non-string values.
func toStringMap(raw any) map[string]string {
	m, ok := raw.(map[string]any)
	if !ok {
		return nil
	}
	result := make(map[string]string, len(m))
	for k, v := range m {
		if s, ok := v.(string); ok {
			result[k] = s
		}
	}
	return result
}
