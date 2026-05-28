package api

import (
	"encoding/json"
	"time"
)

// --- Request types ---

type ChatRequest struct {
	Query   string `json:"query"`
	QueryID string `json:"query_id,omitempty"`
}

// InteractionAnswerRequest is the body for POST /api/v1/chat/interaction.
type InteractionAnswerRequest struct {
	InteractionID string          `json:"interaction_id"`
	Payload       json.RawMessage `json:"payload"`
}

type CreateSessionRequest struct {
	Title string `json:"title,omitempty"`
}

// --- Response types ---

type HealthResponse struct {
	Status  string `json:"status"`
	Version string `json:"version"`
}

type ChatResponse struct {
	QueryID string `json:"query_id"`
	Result  string `json:"result"`
}

type ErrorResponse struct {
	Error  string `json:"error"`
	Detail string `json:"detail,omitempty"`
}

type SessionResponse struct {
	ID           string    `json:"id"`
	Title        string    `json:"title"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
	MessageCount int       `json:"message_count"`
}

type SessionDetailResponse struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Messages  []Message `json:"messages"`
}

type SessionListResponse struct {
	Sessions []SessionResponse `json:"sessions"`
}

type Message struct {
	Role      string    `json:"role"`
	Content   string    `json:"content"`
	Timestamp time.Time `json:"timestamp"`
}

// --- SSE event payload types ---

type PhaseData struct {
	QueryID string `json:"query_id"`
	Phase   string `json:"phase"`
}

type AgentStartData struct {
	QueryID   string `json:"query_id"`
	AgentName string `json:"agent_name"`
}

type AgentDoneData struct {
	QueryID   string        `json:"query_id"`
	Duration  time.Duration `json:"duration"`
	InTokens  int           `json:"in_tokens"`
	OutTokens int           `json:"out_tokens"`
}

type ToolCallData struct {
	QueryID  string `json:"query_id"`
	ToolName string `json:"tool_name"`
	Step     int    `json:"step"`
}

type ToolDoneData struct {
	QueryID  string        `json:"query_id"`
	ToolName string        `json:"tool_name"`
	Step     int           `json:"step"`
	Elapsed  time.Duration `json:"elapsed"`
}

type ToolFailData struct {
	QueryID  string        `json:"query_id"`
	ToolName string        `json:"tool_name"`
	Step     int           `json:"step"`
	Elapsed  time.Duration `json:"elapsed"`
	Error    string        `json:"error"`
}

type LLMTextDeltaData struct {
	QueryID string `json:"query_id"`
	Delta   string `json:"delta"`
}

// InteractionRequestData is emitted as SSE event interaction_request.
type InteractionRequestData struct {
	InteractionID string          `json:"interaction_id"`
	QueryID       string          `json:"query_id"`
	Kind          string          `json:"kind"`
	Payload       json.RawMessage `json:"payload"`
	TotalSteps    int             `json:"total_steps,omitempty"`
}

type UnknownStreamEventData struct {
	EventType string `json:"event_type"`
}

type StreamDoneData struct {
	QueryID string `json:"query_id"`
	Result  string `json:"result"`
}

type StreamErrData struct {
	QueryID string `json:"query_id"`
	Error   string `json:"error"`
}

type SupervisorData struct {
	QueryID  string `json:"query_id"`
	Reason   string `json:"reason"`
	Decision string `json:"decision"`
	Detail   string `json:"detail"`
}
