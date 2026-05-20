package nodes

import (
	"context"

	"github.com/kubewise/kubewise/pkg/agent/deploy/values"
	"github.com/kubewise/kubewise/pkg/agent/deploy/workflow"
	"github.com/kubewise/kubewise/pkg/catalog"
	"github.com/kubewise/kubewise/pkg/tui/events"
)

// GenerateValuesOverrides asks the LLM for Helm override YAML.
func GenerateValuesOverrides(ctx context.Context, wf *workflow.Runner, llm LLMClient, query string, chart *catalog.ChartInfo, defaultValues string) (*values.Result, error) {
	return values.Generate(ctx, wf, llm, values.GenerateInput{
		Query:         query,
		Chart:         chart,
		DefaultValues: defaultValues,
	})
}

// EmitValuesGenerationNotes pushes optional LLM explanation and high-risk prompts to the TUI.
func EmitValuesGenerationNotes(emit func(events.TUIEvent), queryID string, r *values.Result) {
	if r == nil || r.Explanation == "" {
		return
	}
	emit(events.RenderTextEvent{QueryID: queryID, Text: r.Explanation})
	if r.RiskLevel == "high" {
		emit(events.RenderTextEvent{QueryID: queryID, Text: "⚠️ 配置风险等级: high，请仔细确认"})
	}
}
