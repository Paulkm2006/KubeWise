package security

import (
	"reflect"
	"testing"

	toolv2 "github.com/kubewise/kubewise/internal/platform/agentruntime/tool/v2"
)

func TestRegisterAuditToolsRegistersNativeToolV2Tools(t *testing.T) {
	registry := toolv2.NewRegistry()
	if err := RegisterAuditTools(registry, nil); err != nil {
		t.Fatalf("RegisterAuditTools() error = %v", err)
	}

	wantNames := []string{
		"audit_image_security",
		"audit_network_policies",
		"audit_pod_security",
		"audit_rbac",
	}
	if got := registry.Names(); !reflect.DeepEqual(got, wantNames) {
		t.Fatalf("registry.Names() = %v, want %v", got, wantNames)
	}

	bundle := NewAuditBundle()
	if bundle.Name != toolv2.BundleSecurityAudit {
		t.Fatalf("NewAuditBundle().Name = %q, want %q", bundle.Name, toolv2.BundleSecurityAudit)
	}
	if !reflect.DeepEqual(bundle.Tools, wantNames) {
		t.Fatalf("NewAuditBundle().Tools = %v, want %v", bundle.Tools, wantNames)
	}

	for _, name := range wantNames {
		registeredTool, ok := registry.Get(name)
		if !ok {
			t.Fatalf("registry.Get(%q) returned false", name)
		}
		meta := registeredTool.Meta()
		if meta.Name != name {
			t.Fatalf("%s Meta().Name = %q", name, meta.Name)
		}
		if meta.Capability != toolv2.CapabilityAudit {
			t.Fatalf("%s Capability = %q, want %q", name, meta.Capability, toolv2.CapabilityAudit)
		}
		if meta.Risk != toolv2.RiskLow {
			t.Fatalf("%s Risk = %q, want %q", name, meta.Risk, toolv2.RiskLow)
		}
		if meta.Confirm != toolv2.ConfirmNever {
			t.Fatalf("%s Confirm = %q, want %q", name, meta.Confirm, toolv2.ConfirmNever)
		}
		if !reflect.DeepEqual(meta.Tags, []string{"security", "audit"}) {
			t.Fatalf("%s Tags = %v, want [security audit]", name, meta.Tags)
		}
		if meta.Description == "" {
			t.Fatalf("%s Description is empty", name)
		}
		if _, ok := meta.Parameters["properties"].(map[string]any)["namespace"]; !ok {
			t.Fatalf("%s Parameters missing namespace property", name)
		}
	}
}
