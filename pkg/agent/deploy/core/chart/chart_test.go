package chart

import (
	"testing"

	"github.com/kubewise/kubewise/pkg/catalog"
)

func TestScoreChartCandidate_prefersTrustSignalsOverStars(t *testing.T) {
	lowTrustHighStars := catalog.ChartInfo{ChartName: "argocd", Stars: 100}
	highTrust := catalog.ChartInfo{
		ChartName:         "argo-cd",
		Stars:             10,
		VerifiedPublisher: true,
		Signed:            true,
		Official:          true,
	}
	if ScoreChartCandidate(lowTrustHighStars) >= ScoreChartCandidate(highTrust) {
		t.Fatalf("trust signals should beat stars-only: %d vs %d",
			ScoreChartCandidate(lowTrustHighStars), ScoreChartCandidate(highTrust))
	}
}

func TestMergeCuratedIntegration_argocd(t *testing.T) {
	ah := []catalog.ChartInfo{
		{RepoName: "argo", ChartName: "argocd-apps", Stars: 95, Official: true, CNCF: true},
	}
	out := catalog.MergeCuratedChartCandidate("argocd", ah, RankChartCandidates)
	if out[0].ChartName != "argo-cd" {
		t.Fatalf("top = %q, want argo-cd (curated)", out[0].ChartName)
	}
}

func TestRankChartCandidates_trustBeforeStars(t *testing.T) {
	candidates := []catalog.ChartInfo{
		{ChartName: "random-argocd", Stars: 100},
		{
			ChartName:         "argo-cd",
			Stars:             5,
			VerifiedPublisher: true,
			Signed:            true,
			Official:          true,
		},
	}
	ranked := RankChartCandidates("argocd", candidates)
	if ranked[0].ChartName != "argo-cd" {
		t.Fatalf("first = %q, want argo-cd", ranked[0].ChartName)
	}
}

func TestScoreChartCandidate_deprecatedPenalty(t *testing.T) {
	active := catalog.ChartInfo{ChartName: "x", Stars: 1, VerifiedPublisher: true}
	deprecated := catalog.ChartInfo{ChartName: "y", Stars: 100, VerifiedPublisher: true, Deprecated: true}
	if ScoreChartCandidate(deprecated) >= ScoreChartCandidate(active) {
		t.Fatal("deprecated chart should rank lower")
	}
}

func TestSelectionWarnings_companionChart(t *testing.T) {
	w := SelectionWarnings("argocd", &catalog.ChartInfo{ChartName: "argocd-apps"})
	if len(w) == 0 {
		t.Fatal("expected warning for argocd-apps")
	}
}
