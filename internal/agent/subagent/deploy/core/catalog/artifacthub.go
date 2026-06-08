// pkg/catalog/artifacthub.go
package catalog

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"time"
)

const artifactHubBaseURL = "https://artifacthub.io/api/v1"

// ArtifactHubResolver 通过 Artifact Hub REST API 搜索 Helm Chart。
type ArtifactHubResolver struct {
	httpClient *http.Client
	timeout    time.Duration
}

// NewArtifactHubResolver 创建 Artifact Hub resolver。
func NewArtifactHubResolver(httpClient *http.Client) *ArtifactHubResolver {
	if httpClient == nil {
		httpClient = &http.Client{}
	}
	return &ArtifactHubResolver{
		httpClient: httpClient,
		timeout:    5 * time.Second,
	}
}

// artifactHubPackage 对应 Artifact Hub API 返回的 package 结构。
type artifactHubPackage struct {
	PackageID                    string `json:"package_id"`
	Name                         string `json:"name"`
	Description                  string `json:"description"`
	Version                      string `json:"version"`
	Stars                        int    `json:"stars"`
	Official                     bool   `json:"official"`
	CNCF                         bool   `json:"cncf"`
	Deprecated                   bool   `json:"deprecated"`
	Signed                       bool   `json:"signed"`
	ProductionOrganizationsCount int    `json:"production_organizations_count"`
	Repository                   struct {
		Name              string `json:"name"`
		URL               string `json:"url"`
		VerifiedPublisher bool   `json:"verified_publisher"`
	} `json:"repository"`
}

// artifactHubSearchResponse 对应搜索 API 的响应结构。
type artifactHubSearchResponse struct {
	Packages []artifactHubPackage `json:"packages"`
}

// Resolve 搜索 Artifact Hub，返回 stars 最多的 Chart。
// 网络错误视为软错误（返回 nil, nil），让链继续。
func (r *ArtifactHubResolver) Resolve(ctx context.Context, appName string) (*ChartInfo, error) {
	results, err := r.SearchCandidates(ctx, appName)
	if err != nil || len(results) == 0 {
		return nil, nil // 软错误，传递给下一个 resolver
	}
	top := results[0]
	return &top, nil
}

// SearchCandidates 返回最多 10 个候选 Chart，按 stars 降序排列。
// 供 TUI 交互选择使用。
func (r *ArtifactHubResolver) SearchCandidates(ctx context.Context, appName string) ([]ChartInfo, error) {
	ctx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()

	searchURL := fmt.Sprintf("%s/packages/search?kind=0&ts_query_web=%s&limit=10",
		artifactHubBaseURL, url.QueryEscape(appName))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, searchURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("artifact hub returned status %d", resp.StatusCode)
	}

	var result artifactHubSearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	// 按 stars 降序排列
	sort.Slice(result.Packages, func(i, j int) bool {
		return result.Packages[i].Stars > result.Packages[j].Stars
	})

	charts := make([]ChartInfo, 0, len(result.Packages))
	for _, pkg := range result.Packages {
		charts = append(charts, ChartInfo{
			RepoName:                     pkg.Repository.Name,
			RepoURL:                      pkg.Repository.URL,
			ChartName:                    pkg.Name,
			Stars:                        pkg.Stars,
			Description:                  pkg.Description,
			LatestVersion:                pkg.Version,
			Source:                       "artifacthub",
			VerifiedPublisher:            pkg.Repository.VerifiedPublisher,
			Signed:                       pkg.Signed,
			Official:                     pkg.Official,
			CNCF:                         pkg.CNCF,
			Deprecated:                   pkg.Deprecated,
			ProductionOrganizationsCount: pkg.ProductionOrganizationsCount,
		})
	}
	return charts, nil
}
