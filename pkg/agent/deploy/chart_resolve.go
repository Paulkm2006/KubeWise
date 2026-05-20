package deploy

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"go.uber.org/zap"

	"github.com/kubewise/kubewise/pkg/agent/deploy/plan"
	"github.com/kubewise/kubewise/pkg/catalog"
)

func normalizeAppName(appName string) string {
	return strings.ToLower(strings.TrimSpace(appName))
}

const (
	scorePerStar           = 10
	scoreVerifiedPublisher = 500
	scoreSignedPackage     = 300
	scoreOfficialPackage   = 800
	scoreCNCFPackage       = 400
	scorePerProductionOrg  = 50
	scoreProductionOrgCap  = 250
	scoreDeprecatedPackage = -10000
	scoreDeprecatedInText  = -200
)

// scoreChartCandidate ranks ArtifactHub candidates using trust signals and popularity.
// Chart name similarity is intentionally not used.
func scoreChartCandidate(c catalog.ChartInfo) int {
	score := c.Stars * scorePerStar
	if c.VerifiedPublisher {
		score += scoreVerifiedPublisher
	}
	if c.Signed {
		score += scoreSignedPackage
	}
	if c.Official {
		score += scoreOfficialPackage
	}
	if c.CNCF {
		score += scoreCNCFPackage
	}
	if c.ProductionOrganizationsCount > 0 {
		bonus := c.ProductionOrganizationsCount * scorePerProductionOrg
		if bonus > scoreProductionOrgCap {
			bonus = scoreProductionOrgCap
		}
		score += bonus
	}
	if c.Deprecated {
		score += scoreDeprecatedPackage
	}
	if strings.Contains(strings.ToLower(c.Description), "deprecated") {
		score += scoreDeprecatedInText
	}
	return score
}

func pickBestChart(appName string, candidates []catalog.ChartInfo) catalog.ChartInfo {
	ranked := rankChartCandidates(appName, candidates)
	return ranked[0]
}

// rankChartCandidates sorts candidates best-first by trust signals and stars.
func rankChartCandidates(appName string, candidates []catalog.ChartInfo) []catalog.ChartInfo {
	if len(candidates) <= 1 {
		return candidates
	}
	ranked := make([]catalog.ChartInfo, len(candidates))
	copy(ranked, candidates)
	sort.SliceStable(ranked, func(i, j int) bool {
		si, sj := scoreChartCandidate(ranked[i]), scoreChartCandidate(ranked[j])
		if si != sj {
			return si > sj
		}
		// Tie-break: higher stars, then chart name for stable UX.
		if ranked[i].Stars != ranked[j].Stars {
			return ranked[i].Stars > ranked[j].Stars
		}
		return ranked[i].ChartName < ranked[j].ChartName
	})
	_ = appName
	return ranked
}

// chartSelectionWarnings warns when the chosen chart likely is not the main application chart.
func chartSelectionWarnings(appName string, chart *catalog.ChartInfo) []plan.PlanWarning {
	if chart == nil {
		return nil
	}
	app := normalizeAppName(appName)
	chartName := strings.ToLower(chart.ChartName)
	if chart.CuratedPick {
		return nil
	}
	if chartName == app {
		return nil
	}
	if strings.HasPrefix(chartName, app+"-") || strings.HasPrefix(chartName, app+"_") {
		return []plan.PlanWarning{plan.Warn("chart",
			fmt.Sprintf("Chart %q 通常是 %q 的配套/辅助 chart，不一定会安装主应用本体；安装 Argo CD 等服务请选用 chart %q",
				chart.ChartName, appName, app),
		)}
	}
	if !strings.Contains(chartName, app) {
		return []plan.PlanWarning{plan.Warn("chart",
			fmt.Sprintf("Chart 名称 %q 与目标应用 %q 不完全匹配，请确认是否选对", chart.ChartName, appName),
		)}
	}
	return nil
}

// resolveChartFromArtifactHub searches ArtifactHub and selects a chart.
func (a *Agent) resolveChartFromArtifactHub(ctx context.Context, appName string) (*catalog.ChartInfo, error) {
	ahResolver := catalog.NewArtifactHubResolver(nil)
	candidates, err := ahResolver.SearchCandidates(ctx, appName)

	if err != nil {
		a.logWarn("artifacthub search failed, showing manual input", zap.Error(err), zap.String("app", appName))
		candidates = nil
	}
	candidates = catalog.MergeCuratedChartCandidate(appName, candidates, rankChartCandidates)
	if len(candidates) > 0 {
		top := candidates[0]
		fields := []zap.Field{
			zap.String("app", appName),
			zap.String("top", top.ChartName),
			zap.String("repo", top.RepoName),
			zap.Bool("curated", top.CuratedPick),
			zap.Int("count", len(candidates)),
		}
		if !top.CuratedPick {
			fields = append(fields,
				zap.Int("top_score", scoreChartCandidate(top)),
				zap.Bool("verified", top.VerifiedPublisher),
				zap.Bool("signed", top.Signed),
				zap.Bool("official", top.Official),
			)
		}
		a.logDebug("chart candidates ready", fields...)
	}

	if a.selectionHandler == nil {
		if len(candidates) == 0 {
			return nil, fmt.Errorf("未找到应用 %q 的 Chart，请检查应用名称或手动指定 Chart 信息", appName)
		}
		c := pickBestChart(appName, candidates)
		c.Source = "artifacthub"
		a.logInfo("auto-selected best candidate from ArtifactHub",
			zap.String("app", appName),
			zap.String("repo", c.RepoName),
			zap.String("chart", c.ChartName),
		)
		return &c, nil
	}

	return a.selectionHandler.SelectChart(ctx, appName, candidates)
}
