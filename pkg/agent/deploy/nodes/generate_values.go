package nodes

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	chartcore "github.com/kubewise/kubewise/pkg/agent/deploy/core/chart"
	"github.com/kubewise/kubewise/pkg/agent/deploy/core/plan"
	"github.com/kubewise/kubewise/pkg/agent/deploy/core/values"
	"github.com/kubewise/kubewise/pkg/agent/deploy/state"
	"github.com/kubewise/kubewise/pkg/stream"
	"github.com/kubewise/kubewise/pkg/catalog"
)

// GenerateValues asks the LLM for override values and builds the deploy plan.
func GenerateValues(st *state.State) error {
	st.Emit(stream.Phase{QueryID: st.QueryID, Phase: "生成配置建议"})

	genResult, err := state.RunToolWithResult(st, st.Ctx, values.ToolGenerate, 3, func(ctx context.Context) (*values.Result, error) {
		return values.Generate(ctx, st.LLM, values.GenerateInput{
			Query:         st.Query,
			Chart:         st.Chart,
			DefaultValues: st.DefaultValues,
		})
	})
	if err != nil {
		st.LogError("values generation failed", zap.Error(err))
		return fmt.Errorf("生成 values 失败: %w", err)
	}
	st.GenResult = genResult
	st.LogInfo("values generated",
		zap.String("namespace", genResult.Namespace),
		zap.String("risk_level", genResult.RiskLevel),
		zap.Int("override_lines", state.CountLines(genResult.Values)),
	)
	emitValuesNotes(st, genResult)

	st.Plan = buildDeployPlan(st.AppName, st.ReleaseName, st.Chart, st.DefaultValues, genResult)
	if err := validatePlan(st, "initial"); err != nil {
		return err
	}
	st.Next(state.PhaseReviewPlan)
	return nil
}

func emitValuesNotes(st *state.State, r *values.Result) {
	if r == nil || r.Explanation == "" {
		return
	}
	st.Emit(stream.LLMTextDelta{QueryID: st.QueryID, Delta: r.Explanation})
	if r.RiskLevel == "high" {
		st.Emit(stream.LLMTextDelta{QueryID: st.QueryID, Delta: "⚠️ 配置风险等级: high，请仔细确认"})
	}
}

func buildDeployPlan(appName, releaseName string, c *catalog.ChartInfo, defaultValues string, gen *values.Result) plan.DeployPlan {
	customValues := plan.ApplyCRDValues(c, defaultValues, gen.Values)
	p := plan.NewDeployPlan(appName, c, defaultValues, customValues, gen.Namespace, false)
	p.ReleaseName = releaseName
	p.Warnings = append(p.Warnings, chartcore.SelectionWarnings(appName, c)...)
	return p
}
