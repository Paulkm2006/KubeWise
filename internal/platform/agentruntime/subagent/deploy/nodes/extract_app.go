package nodes

import (
	"fmt"

	"go.uber.org/zap"

	"github.com/kubewise/kubewise/internal/agent/subagent/deploy/core/appname"
	"github.com/kubewise/kubewise/internal/agent/subagent/deploy/core/plan"
	"github.com/kubewise/kubewise/internal/agent/subagent/deploy/state"
)

// ExtractApp derives the deployment target name from entities or query text.
func ExtractApp(st *state.State) error {
	st.AppName = appname.FromEntities(st.Entities, st.Query)
	if st.AppName == "" {
		st.LogWarn("could not extract app name from query", zap.String("query", st.Query))
		return fmt.Errorf("无法从查询中提取应用名称，请明确指定要部署的应用")
	}
	st.ReleaseName = plan.SanitizeReleaseName(st.AppName)
	st.LogInfo("deploy pipeline started",
		zap.String("app", st.AppName),
		zap.String("release", st.ReleaseName),
		zap.String("query", st.Query),
	)
	st.Next(state.PhaseResolveChart)
	return nil
}
