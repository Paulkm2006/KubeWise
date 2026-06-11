package nodes

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	"github.com/kubewise/kubewise/internal/agent/event"
	"github.com/kubewise/kubewise/internal/agent/subagent/deploy/state"
)

const (
	toolRepoAdd    = "helm repo add"
	toolShowValues = "helm show values"
)

// FetchDefaults adds the Helm repo and fetches default chart values.
func FetchDefaults(st *state.State) error {
	st.Emit(event.Phase{QueryID: st.QueryID, Phase: "获取 Chart 默认配置"})

	if err := st.RunTool(st.Ctx, toolRepoAdd, 1, func(ctx context.Context) error {
		if err := st.Helm.AddRepo(ctx, st.Chart.RepoName, st.Chart.RepoURL); err != nil {
			return fmt.Errorf("添加 Helm 仓库失败: %w", err)
		}
		return nil
	}); err != nil {
		st.LogError("helm repo add failed", zap.String("repo", st.Chart.RepoName), zap.Error(err))
		return err
	}
	st.LogDebug("helm repo ready", zap.String("repo", st.Chart.RepoName))

	vals, err := state.RunToolWithResult(st, st.Ctx, toolShowValues, 2, func(ctx context.Context) (string, error) {
		vals, err := st.Helm.FetchDefaultValues(ctx, st.Chart.RepoName, st.Chart.RepoURL, st.Chart.ChartName)
		if err != nil {
			return "", fmt.Errorf("获取默认 values 失败: %w", err)
		}
		return vals, nil
	})
	if err != nil {
		st.LogError("fetch default values failed", zap.String("chart", st.Chart.ChartName), zap.Error(err))
		return err
	}
	st.DefaultValues = vals
	st.LogDebug("default values fetched",
		zap.String("chart", st.Chart.ChartName),
		zap.Int("lines", state.CountLines(st.DefaultValues)),
	)
	st.Next(state.PhaseGenerateValues)
	return nil
}
