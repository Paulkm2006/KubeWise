package nodes

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	"github.com/kubewise/kubewise/pkg/agent/deploy/plan"
	"github.com/kubewise/kubewise/pkg/agent/deploy/values"
	"github.com/kubewise/kubewise/pkg/agent/deploy/workflow"
	"github.com/kubewise/kubewise/pkg/catalog"
	"github.com/kubewise/kubewise/pkg/tui/events"
)

const ToolUserConfirm = "user confirm"

// ReviewAndConfirm loops until the user executes, cancels, or stays in correction mode.
func ReviewAndConfirm(
	ctx context.Context,
	wf *workflow.Runner,
	llm LLMClient,
	h HelmClient,
	emit func(events.TUIEvent),
	queryID string,
	query string,
	chart *catalog.ChartInfo,
	defaultValues string,
	p *plan.DeployPlan,
	confirm func(context.Context, events.DeployPlan) (events.DeployDecision, error),
	validate func(context.Context, *plan.DeployPlan, string) error,
	log *zap.Logger,
	countLines func(string) int,
) (finalValues string, correctionHistory []string, err error) {
	if log == nil {
		log = zap.NewNop()
	}
	for {
		var decision events.DeployDecision
		confirmErr := wf.Run(ctx, workflow.Meta{Name: ToolUserConfirm, Step: 4}, func(ctx context.Context) error {
			var e error
			decision, e = confirm(ctx, p.ToEventPlan())
			return e
		})
		if confirmErr != nil {
			return "", correctionHistory, fmt.Errorf("确认部署失败: %w", confirmErr)
		}

		if decision.Action == "cancel" {
			log.Info("deploy cancelled by user")
			return "", correctionHistory, nil
		}

		if decision.Correction == "" {
			p.CustomValues = decision.Values
			log.Info("user confirmed deploy plan", zap.Int("values_lines", countLines(decision.Values)))
			return decision.Values, correctionHistory, nil
		}

		log.Info("user requested values correction", zap.String("correction", decision.Correction))
		correctionHistory = append(correctionHistory, decision.Correction)
		regenResult, err := values.Regenerate(ctx, wf, llm, values.RegenerateInput{
			Query:         query,
			Chart:         chart,
			DefaultValues: defaultValues,
			CurrentValues: decision.Values,
			Correction:    decision.Correction,
		}, 5)
		if err != nil {
			log.Error("values regeneration failed", zap.Error(err))
			return "", correctionHistory, fmt.Errorf("重新生成 values 失败: %w", err)
		}
		log.Info("values regenerated",
			zap.String("namespace", regenResult.Namespace),
			zap.Int("override_lines", countLines(regenResult.Values)),
		)

		if regenResult.Explanation != "" {
			emit(events.RenderTextEvent{QueryID: queryID, Text: regenResult.Explanation})
		}

		p.CustomValues = plan.ApplyCRDValues(chart, defaultValues, regenResult.Values)
		if regenResult.Namespace != p.Namespace {
			p.Namespace = plan.SanitizeNamespace(regenResult.Namespace)
			if err := plan.ValidateNamespace(p.Namespace); err != nil {
				return "", correctionHistory, err
			}
			existingInTarget, _ := h.Status(ctx, p.ReleaseName, p.Namespace)
			p.IsUpgrade = existingInTarget != nil
		}

		if err := validate(ctx, p, "after_correction"); err != nil {
			return "", correctionHistory, err
		}
	}
}
