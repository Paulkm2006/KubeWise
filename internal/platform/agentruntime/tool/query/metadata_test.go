package query

import (
	"testing"

	toolv2 "github.com/kubewise/kubewise/internal/platform/agentruntime/tool/v2"
)

func TestMapString_nilValue(t *testing.T) {
	m := map[string]interface{}{
		"name":      "foo",
		"namespace": nil,
	}
	if got := mapString(m, "namespace"); got != "" {
		t.Fatalf("namespace = %q, want empty", got)
	}
	if got := mapString(m, "name"); got != "foo" {
		t.Fatalf("name = %q", got)
	}
}

func TestRegisterQueryToolsNativeMetadata(t *testing.T) {
	reg := toolv2.NewRegistry()
	if err := RegisterQueryTools(reg, nil); err != nil {
		t.Fatalf("RegisterQueryTools() error = %v", err)
	}

	names := reg.Names()
	if len(names) != len(queryToolFactories) {
		t.Fatalf("registered %d tools, want %d: %v", len(names), len(queryToolFactories), names)
	}

	for _, name := range names {
		registered, ok := reg.Get(name)
		if !ok {
			t.Fatalf("registered tool %q not found", name)
		}
		meta := registered.Meta()
		if meta.Capability != toolv2.CapabilityRead {
			t.Errorf("%s capability = %q, want %q", name, meta.Capability, toolv2.CapabilityRead)
		}
		if meta.Risk != toolv2.RiskNone {
			t.Errorf("%s risk = %q, want %q", name, meta.Risk, toolv2.RiskNone)
		}
		if meta.Confirm != toolv2.ConfirmNever {
			t.Errorf("%s confirm = %q, want %q", name, meta.Confirm, toolv2.ConfirmNever)
		}
		if len(meta.Tags) != 1 || meta.Tags[0] != "query" {
			t.Errorf("%s tags = %v, want [query]", name, meta.Tags)
		}
		if meta.Description == "" {
			t.Errorf("%s description is empty", name)
		}
		if meta.Parameters == nil {
			t.Errorf("%s parameters are nil", name)
		}
	}
}
