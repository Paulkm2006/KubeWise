package nodes

import (
	"context"

	"go.uber.org/zap"

	chartcore "github.com/kubewise/kubewise/pkg/agent/deploy/core/chart"
	"github.com/kubewise/kubewise/pkg/catalog"
)

// SelectChartFn selects a chart from Artifact Hub candidates (nil = auto pick best).
type SelectChartFn = chartcore.SelectChartFn

// ResolveChart searches Artifact Hub, merges curated picks, and runs selection policy.
func ResolveChart(ctx context.Context, appName string, selectChart SelectChartFn, log *zap.Logger) (*catalog.ChartInfo, error) {
	return chartcore.ResolveArtifactHub(ctx, appName, selectChart, log)
}
