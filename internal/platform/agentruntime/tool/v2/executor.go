package v2

import (
	"context"
	"fmt"
	"strings"
	"time"
)

type ExecutePolicy struct {
	AllowedCapabilities []Capability
	AllowedTools        []string
	RequireConfirmation bool
	MaxOutputBytes      int
	EmitEvents          bool
}

type Executor struct {
	registry *Registry
}

func NewExecutor(registry *Registry) *Executor {
	return &Executor{registry: registry}
}

func (e *Executor) Execute(ctx context.Context, name string, args map[string]any, policy ExecutePolicy) (ToolResult, error) {
	if e == nil || e.registry == nil {
		return ToolResult{}, fmt.Errorf("tool executor unavailable")
	}
	t, ok := e.registry.Get(name)
	if !ok {
		return ToolResult{}, fmt.Errorf("tool %q not found", name)
	}
	meta := t.Meta()
	if !policyAllowsTool(policy, meta) {
		return ToolResult{}, fmt.Errorf("tool %q denied by policy", name)
	}
	if policy.RequireConfirmation && meta.Confirm != ConfirmRequired {
		return ToolResult{}, fmt.Errorf("tool %q does not declare required confirmation", name)
	}

	start := time.Now()
	result, err := t.Execute(ctx, args)
	elapsed := time.Since(start)
	if result.Meta.ToolName == "" {
		result.Meta.ToolName = meta.Name
	}
	if result.Meta.ToolVersion == "" {
		result.Meta.ToolVersion = meta.Version
	}
	result.Meta.Elapsed = elapsed
	result.Meta.ExecutedAtUTC = time.Now().UTC()
	if result.DataSchema == "" {
		result.DataSchema = meta.OutputSchema
	}
	if policy.MaxOutputBytes > 0 && len(result.Display) > policy.MaxOutputBytes {
		result.Display = result.Display[:policy.MaxOutputBytes]
		result.Meta.Truncated = true
	}
	return result, err
}

func policyAllowsTool(policy ExecutePolicy, meta ToolMeta) bool {
	if len(policy.AllowedTools) > 0 && !containsString(policy.AllowedTools, meta.Name) {
		return false
	}
	if len(policy.AllowedCapabilities) > 0 && !containsCapability(policy.AllowedCapabilities, meta.Capability) {
		return false
	}
	if meta.Capability == CapabilityWrite && meta.Confirm != ConfirmRequired {
		return false
	}
	return true
}

func containsString(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}

func containsCapability(items []Capability, want Capability) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}

func ToolResultToLLMMessage(result ToolResult) string {
	display := strings.TrimSpace(result.Display)
	if display == "" {
		display = fmt.Sprintf("tool %s completed with structured data schema %s", result.Meta.ToolName, result.DataSchema)
	}
	if len(result.Warnings) == 0 {
		return display
	}
	var b strings.Builder
	b.WriteString(display)
	b.WriteString("\n\nWarnings:\n")
	for _, w := range result.Warnings {
		b.WriteString("- ")
		if w.Code != "" {
			b.WriteString(w.Code)
			b.WriteString(": ")
		}
		b.WriteString(w.Message)
		b.WriteByte('\n')
	}
	return strings.TrimSpace(b.String())
}
