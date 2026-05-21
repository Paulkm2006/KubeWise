# Deploy Agent Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use [executing-plans] mode to implement this plan task-by-task.

**Goal:** 实现 Deploy Agent —— 一个基于 Helm 的应用部署 Agent，替代 Operation Agent 中依赖 LLM 生成原始 YAML 的不可靠部署方式。

**Architecture:** Deploy Agent 作为独立 Agent 与现有四个 Agent 并列，通过 ChainResolver（内置目录 → 本地目录 → Artifact Hub）解析 Helm Chart，LLM 仅负责生成最小化 override values，用户通过 TUI 审查后执行 `helm install/upgrade`。

**Tech Stack:** Go 1.26, `helm.sh/helm/v4`（Go SDK，非 exec），`github.com/charmbracelet/bubbletea`（TUI），`sigs.k8s.io/yaml`，`//go:embed`（内置目录数据）。

---

## File Map

| Action | Path | Responsibility |
|--------|------|----------------|
| Modify | `pkg/types/types.go` | 添加 `TaskTypeDeploy = "deploy"` |
| Create | `pkg/catalog/resolver.go` | `ChartInfo` 结构体、`ChartResolver` 接口、`ChainResolver` |
| Create | `pkg/catalog/builtin_data.yaml` | 内置 24 个常用应用的 Chart 元数据 |
| Create | `pkg/catalog/builtin.go` | `BuiltinCatalogResolver`（`//go:embed` 加载） |
| Create | `pkg/catalog/local.go` | `LocalCatalogResolver`（读取 `~/.kubewise/catalog.yaml`） |
| Create | `pkg/catalog/artifacthub.go` | `ArtifactHubResolver`（REST API 搜索） |
| Create | `pkg/helm/client.go` | Helm Go SDK 封装（install/upgrade/uninstall/status/list） |
| Create | `pkg/helm/values.go` | Values 解析、合并、diff 工具函数 |
| Create | `pkg/agent/deploy/agent.go` | `DeployAgent` 六阶段主流程 + `DeployConfirmationHandler` 接口 |
| Create | `pkg/agent/deploy/values_gen.go` | LLM values 生成逻辑 |
| Create | `pkg/tui/model/chart_select.go` | Chart 选择 TUI（Artifact Hub 结果列表 + 倒计时） |
| Create | `pkg/tui/model/deploy_confirm.go` | Values diff 确认 TUI（左右面板 + 编辑模式） |
| Create | `pkg/tui/model/manual_chart_input.go` | 手动输入 repo URL + chart 名称 TUI |
| Modify | `pkg/tui/events/events.go` | 添加 Deploy 相关 TUI 事件类型 |
| Modify | `pkg/tui/app.go` | 处理新的 Deploy TUI 事件 |
| Modify | `pkg/agent/router/agent.go` | 注册 `deployAgent`，路由 `deploy` 任务类型 |
| Modify | `go.mod` | 添加 `helm.sh/helm/v4` 依赖 |
| Modify | `config.yaml` | 添加 `deploy.artifact_hub` 和 `deploy.helm` 配置节 |

---

## Task 1: 添加 `TaskTypeDeploy` 类型

**Files:**
- Modify: `pkg/types/types.go`

**Step 1: 在 `TaskType` 常量块中添加新类型**

在 [`pkg/types/types.go:12`](pkg/types/types.go:12) 的常量块末尾添加：

```go
TaskTypeDeploy TaskType = "deploy" // 应用部署类（Helm）
```

**Step 2: 验证编译通过**

```bash
go build ./pkg/types/...
```

预期：无错误输出。

**Step 3: 提交**

```bash
git add pkg/types/types.go
git commit -m "feat(types): add TaskTypeDeploy constant"
```

---

## Task 2: 创建 Chart Catalog 基础结构

**Files:**
- Create: `pkg/catalog/resolver.go`

**Step 1: 创建 `resolver.go`**

```go
// pkg/catalog/resolver.go
package catalog

import "context"

// ChartInfo 是解析后的 Chart 元数据。
type ChartInfo struct {
	RepoName         string // "argo" — 用于 helm repo add
	RepoURL          string // "https://argoproj.github.io/argo-helm"
	ChartName        string // "argo-cd"
	DefaultNamespace string // "argocd"
	InstallCRDs      bool   // 是否需要 --set installCRDs=true
	Notes            string // 给 LLM values 生成的额外提示
	// 由 ArtifactHub resolver 填充
	Stars         int    // 流行度指标
	Description   string // 单行描述
	LatestVersion string // 最新 chart 版本
	Source        string // "catalog" | "local" | "artifacthub" | "manual"
}

// ChartResolver 将应用名称解析为 Chart 元数据。
type ChartResolver interface {
	// Resolve 尝试将 appName 解析为 ChartInfo。
	// 返回 (nil, nil) 表示此 resolver 无法处理（传递给下一个）。
	// 返回 (nil, err) 表示硬错误（中止链）。
	Resolve(ctx context.Context, appName string) (*ChartInfo, error)
}

// ChainResolver 按优先级顺序调用多个 resolver。
type ChainResolver struct {
	resolvers []ChartResolver
}

// NewChainResolver 创建一个链式 resolver。
func NewChainResolver(resolvers ...ChartResolver) *ChainResolver {
	return &ChainResolver{resolvers: resolvers}
}

// Resolve 依次调用每个 resolver，返回第一个非 nil 结果。
func (c *ChainResolver) Resolve(ctx context.Context, appName string) (*ChartInfo, error) {
	for _, r := range c.resolvers {
		info, err := r.Resolve(ctx, appName)
		if err != nil {
			return nil, err
		}
		if info != nil {
			return info, nil
		}
	}
	return nil, nil // 所有 resolver 均返回 nil
}

// NewDefaultChainResolver 创建默认的链式 resolver（内置 → 本地 → ArtifactHub）。
func NewDefaultChainResolver(httpClient interface{ Do(req interface{}) (interface{}, error) }) *ChainResolver {
	return NewChainResolver(
		NewBuiltinCatalogResolver(),
		NewLocalCatalogResolver(),
	)
}
```

> **注意：** `NewDefaultChainResolver` 在 Task 5 完成 ArtifactHub resolver 后再更新签名。

**Step 2: 验证编译**

```bash
go build ./pkg/catalog/...
```

**Step 3: 提交**

```bash
git add pkg/catalog/resolver.go
git commit -m "feat(catalog): add ChartInfo, ChartResolver interface, ChainResolver"
```

---

## Task 3: 创建内置 Chart 目录

**Files:**
- Create: `pkg/catalog/builtin_data.yaml`
- Create: `pkg/catalog/builtin.go`

**Step 1: 创建 `builtin_data.yaml`**

