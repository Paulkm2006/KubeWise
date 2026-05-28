// Package deploytypes holds shared types for the deploy pipeline and its consumers.
package deploytypes

import "github.com/kubewise/kubewise/pkg/catalog"

// PlanWarning is a validation or policy advisory shown during deploy review.
type PlanWarning struct {
	Severity string // "warn" | "error"
	Message  string
}

// DeployPlan contains a deployment plan for user confirmation.
type DeployPlan struct {
	ChartInfo     *catalog.ChartInfo
	DefaultValues string // complete default values.yaml (with comments)
	CustomValues  string // LLM-generated override values
	ReleaseName   string
	Namespace     string
	IsUpgrade     bool // true when upgrading an existing release
	Warnings      []PlanWarning
}

// DeployDecision represents the user's decision on the confirmation screen.
type DeployDecision struct {
	Action     string // "execute" | "cancel"
	Values     string // final override values (possibly edited by user)
	Correction string // natural language correction from user
}
