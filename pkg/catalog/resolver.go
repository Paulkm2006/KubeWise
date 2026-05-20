// pkg/catalog/resolver.go
package catalog

// ChartInfo 是解析后的 Chart 元数据。
type ChartInfo struct {
	RepoName         string // "argo" — 用于 helm repo add
	RepoURL          string // "https://argoproj.github.io/argo-helm"
	ChartName        string // "argo-cd"
	DefaultNamespace string // "argocd"
	InstallCRDs      bool   // 是否需要 --set installCRDs=true
	ClusterSingleton bool   // 通常一个集群只能装一套（CRD/集群级资源）；来自 builtin_data.yaml
	Notes            string // 给 LLM values 生成的额外提示
	// 由 ArtifactHub resolver 填充
	Stars         int    // 流行度指标
	Description   string // 单行描述
	LatestVersion string // 最新 chart 版本
	Source        string // "artifacthub" | "curated" | "manual"
	CuratedPick   bool   // 来自内置常用列表且被置顶推荐
	// Artifact Hub 信任/质量信号（用于候选排序，不作同名匹配）
	VerifiedPublisher            bool
	Signed                       bool
	Official                     bool // Artifact Hub official 标记
	CNCF                         bool
	Deprecated                   bool
	ProductionOrganizationsCount int
}