```yaml
# pkg/catalog/builtin_data.yaml
# 内置 Helm Chart 目录，通过 //go:embed 编译进二进制文件
apps:
  argocd:
    aliases: ["argo-cd", "argo cd", "argocd"]
    repo_name: "argo"
    repo_url: "https://argoproj.github.io/argo-helm"
    chart: "argo-cd"
    default_namespace: "argocd"
    notes: "GitOps CD tool. Key values: server.service.type, server.ingress.enabled"

  prometheus:
    aliases: ["prometheus", "prom", "prometheus-stack", "kube-prometheus"]
    repo_name: "prometheus-community"
    repo_url: "https://prometheus-community.github.io/helm-charts"
    chart: "kube-prometheus-stack"
    default_namespace: "monitoring"
    notes: "Full monitoring stack. Key values: grafana.enabled, alertmanager.enabled"

  cert-manager:
    aliases: ["cert-manager", "certmanager"]
    repo_name: "jetstack"
    repo_url: "https://charts.jetstack.io"
    chart: "cert-manager"
    default_namespace: "cert-manager"
    install_crds: true
    notes: "TLS certificate management. Requires installCRDs=true for first install"

  nginx-ingress:
    aliases: ["nginx-ingress", "ingress-nginx", "nginx ingress controller"]
    repo_name: "ingress-nginx"
    repo_url: "https://kubernetes.github.io/ingress-nginx"
    chart: "ingress-nginx"
    default_namespace: "ingress-nginx"

  metallb:
    aliases: ["metallb", "metal-lb"]
    repo_name: "metallb"
    repo_url: "https://metallb.github.io/metallb"
    chart: "metallb"
    default_namespace: "metallb-system"

  redis:
    aliases: ["redis"]
    repo_name: "bitnami"
    repo_url: "https://charts.bitnami.com/bitnami"
    chart: "redis"
    default_namespace: "default"

  mysql:
    aliases: ["mysql"]
    repo_name: "bitnami"
    repo_url: "https://charts.bitnami.com/bitnami"
    chart: "mysql"
    default_namespace: "default"

  postgresql:
    aliases: ["postgresql", "postgres", "pg"]
    repo_name: "bitnami"
    repo_url: "https://charts.bitnami.com/bitnami"
    chart: "postgresql"
    default_namespace: "default"

  grafana:
    aliases: ["grafana"]
    repo_name: "grafana"
    repo_url: "https://grafana.github.io/helm-charts"
    chart: "grafana"
    default_namespace: "monitoring"

  loki:
    aliases: ["loki", "loki-stack"]
    repo_name: "grafana"
    repo_url: "https://grafana.github.io/helm-charts"
    chart: "loki-stack"
    default_namespace: "monitoring"

  traefik:
    aliases: ["traefik"]
    repo_name: "traefik"
    repo_url: "https://traefik.github.io/charts"
    chart: "traefik"
    default_namespace: "traefik"

  harbor:
    aliases: ["harbor"]
    repo_name: "harbor"
    repo_url: "https://helm.goharbor.io"
    chart: "harbor"
    default_namespace: "harbor"

  minio:
    aliases: ["minio"]
    repo_name: "minio"
    repo_url: "https://charts.min.io/"
    chart: "minio"
    default_namespace: "minio"

  elasticsearch:
    aliases: ["elasticsearch", "es", "elastic"]
    repo_name: "elastic"
    repo_url: "https://helm.elastic.co"
    chart: "elasticsearch"
    default_namespace: "elastic"

  kafka:
    aliases: ["kafka"]
    repo_name: "bitnami"
    repo_url: "https://charts.bitnami.com/bitnami"
    chart: "kafka"
    default_namespace: "default"

  rabbitmq:
    aliases: ["rabbitmq"]
    repo_name: "bitnami"
    repo_url: "https://charts.bitnami.com/bitnami"
    chart: "rabbitmq"
    default_namespace: "default"

  vault:
    aliases: ["vault", "hashicorp-vault"]
    repo_name: "hashicorp"
    repo_url: "https://helm.releases.hashicorp.com"
    chart: "vault"
    default_namespace: "vault"

  consul:
    aliases: ["consul", "hashicorp-consul"]
    repo_name: "hashicorp"
    repo_url: "https://helm.releases.hashicorp.com"
    chart: "consul"
    default_namespace: "consul"

  istio:
    aliases: ["istio", "istio-base"]
    repo_name: "istio"
    repo_url: "https://istio-release.storage.googleapis.com/charts"
    chart: "base"
    default_namespace: "istio-system"
    notes: "Istio service mesh base chart. Usually installed with istiod chart."

  linkerd:
    aliases: ["linkerd"]
    repo_name: "linkerd"
    repo_url: "https://helm.linkerd.io/stable"
    chart: "linkerd-control-plane"
    default_namespace: "linkerd"

  velero:
    aliases: ["velero"]
    repo_name: "vmware-tanzu"
    repo_url: "https://vmware-tanzu.github.io/helm-charts"
    chart: "velero"
    default_namespace: "velero"
    notes: "Backup and disaster recovery. Requires cloud provider plugin configuration."

  external-dns:
    aliases: ["external-dns", "externaldns"]
    repo_name: "external-dns"
    repo_url: "https://kubernetes-sigs.github.io/external-dns/"
    chart: "external-dns"
    default_namespace: "external-dns"

  metrics-server:
    aliases: ["metrics-server"]
    repo_name: "metrics-server"
    repo_url: "https://kubernetes-sigs.github.io/metrics-server/"
    chart: "metrics-server"
    default_namespace: "kube-system"

  sealed-secrets:
    aliases: ["sealed-secrets"]
    repo_name: "sealed-secrets"
    repo_url: "https://bitnami-labs.github.io/sealed-secrets"
    chart: "sealed-secrets"
    default_namespace: "kube-system"
```

**Step 2: 创建 `builtin.go`**

```go
// pkg/catalog/builtin.go
package catalog

import (
	_ "embed"
	"strings"

	"sigs.k8s.io/yaml"
)

//go:embed builtin_data.yaml
var builtinCatalogData []byte

// builtinCatalogEntry 对应 YAML 中每个 app 的结构。
type builtinCatalogEntry struct {
	Aliases          []string `yaml:"aliases"`
	RepoName         string   `yaml:"repo_name"`
	RepoURL          string   `yaml:"repo_url"`
	Chart            string   `yaml:"chart"`
	DefaultNamespace string   `yaml:"default_namespace"`
	InstallCRDs      bool     `yaml:"install_crds"`
	Notes            string   `yaml:"notes"`
}

// builtinCatalogFile 对应 YAML 文件的顶层结构。
type builtinCatalogFile struct {
	Apps map[string]builtinCatalogEntry `yaml:"apps"`
}

// BuiltinCatalogResolver 从内置 YAML 数据解析 Chart 信息。
type BuiltinCatalogResolver struct {
	// alias（小写）→ ChartInfo 的查找表
	apps map[string]*ChartInfo
}

// NewBuiltinCatalogResolver 解析内置 YAML 数据，构建 alias→ChartInfo 查找表。
// 如果 YAML 解析失败，panic（这是编译时嵌入的数据，不应失败）。
func NewBuiltinCatalogResolver() *BuiltinCatalogResolver {
	var catalog builtinCatalogFile
	if err := yaml.Unmarshal(builtinCatalogData, &catalog); err != nil {
		panic("failed to parse builtin catalog: " + err.Error())
	}

	apps := make(map[string]*ChartInfo)
	for _, entry := range catalog.Apps {
		info := &ChartInfo{
			RepoName:         entry.RepoName,
			RepoURL:          entry.RepoURL,
			ChartName:        entry.Chart,
			DefaultNamespace: entry.DefaultNamespace,
			InstallCRDs:      entry.InstallCRDs,
			Notes:            entry.Notes,
			Source:           "catalog",
		}
		for _, alias := range entry.Aliases {
			normalized := strings.ToLower(strings.TrimSpace(alias))
			apps[normalized] = info
		}
	}

	return &BuiltinCatalogResolver{apps: apps}
}

// Resolve 通过 alias 查找 ChartInfo（大小写不敏感）。
func (r *BuiltinCatalogResolver) Resolve(ctx context.Context, appName string) (*ChartInfo, error) {
	normalized := strings.ToLower(strings.TrimSpace(appName))
	if info, ok := r.apps[normalized]; ok {
		// 返回副本，避免修改共享状态
		copy := *info
		copy.Source = "catalog"
		return &copy, nil
	}
	return nil, nil
}
```

> **验证行为：** 调用 `NewBuiltinCatalogResolver().Resolve(ctx, "ArgoCD")` 应返回 `RepoName="argo"`, `ChartName="argo-cd"`, `Source="catalog"`。调用 `Resolve(ctx, "unknown-app")` 应返回 `(nil, nil)`。

**Step 3: 验证编译**

```bash
go build ./pkg/catalog/...
```

**Step 4: 提交**

```bash
git add pkg/catalog/builtin_data.yaml pkg/catalog/builtin.go
git commit -m "feat(catalog): add builtin chart catalog with 24 apps"
```

---

## Task 4: 创建本地 Catalog Resolver

**Files:**
- Create: `pkg/catalog/local.go`

**Step 1: 创建 `local.go`**

```go
// pkg/catalog/local.go
package catalog

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"sigs.k8s.io/yaml"
)

// LocalCatalogResolver 从用户本地 ~/.kubewise/catalog.yaml 读取 Chart 信息。
// 文件格式与 builtin_data.yaml 相同。
// 如果文件不存在，Resolve 始终返回 (nil, nil)（不报错）。
type LocalCatalogResolver struct {
	apps map[string]*ChartInfo // 懒加载，首次 Resolve 时初始化
	path string
}

// NewLocalCatalogResolver 创建本地目录 resolver。
// 默认路径为 ~/.kubewise/catalog.yaml。
func NewLocalCatalogResolver() *LocalCatalogResolver {
	home, _ := os.UserHomeDir()
	return &LocalCatalogResolver{
		path: filepath.Join(home, ".kubewise", "catalog.yaml"),
	}
}

// Resolve 从本地文件解析 Chart 信息。
func (r *LocalCatalogResolver) Resolve(ctx context.Context, appName string) (*ChartInfo, error) {
	if r.apps == nil {
		if err := r.load(); err != nil {
			// 加载失败不是硬错误，跳过本地目录
			r.apps = make(map[string]*ChartInfo)
		}
	}

	normalized := strings.ToLower(strings.TrimSpace(appName))
	if info, ok := r.apps[normalized]; ok {
		copy := *info
		copy.Source = "local"
		return &copy, nil
	}
	return nil, nil
}

// load 读取并解析本地 catalog 文件。
func (r *LocalCatalogResolver) load() error {
	data, err := os.ReadFile(r.path)
	if os.IsNotExist(err) {
		r.apps = make(map[string]*ChartInfo)
		return nil
	}
	if err != nil {
		return err
	}

	var catalog builtinCatalogFile // 复用相同的 YAML 结构
	if err := yaml.Unmarshal(data, &catalog); err != nil {
		return err
	}

	r.apps = make(map[string]*ChartInfo)
	for _, entry := range catalog.Apps {
		info := &ChartInfo{
			RepoName:         entry.RepoName,
			RepoURL:          entry.RepoURL,
			ChartName:        entry.Chart,
			DefaultNamespace: entry.DefaultNamespace,
			InstallCRDs:      entry.InstallCRDs,
			Notes:            entry.Notes,
		}
		for _, alias := range entry.Aliases {
			normalized := strings.ToLower(strings.TrimSpace(alias))
			r.apps[normalized] = info
		}
	}
	return nil
}
```

> **验证行为：** 当 `~/.kubewise/catalog.yaml` 不存在时，`Resolve` 返回 `(nil, nil)` 而不是错误。当文件存在且包含匹配条目时，返回对应的 `ChartInfo`，`Source="local"`。

**Step 2: 验证编译**

```bash
go build ./pkg/catalog/...
```

**Step 3: 提交**

```bash
git add pkg/catalog/local.go
git commit -m "feat(catalog): add local catalog resolver (~/.kubewise/catalog.yaml)"
```

