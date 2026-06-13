package troubleshooting

import (
	"fmt"

	toolv2 "github.com/kubewise/kubewise/internal/platform/agentruntime/tool/v2"
	"github.com/kubewise/kubewise/internal/platform/cluster"
)

const BundleTroubleshootingRecovery = "troubleshooting.recovery"

var ToolNames = []string{
	"get_resource_events",
	"get_node_status",
	"get_pod_logs",
	"get_service_endpoints",
}

func Bundle() toolv2.Bundle {
	return toolv2.Bundle{
		Name:  BundleTroubleshootingRecovery,
		Tools: append([]string(nil), ToolNames...),
	}
}

func NewRegistry(k8sClient *cluster.Client) (*toolv2.Registry, error) {
	registry := toolv2.NewRegistry()
	if err := RegisterTools(registry, k8sClient); err != nil {
		return nil, err
	}
	return registry, nil
}

func RegisterTools(registry *toolv2.Registry, k8sClient *cluster.Client) error {
	if registry == nil {
		return fmt.Errorf("toolv2 registry is required")
	}
	for _, t := range []toolv2.Tool{
		NewGetResourceEventsTool(k8sClient),
		NewGetNodeStatusTool(k8sClient),
		NewGetPodLogsTool(k8sClient),
		NewGetServiceEndpointsTool(k8sClient),
	} {
		if err := registry.Register(t); err != nil {
			return err
		}
	}
	return nil
}
