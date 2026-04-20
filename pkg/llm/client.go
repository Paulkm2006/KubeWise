package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// Config LLM配置
type Config struct {
	Model   string `json:"model"`
	APIKey  string `json:"api_key"`
	APIBase string `json:"api_base"`
}

// Message 聊天消息
type Message struct {
	Role         string        `json:"role"`
	Content      string        `json:"content"`
	FunctionCall *FunctionCall `json:"function_call,omitempty"`
}

// FunctionCall 函数调用
type FunctionCall struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments"`
}

// ChatCompletionRequest 聊天补全请求
type ChatCompletionRequest struct {
	Model     string               `json:"model"`
	Messages  []Message            `json:"messages"`
	Functions []FunctionDefinition `json:"functions,omitempty"`
}

// FunctionDefinition 函数定义
type FunctionDefinition struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}

// ChatCompletionResponse 聊天补全响应
type ChatCompletionResponse struct {
	Choices []struct {
		Message struct {
			Role         string        `json:"role"`
			Content      string        `json:"content"`
			FunctionCall *FunctionCall `json:"function_call,omitempty"`
		} `json:"message"`
		// 兼容GLM等模型的返回格式
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
}

// GLMFunctionCall GLM模型返回的函数调用格式
type GLMFunctionCall struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"parameters"`
}

// Client LLM客户端
type Client struct {
	config Config
	client *http.Client
}

// NewClient 创建LLM客户端
func NewClient(config Config) (*Client, error) {
	if config.APIBase == "" {
		config.APIBase = "https://api.openai.com/v1"
	}

	return &Client{
		config: config,
		client: &http.Client{},
	}, nil
}

// ChatCompletion 聊天补全
func (c *Client) ChatCompletion(ctx context.Context, messages []Message, functions []FunctionDefinition) (*Message, error) {
	reqBody := ChatCompletionRequest{
		Model:     c.config.Model,
		Messages:  messages,
		Functions: functions,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("序列化请求失败: %w", err)
	}

	url := fmt.Sprintf("%s/chat/completions", c.config.APIBase)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.config.APIKey))

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("发送请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API请求失败，状态码: %d，响应: %s", resp.StatusCode, string(body))
	}

	var respBody ChatCompletionResponse
	if err := json.NewDecoder(resp.Body).Decode(&respBody); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}

	if len(respBody.Choices) == 0 {
		return nil, fmt.Errorf("API返回空响应")
	}

	choice := respBody.Choices[0]
	message := &Message{
		Role:         choice.Message.Role,
		Content:      choice.Message.Content,
		FunctionCall: choice.Message.FunctionCall,
	}

	// 处理GLM等模型的特殊函数调用格式 <|FunctionCallBegin|>...<|FunctionCallEnd|>
	if message.FunctionCall == nil && strings.Contains(message.Content, "<|FunctionCallBegin|>") {
		// 提取函数调用内容
		startIdx := strings.Index(message.Content, "<|FunctionCallBegin|>") + len("<|FunctionCallBegin|>")
		endIdx := strings.Index(message.Content, "<|FunctionCallEnd|>")
		if endIdx > startIdx {
			funcCallJSON := strings.TrimSpace(message.Content[startIdx:endIdx])

			// 尝试解析为GLM格式的函数调用
			var glmFuncCalls []GLMFunctionCall
			if err := json.Unmarshal([]byte(funcCallJSON), &glmFuncCalls); err == nil && len(glmFuncCalls) > 0 {
				glmFunc := glmFuncCalls[0]
				message.FunctionCall = &FunctionCall{
					Name:      glmFunc.Name,
					Arguments: glmFunc.Arguments,
				}
				// 清空content，避免被当作普通回复
				message.Content = ""
			} else {
				// 尝试直接解析为单个FunctionCall
				var singleFuncCall FunctionCall
				if err := json.Unmarshal([]byte(funcCallJSON), &singleFuncCall); err == nil {
					message.FunctionCall = &singleFuncCall
					message.Content = ""
				}
			}
		}
	}

	return message, nil
}

// Completion 文本补全
func (c *Client) Completion(ctx context.Context, prompt string) (string, error) {
	messages := []Message{
		{Role: "user", Content: prompt},
	}

	resp, err := c.ChatCompletion(ctx, messages, nil)
	if err != nil {
		return "", err
	}

	return resp.Content, nil
}
