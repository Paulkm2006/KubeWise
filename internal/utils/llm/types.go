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
	Usage *Usage `json:"usage,omitempty"`
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

type ToolChoiceMode string

const (
	ToolChoiceAuto     ToolChoiceMode = "auto"
	ToolChoiceNone     ToolChoiceMode = "none"
	ToolChoiceRequired ToolChoiceMode = "required"
	ToolChoiceFunction ToolChoiceMode = "function"
)

type ToolChoice struct {
	Mode ToolChoiceMode `json:"mode"`
	Name string         `json:"name,omitempty"`
}

type ResponseFormatType string

const (
	ResponseFormatText       ResponseFormatType = "text"
	ResponseFormatJSONObject ResponseFormatType = "json_object"
	ResponseFormatJSONSchema ResponseFormatType = "json_schema"
)

type ResponseFormat struct {
	Type   ResponseFormatType `json:"type"`
	Name   string             `json:"name,omitempty"`
	Schema map[string]any     `json:"schema,omitempty"`
	Strict bool               `json:"strict,omitempty"`
}

type StreamEventType string

const (
	StreamEventTextDelta StreamEventType = "text_delta"
	StreamEventDone      StreamEventType = "done"
)

type StreamEvent struct {
	Type                 StreamEventType `json:"type"`
	Content              string          `json:"content,omitempty"`
	AccumulatedToolCalls []ToolCall      `json:"accumulated_tool_calls,omitempty"`
	Usage                *Usage          `json:"usage,omitempty"`
}

// StreamChunk is yielded by ChatCompletionStream for each delta.
type StreamChunk struct {
	Content              string     // text delta (empty for tool call deltas)
	AccumulatedToolCalls []ToolCall // set on the final chunk when finish_reason="tool_calls"
	Done                 bool       // true on the final chunk
	Usage                *Usage     // set on the final chunk
}

type CompletionRequest struct {
	Messages       []Message
	Tools          []FunctionDefinition
	ToolChoice     ToolChoice
	ResponseFormat *ResponseFormat
	Model          string
	Temperature    *float64
	MaxTokens      *int
	OnEvent        func(StreamEvent)
}

type CompletionResponse struct {
	Message      Message
	Usage        *Usage
	FinishReason string
	ParseError   string
}

// Config LLM客户端配置
type Config struct {
	Model   string `json:"model"`
	APIKey  string `json:"api_key"`
	APIBase string `json:"api_base"`
}
