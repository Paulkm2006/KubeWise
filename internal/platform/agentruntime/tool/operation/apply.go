package operation

import (
	"context"
	"fmt"

	toolv2 "github.com/kubewise/kubewise/internal/platform/agentruntime/tool/v2"
	"github.com/kubewise/kubewise/internal/platform/cluster"
	"sigs.k8s.io/yaml"
)

// ApplyResourceTool applies a Kubernetes resource from YAML content via Server-Side Apply.
type ApplyResourceTool struct {
	k8sClient *cluster.Client
}

// NewApplyResourceTool creates an ApplyResourceTool with the given K8s client.
func NewApplyResourceTool(k8sClient *cluster.Client) *ApplyResourceTool {
	return &ApplyResourceTool{k8sClient: k8sClient}
}

func (t *ApplyResourceTool) Name() string { return "apply_resource" }
func (t *ApplyResourceTool) Description() string {
	return "Apply a Kubernetes resource from YAML content via Server-Side Apply"
}
func (t *ApplyResourceTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"yaml_content": map[string]any{"type": "string", "description": "Full YAML content of the resource to apply"},
		},
		"required": []string{"yaml_content"},
	}
}

func (t *ApplyResourceTool) Meta() toolv2.ToolMeta {
	return operationWriteMeta(t.Name(), t.Description(), t.Parameters())
}

func (t *ApplyResourceTool) Execute(ctx context.Context, args map[string]any) (toolv2.ToolResult, error) {
	meta := t.Meta()
	text, err := t.executeText(ctx, args)
	if err != nil {
		return toolv2.ToolResult{}, err
	}
	return toolv2.TextResult(meta, text), nil
}

// executeText applies the YAML content to the cluster via Server-Side Apply.
func (t *ApplyResourceTool) executeText(ctx context.Context, args map[string]any) (string, error) {
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
	if obj == nil || obj["apiVersion"] == nil || obj["kind"] == nil {
		return fmt.Errorf("apply_resource: YAML must contain apiVersion and kind fields")
	}
	return nil
}
