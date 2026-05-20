package catalog

import (
	"testing"

	"sigs.k8s.io/yaml"
)

func TestBuiltinYAML_parsesRepoFields(t *testing.T) {
	var catalog builtinCatalogFile
	if err := yaml.Unmarshal(builtinCatalogData, &catalog); err != nil {
		t.Fatal(err)
	}
	e := catalog.Apps["argocd"]
	if e.RepoName != "argo" || e.RepoURL == "" || e.Chart != "argo-cd" {
		t.Fatalf("entry = %+v", e)
	}
}

func TestLookupBuiltinChart_argocd(t *testing.T) {
	got, ok := LookupBuiltinChart("ArgoCD")
	if !ok {
		t.Fatal("expected curated hit for ArgoCD")
	}
	if got.RepoName != "argo" || got.ChartName != "argo-cd" {
		t.Fatalf("got %s/%s, want argo/argo-cd", got.RepoName, got.ChartName)
	}
}

func TestLookupBuiltinChart_unknown(t *testing.T) {
	_, ok := LookupBuiltinChart("not-a-real-app-xyz")
	if ok {
		t.Fatal("expected no curated hit")
	}
}

func TestMergeCuratedChartCandidate_pinsKnownChart(t *testing.T) {
	ah := []ChartInfo{
		{RepoName: "argo", ChartName: "argocd-apps", Stars: 95, Official: true},
		{RepoName: "argo", ChartName: "argo-cd", Stars: 67, Signed: true, Official: true},
		{RepoName: "nicklasfrahm-argocd", ChartName: "argocd", Stars: 4},
	}
	identityRank := func(_ string, c []ChartInfo) []ChartInfo { return c }
	out := MergeCuratedChartCandidate("argocd", ah, identityRank)
	if !out[0].CuratedPick {
		t.Fatal("expected curated pick at top")
	}
	if out[0].ChartName != "argo-cd" {
		t.Fatalf("top = %q, want argo-cd", out[0].ChartName)
	}
	if out[0].Stars != 67 {
		t.Fatalf("expected AH metadata merged, stars=%d", out[0].Stars)
	}
}

func TestMergeCuratedChartCandidate_prependsWhenMissingFromAH(t *testing.T) {
	ah := []ChartInfo{
		{RepoName: "argo", ChartName: "argocd-apps", Stars: 95},
	}
	identityRank := func(_ string, c []ChartInfo) []ChartInfo { return c }
	out := MergeCuratedChartCandidate("argocd", ah, identityRank)
	if out[0].ChartName != "argo-cd" {
		t.Fatalf("top = %q, want argo-cd", out[0].ChartName)
	}
	if len(out) != 2 {
		t.Fatalf("len = %d, want 2", len(out))
	}
}

func TestChartLikelyClusterSingleton_fromBuiltinYAML(t *testing.T) {
	if !ChartLikelyClusterSingleton(&ChartInfo{RepoName: "argo", ChartName: "argo-cd"}) {
		t.Fatal("argo/argo-cd expected singleton from yaml")
	}
	if ChartLikelyClusterSingleton(&ChartInfo{RepoName: "bitnami", ChartName: "redis"}) {
		t.Fatal("bitnami/redis should not be singleton")
	}
	if !ChartLikelyClusterSingleton(&ChartInfo{RepoName: "jetstack", ChartName: "cert-manager", InstallCRDs: true}) {
		t.Fatal("cert-manager with install_crds expected singleton")
	}
}

func TestBuiltinYAML_clusterSingletonField(t *testing.T) {
	var catalog builtinCatalogFile
	if err := yaml.Unmarshal(builtinCatalogData, &catalog); err != nil {
		t.Fatal(err)
	}
	if !catalog.Apps["argocd"].ClusterSingleton {
		t.Fatal("argocd should be cluster_singleton in yaml")
	}
	if catalog.Apps["redis"].ClusterSingleton {
		t.Fatal("redis should not be cluster_singleton in yaml")
	}
}

func TestMergeCuratedChartCandidate_noCuratedPassthrough(t *testing.T) {
	ah := []ChartInfo{{ChartName: "foo", Stars: 1}}
	out := MergeCuratedChartCandidate("not-a-real-app-xyz", ah, nil)
	if len(out) != 1 || out[0].ChartName != "foo" {
		t.Fatalf("unexpected %v", out)
	}
}
