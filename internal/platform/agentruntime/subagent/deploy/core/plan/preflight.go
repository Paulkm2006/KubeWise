package plan

import (
	"context"
	"fmt"
	"strings"

	"github.com/kubewise/kubewise/internal/utils/helm"
)

// PreflightClient is the subset of helm.Client used for preflight checks.
type PreflightClient interface {
	Validate(ctx context.Context, opts helm.RenderOptions) (*helm.ValidationResult, error)
}

// RunHelmPreflight validates the chart render for the given plan.
func RunHelmPreflight(ctx context.Context, hc PreflightClient, p DeployPlan) error {
	if hc == nil {
		return nil
	}
	opts := helm.RenderOptions{
		ChartOptions: helm.ChartOptions{
			ReleaseName: p.ReleaseName,
			RepoName:    p.Chart.RepoName,
			ChartName:   p.Chart.ChartName,
			RepoURL:     p.Chart.RepoURL,
			Namespace:   p.Namespace,
			Values:      p.CustomValues,
		},
		IsUpgrade: p.IsUpgrade,
	}
	result, err := hc.Validate(ctx, opts)
	if err != nil {
		return fmt.Errorf("Helm 预检失败: %w", err)
	}
	if result != nil && result.HasErrors() {
		return fmt.Errorf("Helm 预检未通过: %s", result.ErrorSummary())
	}
	return nil
}

// IsNoDeployedReleaseError reports Helm errors indicating no deployed release exists.
func IsNoDeployedReleaseError(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "has no deployed releases")
}

// RunRecoveryHelmPreflight runs preflight and retries as install when upgrade finds no release.
func RunRecoveryHelmPreflight(ctx context.Context, hc PreflightClient, p *DeployPlan) error {
	err := RunHelmPreflight(ctx, hc, *p)
	if err == nil || !p.IsUpgrade || !IsNoDeployedReleaseError(err) {
		return err
	}
	p.IsUpgrade = false
	return RunHelmPreflight(ctx, hc, *p)
}
