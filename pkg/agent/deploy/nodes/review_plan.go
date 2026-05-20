package nodes

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	"github.com/kubewise/kubewise/pkg/agent/deploy/core/plan"
	"github.com/kubewise/kubewise/pkg/agent/deploy/core/values"
	"github.com/kubewise/kubewise/pkg/agent/deploy/state"
	"github.com/kubewise/kubewise/pkg/tui/events"
)

const toolUserConfirm = "user confirm"

// ReviewPlan runs the user confirmation loop (execute, cancel, or NL correction).
func ReviewPlan(st *state.State) error {
	st.Emit(events.PhaseEvent{QueryID: st.QueryID, Phase: "等待用户确认"})
	st.LogInfo("awaiting user confirmation",
		zap.String("namespace", st.Plan.Namespace),
		zap.String("release", st.Plan.ReleaseName),
		zap.Bool("upgrade", st.Plan.IsUpgrade),
		zap.Int("warnings", len(st.Plan.Warnings)),
	)

	for {
		var decision events.DeployDecision
		confirmErr := st.RunTool(st.Ctx, toolUserConfirm, 4, func(ctx context.Context) error {
			var e error
			decision, e = confirmDeploy(st)
			return e
		})
		if confirmErr != nil {
			return fmt.Errorf("确认部署失败: %w", confirmErr)
		}

		if decision.Action == "cancel" {
			st.LogInfo("deploy cancelled by user")
			st.Done("部署已取消")
			return nil
		}

		if decision.Correction == "" {
			st.Plan.CustomValues = decision.Values
			st.FinalValues = decision.Values
			st.LogInfo("user confirmed deploy plan", zap.Int("values_lines", state.CountLines(decision.Values)))
			st.Next(state.PhasePreflight)
			return nil
		}

		st.CorrectionAttempts++
		if st.CorrectionAttempts > st.MaxCorrectionAttempts {
			return fmt.Errorf("配置修正次数已达上限（%d 次）", st.MaxCorrectionAttempts)
		}

		st.LogInfo("user requested values correction", zap.String("correction", decision.Correction))
		st.CorrectionHistory = append(st.CorrectionHistory, decision.Correction)

		regenResult, err := state.RunToolWithResult(st, st.Ctx, values.ToolRegenerate, 5, func(ctx context.Context) (*values.Result, error) {
			return values.Regenerate(ctx, st.LLM, values.RegenerateInput{
				Query:         st.Query,
				Chart:         st.Chart,
				DefaultValues: st.DefaultValues,
				CurrentValues: decision.Values,
				Correction:    decision.Correction,
			})
		})
		if err != nil {
			st.LogError("values regeneration failed", zap.Error(err))
			return fmt.Errorf("重新生成 values 失败: %w", err)
		}
		st.LogInfo("values regenerated",
			zap.String("namespace", regenResult.Namespace),
			zap.Int("override_lines", state.CountLines(regenResult.Values)),
		)
		if regenResult.Explanation != "" {
			st.Emit(events.RenderTextEvent{QueryID: st.QueryID, Text: regenResult.Explanation})
		}

		st.Plan.CustomValues = plan.ApplyCRDValues(st.Chart, st.DefaultValues, regenResult.Values)
		if regenResult.Namespace != st.Plan.Namespace {
			st.Plan.Namespace = plan.SanitizeNamespace(regenResult.Namespace)
			if err := plan.ValidateNamespace(st.Plan.Namespace); err != nil {
				return err
			}
			existingInTarget, _ := st.Helm.Status(st.Ctx, st.Plan.ReleaseName, st.Plan.Namespace)
			st.Plan.IsUpgrade = existingInTarget != nil
		}
		if err := validatePlan(st, "after_correction"); err != nil {
			return err
		}
		// stay in PhaseReviewPlan for another confirm round
	}
}
