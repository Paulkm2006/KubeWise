package deploy

import (
	"github.com/kubewise/kubewise/pkg/agent/deploy/plan"
	"github.com/kubewise/kubewise/pkg/agent/deploy/values"
	"github.com/kubewise/kubewise/pkg/catalog"
	"github.com/kubewise/kubewise/pkg/helm"
	"github.com/kubewise/kubewise/pkg/types"
)

// State holds mutable data for a single deploy HandleQuery run.
type State struct {
	Query             string
	Entities          types.Entities
	AppName           string
	ReleaseName       string
	Chart             *catalog.ChartInfo
	DefaultValues     string
	GenResult         *values.Result
	Plan              plan.DeployPlan
	FinalValues       string
	CorrectionHistory []string
	Release           *helm.Release
}
