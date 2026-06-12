package v2

import (
	"context"
	"time"

	"github.com/kubewise/kubewise/internal/utils/llm"
)

type Capability string

const (
	CapabilityRead     Capability = "read"
	CapabilityWrite    Capability = "write"
	CapabilityAudit    Capability = "audit"
	CapabilityCompute  Capability = "compute"
	CapabilityExternal Capability = "external"
)

type RiskLevel string

const (
	RiskNone   RiskLevel = "none"
	RiskLow    RiskLevel = "low"
	RiskMedium RiskLevel = "medium"
	RiskHigh   RiskLevel = "high"
)

type ConfirmPolicy string

const (
	ConfirmNever    ConfirmPolicy = "never"
	ConfirmRequired ConfirmPolicy = "required"
)

type ToolMeta struct {
	Name         string
	Version      string
	Description  string
	Parameters   map[string]any
	OutputSchema string
	Capability   Capability
	Risk         RiskLevel
	Confirm      ConfirmPolicy
	Tags         []string
}

func (m ToolMeta) ToFunctionDefinition() llm.FunctionDefinition {
	return llm.FunctionDefinition{
		Name:        m.Name,
		Description: m.Description,
		Parameters:  m.Parameters,
	}
}

type ResourceRef struct {
	APIVersion string `json:"api_version,omitempty"`
	Kind       string `json:"kind,omitempty"`
	Namespace  string `json:"namespace,omitempty"`
	Name       string `json:"name,omitempty"`
	UID        string `json:"uid,omitempty"`
}

type ResultMeta struct {
	ToolName      string        `json:"tool_name,omitempty"`
	ToolVersion   string        `json:"tool_version,omitempty"`
	Elapsed       time.Duration `json:"elapsed,omitempty"`
	Truncated     bool          `json:"truncated,omitempty"`
	Partial       bool          `json:"partial,omitempty"`
	Source        string        `json:"source,omitempty"`
	ExecutedAtUTC time.Time     `json:"executed_at_utc,omitempty"`
}

type Warning struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type ToolResult struct {
	Display    string
	Data       any
	DataSchema string
	Refs       []ResourceRef
	Meta       ResultMeta
	Warnings   []Warning
}

type Tool interface {
	Meta() ToolMeta
	Execute(ctx context.Context, args map[string]any) (ToolResult, error)
}

func TextMeta(name, version, description string, parameters map[string]any, capability Capability, risk RiskLevel, confirm ConfirmPolicy, tags ...string) ToolMeta {
	return ToolMeta{
		Name:        name,
		Version:     version,
		Description: description,
		Parameters:  parameters,
		Capability:  capability,
		Risk:        risk,
		Confirm:     confirm,
		Tags:        tags,
	}
}

func TextResult(meta ToolMeta, display string) ToolResult {
	return ToolResult{
		Display:    display,
		DataSchema: meta.OutputSchema,
		Meta: ResultMeta{
			ToolName:    meta.Name,
			ToolVersion: meta.Version,
		},
	}
}
