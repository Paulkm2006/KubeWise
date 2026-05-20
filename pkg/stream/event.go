// Package stream defines the event pipeline domain shared by agents, TUI, and HTTP adapters.
package stream

import (
	"encoding/json"
	"time"
)

// Event is the sealed interface for all events flowing from agents to consumers.
type Event interface {
	isStreamEvent()
}

// --- Core progress / render ---

type Phase struct {
	QueryID string
	Phase   string
}

func (Phase) isStreamEvent() {}

type AgentStart struct {
	QueryID   string
	AgentName string
}

func (AgentStart) isStreamEvent() {}

type AgentDone struct {
	QueryID   string
	Duration  time.Duration
	InTokens  int
	OutTokens int
}

func (AgentDone) isStreamEvent() {}

type ToolCall struct {
	QueryID  string
	ToolName string
	Step     int
}

func (ToolCall) isStreamEvent() {}

type ToolDone struct {
	QueryID  string
	ToolName string
	Step     int
	Elapsed  time.Duration
}

func (ToolDone) isStreamEvent() {}

// ToolFail reports a tool invocation that returned an error.
type ToolFail struct {
	QueryID  string
	ToolName string
	Step     int
	Elapsed  time.Duration
	Err      string
}

func (ToolFail) isStreamEvent() {}

type RenderText struct {
	QueryID string
	Text    string
}

func (RenderText) isStreamEvent() {}

type RenderTable struct {
	QueryID string
	Headers []string
	Rows    [][]string
}

func (RenderTable) isStreamEvent() {}

type RenderCode struct {
	QueryID  string
	Language string
	Content  string
}

func (RenderCode) isStreamEvent() {}

type RenderKV struct {
	QueryID string
	Pairs   []KVPair
}

func (RenderKV) isStreamEvent() {}

type RenderList struct {
	QueryID string
	Items   []ListItem
}

func (RenderList) isStreamEvent() {}

type RenderDetail struct {
	QueryID string
	Detail  ResourceDetail
}

func (RenderDetail) isStreamEvent() {}

type Supervisor struct {
	QueryID  string
	Reason   string
	Decision string
	Detail   string
}

func (Supervisor) isStreamEvent() {}

type StreamDone struct {
	QueryID string
	Result  string
}

func (StreamDone) isStreamEvent() {}

type StreamErr struct {
	QueryID string
	Err     error
}

func (StreamErr) isStreamEvent() {}

// --- Human-in-the-loop ---

// InteractionKind identifies the payload shape for InteractionRequest.
type InteractionKind string

const (
	KindOperationStep  InteractionKind = "operation_step"
	KindChartSelect    InteractionKind = "chart_select"
	KindDeployConfirm  InteractionKind = "deploy_confirm"
	KindUnknownPayload InteractionKind = "unknown"
)

// InteractionRequest is a unified HITL event. Response is JSON per kind.
type InteractionRequest struct {
	QueryID       string
	InteractionID string // optional; API layer may fill before send
	Kind          InteractionKind
	Payload       json.RawMessage
	RespCh        chan<- json.RawMessage
	// TotalSteps applies when Kind==KindOperationStep (mirrors ConfirmRequest).
	TotalSteps int
}

func (InteractionRequest) isStreamEvent() {}

// OperationConfirmResponse is the JSON body for KindOperationStep responses.
type OperationConfirmResponse struct {
	Confirmed  bool   `json:"confirmed"`
	Correction string `json:"correction,omitempty"`
}

// ChartSelectResponse is the JSON body for KindChartSelect responses.
type ChartSelectResponse struct {
	Cancelled       bool `json:"cancelled"`
	UseManualChart  bool `json:"use_manual_chart"`
	CandidateIndex  int  `json:"candidate_index"` // 0-based into candidates list
	// When UseManualChart and the client collected repo/chart details (e.g. TUI manual form).
	ManualRepoURL   string `json:"manual_repo_url,omitempty"`
	ManualChartName string `json:"manual_chart_name,omitempty"`
}

// DeployConfirmResponse wraps deploy decision JSON.
type DeployConfirmResponse struct {
	Action     string `json:"action"` // execute | cancel
	Values     string `json:"values,omitempty"`
	Correction string `json:"correction,omitempty"`
}
