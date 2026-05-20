package nodes

import (
	"context"

	"github.com/kubewise/kubewise/pkg/agent/deploy/workflow"
	helmtools "github.com/kubewise/kubewise/pkg/agent/deploy/workflow/helm"
	"github.com/kubewise/kubewise/pkg/catalog"
	"github.com/kubewise/kubewise/pkg/helm"
	"github.com/kubewise/kubewise/pkg/tui/events"
)

// VerifyDeployedRelease emits the verification phase and runs the report builder inside a workflow tool envelope.
func VerifyDeployedRelease(
	ctx context.Context,
	wf *workflow.Runner,
	queryID string,
	emit func(events.TUIEvent),
	rel *helm.Release,
	chart *catalog.ChartInfo,
	namespace, releaseName string,
	buildReport func(context.Context) (string, error),
) (string, error) {
	emit(events.PhaseEvent{QueryID: queryID, Phase: "验证部署状态"})
	return workflow.RunWithResult(wf, ctx, workflow.Meta{Name: helmtools.ToolVerifyDeploy, Step: 7}, buildReport)
}
