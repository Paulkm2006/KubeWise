package loop

import (
	"context"
	"testing"

	toolv2 "github.com/kubewise/kubewise/internal/platform/agentruntime/tool/v2"
	"github.com/kubewise/kubewise/internal/utils/llm"
)

type fakeLLM struct {
	responses []llm.CompletionResponse
	calls     int
}

func (f *fakeLLM) Complete(context.Context, llm.CompletionRequest) (*llm.CompletionResponse, error) {
	f.calls++
	idx := f.calls - 1
	if idx >= len(f.responses) {
		idx = len(f.responses) - 1
	}
	return &f.responses[idx], nil
}

type loopTool struct{}

func (loopTool) Meta() toolv2.ToolMeta {
	return toolv2.ToolMeta{
		Name:       "get_status",
		Version:    "v2",
		Capability: toolv2.CapabilityRead,
		Confirm:    toolv2.ConfirmNever,
		Parameters: map[string]any{"type": "object"},
	}
}

func (loopTool) Execute(context.Context, map[string]any) (toolv2.ToolResult, error) {
	return toolv2.ToolResult{Display: "pod is running"}, nil
}

func TestRunExecutesToolThenReturnsAnswer(t *testing.T) {
	reg := toolv2.NewRegistry()
	if err := reg.Register(loopTool{}); err != nil {
		t.Fatal(err)
	}
	model := &fakeLLM{responses: []llm.CompletionResponse{
		{Message: llm.Message{
			Role: "assistant",
			ToolCalls: []llm.ToolCall{{
				ID: "call-1",
				Function: llm.FunctionCall{
					Name:      "get_status",
					Arguments: map[string]any{},
				},
			}},
		}},
		{Message: llm.Message{Role: "assistant", Content: "pod is running"}},
	}}

	got, err := Run(context.Background(), Config{
		QueryID:  "q1",
		LLM:      model,
		Messages: []llm.Message{{Role: "user", Content: "status?"}},
		Tools:    reg.Definitions(reg.Names()),
		Executor: toolv2.NewExecutor(reg),
		Policy: toolv2.ExecutePolicy{
			AllowedCapabilities: []toolv2.Capability{toolv2.CapabilityRead},
			AllowedTools:        reg.Names(),
		},
		MaxSteps: 3,
	})
	if err != nil {
		t.Fatalf("Run() err = %v", err)
	}
	if got.Content != "pod is running" {
		t.Fatalf("unexpected content %q", got.Content)
	}
	if model.calls != 2 {
		t.Fatalf("expected 2 model calls, got %d", model.calls)
	}
}
