package tool

import (
	"context"
	"testing"
)

func TestLoadGlobalRegistryByCategory(t *testing.T) {
	// Save and restore global registry to avoid polluting other tests.
	saved := globalRegistryEntries
	defer func() { globalRegistryEntries = saved }()

	noop := func(dep any) (Tool, error) {
		return &noopTool{}, nil
	}

	globalRegistryEntries = []ToolMetadata{
		{Name: "read_tool", Category: "", Factory: noop},
		{Name: "op_tool", Category: "operation", Factory: noop},
	}

	t.Run("loads only operation tools", func(t *testing.T) {
		reg, err := LoadGlobalRegistryByCategory(ToolDependency{}, "operation")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if _, ok := reg.GetTool("op_tool"); !ok {
			t.Error("expected op_tool to be present")
		}
		if _, ok := reg.GetTool("read_tool"); ok {
			t.Error("expected read_tool to be absent")
		}
	})

	t.Run("loads only read tools (empty category)", func(t *testing.T) {
		reg, err := LoadGlobalRegistryByCategory(ToolDependency{}, "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if _, ok := reg.GetTool("read_tool"); !ok {
			t.Error("expected read_tool to be present")
		}
		if _, ok := reg.GetTool("op_tool"); ok {
			t.Error("expected op_tool to be absent")
		}
	})
}

// noopTool satisfies the Tool interface for testing.
type noopTool struct{}

func (n *noopTool) Name() string                                                { return "noop" }
func (n *noopTool) Description() string                                         { return "" }
func (n *noopTool) Parameters() map[string]any                                  { return nil }
func (n *noopTool) Execute(_ context.Context, _ map[string]any) (string, error) { return "", nil }
