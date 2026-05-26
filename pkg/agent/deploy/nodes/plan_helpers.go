package nodes

import (
	"fmt"
	"strings"

	"go.uber.org/zap"

	chartcore "github.com/kubewise/kubewise/pkg/agent/deploy/core/chart"
	"github.com/kubewise/kubewise/pkg/agent/deploy/core/plan"
	"github.com/kubewise/kubewise/pkg/agent/deploy/state"
	deploytypes "github.com/kubewise/kubewise/pkg/agent/deploy/types"
)

func validatePlan(st *state.State, stage string) error {
	existingInTarget, _ := st.Helm.Status(st.Ctx, st.Plan.ReleaseName, st.Plan.Namespace)
	st.Plan.IsUpgrade = existingInTarget != nil
	if existingInTarget != nil {
		st.LogDebug("existing release in target namespace",
			zap.String("release", st.Plan.ReleaseName),
			zap.String("namespace", st.Plan.Namespace),
			zap.String("status", existingInTarget.Status),
		)
	}

	validation := plan.ValidateDeployPlan(st.Plan)
	validation.Merge(plan.CheckHelmReleaseConflicts(st.Ctx, st.Helm, st.Plan))
	st.Plan.Warnings = append(chartcore.SelectionWarnings(st.Plan.AppName, st.Plan.Chart), validation.Warnings...)
	logPlanValidation(st, stage, validation)
	if validation.HasBlockingErrors() {
		return fmt.Errorf("部署计划校验失败: %s", strings.Join(validation.Errors, "; "))
	}
	return nil
}

func logPlanValidation(st *state.State, stage string, validation plan.ValidationResult) {
	fields := []zap.Field{
		zap.String("stage", stage),
		zap.String("namespace", st.Plan.Namespace),
		zap.String("release", st.Plan.ReleaseName),
		zap.Int("errors", len(validation.Errors)),
		zap.Int("warnings", len(validation.Warnings)),
	}
	if len(validation.Errors) > 0 {
		fields = append(fields, zap.Strings("error_details", validation.Errors))
	}
	if len(validation.Warnings) > 0 {
		st.LogWarn("deploy plan validation warnings", fields...)
		return
	}
	st.LogDebug("deploy plan validation ok", fields...)
}

func confirmDeploy(st *state.State) (deploytypes.DeployDecision, error) {
	if st.Confirm == nil {
		return deploytypes.DeployDecision{Action: "execute", Values: st.Plan.CustomValues}, nil
	}
	return st.Confirm.ConfirmDeploy(st.Ctx, st.Plan.ToEventPlan())
}
