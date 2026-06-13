// Package stream defines the event pipeline domain shared by agents, TUI, and HTTP adapters.
package event

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
	Summary string
	Payload *Payload
}

func (Phase) isStreamEvent() {}

type AgentStart struct {
	QueryID   string
	AgentName string
}

func (AgentStart) isStreamEvent() {}

type AgentDone struct {
	QueryID   string
	Result    string
	Duration  time.Duration
	InTokens  int
	OutTokens int
	Summary   string
	Payload   *Payload
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
	Summary  string
	Payload  *Payload
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

// LLMTextDelta carries a piece of streaming LLM output text for live display.
type LLMTextDelta struct {
	QueryID string
	Delta   string
}

func (LLMTextDelta) isStreamEvent() {}

type Supervisor struct {
	QueryID  string
	Reason   string
	Decision string
	Detail   string
}

func (Supervisor) isStreamEvent() {}

type StreamDone struct {
	QueryID string
}

func (StreamDone) isStreamEvent() {}

type StreamErr struct {
	QueryID string
	Err     error
}

func (StreamErr) isStreamEvent() {}

// LLMStepDegraded records a diagnosis LLM step that failed and fell back.
type LLMStepDegraded struct {
	QueryID   string
	Step      string
	Phase     string
	Err       string
	Transient bool
	Fallback  string
}

func (LLMStepDegraded) isStreamEvent() {}

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
	Cancelled      bool `json:"cancelled"`
	UseManualChart bool `json:"use_manual_chart"`
	CandidateIndex int  `json:"candidate_index"` // 0-based into candidates list
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