---

## Task 5: 创建 Artifact Hub Resolver

**Files:**
- Create: `pkg/catalog/artifacthub.go`

**Step 1: 创建 `artifacthub.go`**

```go
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
	PackageID   string `json:"package_id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Version     string `json:"version"`
	Stars       int    `json:"stars"`
	Repository  struct {
		Name string `json:"name"`
		URL  string `json:"url"`
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
			RepoName:      pkg.Repository.Name,
			RepoURL:       pkg.Repository.URL,
			ChartName:     pkg.Name,
			Stars:         pkg.Stars,
			Description:   pkg.Description,
			LatestVersion: pkg.Version,
			Source:        "artifacthub",
		})
	}
	return charts, nil
}
```

**Step 2: 更新 `resolver.go` 中的 `NewDefaultChainResolver`**

将 [`pkg/catalog/resolver.go`](pkg/catalog/resolver.go) 中的 `NewDefaultChainResolver` 替换为：

```go
// NewDefaultChainResolver 创建默认的链式 resolver（内置 → 本地 → ArtifactHub）。
func NewDefaultChainResolver(httpClient *http.Client) *ChainResolver {
	return NewChainResolver(
		NewBuiltinCatalogResolver(),
		NewLocalCatalogResolver(),
		NewArtifactHubResolver(httpClient),
	)
}
```

**Step 3: 验证编译**

```bash
go build ./pkg/catalog/...
```

**Step 4: 提交**

```bash
git add pkg/catalog/artifacthub.go pkg/catalog/resolver.go
git commit -m "feat(catalog): add ArtifactHub resolver with candidate search"
```

---

## Task 6: 添加 Helm Go SDK 依赖

**Files:**
- Modify: `go.mod`

**Step 1: 添加 Helm SDK 依赖**

```bash
go get helm.sh/helm/v3@latest
go mod tidy
```

**Step 2: 验证依赖已添加**

```bash
grep "helm.sh/helm/v3" go.mod
```

预期输出：`helm.sh/helm/v3 v3.x.x`

**Step 3: 提交**

```bash
git add go.mod go.sum
git commit -m "chore(deps): add helm.sh/helm/v3 SDK dependency"
```

---

## Task 7: 创建 Helm Client

**Files:**
- Create: `pkg/helm/client.go`
- Create: `pkg/helm/values.go`

**Step 1: 创建 `client.go`**

```go
// pkg/helm/client.go
package helm

import (
	"context"
	"fmt"
	"strings"
	"time"

	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/cli"
	"helm.sh/helm/v3/pkg/release"
	"helm.sh/helm/v3/pkg/repo"
	"sigs.k8s.io/yaml"
)

// Release 表示一个 Helm release 的状态。
type Release struct {
	Name      string
	Namespace string
	Chart     string
	Version   string
	Status    string // "deployed", "failed", "pending-install" 等
	Updated   time.Time
}

// InstallOptions 控制 helm install/upgrade 的行为。
type InstallOptions struct {
	ReleaseName string
	RepoName    string
	ChartName   string
	Namespace   string
	Values      string        // override values YAML 字符串
	CreateNS    bool          // --create-namespace
	Wait        bool          // --wait
	Timeout     time.Duration // --timeout
}

// Client 封装 Helm Go SDK 操作。
type Client struct {
	settings   *cli.EnvSettings
	kubeConfig string
}

// New 创建 Helm Client。
// kubeConfig 为空时使用默认 kubeconfig（~/.kube/config 或 KUBECONFIG 环境变量）。
func New(kubeConfig string) *Client {
	settings := cli.New()
	if kubeConfig != "" {
		settings.KubeConfig = kubeConfig
	}
	return &Client{
		settings:   settings,
		kubeConfig: kubeConfig,
	}
}

// actionConfig 创建 Helm action 配置（每次操作创建新实例以支持不同 namespace）。
func (c *Client) actionConfig(namespace string) (*action.Configuration, error) {
	cfg := new(action.Configuration)
	if err := cfg.Init(c.settings.RESTClientGetter(), namespace, "secrets", func(format string, v ...interface{}) {
		// 静默 Helm 内部日志
	}); err != nil {
		return nil, fmt.Errorf("初始化 helm action 配置失败: %w", err)
	}
	return cfg, nil
}

// AddRepo 添加 Helm 仓库并更新索引。
func (c *Client) AddRepo(ctx context.Context, name, repoURL string) error {
	entry := &repo.Entry{
		Name: name,
		URL:  repoURL,
	}
	r, err := repo.NewChartRepository(entry, getter.All(c.settings))
	if err != nil {
		return fmt.Errorf("创建 chart 仓库失败: %w", err)
	}
	if _, err := r.DownloadIndexFile(); err != nil {
		return fmt.Errorf("下载仓库索引失败: %w", err)
	}

	// 持久化到 repositories.yaml
	repoFile := c.settings.RepositoryConfig
	f, err := repo.LoadFile(repoFile)
	if err != nil {
		f = repo.NewFile()
	}
	f.Update(entry)
	return f.WriteFile(repoFile, 0644)
}

// FetchDefaultValues 运行 helm show values，返回完整的 values.yaml 字符串（含注释）。
func (c *Client) FetchDefaultValues(ctx context.Context, repoName, chartName string) (string, error) {
	cfg, err := c.actionConfig("")
	if err != nil {
		return "", err
	}

	client := action.NewShowWithConfig(action.ShowValues, cfg)
	chartRef := fmt.Sprintf("%s/%s", repoName, chartName)
	output, err := client.Run(chartRef)
	if err != nil {
		return "", fmt.Errorf("获取 chart 默认 values 失败: %w", err)
	}
	return output, nil
}

// InstallOrUpgrade 安装新 release 或升级已有 release。
func (c *Client) InstallOrUpgrade(ctx context.Context, opts InstallOptions) (*Release, error) {
	cfg, err := c.actionConfig(opts.Namespace)
	if err != nil {
		return nil, err
	}

	// 解析 override values
	vals := map[string]interface{}{}
	if opts.Values != "" {
		if err := yaml.Unmarshal([]byte(opts.Values), &vals); err != nil {
			return nil, fmt.Errorf("解析 override values 失败: %w", err)
		}
	}

	chartRef := fmt.Sprintf("%s/%s", opts.RepoName, opts.ChartName)

	// 检查 release 是否已存在
	histClient := action.NewHistory(cfg)
	histClient.Max = 1
	_, histErr := histClient.Run(opts.ReleaseName)

	var rel *release.Release
	if histErr != nil {
		// 全新安装
		installClient := action.NewInstall(cfg)
		installClient.ReleaseName = opts.ReleaseName
		installClient.Namespace = opts.Namespace
		installClient.CreateNamespace = opts.CreateNS
		installClient.Wait = opts.Wait
		if opts.Timeout > 0 {
			installClient.Timeout = opts.Timeout
		}

		chart, err := loader.Load(chartRef)
		if err != nil {
			return nil, fmt.Errorf("加载 chart 失败: %w", err)
		}
		rel, err = installClient.RunWithContext(ctx, chart, vals)
		if err != nil {
			return nil, fmt.Errorf("helm install 失败: %w", err)
		}
	} else {
		// 升级已有 release
		upgradeClient := action.NewUpgrade(cfg)
		upgradeClient.Namespace = opts.Namespace
		upgradeClient.Wait = opts.Wait
		if opts.Timeout > 0 {
			upgradeClient.Timeout = opts.Timeout
		}

		chart, err := loader.Load(chartRef)
		if err != nil {
			return nil, fmt.Errorf("加载 chart 失败: %w", err)
		}
		rel, err = upgradeClient.RunWithContext(ctx, opts.ReleaseName, chart, vals)
		if err != nil {
			return nil, fmt.Errorf("helm upgrade 失败: %w", err)
		}
	}

	return releaseToRelease(rel), nil
}

// Uninstall 卸载一个 release。
func (c *Client) Uninstall(ctx context.Context, releaseName, namespace string) error {
	cfg, err := c.actionConfig(namespace)
	if err != nil {
		return err
	}
	client := action.NewUninstall(cfg)
	_, err = client.Run(releaseName)
	return err
}

// Status 返回指定 release 的状态。
// 如果 release 不存在，返回 (nil, nil)。
func (c *Client) Status(ctx context.Context, releaseName, namespace string) (*Release, error) {
	cfg, err := c.actionConfig(namespace)
	if err != nil {
		return nil, err
	}
	client := action.NewStatus(cfg)
	rel, err := client.Run(releaseName)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return nil, nil
		}
		return nil, err
	}
	return releaseToRelease(rel), nil
}

// ListReleases 返回所有 namespace 中的所有 release。
func (c *Client) ListReleases(ctx context.Context) ([]Release, error) {
	cfg, err := c.actionConfig("")
	if err != nil {
		return nil, err
	}
	client := action.NewList(cfg)
	client.AllNamespaces = true
	client.All = true

	rels, err := client.Run()
	if err != nil {
		return nil, err
	}

	result := make([]Release, 0, len(rels))
	for _, r := range rels {
		result = append(result, *releaseToRelease(r))
	}
	return result, nil
}

