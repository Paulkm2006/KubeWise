package operation

import (
	"fmt"

	toolv2 "github.com/kubewise/kubewise/internal/platform/agentruntime/tool/v2"
	"github.com/kubewise/kubewise/internal/platform/cluster"
)

const operationToolVersion = "v2"

var operationWriteToolNames = []string{
	"apply_resource",
	"cordon_drain_node",
	"delete_resource",
	"label_annotate_resource",
	"restart_resource",
	"scale_resource",
}

func OperationWriteBundle() toolv2.Bundle {
	return toolv2.Bundle{
		Name:  toolv2.BundleOperationWrite,
		Tools: append([]string(nil), operationWriteToolNames...),
	}
}

func NewOperationWriteRegistry(k8sClient *cluster.Client) (*toolv2.Registry, error) {
	reg := toolv2.NewRegistry()
	if err := RegisterOperationWriteTools(reg, k8sClient); err != nil {
		return nil, err
	}
	return reg, nil
}

func RegisterOperationWriteTools(reg *toolv2.Registry, k8sClient *cluster.Client) error {
	if reg == nil {
		return fmt.Errorf("operation write registry is nil")
	}
	tools := []toolv2.Tool{
		NewApplyResourceTool(k8sClient),
		NewCordonDrainNodeTool(k8sClient),
		NewDeleteResourceTool(k8sClient),
		NewLabelAnnotateResourceTool(k8sClient),
		NewRestartResourceTool(k8sClient),
		NewScaleResourceTool(k8sClient),
	}
	for _, t := range tools {
		if err := reg.Register(t); err != nil {
			return err
		}
	}
	return nil
}

func operationWriteMeta(name, description string, parameters map[string]any) toolv2.ToolMeta {
	return toolv2.TextMeta(
		name,
		operationToolVersion,
		description,
		parameters,
		toolv2.CapabilityWrite,
		toolv2.RiskHigh,
		toolv2.ConfirmRequired,
		"operation",
		"write",
	)
}
