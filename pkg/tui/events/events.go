package events

import (
	"time"

	"github.com/kubewise/kubewise/pkg/catalog"
)

// KVPair is a single key-value entry in render output.
type KVPair struct {
	Key   string
	Value string
}

// ListItem is a single status-bearing line in render output.
type ListItem struct {
	Status string // "ok" | "warn" | "error" | "info"
	Text   string
}

// ResourceDetail carries structured information about a specific resource.
type ResourceDetail struct {
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

// ContainerInfo describes a container within a Pod.
type ContainerInfo struct {
	Name         string            `json:"name"`
	Image        string            `json:"image"`
	Ready        bool              `json:"ready"`
	RestartCount int32             `json:"restart_count"`
	State        string            `json:"state"`
	Resources    map[string]string `json:"resources,omitempty"`
}

// ConditionInfo describes a resource condition.
type ConditionInfo struct {
	Type    string `json:"type"`
	Status  string `json:"status"`
	Reason  string `json:"reason"`
	Message string `json:"message"`
}

// EventInfo describes a Kubernetes event.
type EventInfo struct {
	Type      string `json:"type"`
	Reason    string `json:"reason"`
	Message   string `json:"message"`
	Timestamp string `json:"timestamp"`
}

// TUIEvent is the sealed interface for all events flowing from agents to the TUI.
type TUIEvent interface{ isTUIEvent() }

// AgentStartEvent fires when an agent begins processing a query.
type AgentStartEvent struct {
	QueryID   string
	AgentName string
}

func (AgentStartEvent) isTUIEvent() {}

// AgentDoneEvent fires when an agent finishes.
type AgentDoneEvent struct {
	QueryID   string
	Duration  time.Duration
	InTokens  int
	OutTokens int
}

func (AgentDoneEvent) isTUIEvent() {}

// ToolCallEvent fires immediately before a tool is invoked.
type ToolCallEvent struct {
	QueryID  string
	ToolName string
	Step     int
}

func (ToolCallEvent) isTUIEvent() {}

// ToolDoneEvent fires after a tool returns.
type ToolDoneEvent struct {
	QueryID  string
	ToolName string
	Step     int
	Elapsed  time.Duration
}

func (ToolDoneEvent) isTUIEvent() {}

// ToolFailEvent marks a failed tool on the progress card (no error body; see stream.ToolFail for API).
type ToolFailEvent struct {
	QueryID  string
	ToolName string
	Step     int
	Elapsed  time.Duration
}

func (ToolFailEvent) isTUIEvent() {}

// RenderTextEvent carries a plain-text reply.
type RenderTextEvent struct {
	QueryID string
	Text    string
}

func (RenderTextEvent) isTUIEvent() {}

// RenderTableEvent carries a pipe-delimited table reply.
type RenderTableEvent struct {
	QueryID string
	Headers []string
	Rows    [][]string
}

func (RenderTableEvent) isTUIEvent() {}

// RenderCodeEvent carries a fenced code block reply.
type RenderCodeEvent struct {
	QueryID  string
	Language string
	Content  string
}

func (RenderCodeEvent) isTUIEvent() {}

// RenderKVEvent carries a key-value list reply.
type RenderKVEvent struct {
	QueryID string
	Pairs   []KVPair
}

func (RenderKVEvent) isTUIEvent() {}

// RenderListEvent carries a status-list reply.
type RenderListEvent struct {
	QueryID string
	Items   []ListItem
}

func (RenderListEvent) isTUIEvent() {}

// StreamDoneEvent carries the final result string after a full query completes.
type StreamDoneEvent struct {
	QueryID string
	Result  string
}

func (StreamDoneEvent) isTUIEvent() {}

// StreamErrEvent carries an unrecoverable error.
type StreamErrEvent struct {
	QueryID string
	Err     error
}

func (StreamErrEvent) isTUIEvent() {}

// PhaseEvent carries a human-readable phase label for the thinking indicator.
type PhaseEvent struct {
	QueryID string
	Phase   string // e.g. "classifying intent", "thinking", "running tool: get_pods"
}

func (PhaseEvent) isTUIEvent() {}

// PlanWarning is a validation or policy advisory shown during deploy review.
type PlanWarning struct {
	Severity string // "warn" | "error"
	Message  string
}

// DeployPlan 包含部署计划的所有信息，用于 TUI 展示和用户确认。
// 定义在 events 包中以避免 deploy 包与 tui 包之间的循环依赖。
type DeployPlan struct {
	ChartInfo     *catalog.ChartInfo
	DefaultValues string // 完整的默认 values.yaml（含注释）
	CustomValues  string // LLM 生成的 override values
	ReleaseName   string
	Namespace     string
	IsUpgrade     bool // true 表示升级已有 release
	Warnings      []PlanWarning
}

// DeployDecision 表示用户在确认界面的决策。
type DeployDecision struct {
	Action     string // "execute" | "cancel"
	Values     string // 最终的 override values（可能被用户编辑过）
	Correction string // 如果用户使用了自然语言修正
}

// SupervisorEvent fires when the supervisor intervenes in an agent loop.
type SupervisorEvent struct {
	QueryID  string
	Reason   string // "loop detected" | "max steps reached"
	Decision string // "continue" | "reset" | "abort"
	Detail   string // Human-readable explanation or hint
}

func (SupervisorEvent) isTUIEvent() {}

// RenderDetailEvent carries structured resource detail for rich rendering.
type RenderDetailEvent struct {
	QueryID string         `json:"query_id"`
	Detail  ResourceDetail `json:"detail"`
}

func (RenderDetailEvent) isTUIEvent() {}
