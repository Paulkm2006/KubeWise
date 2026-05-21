package llm

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/openai/openai-go/v3"
)

func summarizeMessages(messages []Message) string {
	parts := make([]string, 0, len(messages))
	for i, m := range messages {
		var extra string
		switch m.Role {
		case "assistant":
			extra = fmt.Sprintf(" tool_calls=%d", len(m.ToolCalls))
		case "tool", "function":
			extra = fmt.Sprintf(" tool_call_id=%q", m.ToolCallID)
		}
		parts = append(parts, fmt.Sprintf("[%d]%s%s", i, m.Role, extra))
	}
	return strings.Join(parts, " ")
}

func messageToOpenAIParam(msg Message) (openai.ChatCompletionMessageParamUnion, error) {
	switch msg.Role {
	case "user":
		return openai.UserMessage(msg.Content), nil
	case "system":
		return openai.SystemMessage(msg.Content), nil
	case "developer":
		return openai.DeveloperMessage(msg.Content), nil
	case "assistant":
		var asst openai.ChatCompletionAssistantMessageParam
		if msg.Content != "" {
			asst.Content.OfString = openai.String(msg.Content)
		}
		for i, tc := range msg.ToolCalls {
			param, err := toolCallToOpenAIParam(tc, i)
			if err != nil {
				return openai.ChatCompletionMessageParamUnion{}, err
			}
			asst.ToolCalls = append(asst.ToolCalls, param)
		}
		return openai.ChatCompletionMessageParamUnion{OfAssistant: &asst}, nil
	case "tool", "function":
		if msg.ToolCallID == "" {
			return openai.ChatCompletionMessageParamUnion{}, fmt.Errorf("tool message missing tool_call_id")
		}
		return openai.ToolMessage(msg.Content, msg.ToolCallID), nil
	default:
		return openai.ChatCompletionMessageParamUnion{}, fmt.Errorf("unsupported message role: %s", msg.Role)
	}
}

func toolCallToOpenAIParam(tc ToolCall, index int) (openai.ChatCompletionMessageToolCallUnionParam, error) {
	id := tc.ID
	if id == "" {
		id = fmt.Sprintf("call_%d", index)
	}
	argsJSON, err := json.Marshal(tc.Function.Arguments)
	if err != nil {
		return openai.ChatCompletionMessageToolCallUnionParam{}, err
	}
	return openai.ChatCompletionMessageToolCallUnionParam{
		OfFunction: &openai.ChatCompletionMessageFunctionToolCallParam{
			ID: id,
			Function: openai.ChatCompletionMessageFunctionToolCallFunctionParam{
				Name:      tc.Function.Name,
				Arguments: string(argsJSON),
			},
		},
	}, nil
}

func toolCallsFromCompletionMessage(msg openai.ChatCompletionMessage) []ToolCall {
	if len(msg.ToolCalls) == 0 {
		return nil
	}
	out := make([]ToolCall, 0, len(msg.ToolCalls))
	for _, tc := range msg.ToolCalls {
		if tc.Type != "" && tc.Type != "function" {
			continue
		}
		id := tc.ID
		name := tc.Function.Name
		argsStr := tc.Function.Arguments
		var args map[string]any
		if argsStr != "" {
			if err := json.Unmarshal([]byte(argsStr), &args); err != nil {
				args = map[string]any{"raw_arguments": argsStr}
			}
		}
		if args == nil {
			args = map[string]any{}
		}
		out = append(out, ToolCall{
			ID:   id,
			Type: tc.Type,
			Function: FunctionCall{
				Name:      name,
				Arguments: args,
			},
		})
	}
	return out
}