// releaseToRelease 将 Helm SDK release 转换为本地 Release 结构。
func releaseToRelease(r *release.Release) *Release {
	if r == nil {
		return nil
	}
	chartName := ""
	if r.Chart != nil && r.Chart.Metadata != nil {
		chartName = fmt.Sprintf("%s-%s", r.Chart.Metadata.Name, r.Chart.Metadata.Version)
	}
	return &Release{
		Name:      r.Name,
		Namespace: r.Namespace,
		Chart:     chartName,
		Version:   fmt.Sprintf("%d", r.Version),
		Status:    r.Info.Status.String(),
		Updated:   r.Info.LastDeployed.Time,
	}
}
```

**Step 2: 创建 `values.go`**

```go
// pkg/helm/values.go
package helm

import (
	"fmt"
	"strings"

	"sigs.k8s.io/yaml"
)

// MergeValues 将 override values YAML 合并到 base values YAML 中。
// override 中的字段会覆盖 base 中的同名字段。
// 返回合并后的 YAML 字符串。
func MergeValues(baseYAML, overrideYAML string) (string, error) {
	base := map[string]interface{}{}
	if err := yaml.Unmarshal([]byte(baseYAML), &base); err != nil {
		return "", fmt.Errorf("解析 base values 失败: %w", err)
	}

	override := map[string]interface{}{}
	if overrideYAML != "" {
		if err := yaml.Unmarshal([]byte(overrideYAML), &override); err != nil {
			return "", fmt.Errorf("解析 override values 失败: %w", err)
		}
	}

	merged := mergeMaps(base, override)
	result, err := yaml.Marshal(merged)
	if err != nil {
		return "", fmt.Errorf("序列化合并后的 values 失败: %w", err)
	}
	return string(result), nil
}

// ValidateYAML 验证字符串是否为合法的 YAML。
func ValidateYAML(yamlStr string) error {
	var v interface{}
	return yaml.Unmarshal([]byte(yamlStr), &v)
}

// mergeMaps 递归合并两个 map，override 中的值优先。
func mergeMaps(base, override map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{})
	for k, v := range base {
		result[k] = v
	}
	for k, v := range override {
		if baseVal, ok := result[k]; ok {
			if baseMap, ok := baseVal.(map[string]interface{}); ok {
				if overrideMap, ok := v.(map[string]interface{}); ok {
					result[k] = mergeMaps(baseMap, overrideMap)
					continue
				}
			}
		}
		result[k] = v
	}
	return result
}

// TruncateValues 截断过长的 values YAML，保留前 maxLines 行。
// 用于处理超大 values.yaml（如 Prometheus Stack ~3000 行）。
func TruncateValues(valuesYAML string, maxLines int) string {
	lines := strings.Split(valuesYAML, "\n")
	if len(lines) <= maxLines {
		return valuesYAML
	}
	truncated := lines[:maxLines]
	truncated = append(truncated, fmt.Sprintf("\n# ... (已截断，原文件共 %d 行)", len(lines)))
	return strings.Join(truncated, "\n")
}
```

**Step 3: 验证编译**

```bash
go build ./pkg/helm/...
```

**Step 4: 提交**

```bash
git add pkg/helm/client.go pkg/helm/values.go
git commit -m "feat(helm): add Helm Go SDK client wrapper and values utilities"
```

---

## Task 8: 添加 Deploy 相关 TUI 事件

**Files:**
- Modify: `pkg/tui/events/events.go`

**Step 1: 查看现有事件类型**

先阅读 [`pkg/tui/events/events.go`](pkg/tui/events/events.go) 了解现有事件结构。

**Step 2: 添加 Deploy 事件类型**

在 `events.go` 中添加以下事件类型：

```go
// ChartSelectRequestEvent 请求 TUI 显示 Chart 选择界面。
type ChartSelectRequestEvent struct {
	QueryID    string
	AppName    string
	Candidates []catalog.ChartInfo // 候选 Chart 列表（来自 ArtifactHub）
}

// ChartSelectResponseEvent 用户从 TUI 选择了 Chart（或手动输入）。
type ChartSelectResponseEvent struct {
	QueryID   string
	ChartInfo *catalog.ChartInfo // nil 表示用户取消
}

// ManualChartInputRequestEvent 请求 TUI 显示手动输入界面。
type ManualChartInputRequestEvent struct {
	QueryID string
}

// ManualChartInputResponseEvent 用户完成手动输入。
type ManualChartInputResponseEvent struct {
	QueryID  string
	RepoURL  string
	ChartName string
	Cancelled bool
}

// DeployConfirmRequestEvent 请求 TUI 显示 Deploy 确认界面。
type DeployConfirmRequestEvent struct {
	QueryID       string
	Plan          deploy.DeployPlan
}

// DeployConfirmResponseEvent 用户完成 Deploy 确认。
type DeployConfirmResponseEvent struct {
	QueryID  string
	Decision deploy.DeployDecision
}
```

> **注意：** 需要在 `events.go` 中 import `pkg/catalog` 和 `pkg/agent/deploy`。如果存在循环依赖，将 `DeployPlan` 和 `DeployDecision` 移到 `pkg/types` 包中。

**Step 3: 验证编译**

```bash
go build ./pkg/tui/events/...
```

**Step 4: 提交**

```bash
git add pkg/tui/events/events.go
git commit -m "feat(events): add Deploy-related TUI event types"
```

---

## Task 9: 创建 LLM Values 生成逻辑

**Files:**
- Create: `pkg/agent/deploy/values_gen.go`

**Step 1: 创建 `values_gen.go`**

```go
// pkg/agent/deploy/values_gen.go
package deploy

import (
	"context"
	"fmt"
	"strings"

	"github.com/kubewise/kubewise/pkg/catalog"
	"github.com/kubewise/kubewise/pkg/helm"
	"github.com/kubewise/kubewise/pkg/llm"
)

const valuesGenSystemPrompt = `你是 Helm values 配置专家。

用户意图：%s
应用：%s（%s）
目标命名空间：%s
%s

以下是该 chart 的完整默认 values.yaml（带注释，作为参考）：
---
%s
---

请根据用户意图，生成最小化的 override values YAML。

规则：
1. 只包含需要修改的字段，不要重复默认值
2. 保持 YAML 层级结构正确
3. 如果用户没有明确要求，不要修改安全相关配置（密码、证书等）
4. 在每个修改项上方加注释说明修改原因
5. 如果用户意图不需要修改任何值（使用默认配置即可），输出空 YAML 并说明

输出格式：纯 YAML，不要包含 markdown 代码块标记。`

const maxDefaultValuesLines = 2000 // 超过此行数时截断

// generateValues 调用 LLM 生成 override values YAML。
func generateValues(ctx context.Context, llmClient *llm.Client, query string, chartInfo *catalog.ChartInfo, defaultValues string) (string, error) {
	// 截断过长的 values
	truncated := helm.TruncateValues(defaultValues, maxDefaultValuesLines)

	extraNotes := ""
	if chartInfo.Notes != "" {
		extraNotes = "额外提示：" + chartInfo.Notes
	}
	if chartInfo.InstallCRDs {
		extraNotes += "\n注意：此 chart 需要安装 CRDs，已自动添加 installCRDs=true。"
	}

	prompt := fmt.Sprintf(valuesGenSystemPrompt,
		query,
		chartInfo.ChartName,
		chartInfo.Description,
		chartInfo.DefaultNamespace,
		extraNotes,
		truncated,
	)

	response, err := llmClient.Complete(ctx, prompt)
	if err != nil {
		return "", fmt.Errorf("LLM values 生成失败: %w", err)
	}

	// 清理 LLM 可能输出的 markdown 代码块标记
	result := strings.TrimSpace(response)
	result = strings.TrimPrefix(result, "```yaml")
	result = strings.TrimPrefix(result, "```")
	result = strings.TrimSuffix(result, "```")
	result = strings.TrimSpace(result)

	return result, nil
}

// regenerateValues 根据用户的自然语言修正指令重新生成 values。
func regenerateValues(ctx context.Context, llmClient *llm.Client, query string, chartInfo *catalog.ChartInfo, defaultValues, currentValues, correction string) (string, error) {
	prompt := fmt.Sprintf(`你是 Helm values 配置专家。

原始用户意图：%s
应用：%s
当前 override values：
---
%s
---

用户修正指令：%s

请根据修正指令更新 override values YAML。保持最小化原则，只包含需要修改的字段。
输出格式：纯 YAML，不要包含 markdown 代码块标记。`,
		query,
		chartInfo.ChartName,
		currentValues,
		correction,
	)

	response, err := llmClient.Complete(ctx, prompt)
	if err != nil {
		return "", fmt.Errorf("LLM values 重新生成失败: %w", err)
	}

	result := strings.TrimSpace(response)
	result = strings.TrimPrefix(result, "```yaml")
	result = strings.TrimPrefix(result, "```")
	result = strings.TrimSuffix(result, "```")
	return strings.TrimSpace(result), nil
}
```

**Step 2: 验证编译**

```bash
go build ./pkg/agent/deploy/...
```

**Step 3: 提交**

```bash
git add pkg/agent/deploy/values_gen.go
git commit -m "feat(deploy): add LLM values generation logic"
```

---

## Task 10: 创建 Deploy Agent 主体

**Files:**
- Create: `pkg/agent/deploy/agent.go`

**Step 1: 创建 `agent.go`**

```go
// pkg/agent/deploy/agent.go
package deploy

