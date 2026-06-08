package helm

import (
	"strings"
	"time"
)

// ChartOptions identifies a chart and release context for read-only operations.
type ChartOptions struct {
	ReleaseName string
	RepoName    string
	ChartName   string
	RepoURL     string
	Namespace   string
	Values      string
}

// InstallOptions controls a mutating helm install/upgrade.
type InstallOptions struct {
	ChartOptions
	CreateNS bool
	Wait     bool
	Timeout  time.Duration
}

// RenderOptions controls client-side manifest rendering (dry-run).
type RenderOptions struct {
	ChartOptions
	IsUpgrade bool
}

// LintOptions controls helm lint.
type LintOptions struct {
	ChartOptions
	Strict               bool
	WithSubcharts        bool
	SkipSchemaValidation bool
}

// RenderResult holds rendered manifest output.
type RenderResult struct {
	Manifest string
	Notes    string
}

// LintResult holds lint messages.
type LintResult struct {
	Messages []string
	Errors   []error
}

// ValidationResult aggregates preflight checks.
type ValidationResult struct {
	ValuesOK  error
	Lint      *LintResult
	Render    *RenderResult
	RenderErr error
}

// HasErrors returns true when any preflight step failed.
func (r *ValidationResult) HasErrors() bool {
	if r == nil {
		return false
	}
	if r.ValuesOK != nil || r.RenderErr != nil {
		return true
	}
	if r.Lint != nil && len(r.Lint.Errors) > 0 {
		return true
	}
	return false
}

// ErrorSummary returns a human-readable summary of failures.
func (r *ValidationResult) ErrorSummary() string {
	if r == nil {
		return ""
	}
	var parts []string
	if r.ValuesOK != nil {
		parts = append(parts, r.ValuesOK.Error())
	}
	if r.Lint != nil {
		for _, e := range r.Lint.Errors {
			if e != nil {
				parts = append(parts, e.Error())
			}
		}
	}
	if r.RenderErr != nil {
		parts = append(parts, r.RenderErr.Error())
	}
	return strings.Join(parts, "; ")
}
