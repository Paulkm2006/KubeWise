package query

import (
	"fmt"

	toolv2 "github.com/kubewise/kubewise/internal/platform/agentruntime/tool/v2"
	"github.com/kubewise/kubewise/internal/platform/cluster"
)

type queryToolFactory func(*cluster.Client) toolv2.Tool

var queryToolFactories = []queryToolFactory{
	func(k8sClient *cluster.Client) toolv2.Tool { return NewFindPodsUsingPVCTool(k8sClient) },
	func(k8sClient *cluster.Client) toolv2.Tool { return NewGetConfigMapContentTool(k8sClient) },
	func(k8sClient *cluster.Client) toolv2.Tool { return NewGetPodDetailTool(k8sClient) },
	func(k8sClient *cluster.Client) toolv2.Tool { return NewGetPodResourceUsageTool(k8sClient) },
	func(k8sClient *cluster.Client) toolv2.Tool { return NewGetResourceByGvrAndNameTool(k8sClient) },
	func(k8sClient *cluster.Client) toolv2.Tool { return NewGetCustomResourceByGvrAndNameTool(k8sClient) },
	func(k8sClient *cluster.Client) toolv2.Tool { return NewGetResourceDetailTool(k8sClient) },
	func(k8sClient *cluster.Client) toolv2.Tool { return NewListConfigMapsInNamespaceTool(k8sClient) },
	func(k8sClient *cluster.Client) toolv2.Tool { return NewListCustomResourcesByGvrTool(k8sClient) },
	func(k8sClient *cluster.Client) toolv2.Tool { return NewListNamespacesTool(k8sClient) },
	func(k8sClient *cluster.Client) toolv2.Tool { return NewListPersistentVolumeClaimsTool(k8sClient) },
	func(k8sClient *cluster.Client) toolv2.Tool { return NewListPersistentVolumesTool(k8sClient) },
	func(k8sClient *cluster.Client) toolv2.Tool { return NewListPodsInNamespaceTool(k8sClient) },
	func(k8sClient *cluster.Client) toolv2.Tool { return NewListResourcesByGvrTool(k8sClient) },
}

// RegisterQueryTools adds every native query tool to the provided registry.
func RegisterQueryTools(reg *toolv2.Registry, k8sClient *cluster.Client) error {
	if reg == nil {
		return fmt.Errorf("register query tools: registry is nil")
	}
	for _, factory := range queryToolFactories {
		if err := reg.Register(factory(k8sClient)); err != nil {
			return err
		}
	}
	return nil
}

// mapString safely reads a string field from unstructured object metadata.
func mapString(m map[string]interface{}, key string) string {
	if m == nil {
		return ""
	}
	v, ok := m[key]
	if !ok || v == nil {
		return ""
	}
	s, _ := v.(string)
	return s
}
