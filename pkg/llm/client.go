package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"go.uber.org/zap"
)

// Client LLM客户端，封装openai-go SDK
type Client struct {
	client openai.Client
	config Config
	log    *zap.Logger
}

// SetLogger injects a logger for debug output.
func (c *Client) SetLogger(l *zap.Logger) { c.log = l }

func (c *Client) logger() *zap.Logger {
	if c.log == nil {
		return zap.NewNop()
	}
	return c.log
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
		log:    zap.NewNop(),
	}
	c.logger().Debug("llm client initialized", zap.String("model", config.Model))
	return c, nil
}

// toolCallAccum stores incremental tool call state during streaming.
type toolCallAccum struct {
	ID            string
	Type          string
	Name          string
	ArgumentsJSON strings.Builder
}

// ChatCompletion 聊天补全接口，支持工具调用。
// 内部始终使用流式 API 实现，若 onChunk 已设置则在每个 token delta 时回调，
// 对外仍表现为阻塞返回完整 *Message 的同步接口。
func (c *Client) ChatCompletion(ctx context.Context, messages []Message, functions []FunctionDefinition, onChunk func(StreamChunk)) (*Message, error) {
	openaiMessages := make([]openai.ChatCompletionMessageParamUnion, len(messages))
	for i, msg := range messages {
		param, err := messageToOpenAIParam(msg)
		if err != nil {
			return nil, fmt.Errorf("message[%d]: %w", i, err)
		}
		openaiMessages[i] = param
	}

	params := openai.ChatCompletionNewParams{
		Messages: openaiMessages,
		Model:    openai.ChatModel(c.config.Model),
	}

	reqOpts := []option.RequestOption{}

	if len(functions) > 0 {
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
		reqOpts = append(reqOpts, option.WithJSONSet("tools", tools))
	}

	// 请求最终 chunk 中的用量信息
	reqOpts = append(reqOpts, option.WithJSONSet("stream_options", map[string]any{
		"include_usage": true,
	}))

	toolNames := make([]string, 0, len(functions))
	for _, fn := range functions {
		toolNames = append(toolNames, fn.Name)
	}
	c.logger().Debug("chat completion request",
		zap.String("model", string(params.Model)),
		zap.Int("messages", len(messages)),
		zap.Strings("tools", toolNames),
	)

	s := c.client.Chat.Completions.NewStreaming(ctx, params, reqOpts...)
	defer s.Close()

	var content strings.Builder
	accum := make(map[int64]*toolCallAccum)
	var toolOrder []int64
	result := &Message{Role: "assistant"}
	var finalized bool    // 是否已通过 finish_reason 确定 content + tool_calls

	for s.Next() {
		chunk := s.Current()

		// 用法信息终包（stream_options: include_usage）
		if len(chunk.Choices) == 0 {
			if chunk.Usage.TotalTokens > 0 {
				result.Usage = &Usage{
					PromptTokens:     int(chunk.Usage.PromptTokens),
					CompletionTokens: int(chunk.Usage.CompletionTokens),
					TotalTokens:      int(chunk.Usage.TotalTokens),
				}
			}
			if finalized && onChunk != nil {
				onChunk(StreamChunk{Done: true, AccumulatedToolCalls: result.ToolCalls, Usage: result.Usage})
				onChunk = nil // 只发一次
			}
			continue
		}

		choice := chunk.Choices[0]
		delta := choice.Delta

		// 文本内容 delta
		if delta.Content != "" {
			content.WriteString(delta.Content)
			if onChunk != nil {
				onChunk(StreamChunk{Content: delta.Content})
			}
		}

		// 工具调用 delta（按 index 增量到达）
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

		// finish_reason → 组装最终 content 和 tool_calls
		if choice.FinishReason != "" {
			result.Content = content.String()

			if len(accum) > 0 {
				tcs := make([]ToolCall, 0, len(toolOrder))
				for _, idx := range toolOrder {
					a := accum[idx]
					var args map[string]any
					if a.ArgumentsJSON.Len() > 0 {
						_ = json.Unmarshal([]byte(a.ArgumentsJSON.String()), &args)
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
				result.ToolCalls = tcs
			}
			finalized = true
		}
	}

	if err := s.Err(); err != nil {
		c.logger().Error("chat completion failed",
			zap.Error(err),
			zap.String("model", string(params.Model)),
			zap.Int("messages", len(messages)),
			zap.String("message_summary", summarizeMessages(messages)),
		)
		return nil, fmt.Errorf("chat completion failed: %w", err)
	}

	// 未收到 usage-only chunk 时在此处发 Done
	if onChunk != nil && finalized {
		onChunk(StreamChunk{Done: true, AccumulatedToolCalls: result.ToolCalls, Usage: result.Usage})
	}

	// 如果全程未收到 finish_reason（极少见），补全 content
	if !finalized {
		result.Content = content.String()
	}

	fields := []zap.Field{
		zap.String("model", string(params.Model)),
		zap.Int("content_len", len(result.Content)),
		zap.Int("tool_calls", len(result.ToolCalls)),
	}
	if len(result.ToolCalls) > 0 {
		names := make([]string, 0, len(result.ToolCalls))
		for _, tc := range result.ToolCalls {
			names = append(names, tc.Function.Name)
		}
		fields = append(fields, zap.Strings("tool_call_names", names))
	}
	if result.Usage != nil {
		fields = append(fields,
			zap.Int("prompt_tokens", result.Usage.PromptTokens),
			zap.Int("completion_tokens", result.Usage.CompletionTokens),
			zap.Int("total_tokens", result.Usage.TotalTokens),
		)
	}
	c.logger().Info("chat completion response", fields...)

	return result, nil
}