import (
	"context"
	"fmt"
	"time"

	"github.com/kubewise/kubewise/pkg/catalog"
	"github.com/kubewise/kubewise/pkg/helm"
	"github.com/kubewise/kubewise/pkg/llm"
	"github.com/kubewise/kubewise/pkg/tui/events"
	"github.com/kubewise/kubewise/pkg/types"
)

// DeployPlan 包含部署计划的所有信息，用于 TUI 展示和用户确认。
type DeployPlan struct {
	ChartInfo     *catalog.ChartInfo
	DefaultValues string // 完整的默认 values.yaml（含注释）
	CustomValues  string // LLM 生成的 override values
	ReleaseName   string
	Namespace     string
	IsUpgrade     bool // true 表示升级已有 release
}

// DeployDecision 表示用户在确认界面的决策。
type DeployDecision struct {
	Action     string // "execute" | "cancel"
	Values     string // 最终的 override values（可能被用户编辑过）
	Correction string // 如果用户使用了自然语言修正
}

// DeployConfirmationHandler 负责向用户展示部署计划并等待决策。
// 通过接口注入，便于 TUI 和测试场景替换实现。
type DeployConfirmationHandler interface {
	ConfirmDeploy(ctx context.Context, plan DeployPlan) (DeployDecision, error)
}

// ChartSelectionHandler 负责向用户展示 Chart 候选列表并等待选择。
type ChartSelectionHandler interface {
	SelectChart(ctx context.Context, appName string, candidates []catalog.ChartInfo) (*catalog.ChartInfo, error)
}

// Agent 是 Deploy Agent 的主体，实现六阶段部署流程。
type Agent struct {
	llmClient        *llm.Client
	helmClient       *helm.Client
	chartResolver    *catalog.ChainResolver
	confirmHandler   DeployConfirmationHandler
	selectionHandler ChartSelectionHandler
	eventCh          chan<- events.TUIEvent
	queryID          string
}

// Option 是 Agent 的函数式配置选项。
type Option func(*Agent)

// WithConfirmHandler 设置自定义确认处理器。
func WithConfirmHandler(h DeployConfirmationHandler) Option {
	return func(a *Agent) { a.confirmHandler = h }
}

// WithSelectionHandler 设置自定义 Chart 选择处理器。
func WithSelectionHandler(h ChartSelectionHandler) Option {
	return func(a *Agent) { a.selectionHandler = h }
}

// WithEventChannel 设置 TUI 事件通道。
func WithEventChannel(ch chan<- events.TUIEvent, queryID string) Option {
	return func(a *Agent) {
		a.eventCh = ch
		a.queryID = queryID
	}
}

// New 创建 Deploy Agent。
func New(llmClient *llm.Client, helmClient *helm.Client, chartResolver *catalog.ChainResolver, opts ...Option) *Agent {
	a := &Agent{
		llmClient:     llmClient,
		helmClient:    helmClient,
		chartResolver: chartResolver,
	}
	for _, opt := range opts {
		opt(a)
	}
	return a
}

// HandleQuery 实现六阶段部署流程。
func (a *Agent) HandleQuery(ctx context.Context, query string, entities types.Entities) (string, error) {
	// Phase 1: 提取应用名称
	appName := a.extractAppName(entities, query)
	if appName == "" {
		return "", fmt.Errorf("无法从查询中提取应用名称，请明确指定要部署的应用")
	}

	a.emit(events.PhaseEvent{QueryID: a.queryID, Phase: fmt.Sprintf("解析 Chart: %s", appName)})

	// Phase 2: 解析 Chart
	chartInfo, err := a.chartResolver.Resolve(ctx, appName)
	if err != nil {
		return "", fmt.Errorf("Chart 解析失败: %w", err)
	}

	if chartInfo == nil {
		// 内置目录和本地目录均未找到，尝试 ArtifactHub 交互选择
		chartInfo, err = a.handleChartNotFound(ctx, appName)
		if err != nil {
			return "", err
		}
		if chartInfo == nil {
			return "部署已取消", nil
		}
	}

	// Phase 2.5: 检查是否已部署
	existingRelease, _ := a.helmClient.Status(ctx, appName, chartInfo.DefaultNamespace)

	// Phase 3: 获取默认 values
	a.emit(events.PhaseEvent{QueryID: a.queryID, Phase: "获取 Chart 默认配置"})
	if err := a.helmClient.AddRepo(ctx, chartInfo.RepoName, chartInfo.RepoURL); err != nil {
		return "", fmt.Errorf("添加 Helm 仓库失败: %w", err)
	}
	defaultValues, err := a.helmClient.FetchDefaultValues(ctx, chartInfo.RepoName, chartInfo.ChartName)
	if err != nil {
		return "", fmt.Errorf("获取默认 values 失败: %w", err)
	}

	// Phase 4: LLM 生成 override values
	a.emit(events.PhaseEvent{QueryID: a.queryID, Phase: "生成配置建议"})
	customValues, err := generateValues(ctx, a.llmClient, query, chartInfo, defaultValues)
	if err != nil {
		return "", fmt.Errorf("生成 values 失败: %w", err)
	}

	// 如果需要安装 CRDs，自动添加
	if chartInfo.InstallCRDs {
		customValues = "installCRDs: true\n" + customValues
	}

	// Phase 5: 人工审查
	plan := DeployPlan{
		ChartInfo:     chartInfo,
		DefaultValues: defaultValues,
		CustomValues:  customValues,
		ReleaseName:   appName,
		Namespace:     chartInfo.DefaultNamespace,
		IsUpgrade:     existingRelease != nil,
	}

	decision, err := a.confirmDeploy(ctx, plan)
	if err != nil {
		return "", fmt.Errorf("确认部署失败: %w", err)
	}
	if decision.Action == "cancel" {
		return "部署已取消", nil
	}

	// 处理自然语言修正循环
	finalValues := decision.Values
	if decision.Correction != "" {
		finalValues, err = regenerateValues(ctx, a.llmClient, query, chartInfo, defaultValues, decision.Values, decision.Correction)
		if err != nil {
			return "", fmt.Errorf("重新生成 values 失败: %w", err)
		}
		// 再次确认
		plan.CustomValues = finalValues
		decision2, err := a.confirmDeploy(ctx, plan)
		if err != nil {
			return "", err
		}
		if decision2.Action == "cancel" {
			return "部署已取消", nil
		}
		finalValues = decision2.Values
	}

	// Phase 6: 执行 helm install/upgrade
	a.emit(events.PhaseEvent{QueryID: a.queryID, Phase: "执行部署"})
	rel, err := a.helmClient.InstallOrUpgrade(ctx, helm.InstallOptions{
		ReleaseName: appName,
		RepoName:    chartInfo.RepoName,
		ChartName:   chartInfo.ChartName,
		Namespace:   chartInfo.DefaultNamespace,
		Values:      finalValues,
		CreateNS:    true,
		Wait:        true,
		Timeout:     5 * time.Minute,
	})
	if err != nil {
		return "", fmt.Errorf("部署失败: %w", err)
	}

	return a.buildReport(rel, chartInfo), nil
}

// extractAppName 从 entities 或 query 中提取应用名称。
func (a *Agent) extractAppName(entities types.Entities, query string) string {
	if entities.AppName != "" {
		return entities.AppName
	}
	// 回退：使用 ResourceName
	if entities.ResourceName != "" {
		return entities.ResourceName
	}
	return ""
}

// handleChartNotFound 处理 Chart 未在目录中找到的情况。
// 触发 ArtifactHub 搜索 + TUI 交互选择。
func (a *Agent) handleChartNotFound(ctx context.Context, appName string) (*catalog.ChartInfo, error) {
	if a.selectionHandler == nil {
		return nil, fmt.Errorf("未找到应用 %q 的 Chart，请手动指定 repo URL 和 chart 名称", appName)
	}

	// 通过 ArtifactHub 搜索候选
	ahResolver := catalog.NewArtifactHubResolver(nil)
	candidates, err := ahResolver.SearchCandidates(ctx, appName)
	if err != nil || len(candidates) == 0 {
		candidates = nil // 搜索失败时显示手动输入界面
	}

	return a.selectionHandler.SelectChart(ctx, appName, candidates)
}

// confirmDeploy 调用确认处理器，如果未设置则返回默认执行决策。
func (a *Agent) confirmDeploy(ctx context.Context, plan DeployPlan) (DeployDecision, error) {
	if a.confirmHandler == nil {
		// 无 TUI 时自动执行（用于测试/CLI 模式）
		return DeployDecision{Action: "execute", Values: plan.CustomValues}, nil
	}
	return a.confirmHandler.ConfirmDeploy(ctx, plan)
}

// emit 向 TUI 事件通道发送事件（非阻塞）。
func (a *Agent) emit(e events.TUIEvent) {
	if a.eventCh == nil {
		return
	}
	select {
	case a.eventCh <- e:
	default:
	}
}

