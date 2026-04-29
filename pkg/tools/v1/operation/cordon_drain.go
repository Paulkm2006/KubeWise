package operation

import (
	"context"
	"fmt"

	"github.com/kubewise/kubewise/pkg/k8s"
	"github.com/kubewise/kubewise/pkg/tool"
)

func init() {
	t := NewCordonDrainNodeTool(nil)
	tool.RegisterGlobal(tool.ToolMetadata{
		Name:        t.Name(),
		Description: t.Description(),
		Parameters:  t.Parameters(),
		Category:    "operation",
		Factory: func(dep any) (tool.Tool, error) {
			d, ok := dep.(tool.ToolDependency)
			if !ok {
				return nil, fmt.Errorf("cordon_drain_node: invalid dependency type")
			}
			return NewCordonDrainNodeTool(d.K8sClient), nil
		},
	})
}

// CordonDrainNodeTool cordons, uncordons, or drains a Kubernetes node.
type CordonDrainNodeTool struct {
	k8sClient *k8s.Client
}

// NewCordonDrainNodeTool creates a CordonDrainNodeTool with the given K8s client.
func NewCordonDrainNodeTool(k8sClient *k8s.Client) *CordonDrainNodeTool {
	return &CordonDrainNodeTool{k8sClient: k8sClient}
}

func (t *CordonDrainNodeTool) Name() string        { return "cordon_drain_node" }
func (t *CordonDrainNodeTool) Description() string { return "Cordon, uncordon, or drain a Kubernetes node" }
func (t *CordonDrainNodeTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"node_name": map[string]any{"type": "string", "description": "Name of the node"},
			"action":    map[string]any{"type": "string", "description": "Action to perform: cordon, uncordon, or drain"},
		},
		"required": []string{"node_name", "action"},
	}
}

// Execute performs the cordon, uncordon, or drain action on the target node.
func (t *CordonDrainNodeTool) Execute(ctx context.Context, args map[string]any) (string, error) {
	nodeName, ok := args["node_name"].(string)
	if !ok || nodeName == "" {
		return "", fmt.Errorf("cordon_drain_node: missing or invalid 'node_name' argument")
	}
	action, ok := args["action"].(string)
	if !ok || action == "" {
		return "", fmt.Errorf("cordon_drain_node: missing or invalid 'action' argument")
	}

	switch action {
	case "cordon":
		if err := t.k8sClient.CordonNode(ctx, nodeName, true); err != nil {
			return "", err
		}
		return fmt.Sprintf("Successfully cordoned node %s", nodeName), nil
	case "uncordon":
		if err := t.k8sClient.CordonNode(ctx, nodeName, false); err != nil {
			return "", err
		}
		return fmt.Sprintf("Successfully uncordoned node %s", nodeName), nil
	case "drain":
		// Cordon first to prevent new pods from being scheduled during the drain.
		if err := t.k8sClient.CordonNode(ctx, nodeName, true); err != nil {
			return "", fmt.Errorf("cordon_drain_node: failed to cordon node before drain: %w", err)
		}
		evicted, remaining, err := t.k8sClient.DrainNode(ctx, nodeName)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("Drained node %s: evicted %d pods, %d pods remaining", nodeName, len(evicted), len(remaining)), nil
	default:
		return "", fmt.Errorf("cordon_drain_node: unknown action %s, must be cordon/uncordon/drain", action)
	}
}
