package recovery

import (
	"fmt"

	"github.com/kubewise/kubewise/internal/utils/llm"
)

const maxRecoveryToolOutput = 12000

// Context tracks LLM conversation state during recovery.
type Context struct {
	messages        []llm.Message
	toolOutputLimit int
}

// NewContext creates a recovery conversation context.
func NewContext(initial []llm.Message, toolOutputLimit int) *Context {
	messages := make([]llm.Message, len(initial))
	copy(messages, initial)
	return &Context{messages: messages, toolOutputLimit: toolOutputLimit}
}

func (c *Context) Messages() []llm.Message {
	out := make([]llm.Message, len(c.messages))
	copy(out, c.messages)
	return out
}

func (c *Context) Len() int {
	return len(c.messages)
}

func (c *Context) AppendAssistant(msg llm.Message) {
	c.messages = append(c.messages, msg)
}

func (c *Context) AppendUser(content string) {
	c.messages = append(c.messages, llm.Message{Role: "user", Content: content})
}

func (c *Context) AppendToolResult(toolCallID, content string) {
	c.messages = append(c.messages, llm.Message{
		Role:       "tool",
		Content:    truncateToolResult(content, c.toolOutputLimit),
		ToolCallID: toolCallID,
	})
}

func truncateToolResult(s string, limit int) string {
	if limit <= 0 || len(s) <= limit {
		return s
	}
	return s[:limit] + fmt.Sprintf("\n...(输出已截断，原始长度 %d 字节)", len(s))
}

func truncateRecoveryToolOutput(s string) string {
	return truncateToolResult(s, maxRecoveryToolOutput)
}