// buildReport 构建部署完成后的报告文本。
func (a *Agent) buildReport(rel *helm.Release, chartInfo *catalog.ChartInfo) string {
	if rel == nil {
		return fmt.Sprintf("✅ %s 部署完成", chartInfo.ChartName)
	}
	return fmt.Sprintf(`✅ 部署成功

Release:   %s
Namespace: %s
Chart:     %s
Status:    %s

提示：使用 kubectl get pods -n %s 查看 Pod 状态`,
		rel.Name,
		rel.Namespace,
		rel.Chart,
		rel.Status,
		rel.Namespace,
	)
}
```

**Step 2: 验证编译**

```bash
go build ./pkg/agent/deploy/...
```

**Step 3: 提交**

```bash
git add pkg/agent/deploy/agent.go
git commit -m "feat(deploy): add DeployAgent with 6-phase flow"
```

---

## Task 11: 创建 Chart 选择 TUI

**Files:**
- Create: `pkg/tui/model/chart_select.go`

**Step 1: 创建 `chart_select.go`**

此组件实现设计文档中的 Chart 选择界面，包含：
- 候选列表展示（repo/chart 名称 + stars + 描述）
- 数字键快速选择（`1`-`9`）
- 方向键导航 + Enter 确认
- 10 秒倒计时自动选择第一项
- `[0]` 触发手动输入
- `Esc` 取消

```go
// pkg/tui/model/chart_select.go
package model

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/kubewise/kubewise/pkg/catalog"
)

// ChartSelectedMsg 用户选择了 Chart 后发送的消息。
type ChartSelectedMsg struct {
	QueryID   string
	ChartInfo *catalog.ChartInfo // nil 表示取消；Source="manual" 表示手动输入
}

// chartSelectTickMsg 倒计时 tick 消息。
type chartSelectTickMsg struct{}

// ChartSelectModel 是 Chart 选择界面的 Bubble Tea 模型。
type ChartSelectModel struct {
	queryID    string
	appName    string
	candidates []catalog.ChartInfo
	cursor     int
	countdown  int  // 剩余秒数，0 表示已禁用
	active     bool // 是否正在显示
}

// NewChartSelectModel 创建 Chart 选择模型。
func NewChartSelectModel(queryID, appName string, candidates []catalog.ChartInfo) ChartSelectModel {
	return ChartSelectModel{
		queryID:    queryID,
		appName:    appName,
		candidates: candidates,
		cursor:     0,
		countdown:  10,
		active:     true,
	}
}

func (m ChartSelectModel) Init() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg {
		return chartSelectTickMsg{}
	})
}

func (m ChartSelectModel) Update(msg tea.Msg) (ChartSelectModel, tea.Cmd) {
	if !m.active {
		return m, nil
	}

	switch msg := msg.(type) {
	case chartSelectTickMsg:
		if m.countdown > 0 {
			m.countdown--
			if m.countdown == 0 {
				// 自动选择第一项
				m.active = false
				if len(m.candidates) > 0 {
					selected := m.candidates[0]
					return m, func() tea.Msg {
						return ChartSelectedMsg{QueryID: m.queryID, ChartInfo: &selected}
					}
				}
			}
			return m, tea.Tick(time.Second, func(t time.Time) tea.Msg {
				return chartSelectTickMsg{}
			})
		}

	case tea.KeyMsg:
		m.countdown = -1 // 任意按键取消倒计时
		switch msg.String() {
		case "esc":
			m.active = false
			return m, func() tea.Msg {
				return ChartSelectedMsg{QueryID: m.queryID, ChartInfo: nil}
			}
		case "0":
			m.active = false
			return m, func() tea.Msg {
				return ChartSelectedMsg{QueryID: m.queryID, ChartInfo: &catalog.ChartInfo{Source: "manual"}}
			}
		case "1", "2", "3", "4", "5", "6", "7", "8", "9":
			idx := int(msg.String()[0]-'1')
			if idx < len(m.candidates) {
				m.active = false
				selected := m.candidates[idx]
				return m, func() tea.Msg {
					return ChartSelectedMsg{QueryID: m.queryID, ChartInfo: &selected}
				}
			}
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.candidates)-1 {
				m.cursor++
			}
		case "enter":
			if len(m.candidates) > 0 {
				m.active = false
				selected := m.candidates[m.cursor]
				return m, func() tea.Msg {
					return ChartSelectedMsg{QueryID: m.queryID, ChartInfo: &selected}
				}
			}
		}
	}
	return m, nil
}

func (m ChartSelectModel) View() string {
	if !m.active {
		return ""
	}

	var sb strings.Builder

	// 标题
	title := fmt.Sprintf("找到 %d 个 Helm Chart，请选择", len(m.candidates))
	if m.countdown > 0 {
		title += fmt.Sprintf("（%d 秒后自动选择 #1）", m.countdown)
	}
	sb.WriteString(lipgloss.NewStyle().Bold(true).Render(title) + "\n\n")

	// 候选列表
	for i, c := range m.candidates {
		cursor := "  "
		if i == m.cursor {
			cursor = "> "
		}
		stars := ""
		if c.Stars > 0 {
			stars = fmt.Sprintf("⭐ %d", c.Stars)
		}
		line := fmt.Sprintf("%s[%d] %s/%s  %s\n    %s\n    %s\n",
			cursor, i+1,
			c.RepoName, c.ChartName,
			stars,
			c.Description,
			c.RepoURL,
		)
		if i == m.cursor {
			sb.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("12")).Render(line))
		} else {
			sb.WriteString(line)
		}
		sb.WriteString("\n")
	}

	// 手动输入选项
	sb.WriteString("  [0] 手动指定 repo URL 和 chart 名称\n\n")
	sb.WriteString("↑↓ 选择  Enter/数字键 确认  Esc 取消\n")

	return sb.String()
}
```

**Step 2: 验证编译**

```bash
go build ./pkg/tui/model/...
```

**Step 3: 提交**

```bash
git add pkg/tui/model/chart_select.go
git commit -m "feat(tui): add ChartSelectModel with countdown and quick-select"
```

---

## Task 12: 创建手动 Chart 输入 TUI

**Files:**
- Create: `pkg/tui/model/manual_chart_input.go`

**Step 1: 创建 `manual_chart_input.go`**

此组件实现两个输入框（Repo URL + Chart 名称），Tab 切换焦点，Enter 确认，Esc 取消。

```go
// pkg/tui/model/manual_chart_input.go
package model

import (
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/kubewise/kubewise/pkg/catalog"
)

// ManualChartInputDoneMsg 用户完成手动输入后发送的消息。
type ManualChartInputDoneMsg struct {
	QueryID   string
	ChartInfo *catalog.ChartInfo // nil 表示取消
	Error     string             // 验证错误信息
}

// ManualChartInputModel 是手动 Chart 输入界面的 Bubble Tea 模型。
type ManualChartInputModel struct {
	queryID    string
	repoInput  textinput.Model
	chartInput textinput.Model
	focusIdx   int // 0=repoURL, 1=chartName
	active     bool
	errMsg     string
}

// NewManualChartInputModel 创建手动输入模型。
func NewManualChartInputModel(queryID string) ManualChartInputModel {
	repoInput := textinput.New()
	repoInput.Placeholder = "https://argoproj.github.io/argo-helm"
	repoInput.Focus()
	repoInput.Width = 60

	chartInput := textinput.New()
	chartInput.Placeholder = "argo-cd"
	chartInput.Width = 60

	return ManualChartInputModel{
		queryID:    queryID,
		repoInput:  repoInput,
		chartInput: chartInput,
		focusIdx:   0,
		active:     true,
	}
}

func (m ManualChartInputModel) Init() tea.Cmd {
	return textinput.Blink
}

func (m ManualChartInputModel) Update(msg tea.Msg) (ManualChartInputModel, tea.Cmd) {
	if !m.active {
		return m, nil
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			m.active = false
			return m, func() tea.Msg {
				return ManualChartInputDoneMsg{QueryID: m.queryID, ChartInfo: nil}
			}
		case "tab", "shift+tab":
			m.focusIdx = 1 - m.focusIdx
			if m.focusIdx == 0 {
				m.repoInput.Focus()
				m.chartInput.Blur()
			} else {
				m.chartInput.Focus()
				m.repoInput.Blur()
			}
		case "enter":
			repoURL := strings.TrimSpace(m.repoInput.Value())
			chartName := strings.TrimSpace(m.chartInput.Value())
			if repoURL == "" || chartName == "" {
				m.errMsg = "Repo URL 和 Chart 名称不能为空"
				return m, nil
			}
			m.active = false
			// 从 URL 中提取 repo 名称（取最后一段路径）
			parts := strings.Split(strings.TrimRight(repoURL, "/"), "/")
			repoName := parts[len(parts)-1]
			return m, func() tea.Msg {
				return ManualChartInputDoneMsg{
					QueryID: m.queryID,
					ChartInfo: &catalog.ChartInfo{
						RepoName:  repoName,
						RepoURL:   repoURL,
						ChartName: chartName,
						Source:    "manual",
					},
				}
			}
		}
	}

	var cmd tea.Cmd
	if m.focusIdx == 0 {
		m.repoInput, cmd = m.repoInput.Update(msg)
	} else {
		m.chartInput, cmd = m.chartInput.Update(msg)
	}
	return m, cmd
}

