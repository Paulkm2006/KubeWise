package nodes

import (
	"fmt"

	"go.uber.org/zap"

	chartcore "github.com/kubewise/kubewise/pkg/agent/deploy/core/chart"
	"github.com/kubewise/kubewise/pkg/agent/deploy/state"
	"github.com/kubewise/kubewise/pkg/tui/events"
)

// ResolveChart searches Artifact Hub and applies selection policy.
func ResolveChart(st *state.State) error {
	st.Emit(events.PhaseEvent{QueryID: st.QueryID, Phase: fmt.Sprintf("搜索 Chart: %s", st.AppName)})

	var sel chartcore.SelectChartFn
	if st.Select != nil {
		sel = st.Select.SelectChart
	}
	chosen, err := chartcore.ResolveArtifactHub(st.Ctx, st.AppName, sel, st.Log)
	if err != nil {
		st.LogError("chart resolution failed", zap.String("app", st.AppName), zap.Error(err))
		return err
	}
	if chosen == nil {
		st.LogInfo("chart selection cancelled", zap.String("app", st.AppName))
		st.Done("部署已取消")
		return nil
	}
	st.Chart = chosen
	st.LogInfo("chart resolved",
		zap.String("app", st.AppName),
		zap.String("repo", st.Chart.RepoName),
		zap.String("chart", st.Chart.ChartName),
		zap.String("source", st.Chart.Source),
		zap.String("default_namespace", st.Chart.DefaultNamespace),
	)
	st.Next(state.PhaseFetchDefaults)
	return nil
}
