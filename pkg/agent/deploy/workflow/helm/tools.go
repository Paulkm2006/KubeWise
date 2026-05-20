package helm

import (
	"context"
	"fmt"
	"time"

	"github.com/kubewise/kubewise/pkg/agent/deploy/plan"
	"github.com/kubewise/kubewise/pkg/agent/deploy/workflow"
	"github.com/kubewise/kubewise/pkg/catalog"
	helmpkg "github.com/kubewise/kubewise/pkg/helm"
)

const (
	ToolRepoAdd        = "helm repo add"
	ToolShowValues     = "helm show values"
	ToolPreflight      = "helm preflight"
	ToolInstallUpgrade = "helm install/upgrade"
	ToolVerifyDeploy   = "verify deployment"
)

// Client is the Helm operations needed by deploy workflow tools.
type Client interface {
	AddRepo(ctx context.Context, name, repoURL string) error
	FetchDefaultValues(ctx context.Context, repoName, repoURL, chartName string) (string, error)
	InstallOrUpgrade(ctx context.Context, opts helmpkg.InstallOptions) (*helmpkg.Release, error)
	Validate(ctx context.Context, opts helmpkg.RenderOptions) (*helmpkg.ValidationResult, error)
}

// RepoAddInput is input for the helm repo add workflow tool.
type RepoAddInput struct {
	RepoName string
	RepoURL  string
}

// RepoAdd adds a Helm repository.
func RepoAdd(ctx context.Context, r *workflow.Runner, hc Client, in RepoAddInput) error {
	return r.Run(ctx, workflow.Meta{Name: ToolRepoAdd, Step: 1}, func(ctx context.Context) error {
		if err := hc.AddRepo(ctx, in.RepoName, in.RepoURL); err != nil {
			return fmt.Errorf("添加 Helm 仓库失败: %w", err)
		}
		return nil
	})
}

// ShowValuesInput is input for fetching default chart values.
type ShowValuesInput struct {
	RepoName  string
	RepoURL   string
	ChartName string
}

// ShowValues fetches default values for a chart.
func ShowValues(ctx context.Context, r *workflow.Runner, hc Client, in ShowValuesInput) (string, error) {
	return workflow.RunWithResult(r, ctx, workflow.Meta{Name: ToolShowValues, Step: 2}, func(ctx context.Context) (string, error) {
		vals, err := hc.FetchDefaultValues(ctx, in.RepoName, in.RepoURL, in.ChartName)
		if err != nil {
			return "", fmt.Errorf("获取默认 values 失败: %w", err)
		}
		return vals, nil
	})
}

// Preflight runs Helm render validation for a deploy plan.
func Preflight(ctx context.Context, r *workflow.Runner, hc plan.PreflightClient, p plan.DeployPlan, step int) error {
	return r.Run(ctx, workflow.Meta{Name: ToolPreflight, Step: step}, func(ctx context.Context) error {
		return plan.RunHelmPreflight(ctx, hc, p)
	})
}

// InstallInput is input for helm install/upgrade.
type InstallInput struct {
	ReleaseName string
	Chart       *catalog.ChartInfo
	Namespace   string
	Values      string
}

// InstallUpgrade installs or upgrades a Helm release.
func InstallUpgrade(ctx context.Context, r *workflow.Runner, hc Client, in InstallInput, step int) (*helmpkg.Release, error) {
	return workflow.RunWithResult(r, ctx, workflow.Meta{Name: ToolInstallUpgrade, Step: step}, func(ctx context.Context) (*helmpkg.Release, error) {
		return hc.InstallOrUpgrade(ctx, helmpkg.InstallOptions{
			ChartOptions: helmpkg.ChartOptions{
				ReleaseName: in.ReleaseName,
				RepoName:    in.Chart.RepoName,
				ChartName:   in.Chart.ChartName,
				RepoURL:     in.Chart.RepoURL,
				Namespace:   in.Namespace,
				Values:      in.Values,
			},
			CreateNS: true,
			Wait:     true,
			Timeout:  5 * time.Minute,
		})
	})
}
