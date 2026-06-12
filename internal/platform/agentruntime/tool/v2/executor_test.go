package v2

import (
	"context"
	"testing"
)

type fakeTool struct {
	meta ToolMeta
}

func (f fakeTool) Meta() ToolMeta { return f.meta }

func (f fakeTool) Execute(context.Context, map[string]any) (ToolResult, error) {
	return ToolResult{Display: "ok"}, nil
}

func TestExecutorDeniesCapabilityNotAllowed(t *testing.T) {
	reg := NewRegistry()
	if err := reg.Register(fakeTool{meta: ToolMeta{
		Name:       "delete_pod",
		Capability: CapabilityWrite,
		Confirm:    ConfirmRequired,
	}}); err != nil {
		t.Fatal(err)
	}
	exec := NewExecutor(reg)

	_, err := exec.Execute(context.Background(), "delete_pod", nil, ExecutePolicy{
		AllowedCapabilities: []Capability{CapabilityRead},
	})
	if err == nil {
		t.Fatal("expected write tool to be denied by read-only policy")
	}
}

func TestExecutorAllowsReadToolAndSetsMetadata(t *testing.T) {
	reg := NewRegistry()
	if err := reg.Register(fakeTool{meta: ToolMeta{
		Name:         "get_pod",
		Version:      "v2",
		OutputSchema: "kubernetes.pod.v1",
		Capability:   CapabilityRead,
	}}); err != nil {
		t.Fatal(err)
	}
	exec := NewExecutor(reg)

	got, err := exec.Execute(context.Background(), "get_pod", nil, ExecutePolicy{
		AllowedCapabilities: []Capability{CapabilityRead},
	})
	if err != nil {
		t.Fatalf("Execute() err = %v", err)
	}
	if got.Meta.ToolName != "get_pod" || got.DataSchema != "kubernetes.pod.v1" {
		t.Fatalf("metadata not populated: %+v", got)
	}
}

func TestToolResultToLLMMessageIncludesWarnings(t *testing.T) {
	got := ToolResultToLLMMessage(ToolResult{
		Display: "pod fetched",
		Warnings: []Warning{
			{Code: "partial", Message: "logs unavailable"},
		},
	})
	if got == "pod fetched" {
		t.Fatal("expected warning text to be included")
	}
}
