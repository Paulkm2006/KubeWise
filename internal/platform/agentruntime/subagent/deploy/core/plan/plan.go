package plan

import (
	"strings"

	"github.com/kubewise/kubewise/internal/platform/agentruntime/subagent/deploy/core/catalog"
	deploytypes "github.com/kubewise/kubewise/internal/platform/agentruntime/subagent/deploy/types"
)

// DeployPlan is the internal deployment plan used by the deploy pipeline.
type DeployPlan struct {
	AppName       string
	ReleaseName   string
	Namespace     string
	Chart         *catalog.ChartInfo
	DefaultValues string
	CustomValues  string
	IsUpgrade     bool
	Warnings      []Warning
}

// Warning is a non-blocking advisory shown during review.
type Warning struct {
	Severity string // "warn" | "error" (error-level warnings block execute in validator)
	Message  string
}

// ToEventPlan converts the internal plan to the TUI-facing DeployPlan.
func (p DeployPlan) ToEventPlan() deploytypes.DeployPlan {
	warnings := make([]deploytypes.PlanWarning, len(p.Warnings))
	for i, w := range p.Warnings {
		warnings[i] = deploytypes.PlanWarning{Severity: w.Severity, Message: w.Message}
	}
	return deploytypes.DeployPlan{
		ChartInfo:     p.Chart,
		DefaultValues: p.DefaultValues,
		CustomValues:  p.CustomValues,
		ReleaseName:   p.ReleaseName,
		Namespace:     p.Namespace,
		IsUpgrade:     p.IsUpgrade,
		Warnings:      warnings,
	}
}

// NewDeployPlan builds a plan with sanitized release name and namespace.
func NewDeployPlan(appName string, chart *catalog.ChartInfo, defaultValues, customValues, namespace string, isUpgrade bool) DeployPlan {
	ns := SanitizeNamespace(namespace)
	if ns == "" {
		ns = "default"
	}
	return DeployPlan{
		AppName:       appName,
		ReleaseName:   SanitizeReleaseName(appName),
		Namespace:     ns,
		Chart:         chart,
		DefaultValues: defaultValues,
		CustomValues:  customValues,
		IsUpgrade:     isUpgrade,
	}
}

// SanitizeReleaseName normalizes a string into a valid Helm release name (max 53 chars).
func SanitizeReleaseName(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" {
		return "release"
	}
	var b strings.Builder
	prevHyphen := false
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			prevHyphen = false
		case r == '-' || r == '_' || r == '.':
			if !prevHyphen && b.Len() > 0 {
				b.WriteByte('-')
				prevHyphen = true
			}
		default:
			if !prevHyphen && b.Len() > 0 {
				b.WriteByte('-')
				prevHyphen = true
			}
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		out = "release"
	}
	if len(out) > 53 {
		out = strings.Trim(out[:53], "-")
	}
	return out
}

// SanitizeNamespace trims and lowercases namespace input from LLM.
func SanitizeNamespace(ns string) string {
	ns = strings.ToLower(strings.TrimSpace(ns))
	if idx := strings.Index(ns, " "); idx > 0 {
		ns = ns[:idx]
	}
	return ns
}

// ApplyCRDValues prepends chart-specific CRD install values when appropriate.
func ApplyCRDValues(chart *catalog.ChartInfo, defaultValues, customValues string) string {
	if chart == nil || !chart.InstallCRDs {
		return customValues
	}
	if valuesHasKey(defaultValues, "installCRDs") || valuesHasKey(customValues, "installCRDs") {
		return customValues
	}
	return "installCRDs: true\n" + customValues
}

func valuesHasKey(valuesYAML, key string) bool {
	if valuesYAML == "" {
		return false
	}
	lines := strings.Split(valuesYAML, "\n")
	prefix := key + ":"
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, prefix) {
			return true
		}
	}
	return false
}
