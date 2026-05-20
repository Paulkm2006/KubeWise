package deploy

import (
	"context"

	"go.uber.org/zap"

	"github.com/kubewise/kubewise/pkg/agent/deploy/core/report"
	"github.com/kubewise/kubewise/pkg/agent/deploy/plan"
	"github.com/kubewise/kubewise/pkg/catalog"
	"github.com/kubewise/kubewise/pkg/helm"
)

func (a *Agent) logPlanValidation(stage string, p plan.DeployPlan, validation plan.ValidationResult) {
	fields := []zap.Field{
		zap.String("stage", stage),
		zap.String("namespace", p.Namespace),
		zap.String("release", p.ReleaseName),
		zap.Int("errors", len(validation.Errors)),
		zap.Int("warnings", len(validation.Warnings)),
	}
	if len(validation.Errors) > 0 {
		fields = append(fields, zap.Strings("error_details", validation.Errors))
	}
	if len(validation.Warnings) > 0 {
		a.logWarn("deploy plan validation warnings", fields...)
		return
	}
	a.logDebug("deploy plan validation ok", fields...)
}

func (a *Agent) buildReport(ctx context.Context, rel *helm.Release, chartInfo *catalog.ChartInfo, namespace, releaseName string) string {
	return report.SuccessMessage(ctx, rel, chartInfo, namespace, releaseName, a.k8sClient, a.logger())
}
