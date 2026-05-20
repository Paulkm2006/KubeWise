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

type ConfirmRequest struct {
	ConfirmID  string `json:"confirm_id"`
	Confirmed  bool   `json:"confirmed"`
	Correction string `json:"correction,omitempty"`
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

type RenderTextData struct {
	QueryID string `json:"query_id"`
	Text    string `json:"text"`
}

type RenderTableData struct {
	QueryID string     `json:"query_id"`
	Headers []string   `json:"headers"`
	Rows    [][]string `json:"rows"`
}

type RenderCodeData struct {
	QueryID  string `json:"query_id"`
	Language string `json:"language"`
	Content  string `json:"content"`
}

type RenderKVData struct {
	QueryID string   `json:"query_id"`
	Pairs   []KVPair `json:"pairs"`
}

type KVPair struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type RenderListData struct {
	QueryID string     `json:"query_id"`
	Items   []ListItem `json:"items"`
}

type ListItem struct {
	Status string `json:"status"`
	Text   string `json:"text"`
}

type ConfirmRequestData struct {
	ConfirmID  string          `json:"confirm_id"`
	QueryID    string          `json:"query_id"`
	Step       json.RawMessage `json:"step"`
	TotalSteps int             `json:"total_steps"`
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

type RenderDetailData struct {
	QueryID string             `json:"query_id"`
	Detail  ResourceDetailData `json:"detail"`
}

type ResourceDetailData struct {
	Kind       string               `json:"kind"`
	Name       string               `json:"name"`
	Namespace  string               `json:"namespace"`
	Status     map[string]string    `json:"status"`
	Containers []ContainerInfoData  `json:"containers,omitempty"`
	Conditions []ConditionInfoData  `json:"conditions,omitempty"`
	Events     []EventInfoData      `json:"events,omitempty"`
	RecentLogs string               `json:"recent_logs,omitempty"`
	Labels     map[string]string    `json:"labels,omitempty"`
}

type ContainerInfoData struct {
	Name         string            `json:"name"`
	Image        string            `json:"image"`
	Ready        bool              `json:"ready"`
	RestartCount int32             `json:"restart_count"`
	State        string            `json:"state"`
	Resources    map[string]string `json:"resources,omitempty"`
}

type ConditionInfoData struct {
	Type    string `json:"type"`
	Status  string `json:"status"`
	Reason  string `json:"reason"`
	Message string `json:"message"`
}

type EventInfoData struct {
	Type      string `json:"type"`
	Reason    string `json:"reason"`
	Message   string `json:"message"`
	Timestamp string `json:"timestamp"`
}
