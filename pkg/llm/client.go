package llm

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
)

// Client LLM客户端，封装openai-go SDK
type Client struct {
	client openai.Client
	config Config
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

	return &Client{
		client: client,
		config: config,
	}, nil
}

// ChatCompletion 聊天补全接口，支持工具调用
func (c *Client) ChatCompletion(ctx context.Context, messages []Message, functions []FunctionDefinition) (*Message, error) {
	// 转换消息格式到openai格式
	openaiMessages := make([]openai.ChatCompletionMessageParamUnion, len(messages))
	for i, msg := range messages {
		switch msg.Role {
		case "user":
			openaiMessages[i] = openai.UserMessage(msg.Content)
		case "assistant":
			openaiMessages[i] = openai.AssistantMessage(msg.Content)
		case "system":
			openaiMessages[i] = openai.SystemMessage(msg.Content)
		case "developer":
			openaiMessages[i] = openai.DeveloperMessage(msg.Content)
		case "tool", "function":
			// 工具返回消息
			openaiMessages[i] = openai.ToolMessage(msg.Content, msg.ToolCallID)
		default:
			return nil, fmt.Errorf("unsupported message role: %s", msg.Role)
		}
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

	// 调用OpenAI API
	resp, err := c.client.Chat.Completions.New(ctx, params, reqOpts...)
	if err != nil {
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

	// 检查是否有工具调用（通过原始JSON解析）
	var rawResp map[string]any
	respJSON, err := json.Marshal(resp)
	if err == nil {
		if err := json.Unmarshal(respJSON, &rawResp); err == nil {
			if choices, ok := rawResp["choices"].([]any); ok && len(choices) > 0 {
				if choice, ok := choices[0].(map[string]any); ok {
					if message, ok := choice["message"].(map[string]any); ok {
						// 处理工具调用
						if toolCallsRaw, ok := message["tool_calls"].([]any); ok && len(toolCallsRaw) > 0 {
							result.ToolCalls = make([]ToolCall, len(toolCallsRaw))
							for j, tcRaw := range toolCallsRaw {
								if tc, ok := tcRaw.(map[string]any); ok {
									functionRaw, _ := tc["function"].(map[string]any)
									if functionRaw == nil {
										continue
									}

									name, _ := functionRaw["name"].(string)
									argsStr, _ := functionRaw["arguments"].(string)

									var args map[string]any
									if err := json.Unmarshal([]byte(argsStr), &args); err != nil {
										args = map[string]any{
											"raw_arguments": argsStr,
										}
									}

									id, _ := tc["id"].(string)
									typeStr, _ := tc["type"].(string)

									result.ToolCalls[j] = ToolCall{
										ID:   id,
										Type: typeStr,
										Function: FunctionCall{
											Name:      name,
											Arguments: args,
										},
									}
								}
							}

						}
					}
				}
			}
		}
	}

	return result, nil
}
