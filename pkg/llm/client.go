package llm

import (
	"context"
	"fmt"

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

// ChatCompletion 聊天补全接口，支持工具调用
func (c *Client) ChatCompletion(ctx context.Context, messages []Message, functions []FunctionDefinition) (*Message, error) {
	openaiMessages := make([]openai.ChatCompletionMessageParamUnion, len(messages))
	for i, msg := range messages {
		param, err := messageToOpenAIParam(msg)
		if err != nil {
			return nil, fmt.Errorf("message[%d]: %w", i, err)
		}
		openaiMessages[i] = param
	}

	// 构建请求参数
	params := openai.ChatCompletionNewParams{
		Messages: openaiMessages,
		Model:    openai.ChatModel(c.config.Model),
	}

	// 构建请求选项
	reqOpts := []option.RequestOption{}

	// 如果有工具定义，通过JSON Set添加到请求中
	if len(functions) > 0 {
		// 转换工具定义
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

	toolNames := make([]string, 0, len(functions))
	for _, fn := range functions {
		toolNames = append(toolNames, fn.Name)
	}
	c.logger().Debug("chat completion request",
		zap.String("model", string(params.Model)),
		zap.Int("messages", len(messages)),
		zap.Strings("tools", toolNames),
	)
	resp, err := c.client.Chat.Completions.New(ctx, params, reqOpts...)
	if err != nil {
		c.logger().Error("chat completion failed",
			zap.Error(err),
			zap.String("model", string(params.Model)),
			zap.Int("messages", len(messages)),
			zap.String("message_summary", summarizeMessages(messages)),
		)
		return nil, fmt.Errorf("chat completion failed: %w", err)
	}

	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("no response from LLM")
	}

	choice := resp.Choices[0]

	// 转换响应回我们的Message格式
	result := &Message{
		Role:    string(choice.Message.Role),
		Content: choice.Message.Content,
	}

	if resp.Usage.TotalTokens > 0 {
		result.Usage = &Usage{
			PromptTokens:     int(resp.Usage.PromptTokens),
			CompletionTokens: int(resp.Usage.CompletionTokens),
			TotalTokens:      int(resp.Usage.TotalTokens),
		}
	}

	result.ToolCalls = toolCallsFromCompletionMessage(choice.Message)

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
