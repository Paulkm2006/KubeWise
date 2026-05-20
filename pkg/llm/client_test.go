package llm

import (
	"encoding/json"
	"testing"
)

func TestMessageToOpenAIParam_AssistantWithToolCalls(t *testing.T) {
	msg := Message{
		Role:    "assistant",
		Content: "",
		ToolCalls: []ToolCall{
			{
				ID:   "call_abc",
				Type: "function",
				Function: FunctionCall{
					Name:      "list_namespaces",
					Arguments: map[string]any{},
				},
			},
		},
	}
	param, err := messageToOpenAIParam(msg)
	if err != nil {
		t.Fatal(err)
	}
	if param.OfAssistant == nil {
		t.Fatal("expected assistant param")
	}
	if len(param.OfAssistant.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(param.OfAssistant.ToolCalls))
	}
	fn := param.OfAssistant.ToolCalls[0].GetFunction()
	if fn == nil || fn.Name != "list_namespaces" {
		t.Fatalf("unexpected function: %+v", fn)
	}
	raw, err := json.Marshal(param)
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	toolCalls, ok := decoded["tool_calls"].([]any)
	if !ok || len(toolCalls) != 1 {
		t.Fatalf("marshaled JSON missing tool_calls: %s", string(raw))
	}
}

func TestMessageToOpenAIParam_ToolRequiresID(t *testing.T) {
	_, err := messageToOpenAIParam(Message{Role: "tool", Content: "ok"})
	if err == nil {
		t.Fatal("expected error for missing tool_call_id")
	}
}
