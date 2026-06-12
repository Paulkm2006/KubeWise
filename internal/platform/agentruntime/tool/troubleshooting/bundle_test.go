package troubleshooting

import (
	"reflect"
	"testing"

	toolv2 "github.com/kubewise/kubewise/internal/platform/agentruntime/tool/v2"
)

func TestTroubleshootingToolsExposeToolV2Meta(t *testing.T) {
	tools := []toolv2.Tool{
		NewGetResourceEventsTool(nil),
		NewGetNodeStatusTool(nil),
		NewGetPodLogsTool(nil),
		NewGetServiceEndpointsTool(nil),
	}

	for _, tt := range tools {
		meta := tt.Meta()
		if meta.Version != "v2" {
			t.Fatalf("%s version = %q, want v2", meta.Name, meta.Version)
		}
		if meta.Capability != toolv2.CapabilityRead {
			t.Fatalf("%s capability = %q, want read", meta.Name, meta.Capability)
		}
		if meta.Risk != toolv2.RiskNone {
			t.Fatalf("%s risk = %q, want none", meta.Name, meta.Risk)
		}
		if meta.Confirm != toolv2.ConfirmNever {
			t.Fatalf("%s confirm = %q, want never", meta.Name, meta.Confirm)
		}
		if !reflect.DeepEqual(meta.Tags, []string{"troubleshooting", "recovery"}) {
			t.Fatalf("%s tags = %#v, want troubleshooting/recovery", meta.Name, meta.Tags)
		}
		if meta.Name == "" || meta.Description == "" || meta.Parameters == nil {
			t.Fatalf("%s has incomplete metadata: %#v", meta.Name, meta)
		}
	}
}

func TestRegisterToolsRegistersAllTroubleshootingTools(t *testing.T) {
	registry := toolv2.NewRegistry()
	if err := RegisterTools(registry, nil); err != nil {
		t.Fatalf("RegisterTools() error = %v", err)
	}

	want := []string{
		"get_node_status",
		"get_pod_logs",
		"get_resource_events",
		"get_service_endpoints",
	}
	if got := registry.Names(); !reflect.DeepEqual(got, want) {
		t.Fatalf("registry.Names() = %#v, want %#v", got, want)
	}
}
