package llm

// Usage holds token counts returned by the LLM provider.
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// Message 聊天消息结构体，兼容多种LLM格式
type Message struct {
	Role       string     `json:"role"`
	Content    string     `json:"content,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
	// Usage is populated on assistant messages returned by ChatCompletion; ignored on input.
	Usage      *Usage     `json:"usage,omitempty"`
}

// ToolCall 工具调用结构体
type ToolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"`
	Function FunctionCall `json:"function"`
}

// FunctionCall 函数调用结构体
type FunctionCall struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments"`
}

// FunctionDefinition 工具定义结构体，用于LLM函数调用
type FunctionDefinition struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}

// Config LLM客户端配置
type Config struct {
	Model   string `json:"model"`
	APIKey  string `json:"api_key"`
	APIBase string `json:"api_base"`
}