func (m ManualChartInputModel) View() string {
	if !m.active {
		return ""
	}

	var sb strings.Builder
	sb.WriteString(lipgloss.NewStyle().Bold(true).Render("手动指定 Helm Chart") + "\n\n")
	sb.WriteString("Repo URL:\n")
	sb.WriteString(m.repoInput.View() + "\n\n")
	sb.WriteString("Chart 名称:\n")
	sb.WriteString(m.chartInput.View() + "\n\n")
	sb.WriteString("💡 提示：可在 https://artifacthub.io 搜索 chart 的 repo URL\n\n")
	if m.errMsg != "" {
		sb.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Render("❌ "+m.errMsg) + "\n\n")
	}
	sb.WriteString("Tab 切换字段  Enter 确认  Esc 取消\n")
	return sb.String()
}
```

**Step 2: 验证编译**

```bash
go build ./pkg/tui/model/...
```

**Step 3: 提交**

```bash
git add pkg/tui/model/manual_chart_input.go
git commit -m "feat(tui): add ManualChartInputModel for manual repo/chart entry"
```

---

## Task 13: 创建 Deploy 确认 TUI

**Files:**
- Create: `pkg/tui/model/deploy_confirm.go`

**Step 1: 创建 `deploy_confirm.go`**

此组件实现设计文档中的 Values diff 确认界面，包含：
- 左右双面板（默认 values vs override values）
- `Y` 执行 / `E` 编辑 YAML / `C` 自然语言修正 / `V` 完整预览 / `N` 取消
- YAML 编辑模式（右面板变为可编辑 textarea，Ctrl+S 保存，Esc 放弃）
- 自然语言修正模式（底部输入框，Enter 提交，Esc 取消）

```go
// pkg/tui/model/deploy_confirm.go
package model

import (
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/kubewise/kubewise/pkg/agent/deploy"
	"github.com/kubewise/kubewise/pkg/catalog"
	"github.com/kubewise/kubewise/pkg/helm"
)

// confirmMode 表示确认界面的当前交互模式。
type confirmMode int

const (
	confirmModeView      confirmMode = iota // 初始查看模式
	confirmModeEditYAML                     // YAML 编辑模式
	confirmModeEditNL                       // 自然语言修正模式
	confirmModeFullPreview                  // 完整 values 预览模式
)

// DeployConfirmDoneMsg 用户完成确认后发送的消息。
type DeployConfirmDoneMsg struct {
	QueryID  string
	Decision deploy.DeployDecision
}

// DeployConfirmModel 是 Deploy 确认界面的 Bubble Tea 模型。
type DeployConfirmModel struct {
	queryID       string
	plan          deploy.DeployPlan
	mode          confirmMode
	active        bool
	// 左面板：默认 values 滚动视图
	defaultVP     viewport.Model
	// 右面板：override values（查看模式）
	overrideVP    viewport.Model
	// YAML 编辑模式
	yamlEditor    textarea.Model
	yamlEditErr   string
	// 自然语言修正模式
	nlInput       textinput.Model
	// 完整预览模式
	fullPreviewVP viewport.Model
	width         int
	height        int
}

// NewDeployConfirmModel 创建 Deploy 确认模型。
func NewDeployConfirmModel(queryID string, plan deploy.DeployPlan) DeployConfirmModel {
	defaultVP := viewport.New(40, 20)
	defaultVP.SetContent(plan.DefaultValues)

	overrideVP := viewport.New(40, 20)
	overrideVP.SetContent(plan.CustomValues)

	yamlEditor := textarea.New()
	yamlEditor.SetValue(plan.CustomValues)
	yamlEditor.SetWidth(40)
	yamlEditor.SetHeight(20)

	nlInput := textinput.New()
	nlInput.Placeholder = "例如：把 NodePort 改成 30090，副本数改成 3"
	nlInput.Width = 60

	// 预计算完整合并 values
	merged, _ := helm.MergeValues(plan.DefaultValues, plan.CustomValues)
	fullPreviewVP := viewport.New(80, 30)
	fullPreviewVP.SetContent(merged)

	return DeployConfirmModel{
		queryID:       queryID,
		plan:          plan,
		mode:          confirmModeView,
		active:        true,
		defaultVP:     defaultVP,
		overrideVP:    overrideVP,
		yamlEditor:    yamlEditor,
		fullPreviewVP: fullPreviewVP,
		nlInput:       nlInput,
	}
}

func (m DeployConfirmModel) Init() tea.Cmd {
	return nil
}

func (m DeployConfirmModel) Update(msg tea.Msg) (DeployConfirmModel, tea.Cmd) {
	if !m.active {
		return m, nil
	}

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		panelW := (msg.Width - 4) / 2
		panelH := msg.Height - 10
		m.defaultVP.Width = panelW
		m.defaultVP.Height = panelH
		m.overrideVP.Width = panelW
		m.overrideVP.Height = panelH
		m.yamlEditor.SetWidth(panelW)
		m.yamlEditor.SetHeight(panelH)
		m.fullPreviewVP.Width = msg.Width - 4
		m.fullPreviewVP.Height = msg.Height - 6

	case tea.KeyMsg:
		switch m.mode {
		case confirmModeView:
			return m.handleViewMode(msg)
		case confirmModeEditYAML:
			return m.handleEditYAMLMode(msg)
		case confirmModeEditNL:
			return m.handleEditNLMode(msg)
		case confirmModeFullPreview:
			if msg.String() == "esc" {
				m.mode = confirmModeView
			} else {
				var cmd tea.Cmd
				m.fullPreviewVP, cmd = m.fullPreviewVP.Update(msg)
				return m, cmd
			}
		}
	}

	// 转发滚动事件到当前活跃面板
	var cmd tea.Cmd
	if m.mode == confirmModeView {
		m.defaultVP, cmd = m.defaultVP.Update(msg)
	}
	return m, cmd
}

func (m DeployConfirmModel) handleViewMode(msg tea.KeyMsg) (DeployConfirmModel, tea.Cmd) {
	switch strings.ToLower(msg.String()) {
	case "y":
		m.active = false
		values := m.plan.CustomValues
		return m, func() tea.Msg {
			return DeployConfirmDoneMsg{
				QueryID:  m.queryID,
				Decision: deploy.DeployDecision{Action: "execute", Values: values},
			}
		}
	case "n", "esc":
		m.active = false
		return m, func() tea.Msg {
			return DeployConfirmDoneMsg{
				QueryID:  m.queryID,
				Decision: deploy.DeployDecision{Action: "cancel"},
			}
		}
	case "e":
		m.mode = confirmModeEditYAML
		m.yamlEditor.SetValue(m.plan.CustomValues)
		m.yamlEditor.Focus()
		m.yamlEditErr = ""
	case "c":
		m.mode = confirmModeEditNL
		m.nlInput.SetValue("")
		m.nlInput.Focus()
	case "v":
		merged, _ := helm.MergeValues(m.plan.DefaultValues, m.plan.CustomValues)
		m.fullPreviewVP.SetContent(merged)
		m.mode = confirmModeFullPreview
	}
	return m, nil
}

