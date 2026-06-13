package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"go.uber.org/zap"

	kcfg "github.com/kubewise/kubewise/internal/config"
)

// Client LLM客户端，封装openai-go SDK
type Client struct {
	client openai.Client
	config Config
}

func (c *Client) logger() *zap.Logger {
	return kcfg.L()
}

// NewClient 创建新的LLM客户端
func NewClient(config Config) (*Client, error) {
	if config.APIKey == "" {
		return nil, fmt.Errorf("API key is required")
	}
	if config.Model == "" {
		config.Model = "glm-5.1" // 默认模型
	}

	// 初始化openai客户端
	opts := []option.RequestOption{
		option.WithAPIKey(config.APIKey),
	}
	if config.APIBase != "" {
		opts = append(opts, option.WithBaseURL(config.APIBase))
	}

	client := openai.NewClient(opts...)

	c := &Client{
		client: client,
		config: config,
	}
	kcfg.L().Debug("llm client initialized", zap.String("model", config.Model))
	return c, nil
}

// toolCallAccum stores incremental tool call state during streaming.
type toolCallAccum struct {
	ID            string
	Type          string
	Name          string
	ArgumentsJSON strings.Builder
}

// ChatCompletion keeps the legacy call shape while delegating to the platform
// request/response API.
func (c *Client) ChatCompletion(ctx context.Context, messages []Message, functions []FunctionDefinition, onChunk func(StreamChunk)) (*Message, error) {
	var onEvent func(StreamEvent)
	if onChunk != nil {
		onEvent = func(ev StreamEvent) {
			onChunk(StreamChunk{
				Content:              ev.Content,
				AccumulatedToolCalls: ev.AccumulatedToolCalls,
				Done:                 ev.Type == StreamEventDone,
				Usage:                ev.Usage,
			})
		}
	}
	resp, err := c.Complete(ctx, CompletionRequest{Messages: messages, Tools: functions, OnEvent: onEvent})
	if err != nil {
		return nil, err
	}
	return &resp.Message, nil
}

// Complete is the platform LLM port used by agent runtimes. It preserves the
// current streaming implementation while accepting per-call options.
func (c *Client) Complete(ctx context.Context, req CompletionRequest) (*CompletionResponse, error) {
	openaiMessages := make([]openai.ChatCompletionMessageParamUnion, len(req.Messages))
	for i, msg := range req.Messages {
		param, err := messageToOpenAIParam(msg)
		if err != nil {
			return nil, fmt.Errorf("message[%d]: %w", i, err)
		}
		openaiMessages[i] = param
	}

	model := req.Model
	if model == "" {
		model = c.config.Model
	}
	params := openai.ChatCompletionNewParams{
		Messages: openaiMessages,
		Model:    openai.ChatModel(model),
	}

	reqOpts := []option.RequestOption{
		option.WithJSONSet("stream_options", map[string]any{"include_usage": true}),
	}
	if req.Temperature != nil {
		reqOpts = append(reqOpts, option.WithJSONSet("temperature", *req.Temperature))
	}
	if req.MaxTokens != nil {
		reqOpts = append(reqOpts, option.WithJSONSet("max_tokens", *req.MaxTokens))
	}
	if req.ResponseFormat != nil {
		reqOpts = append(reqOpts, option.WithJSONSet("response_format", responseFormatJSON(*req.ResponseFormat)))
	}
	if req.ToolChoice.Mode != "" {
		reqOpts = append(reqOpts, option.WithJSONSet("tool_choice", toolChoiceJSON(req.ToolChoice)))
	}
	if len(req.Tools) > 0 {
		reqOpts = append(reqOpts, option.WithJSONSet("tools", functionDefinitionsJSON(req.Tools)))
	}

	toolNames := make([]string, 0, len(req.Tools))
	for _, fn := range req.Tools {
		toolNames = append(toolNames, fn.Name)
	}
	c.logger().Debug("chat completion request",
		zap.String("model", string(params.Model)),
		zap.Int("messages", len(req.Messages)),
		zap.Strings("tools", toolNames),
	)

	s := c.client.Chat.Completions.NewStreaming(ctx, params, reqOpts...)
	defer s.Close()

	var content strings.Builder
	accum := make(map[int64]*toolCallAccum)
	var toolOrder []int64
	result := &Message{Role: "assistant"}
	var finishReason string
	var finalized bool
	onEvent := req.OnEvent

	for s.Next() {
		chunk := s.Current()
		if len(chunk.Choices) == 0 {
			if chunk.Usage.TotalTokens > 0 {
				result.Usage = &Usage{
					PromptTokens:     int(chunk.Usage.PromptTokens),
					CompletionTokens: int(chunk.Usage.CompletionTokens),
					TotalTokens:      int(chunk.Usage.TotalTokens),
				}
			}
			if finalized && onEvent != nil {
				onEvent(StreamEvent{Type: StreamEventDone, AccumulatedToolCalls: result.ToolCalls, Usage: result.Usage})
				onEvent = nil
			}
			continue
		}

		choice := chunk.Choices[0]
		delta := choice.Delta
		if delta.Content != "" {
			content.WriteString(delta.Content)
			if onEvent != nil {
				onEvent(StreamEvent{Type: StreamEventTextDelta, Content: delta.Content})
			}
		}

		for _, tc := range delta.ToolCalls {
			idx := tc.Index
			a, exists := accum[idx]
			if !exists {
				toolOrder = append(toolOrder, idx)
				a = &toolCallAccum{}
				accum[idx] = a
			}
			if tc.ID != "" {
				a.ID = tc.ID
			}
			if tc.Type != "" {
				a.Type = tc.Type
			}
			if tc.Function.Name != "" {
				a.Name = tc.Function.Name
			}
			if tc.Function.Arguments != "" {
				a.ArgumentsJSON.WriteString(tc.Function.Arguments)
			}
		}

		if choice.FinishReason != "" {
			finishReason = string(choice.FinishReason)
			result.Content = content.String()
			result.ToolCalls = accumulatedToolCalls(toolOrder, accum)
			finalized = true
		}
	}

	if err := s.Err(); err != nil {
		c.logger().Error("chat completion failed",
			zap.Error(err),
			zap.String("model", string(params.Model)),
			zap.Int("messages", len(req.Messages)),
			zap.String("message_summary", summarizeMessages(req.Messages)),
		)
		return nil, fmt.Errorf("chat completion failed: %w", err)
	}

	if onEvent != nil && finalized {
		onEvent(StreamEvent{Type: StreamEventDone, AccumulatedToolCalls: result.ToolCalls, Usage: result.Usage})
	}
	if !finalized {
		result.Content = content.String()
		result.ToolCalls = accumulatedToolCalls(toolOrder, accum)
	}

	fields := []zap.Field{
		zap.String("model", string(params.Model)),
		zap.Int("content_len", len(result.Content)),
		zap.Int("tool_calls", len(result.ToolCalls)),
	}
	if result.Usage != nil {
		fields = append(fields,
			zap.Int("prompt_tokens", result.Usage.PromptTokens),
			zap.Int("completion_tokens", result.Usage.CompletionTokens),
			zap.Int("total_tokens", result.Usage.TotalTokens),
		)
	}
	c.logger().Info("chat completion response", fields...)

	return &CompletionResponse{
		Message:      *result,
		Usage:        result.Usage,
		FinishReason: finishReason,
	}, nil
}

