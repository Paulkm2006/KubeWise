package plan

import (
	"context"
	"fmt"

	"github.com/kubewise/kubewise/pkg/catalog"
	"github.com/kubewise/kubewise/pkg/helm"
)

// ReleaseLister lists Helm releases for conflict checks.
type ReleaseLister interface {
	ListReleases(ctx context.Context) ([]helm.Release, error)
}

// CheckHelmReleaseConflicts checks for an existing Helm release with the same name
// in another namespace. Per-namespace charts only get a warning; cluster_singleton charts block.
func CheckHelmReleaseConflicts(ctx context.Context, hc ReleaseLister, p DeployPlan) ValidationResult {
	var result ValidationResult
	if hc == nil || p.ReleaseName == "" {
		return result
	}

	releases, err := hc.ListReleases(ctx)
	if err != nil {
		result.Warnings = append(result.Warnings, Warn("helm",
			fmt.Sprintf("无法检查集群内已有 Helm release: %v", err)))
		return result
	}

	for _, rel := range releases {
		if rel.Name != p.ReleaseName || rel.Namespace == p.Namespace {
			continue
		}

		chartName := ""
		if p.Chart != nil {
			chartName = p.Chart.ChartName
		}

		if catalog.ChartLikelyClusterSingleton(p.Chart) {
			result.Errors = append(result.Errors, fmt.Sprintf(
				"集群中已有 Helm release %q 在 namespace %q（状态: %s），无法在 %q 再全新安装 chart %q。"+
					"该 chart 在 builtin 目录中标记为 cluster_singleton，重复安装通常会因集群级 CRD 失败。"+
					"请改为在 %q 执行 upgrade，或先卸载旧实例，或使用不同的 release 名称。",
				rel.Name, rel.Namespace, rel.Status, p.Namespace, chartName, rel.Namespace,
			))
			continue
		}

		result.Warnings = append(result.Warnings, Warn("helm",
			fmt.Sprintf("集群中已有同名 release %q 在 namespace %q（状态: %s）；在 %q 再装同名 release 对多数 chart（如 nginx）是允许的，请确认无端口/集群级资源冲突。",
				rel.Name, rel.Namespace, rel.Status, p.Namespace,
			)))
	}

	return result
}
