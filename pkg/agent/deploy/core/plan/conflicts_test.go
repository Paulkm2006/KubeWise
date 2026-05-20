package plan

import (
	"context"
	"testing"

	"github.com/kubewise/kubewise/pkg/catalog"
	"github.com/kubewise/kubewise/pkg/helm"
)

type stubReleaseLister struct {
	releases []helm.Release
	err      error
}

func (s *stubReleaseLister) ListReleases(ctx context.Context) ([]helm.Release, error) {
	return s.releases, s.err
}

func TestCheckHelmReleaseConflicts_clusterSingletonBlocks(t *testing.T) {
	hc := &stubReleaseLister{releases: []helm.Release{
		{Name: "argocd", Namespace: "argocd", Status: "deployed"},
	}}
	p := DeployPlan{
		ReleaseName: "argocd",
		Namespace:   "argocd-new",
		Chart:       &catalog.ChartInfo{RepoName: "argo", ChartName: "argo-cd"},
	}
	r := CheckHelmReleaseConflicts(context.Background(), hc, p)
	if !r.HasBlockingErrors() {
		t.Fatal("expected blocking conflict for argo-cd")
	}
}

func TestCheckHelmReleaseConflicts_perNamespaceChartWarnsOnly(t *testing.T) {
	hc := &stubReleaseLister{releases: []helm.Release{
		{Name: "nginx", Namespace: "team-a", Status: "deployed"},
	}}
	p := DeployPlan{
		ReleaseName: "nginx",
		Namespace:   "team-b",
		Chart:       &catalog.ChartInfo{ChartName: "nginx"},
	}
	r := CheckHelmReleaseConflicts(context.Background(), hc, p)
	if r.HasBlockingErrors() {
		t.Fatalf("nginx per-ns install should not block: %v", r.Errors)
	}
	if len(r.Warnings) == 0 {
		t.Fatal("expected informational warning")
	}
}

func TestCheckHelmReleaseConflicts_sameNamespaceOK(t *testing.T) {
	hc := &stubReleaseLister{releases: []helm.Release{
		{Name: "argocd", Namespace: "argocd-new", Status: "deployed"},
	}}
	p := DeployPlan{
		ReleaseName: "argocd",
		Namespace:   "argocd-new",
		Chart:       &catalog.ChartInfo{RepoName: "argo", ChartName: "argo-cd"},
	}
	r := CheckHelmReleaseConflicts(context.Background(), hc, p)
	if r.HasBlockingErrors() {
		t.Fatalf("unexpected block: %v", r.Errors)
	}
}
