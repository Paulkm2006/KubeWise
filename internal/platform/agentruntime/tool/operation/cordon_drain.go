package operation

import (
	"context"
	"fmt"

	toolv2 "github.com/kubewise/kubewise/internal/platform/agentruntime/tool/v2"
	"github.com/kubewise/kubewise/internal/platform/cluster"
)

// CordonDrainNodeTool cordons, uncordons, or drains a Kubernetes node.
type CordonDrainNodeTool struct {
	k8sClient *cluster.Client
}

// NewCordonDrainNodeTool creates a CordonDrainNodeTool with the given K8s client.
func NewCordonDrainNodeTool(k8sClient *cluster.Client) *CordonDrainNodeTool {
	return &CordonDrainNodeTool{k8sClient: k8sClient}
}

func (t *CordonDrainNodeTool) Name() string { return "cordon_drain_node" }
func (t *CordonDrainNodeTool) Description() string {
	return "Cordon, uncordon, or drain a Kubernetes node"
}
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

func (t *CordonDrainNodeTool) Meta() toolv2.ToolMeta {
	return operationWriteMeta(t.Name(), t.Description(), t.Parameters())
}

func (t *CordonDrainNodeTool) Execute(ctx context.Context, args map[string]any) (toolv2.ToolResult, error) {
	meta := t.Meta()
	text, err := t.executeText(ctx, args)
	if err != nil {
		return toolv2.ToolResult{}, err
	}
	return toolv2.TextResult(meta, text), nil
}

// executeText performs the cordon, uncordon, or drain action on the target node.
func (t *CordonDrainNodeTool) executeText(ctx context.Context, args map[string]any) (string, error) {
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
