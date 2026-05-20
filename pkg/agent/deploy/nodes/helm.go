package nodes

import (
	"context"

	"github.com/kubewise/kubewise/pkg/agent/deploy/plan"
	"github.com/kubewise/kubewise/pkg/agent/deploy/workflow"
	helmtools "github.com/kubewise/kubewise/pkg/agent/deploy/workflow/helm"
	"github.com/kubewise/kubewise/pkg/helm"
)

// AddHelmRepository runs helm repo add as a workflow tool step.
func AddHelmRepository(ctx context.Context, wf *workflow.Runner, hc HelmClient, repoName, repoURL string) error {
	return helmtools.RepoAdd(ctx, wf, hc, helmtools.RepoAddInput{
		RepoName: repoName,
		RepoURL:  repoURL,
	})
}

// HelmDefaultValues runs helm show values as a workflow tool step.
func HelmDefaultValues(ctx context.Context, wf *workflow.Runner, hc HelmClient, repoName, repoURL, chartName string) (string, error) {
	return helmtools.ShowValues(ctx, wf, hc, helmtools.ShowValuesInput{
		RepoName:  repoName,
		RepoURL:   repoURL,
		ChartName: chartName,
	})
}

// HelmPreflight validates the rendered chart for the current deploy plan.
func HelmPreflight(ctx context.Context, wf *workflow.Runner, hc HelmClient, p plan.DeployPlan, step int) error {
	return helmtools.Preflight(ctx, wf, hc, p, step)
}

// HelmInstallOrUpgrade installs or upgrades the Helm release for the pipeline.
func HelmInstallOrUpgrade(ctx context.Context, wf *workflow.Runner, hc HelmClient, in helmtools.InstallInput, step int) (*helm.Release, error) {
	return helmtools.InstallUpgrade(ctx, wf, hc, in, step)
}
