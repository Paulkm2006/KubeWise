package domain

import (
	"encoding/json"
	"fmt"
	"time"
)

// Conversation is a persisted chat thread in the Conversation bounded context.
type Conversation struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Messages  []Message `json:"messages"`

	InterruptedQuery string `json:"interrupted_query,omitempty"`
}

type Message struct {
	Role      string    `json:"role"`
	Content   string    `json:"content"`
	Blocks    []Block   `json:"blocks,omitempty"`
	Timestamp time.Time `json:"timestamp"`
	InTokens  int       `json:"in_tokens,omitempty"`
	OutTokens int       `json:"out_tokens,omitempty"`
	DurationS float64   `json:"duration_s,omitempty"`
}

type Block struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

type TablePayload struct {
	Headers []string   `json:"headers"`
	Rows    [][]string `json:"rows"`
}

type CodePayload struct {
	Language string `json:"language"`
	Content  string `json:"content"`
}

type KVPayload struct {
	Pairs []KVPair `json:"pairs"`
}

type KVPair struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type ListPayload struct {
	Items []ListItem `json:"items"`
}

type ListItem struct {
	Status string `json:"status"`
	Text   string `json:"text"`
}

type DetailPayload struct {
	Kind       string            `json:"kind"`
	Name       string            `json:"name"`
	Namespace  string            `json:"namespace"`
	Status     map[string]string `json:"status"`
	Containers []ContainerInfo   `json:"containers,omitempty"`
	Conditions []ConditionInfo   `json:"conditions,omitempty"`
	Events     []EventInfo       `json:"events,omitempty"`
	RecentLogs string            `json:"recent_logs,omitempty"`
	Labels     map[string]string `json:"labels,omitempty"`
}

type ContainerInfo struct {
	Name         string            `json:"name"`
	Image        string            `json:"image"`
	Ready        bool              `json:"ready"`
	RestartCount int32             `json:"restart_count"`
	State        string            `json:"state"`
	Resources    map[string]string `json:"resources,omitempty"`
}

type ConditionInfo struct {
	Type    string `json:"type"`
	Status  string `json:"status"`
	Reason  string `json:"reason"`
	Message string `json:"message"`
}

type EventInfo struct {
	Type      string `json:"type"`
	Reason    string `json:"reason"`
	Message   string `json:"message"`
	Timestamp string `json:"timestamp"`
}

func NewConversation() *Conversation {
	now := time.Now()
	id := fmt.Sprintf("%x", now.UnixNano())[:8]
	return &Conversation{
		ID:        id,
		CreatedAt: now,
		UpdatedAt: now,
	}
}

func TitleFromFirstMessage(content string) string {
	runes := []rune(content)
	if len(runes) <= 20 {
		return content
	}
	return string(runes[:20]) + "…"
}
