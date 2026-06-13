package operation

import (
	"reflect"
	"testing"

	toolv2 "github.com/kubewise/kubewise/internal/platform/agentruntime/tool/v2"
)

func TestOperationWriteRegistryRegistersNativeV2Tools(t *testing.T) {
	reg, err := NewOperationWriteRegistry(nil)
	if err != nil {
		t.Fatalf("NewOperationWriteRegistry() error = %v", err)
	}

	wantNames := []string{
		"apply_resource",
		"cordon_drain_node",
		"delete_resource",
		"label_annotate_resource",
		"restart_resource",
		"scale_resource",
	}
	if got := reg.Names(); !reflect.DeepEqual(got, wantNames) {
		t.Fatalf("registry names = %#v, want %#v", got, wantNames)
	}

	bundle := OperationWriteBundle()
	if bundle.Name != toolv2.BundleOperationWrite {
		t.Fatalf("bundle name = %q, want %q", bundle.Name, toolv2.BundleOperationWrite)
	}
	if !reflect.DeepEqual(bundle.Tools, wantNames) {
		t.Fatalf("bundle tools = %#v, want %#v", bundle.Tools, wantNames)
	}

	for _, name := range wantNames {
		registered, ok := reg.Get(name)
		if !ok {
			t.Fatalf("tool %q not registered", name)
		}
		meta := registered.Meta()
		if meta.Name != name {
			t.Errorf("%s meta name = %q", name, meta.Name)
		}
		if meta.Version != "v2" {
			t.Errorf("%s version = %q, want v2", name, meta.Version)
		}
		if meta.Capability != toolv2.CapabilityWrite {
			t.Errorf("%s capability = %q, want %q", name, meta.Capability, toolv2.CapabilityWrite)
		}
		if meta.Risk != toolv2.RiskHigh {
			t.Errorf("%s risk = %q, want %q", name, meta.Risk, toolv2.RiskHigh)
		}
		if meta.Confirm != toolv2.ConfirmRequired {
			t.Errorf("%s confirm = %q, want %q", name, meta.Confirm, toolv2.ConfirmRequired)
		}
		if !reflect.DeepEqual(meta.Tags, []string{"operation", "write"}) {
			t.Errorf("%s tags = %#v, want operation/write", name, meta.Tags)
		}
		if meta.Description == "" {
			t.Errorf("%s description is empty", name)
		}
		if meta.Parameters == nil {
			t.Errorf("%s parameters are nil", name)
		}
	}
}
