package nodes

import (
	"context"
	"fmt"
	"strings"

	"github.com/kubewise/kubewise/pkg/agent/deploy/core/chart"
	"github.com/kubewise/kubewise/pkg/agent/deploy/plan"
	"github.com/kubewise/kubewise/pkg/agent/deploy/values"
	"github.com/kubewise/kubewise/pkg/catalog"
	"github.com/kubewise/kubewise/pkg/helm"
)

// BuildDeployPlan composes the internal deploy plan from generation output.
func BuildDeployPlan(appName, releaseName string, c *catalog.ChartInfo, defaultValues string, gen *values.Result) plan.DeployPlan {
	customValues := plan.ApplyCRDValues(c, defaultValues, gen.Values)
	p := plan.NewDeployPlan(appName, c, defaultValues, customValues, gen.Namespace, false)
	p.ReleaseName = releaseName
	p.Warnings = append(p.Warnings, chart.SelectionWarnings(appName, c)...)
	return p
}

// ValidatePlan refreshes upgrade flag, merges static validation + cluster checks, and surfaces blocking errors.
func ValidatePlan(
	ctx context.Context,
	h HelmClient,
	p *plan.DeployPlan,
	stage string,
	onValidation func(stage string, dp plan.DeployPlan, v plan.ValidationResult),
	onExistingRelease func(p *plan.DeployPlan, existing *helm.Release),
) error {
	existingInTarget, _ := h.Status(ctx, p.ReleaseName, p.Namespace)
	p.IsUpgrade = existingInTarget != nil
	if existingInTarget != nil && onExistingRelease != nil {
		onExistingRelease(p, existingInTarget)
	}

	validation := plan.ValidateDeployPlan(*p)
	validation.Merge(plan.CheckHelmReleaseConflicts(ctx, h, *p))
	p.Warnings = append(p.Warnings, validation.Warnings...)
	if onValidation != nil {
		onValidation(stage, *p, validation)
	}
	if validation.HasBlockingErrors() {
		return fmt.Errorf("部署计划校验失败: %s", strings.Join(validation.Errors, "; "))
	}
	return nil
}
