package operation

import (
	"context"
	"fmt"

	"github.com/kubewise/kubewise/pkg/k8s"
	"github.com/kubewise/kubewise/pkg/tool"
	"sigs.k8s.io/yaml"
)

func init() {
	t := NewApplyResourceTool(nil)
	tool.RegisterGlobal(tool.ToolMetadata{
		Name:        t.Name(),
		Description: t.Description(),
		Parameters:  t.Parameters(),
		Category:    "operation",
		Factory: func(dep any) (tool.Tool, error) {
			d, ok := dep.(tool.ToolDependency)
			if !ok {
				return nil, fmt.Errorf("apply_resource: invalid dependency type")
			}
			return NewApplyResourceTool(d.K8sClient), nil
		},
	})
}

// ApplyResourceTool applies a Kubernetes resource from YAML content via Server-Side Apply.
type ApplyResourceTool struct {
	k8sClient *k8s.Client
}

// NewApplyResourceTool creates an ApplyResourceTool with the given K8s client.
func NewApplyResourceTool(k8sClient *k8s.Client) *ApplyResourceTool {
	return &ApplyResourceTool{k8sClient: k8sClient}
}

func (t *ApplyResourceTool) Name() string        { return "apply_resource" }
func (t *ApplyResourceTool) Description() string { return "Apply a Kubernetes resource from YAML content via Server-Side Apply" }
func (t *ApplyResourceTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"yaml_content": map[string]any{"type": "string", "description": "Full YAML content of the resource to apply"},
		},
		"required": []string{"yaml_content"},
	}
}

// Execute applies the YAML content to the cluster via Server-Side Apply.
func (t *ApplyResourceTool) Execute(ctx context.Context, args map[string]any) (string, error) {
	yamlContent, ok := args["yaml_content"].(string)
	if !ok || yamlContent == "" {
		return "", fmt.Errorf("apply_resource: missing or invalid 'yaml_content' argument")
	}

	if err := validateYAML(yamlContent); err != nil {
		return "", err
	}

	if err := t.k8sClient.ApplyResource(ctx, yamlContent); err != nil {
		return "", err
	}
	return "Successfully applied resource", nil
}

// validateYAML checks that the content is valid YAML and contains apiVersion and kind fields.
func validateYAML(content string) error {
	var obj map[string]any
	if err := yaml.Unmarshal([]byte(content), &obj); err != nil {
		return fmt.Errorf("apply_resource: invalid YAML: %w", err)
	}
	if obj["apiVersion"] == nil || obj["kind"] == nil {
		return fmt.Errorf("apply_resource: YAML must contain apiVersion and kind fields")
	}
	return nil
}