func accumulatedToolCalls(order []int64, accum map[int64]*toolCallAccum) []ToolCall {
	if len(accum) == 0 {
		return nil
	}
	tcs := make([]ToolCall, 0, len(order))
	for _, idx := range order {
		a := accum[idx]
		var args map[string]any
		if a.ArgumentsJSON.Len() > 0 {
			argsStr := a.ArgumentsJSON.String()
			if err := json.Unmarshal([]byte(argsStr), &args); err != nil {
				args = map[string]any{"raw_arguments": argsStr}
			}
		}
		if args == nil {
			args = make(map[string]any)
		}
		tcs = append(tcs, ToolCall{
			ID:   a.ID,
			Type: a.Type,
			Function: FunctionCall{
				Name:      a.Name,
				Arguments: args,
			},
		})
	}
	return tcs
}

func functionDefinitionsJSON(functions []FunctionDefinition) []map[string]any {
	tools := make([]map[string]any, len(functions))
	for i, fn := range functions {
		tools[i] = map[string]any{
			"type": "function",
			"function": map[string]any{
				"name":        fn.Name,
				"description": fn.Description,
				"parameters":  fn.Parameters,
			},
		}
	}
	return tools
}

func toolChoiceJSON(choice ToolChoice) any {
	switch choice.Mode {
	case ToolChoiceFunction:
		return map[string]any{
			"type": "function",
			"function": map[string]any{
				"name": choice.Name,
			},
		}
	case ToolChoiceAuto, ToolChoiceNone, ToolChoiceRequired:
		return string(choice.Mode)
	default:
		return string(ToolChoiceAuto)
	}
}

func responseFormatJSON(format ResponseFormat) map[string]any {
	switch format.Type {
	case ResponseFormatJSONSchema:
		return map[string]any{
			"type": "json_schema",
			"json_schema": map[string]any{
				"name":   format.Name,
				"schema": format.Schema,
				"strict": format.Strict,
			},
		}
	case ResponseFormatJSONObject:
		return map[string]any{"type": "json_object"}
	default:
		return map[string]any{"type": "text"}
	}
}
