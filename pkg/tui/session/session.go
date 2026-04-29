package session

import (
	"encoding/json"
	"fmt"
	"time"
)

// Session represents one conversation thread.
type Session struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Messages  []Message `json:"messages"`

	InterruptedQuery string `json:"interrupted_query,omitempty"`
}

// Message is a single turn in the conversation.
type Message struct {
	Role      string    `json:"role"` // "user" | "assistant"
	Content   string    `json:"content"`
	Blocks    []Block   `json:"blocks,omitempty"`
	Timestamp time.Time `json:"timestamp"`
	InTokens  int       `json:"in_tokens,omitempty"`
	OutTokens int       `json:"out_tokens,omitempty"`
	DurationS float64   `json:"duration_s,omitempty"`
}

// Block is a typed payload inside an assistant message.
type Block struct {
	Type    string          `json:"type"` // "table"|"code"|"kv"|"list"|"text"
	Payload json.RawMessage `json:"payload"`
}

// TablePayload is the payload for a "table" block.
type TablePayload struct {
	Headers []string   `json:"headers"`
	Rows    [][]string `json:"rows"`
}

// CodePayload is the payload for a "code" block.
type CodePayload struct {
	Language string `json:"language"`
	Content  string `json:"content"`
}

// KVPayload is the payload for a "kv" block.
type KVPayload struct {
	Pairs []KVPair `json:"pairs"`
}

// KVPair is a single key-value pair.
type KVPair struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// ListPayload is the payload for a "list" block.
type ListPayload struct {
	Items []ListItem `json:"items"`
}

// ListItem is one status-bearing row in a list block.
type ListItem struct {
	Status string `json:"status"` // "ok"|"warn"|"error"|"info"
	Text   string `json:"text"`
}

// New creates a new session with a generated ID.
func New() *Session {
	now := time.Now()
	id := fmt.Sprintf("%x", now.UnixNano())[:8]
	return &Session{
		ID:        id,
		CreatedAt: now,
		UpdatedAt: now,
	}
}

// TitleFromFirstMessage derives a display title (max 20 chars) from the first user message.
func TitleFromFirstMessage(content string) string {
	runes := []rune(content)
	if len(runes) <= 20 {
		return content
	}
	return string(runes[:20]) + "…"
}