func (m DeployConfirmModel) handleEditYAMLMode(msg tea.KeyMsg) (DeployConfirmModel, tea.Cmd) {
	switch msg.String() {
	case "ctrl+s":
		newValues := m.yamlEditor.Value()
		if err := helm.ValidateYAML(newValues); err != nil {
			m.yamlEditErr = "YAML 语法错误: " + err.Error()
			return m, nil
		}
		m.plan.CustomValues = newValues
		m.overrideVP.SetContent(newValues)
		m.mode = confirmModeView
		m.yamlEditErr = ""
	case "esc":
		m.mode = confirmModeView
		m.yamlEditErr = ""
	default:
		var cmd tea.Cmd
		m.yamlEditor, cmd = m.yamlEditor.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m DeployConfirmModel) handleEditNLMode(msg tea.KeyMsg) (DeployConfirmModel, tea.Cmd) {
	switch msg.String() {
	case "enter":
		correction := strings.TrimSpace(m.nlInput.Value())
		if correction == "" {
			m.mode = confirmModeView
			return m, nil
		}
		m.active = false
		values := m.plan.CustomValues
		return m, func() tea.Msg {
			return DeployConfirmDoneMsg{
				QueryID: m.queryID,
				Decision: deploy.DeployDecision{
					Action:     "execute",
					Values:     values,
					Correction: correction,
				},
			}
		}
	case "esc":
		m.mode = confirmModeView
	default:
		var cmd tea.Cmd
		m.nlInput, cmd = m.nlInput.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m DeployConfirmModel) View() string {
	if !m.active {
		return ""
	}

	var sb strings.Builder

	// 标题栏
	chartInfo := m.plan.ChartInfo
	action := "安装"
	if m.plan.IsUpgrade {
		action = "升级"
	}
	header := lipgloss.NewStyle().Bold(true).Render(
		fmt.Sprintf("Helm Deploy 确认 [%s]  Chart: %s/%s  来源: %s  Release: %s → Namespace: %s",
			action,
			chartInfo.RepoName, chartInfo.ChartName,
			chartInfo.Source,
			m.plan.ReleaseName, m.plan.Namespace,
		),
	)
	sb.WriteString(header + "\n\n")

	switch m.mode {
	case confirmModeView:
		// 左右双面板
		leftPanel := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			Padding(0, 1).
			Render("默认 Values (参考)\n" + m.defaultVP.View())
		rightPanel := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			Padding(0, 1).
			Render("Override Values (Agent 生成)\n" + m.overrideVP.View())
		sb.WriteString(lipgloss.JoinHorizontal(lipgloss.Top, leftPanel, "  ", rightPanel))
		sb.WriteString("\n\n[Y] 执行  [E] 编辑 YAML  [C] 自然语言修正  [V] 完整预览  [N] 取消\n")

	case confirmModeEditYAML:
		sb.WriteString(lipgloss.NewStyle().Bold(true).Render("编辑 Override Values (Ctrl+S 保存, Esc 放弃)") + "\n")
		sb.WriteString(m.yamlEditor.View() + "\n")
		if m.yamlEditErr != "" {
			sb.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Render("❌ "+m.yamlEditErr) + "\n")
		}

	case confirmModeEditNL:
		sb.WriteString(lipgloss.NewStyle().Bold(true).Render("自然语言修正 (Enter 提交, Esc 取消)") + "\n")
		sb.WriteString(m.nlInput.View() + "\n")

	case confirmModeFullPreview:
		sb.WriteString(lipgloss.NewStyle().Bold(true).Render("完整合并 Values 预览 (Esc 返回)") + "\n")
		sb.WriteString(m.fullPreviewVP.View() + "\n")
	}

	return sb.String()
}
```

**Step 2: 验证编译**

```bash
go build ./pkg/tui/model/...
```

**Step 3: 提交**

```bash
git add pkg/tui/model/deploy_confirm.go
git commit -m "feat(tui): add DeployConfirmModel with YAML edit and NL correction modes"
```

---

## Task 14: 集成 Router Agent

**Files:**
- Modify: `pkg/agent/router/agent.go`

**Step 1: 在 Router Agent 中注册 Deploy Agent**

在 [`pkg/agent/router/agent.go`](pkg/agent/router/agent.go) 中：

1. 在 `Agent` 结构体中添加 `deployAgent *deploy.Agent` 字段
2. 在 `New()` 函数中初始化 Deploy Agent：

```go
import (
    // ... 现有 import
    "github.com/kubewise/kubewise/pkg/agent/deploy"
    "github.com/kubewise/kubewise/pkg/catalog"
    "github.com/kubewise/kubewise/pkg/helm"
)

// 在 New() 中添加：
helmClient := helm.New("")
chartResolver := catalog.NewDefaultChainResolver(nil)
deployAgent := deploy.New(llmClient, helmClient, chartResolver)
```

3. 在 `HandleQueryStream()` 的 switch 语句中添加 `deploy` 分支：

```go
case types.TaskTypeDeploy:
    // 创建带事件通道的 Deploy Agent 实例
    deployAgentWithEvents := deploy.New(
        a.llmClient,
        a.helmClient,
        a.chartResolver,
        deploy.WithEventChannel(eventCh, queryID),
        deploy.WithConfirmHandler(newTUIConfirmHandler(eventCh, queryID)),
        deploy.WithSelectionHandler(newTUISelectionHandler(eventCh, queryID)),
    )
    result, err = deployAgentWithEvents.HandleQuery(ctx, userQuery, intent.Entities)
```

**Step 2: 更新 Router 的 system prompt 中的任务类型描述**

在 `classifyIntent()` 使用的 system prompt 中添加 `deploy` 任务类型说明：

```
- deploy: 用户想要部署、安装、升级或卸载一个完整应用（如 ArgoCD、Prometheus、Nginx Ingress 等）。
  与 operation 的区别：deploy 用于安装完整应用，operation 用于对已有资源的原子操作（扩缩容、重启、删除等）。
```

**Step 3: 验证编译**

```bash
go build ./pkg/agent/router/...
```

**Step 4: 提交**

```bash
git add pkg/agent/router/agent.go
git commit -m "feat(router): register DeployAgent and add deploy task type routing"
```

---

## Task 15: 集成 TUI App

**Files:**
- Modify: `pkg/tui/app.go`

**Step 1: 在 TUI App 中处理 Deploy 事件**

在 [`pkg/tui/app.go`](pkg/tui/app.go) 中：

1. 在 App 模型中添加 Deploy TUI 组件字段：

```go
chartSelectModel    *model.ChartSelectModel
manualInputModel    *model.ManualChartInputModel
deployConfirmModel  *model.DeployConfirmModel
```

2. 在 `Update()` 方法中处理新的事件类型：

```go
case events.ChartSelectRequestEvent:
    m := model.NewChartSelectModel(msg.QueryID, msg.AppName, msg.Candidates)
    app.chartSelectModel = &m
    return app, m.Init()

case model.ChartSelectedMsg:
    // 将用户选择通过 channel 传回 Deploy Agent
    if app.chartSelectResponseCh != nil {
        app.chartSelectResponseCh <- msg
    }
    app.chartSelectModel = nil

case events.DeployConfirmRequestEvent:
    m := model.NewDeployConfirmModel(msg.QueryID, msg.Plan)
    app.deployConfirmModel = &m
    return app, m.Init()

case model.DeployConfirmDoneMsg:
    // 将用户决策通过 channel 传回 Deploy Agent
    if app.deployConfirmResponseCh != nil {
        app.deployConfirmResponseCh <- msg
    }
    app.deployConfirmModel = nil
```

3. 在 `View()` 方法中渲染 Deploy 组件（当 active 时覆盖主界面）：

```go
if app.chartSelectModel != nil {
    return app.chartSelectModel.View()
}
if app.manualInputModel != nil {
    return app.manualInputModel.View()
}
if app.deployConfirmModel != nil {
    return app.deployConfirmModel.View()
}
```

**Step 2: 验证编译**

```bash
go build ./pkg/tui/...
```

**Step 3: 提交**

```bash
git add pkg/tui/app.go
git commit -m "feat(tui): handle Deploy TUI events in App model"
```

---

## Task 16: 更新配置文件

**Files:**
- Modify: `config.yaml`
- Modify: `pkg/config/config.go`

**Step 1: 在 `config.yaml` 中添加 deploy 配置节**

```yaml
# 在 config.yaml 末尾添加：
deploy:
  artifact_hub:
    enabled: true
    timeout: 5s
    selection_timeout: 10s
  helm:
    wait_timeout: 5m
```

**Step 2: 在 `pkg/config/config.go` 中添加对应的 Go 结构体**

```go
// DeployConfig 部署 Agent 配置
type DeployConfig struct {
	ArtifactHub ArtifactHubConfig `mapstructure:"artifact_hub"`
	Helm        HelmDeployConfig  `mapstructure:"helm"`
}

// ArtifactHubConfig Artifact Hub 搜索配置
type ArtifactHubConfig struct {
	Enabled          bool          `mapstructure:"enabled"`
	Timeout          time.Duration `mapstructure:"timeout"`
	SelectionTimeout time.Duration `mapstructure:"selection_timeout"`
}

// HelmDeployConfig Helm 部署配置
type HelmDeployConfig struct {
	WaitTimeout time.Duration `mapstructure:"wait_timeout"`
}
```

在顶层 `Config` 结构体中添加 `Deploy DeployConfig` 字段。

**Step 3: 验证编译**

```bash
go build ./...
```

**Step 4: 提交**

```bash
git add config.yaml pkg/config/config.go
git commit -m "feat(config): add deploy agent configuration section"
```

---

## Task 17: 端到端验证

**Step 1: 完整编译验证**

```bash
go build ./...
```

预期：无编译错误。

**Step 2: 手动集成测试**

启动 KubeWise 并输入以下查询，验证六阶段流程：

```
帮我部署一个 argocd
```

预期行为：
1. Router 将请求路由到 Deploy Agent（`TaskTypeDeploy`）
2. Phase 2：内置目录命中 `argocd`，返回 `argo/argo-cd`
3. Phase 3：显示 "获取 Chart 默认配置" 进度提示，执行 `helm repo add argo` + `helm show values`
4. Phase 4：显示 "生成配置建议" 进度提示，LLM 生成 override values
5. Phase 5：TUI 显示左右双面板确认界面
6. 用户按 `Y`：执行 `helm install argocd argo/argo-cd -n argocd --create-namespace --wait`
7. 显示部署成功报告

**Step 3: 验证 ArtifactHub 回退**

输入一个内置目录中不存在的应用名：

```
帮我部署一个 keda
```

预期行为：
1. 内置目录未命中
2. 本地目录未命中
3. ArtifactHub 搜索返回候选列表
4. TUI 显示 Chart 选择界面（含 10 秒倒计时）

**Step 4: 验证 YAML 编辑模式**

在确认界面按 `E`，修改 override values，按 `Ctrl+S` 保存，验证修改生效。

**Step 5: 验证自然语言修正**

在确认界面按 `C`，输入 "把 service type 改成 NodePort"，按 `Enter`，验证 LLM 重新生成 values。

**Step 6: 提交**

```bash
git add -A
git commit -m "feat(deploy): complete Deploy Agent integration - end-to-end verified"
```

---

## 实现顺序总结

| Task | 内容 | 依赖 |
|------|------|------|
| 1 | 添加 `TaskTypeDeploy` | 无 |
| 2 | Catalog 基础结构（接口 + ChainResolver） | 无 |
| 3 | 内置 Catalog（YAML + BuiltinResolver） | Task 2 |
| 4 | 本地 Catalog Resolver | Task 3 |
| 5 | ArtifactHub Resolver | Task 2 |
| 6 | 添加 Helm SDK 依赖 | 无 |
| 7 | Helm Client + Values 工具 | Task 6 |
| 8 | Deploy TUI 事件类型 | Task 2 |
| 9 | LLM Values 生成逻辑 | Task 7 |
| 10 | Deploy Agent 主体 | Task 2, 7, 9 |
| 11 | Chart 选择 TUI | Task 2, 8 |
| 12 | 手动 Chart 输入 TUI | Task 2 |
| 13 | Deploy 确认 TUI | Task 7, 10 |
| 14 | Router Agent 集成 | Task 1, 10 |
| 15 | TUI App 集成 | Task 11, 12, 13 |
| 16 | 配置文件更新 | 无 |
| 17 | 端到端验证 | 全部 |
