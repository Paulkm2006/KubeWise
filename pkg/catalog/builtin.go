package catalog

import (
	_ "embed"
	"strings"
	"sync"

	"sigs.k8s.io/yaml"
)

//go:embed builtin_data.yaml
var builtinCatalogData []byte

type builtinCatalogEntry struct {
	Aliases          []string `json:"aliases" yaml:"aliases"`
	RepoName         string   `json:"repo_name" yaml:"repo_name"`
	RepoURL          string   `json:"repo_url" yaml:"repo_url"`
	Chart            string   `json:"chart" yaml:"chart"`
	DefaultNamespace string   `json:"default_namespace" yaml:"default_namespace"`
	InstallCRDs      bool     `json:"install_crds" yaml:"install_crds"`
	ClusterSingleton bool     `json:"cluster_singleton" yaml:"cluster_singleton"`
	Notes            string   `json:"notes" yaml:"notes"`
}

type builtinCatalogFile struct {
	Apps map[string]builtinCatalogEntry `json:"apps" yaml:"apps"`
}

var (
	builtinOnce             sync.Once
	builtinApps             map[string]ChartInfo
	builtinClusterSingleton map[string]bool // key: repo/chart (lowercase)
)

func chartCoordKey(repoName, chartName string) string {
	return strings.ToLower(strings.TrimSpace(repoName)) + "/" + strings.ToLower(strings.TrimSpace(chartName))
}

func loadBuiltinCatalog() {
	var catalog builtinCatalogFile
	if err := yaml.Unmarshal(builtinCatalogData, &catalog); err != nil {
		panic("failed to parse builtin catalog: " + err.Error())
	}

	builtinApps = make(map[string]ChartInfo)
	builtinClusterSingleton = make(map[string]bool)
	for appKey, entry := range catalog.Apps {
		singleton := entry.ClusterSingleton || entry.InstallCRDs
		info := ChartInfo{
			RepoName:         entry.RepoName,
			RepoURL:          entry.RepoURL,
			ChartName:        entry.Chart,
			DefaultNamespace: entry.DefaultNamespace,
			InstallCRDs:      entry.InstallCRDs,
			ClusterSingleton: singleton,
			Notes:            entry.Notes,
			Source:           "curated",
			CuratedPick:      true,
		}
		if entry.RepoName != "" && entry.Chart != "" {
			builtinClusterSingleton[chartCoordKey(entry.RepoName, entry.Chart)] = singleton
		}
		keys := append([]string{appKey}, entry.Aliases...)
		for _, alias := range keys {
			normalized := normalizeCatalogAlias(alias)
			if normalized == "" {
				continue
			}
			builtinApps[normalized] = info
		}
	}
}

func normalizeCatalogAlias(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

// LookupBuiltinChart returns a curated chart entry when appName matches an alias.
func LookupBuiltinChart(appName string) (*ChartInfo, bool) {
	builtinOnce.Do(loadBuiltinCatalog)
	normalized := normalizeCatalogAlias(appName)
	if normalized == "" {
		return nil, false
	}
	info, ok := builtinApps[normalized]
	if !ok {
		return nil, false
	}
	copy := info
	return &copy, true
}

// ChartLikelyClusterSingleton reports whether a chart typically cannot be installed
// twice under the same release name in different namespaces (cluster CRDs, etc.).
// Rules come from embedded builtin_data.yaml (cluster_singleton / install_crds).
func ChartLikelyClusterSingleton(chart *ChartInfo) bool {
	if chart == nil {
		return false
	}
	if chart.ClusterSingleton || chart.InstallCRDs {
		return true
	}
	builtinOnce.Do(loadBuiltinCatalog)
	if singleton, ok := builtinClusterSingleton[chartCoordKey(chart.RepoName, chart.ChartName)]; ok {
		return singleton
	}
	return false
}

// ChartsMatch reports whether two charts refer to the same repo/chart pair.
func ChartsMatch(a, b ChartInfo) bool {
	return strings.EqualFold(strings.TrimSpace(a.RepoName), strings.TrimSpace(b.RepoName)) &&
		strings.EqualFold(strings.TrimSpace(a.ChartName), strings.TrimSpace(b.ChartName))
}

// MergeCuratedChartCandidate ranks Artifact Hub results and pins the curated chart at #1.
// Artifact Hub is always searched first; curated only affects recommendation order.
func MergeCuratedChartCandidate(appName string, ahCandidates []ChartInfo, rank func(string, []ChartInfo) []ChartInfo) []ChartInfo {
	curated, ok := LookupBuiltinChart(appName)
	if !ok {
		if rank == nil {
			return ahCandidates
		}
		return rank(appName, ahCandidates)
	}

	ranked := ahCandidates
	if rank != nil && len(ahCandidates) > 0 {
		ranked = rank(appName, ahCandidates)
	}

	pinned := *curated
	pinned.CuratedPick = true

	for i, c := range ranked {
		if ChartsMatch(c, pinned) {
			merged := mergeArtifactHubIntoCurated(pinned, c)
			rest := append([]ChartInfo{}, ranked[:i]...)
			rest = append(rest, ranked[i+1:]...)
			return append([]ChartInfo{merged}, rest...)
		}
	}

	// Curated chart not in AH page — still show it first, then AH results.
	pinned.Source = "curated"
	return append([]ChartInfo{pinned}, ranked...)
}

func mergeArtifactHubIntoCurated(curated, ah ChartInfo) ChartInfo {
	out := curated
	out.Stars = ah.Stars
	out.Description = ah.Description
	out.LatestVersion = ah.LatestVersion
	out.VerifiedPublisher = ah.VerifiedPublisher
	out.Signed = ah.Signed
	out.Official = ah.Official
	out.CNCF = ah.CNCF
	out.Deprecated = ah.Deprecated
	out.ProductionOrganizationsCount = ah.ProductionOrganizationsCount
	if ah.Source != "" {
		out.Source = ah.Source
	} else {
		out.Source = "artifacthub"
	}
	out.CuratedPick = true
	out.ClusterSingleton = curated.ClusterSingleton
	out.InstallCRDs = curated.InstallCRDs
	if out.DefaultNamespace == "" {
		out.DefaultNamespace = ah.DefaultNamespace
	}
	if out.Notes == "" {
		out.Notes = ah.Notes
	}
	return out
}
