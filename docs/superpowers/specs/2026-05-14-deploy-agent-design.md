# Deploy Agent Design

**Date:** 2026-05-14  
**Status:** Draft  
**Author:** Brainstorming Session

---

## Overview

Implement a new `Deploy Agent` in KubeWise that handles application deployment via Helm charts. This replaces the current unreliable approach where the Operation Agent's `apply` tool relies on LLM-generated raw Kubernetes YAML manifests — which suffers from severe hallucination issues when deploying complex applications (e.g., ArgoCD requires CRDs, RBAC, Deployments, Services, ConfigMaps, etc.).

**Core principle:** Let the LLM do what it's good at (understanding intent, generating configuration values), and let Helm do what it's good at (templating, orchestration, dependency management).

### Problem Statement

Current state when a user says "帮我部署一个 ArgoCD":

1. Operation Agent's planning phase takes 3+ minutes of invisible thinking
2. LLM generates raw K8s YAML manifests (high hallucination rate for complex apps)
3. Confirmation UI shows only a description, not the actual content being applied
4. Steps are applied one-by-one with no orchestration
5. Failures are not recovered from
6. End result: namespace created but nothing actually deployed

### Design Goals

1. **Reliable deployment** via Helm charts instead of LLM-generated YAML
2. **Transparent chart discovery** via built-in catalog + Artifact Hub fallback
3. **Human-reviewable** values diff before execution
4. **Minimal LLM responsibility** — only generates values override, not full manifests
5. **Clean architecture** — independent Deploy Agent, no pollution of existing Operation Agent

---

## Architecture

### Deploy Agent as a New Agent

The Deploy Agent is a **peer** to the existing four agents, not a sub-type of Operation Agent. The two have fundamentally different workflows:

| Dimension | Operation Agent | Deploy Agent |
|-----------|----------------|--------------|
| Planning | LLM + read tools → multi-step plan | Chart resolution + LLM values generation |
| Confirmation | Per-step sequential | Single values diff review |
| Execution | Multiple tool calls in sequence | Single `helm install/upgrade` |
| Scope | Atomic mutations on existing resources | Full application lifecycle |

```
Router Agent
  ├─ Query Agent          (query)
  ├─ Operation Agent      (operation) — scale/restart/delete/apply/cordon_drain/label_annotate
  ├─ Deploy Agent         (deploy)    — NEW: helm-based application deployment
  ├─ Troubleshooting Agent (troubleshooting)
  └─ Security Agent       (security)
```

### Router Integration

Router Agent's system prompt adds a new task type:

```
- deploy: 用户想要部署、安装、升级或卸载一个完整应用（如 ArgoCD、Prometheus、Nginx Ingress 等）。
  与 operation 的区别：deploy 用于安装完整应用，operation 用于对已有资源的原子操作（扩缩容、重启、删除等）。
```

### New Files

```
pkg/
├── agent/
│   └── deploy/
│       ├── agent.go              # DeployAgent: main 6-phase flow
│       └── values_gen.go         # LLM values generation logic
├── helm/
│   ├── client.go                 # Helm SDK wrapper (install/upgrade/uninstall/status)
│   ├── repo.go                   # Helm repo add/update operations
│   └── values.go                 # Values parsing, merging, diff utilities
├── catalog/
│   ├── resolver.go               # ChartResolver interface + ChainResolver
│   ├── builtin.go                # Built-in catalog (embedded in binary)
│   ├── builtin_data.yaml         # Embedded catalog data
│   ├── local.go                  # User-local catalog (~/.kubewise/catalog.yaml)
│   └── artifacthub.go            # Artifact Hub REST API client
├── tools/v1/deploy/
│   └── helm_deploy.go            # "helm_deploy" tool registration
└── tui/
    └── model/
        ├── chart_select.go       # Chart selection UI (Artifact Hub results)
        ├── deploy_confirm.go     # Values diff confirmation UI
        └── manual_chart_input.go # Manual repo URL + chart name input
```

### Integration Points

- `pkg/agent/router/agent.go`: add `deploy` task type routing to `deployAgent.HandleQuery()`
- `pkg/types/types.go`: add `TaskTypeDeploy = "deploy"`
- `pkg/tui/app.go`: handle new TUI events for chart selection and deploy confirmation
- `go.mod`: add `helm.sh/helm/v3` dependency

---

## Six-Phase Flow

```
User Input: "帮我部署一个 argocd"
    │
    ▼
[Phase 1: Intent Parsing]
    │  Router identifies deploy intent, extracts app name "argocd"
    │
    ▼
[Phase 2: Chart Resolution]
    │  ChainResolver: Catalog → Local → ArtifactHub
    │  If ArtifactHub: interactive chart selection UI
    │  If all fail: manual input UI
    │
    ▼
[Phase 3: Default Values Fetch]
    │  helm repo add + helm show values → full default values.yaml
    │
    ▼
[Phase 4: Values Generation]
    │  LLM receives: user query + default values (with comments)
    │  LLM outputs: minimal override values YAML
    │
    ▼
[Phase 5: Human Review]
    │  TUI: left panel (default values) | right panel (override values)
    │  User: [Y] execute / [E] edit YAML / [C] natural language correction / [N] cancel
    │
    ▼
[Phase 6: Helm Install + Verification]
    │  helm install/upgrade → wait for ready → report access info
```

---

## Chart Resolution

### ChartResolver Interface

```go
// pkg/catalog/resolver.go

// ChartInfo is the resolved chart metadata.
type ChartInfo struct {
    RepoName         string // "argo" — used for helm repo add
    RepoURL          string // "https://argoproj.github.io/argo-helm"
    ChartName        string // "argo-cd"
    DefaultNamespace string // "argocd"
    InstallCRDs      bool   // whether to set --set installCRDs=true
    Notes            string // extra hints for LLM values generation
    // Populated by ArtifactHub resolver
    Stars            int    // popularity indicator
    Description      string // one-line description
    LatestVersion    string // latest chart version
    Source           string // "catalog" | "local" | "artifacthub" | "manual"
}

// ChartResolver resolves an application name to chart metadata.
type ChartResolver interface {
    // Resolve attempts to resolve appName to ChartInfo.
    // Returns (nil, nil) if this resolver cannot handle it (pass to next).
    // Returns (nil, err) on hard errors (abort chain).
    Resolve(ctx context.Context, appName string) (*ChartInfo, error)
}

// ChainResolver calls resolvers in priority order.
type ChainResolver struct {
    resolvers []ChartResolver
}

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
    return nil, nil // all resolvers returned nil
}
```

### Resolver Chain

```go
func NewDefaultChainResolver(httpClient *http.Client) *ChainResolver {
    return &ChainResolver{
        resolvers: []ChartResolver{
            NewBuiltinCatalogResolver(),        // 1. Embedded catalog (fastest, most reliable)
            NewLocalCatalogResolver(),          // 2. User-local ~/.kubewise/catalog.yaml
            NewArtifactHubResolver(httpClient), // 3. Artifact Hub API (network-dependent)
        },
    }
}
```

### Built-in Catalog

Embedded via `//go:embed` into the binary:

```yaml
# pkg/catalog/builtin_data.yaml
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

### Builtin Catalog Resolver

```go
// pkg/catalog/builtin.go

import _ "embed"

//go:embed builtin_data.yaml
var builtinCatalogData []byte

type BuiltinCatalogResolver struct {
    apps map[string]*ChartInfo // alias → ChartInfo
}

func NewBuiltinCatalogResolver() *BuiltinCatalogResolver {
    // Parse builtinCatalogData, build alias→ChartInfo lookup map
    // Normalize aliases to lowercase for fuzzy matching
}

func (r *BuiltinCatalogResolver) Resolve(ctx context.Context, appName string) (*ChartInfo, error) {
    normalized := strings.ToLower(strings.TrimSpace(appName))
    if info, ok := r.apps[normalized]; ok {
        info.Source = "catalog"
        return info, nil
    }
    return nil, nil
}
```

### Artifact Hub Resolver

```go
// pkg/catalog/artifacthub.go

const artifactHubBaseURL = "https://artifacthub.io/api/v1"

type ArtifactHubResolver struct {
    httpClient *http.Client
    timeout    time.Duration // default 5s
}

type artifactHubSearchResult struct {
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

// Resolve searches Artifact Hub and returns candidates.
// Unlike other resolvers, this one may return multiple candidates
// that need user selection. It returns the top candidate by stars,
// but also populates a Candidates field for the UI to display.
func (r *ArtifactHubResolver) Resolve(ctx context.Context, appName string) (*ChartInfo, error) {
    ctx, cancel := context.WithTimeout(ctx, r.timeout)
    defer cancel()

    results, err := r.search(ctx, appName)
    if err != nil {
        // Network error: return nil (not a hard error, let chain continue)
        return nil, nil
    }
    if len(results) == 0 {
        return nil, nil
    }

    // Sort by stars descending
    sort.Slice(results, func(i, j int) bool {
        return results[i].Stars > results[j].Stars
    })

    // Return top result, but attach all candidates for UI selection
    top := results[0]
    info := &ChartInfo{
        RepoName:      top.Repository.Name,
        RepoURL:       top.Repository.URL,
        ChartName:     top.Name,
        Stars:         top.Stars,
        Description:   top.Description,
        LatestVersion: top.Version,
        Source:         "artifacthub",
    }
    return info, nil
}

// SearchCandidates returns all matching charts for UI display.
// Called separately from Resolve when interactive selection is needed.
func (r *ArtifactHubResolver) SearchCandidates(ctx context.Context, appName string) ([]ChartInfo, error) {
    // Returns up to 10 candidates sorted by stars
}

func (r *ArtifactHubResolver) search(ctx context.Context, query string) ([]artifactHubSearchResult, error) {
    url := fmt.Sprintf("%s/packages/search?kind=0&ts_query_web=%s&limit=10",
        artifactHubBaseURL, url.QueryEscape(query))
    // HTTP GET with timeout
}
```

---

## Artifact Hub Interactive Selection UI

When the built-in catalog does not match, the TUI displays an interactive chart selection list.

### TUI Component: ChartSelectModel

```
┌─────────────────────────────────────────────────────────────────┐
│ 找到 3 个 Helm Chart，请选择（10 秒后自动选择 #1）:              │
├─────────────────────────────────────────────────────────────────┤
│ > [1] argo/argo-cd                                    ⭐ 1,200  │
│       GitOps continuous delivery tool for Kubernetes            │
│       https://argoproj.github.io/argo-helm                     │
│                                                                 │
│   [2] bitnami/argo-cd                        [已部署 v5.40.0]  ⭐ 300  │
│       Argo CD packaged by Bitnami                               │
│       https://charts.bitnami.com/bitnami                        │
│                                                                 │
│   [3] community/argocd-lite                           ⭐ 15     │
│       Lightweight Argo CD deployment                            │
│       https://community-charts.example.com                      │
│                                                                 │
│   [0] 手动指定 repo URL 和 chart 名称                            │
├─────────────────────────────────────────────────────────────────┤
│ ↑↓ 选择  Enter/数字键 确认  Esc 取消                             │
└─────────────────────────────────────────────────────────────────┘
```

### Features

| Feature | Description |
|---------|-------------|
| **Auto-timeout** | Configurable countdown (default 10s). Auto-selects #1 when expired. Any keypress cancels countdown. |
| **Deployed marker** | Queries `helm list -A` to check if chart is already installed. Shows `[已部署 vX.Y.Z]` with version. |
| **Smart deployed handling** | If selected chart is already deployed at latest version, shows upgrade/reconfigure prompt instead of install. |
| **Quick select** | Number keys `1`-`9` for instant selection. `0` for manual input. |
| **Arrow navigation** | `↑`/`↓` to move cursor, `Enter` to confirm. |

### Already-Deployed Smart Handling

When user selects a chart that is already deployed at the latest version:

```
┌─────────────────────────────────────────────────────────────────┐
│ ℹ️  argocd 已部署且是最新版本 (v5.46.0)                          │
│                                                                 │
│ Release: argocd  Namespace: argocd  状态: deployed ✅           │
│                                                                 │
│   [1] 重新配置（修改 values 并 upgrade）                         │
│   [2] 查看当前配置（helm get values）                            │
│   [3] 取消                                                      │
└─────────────────────────────────────────────────────────────────┘
```

When already deployed but a newer version exists:

```
┌─────────────────────────────────────────────────────────────────┐
│ ℹ️  argocd 已部署，有新版本可用                                   │
│                                                                 │
│ Release: argocd  Namespace: argocd                              │
│ 当前版本: v5.40.0 → 最新版本: v5.46.0                            │
│                                                                 │
│   [1] 升级到最新版本（可修改 values）                             │
│   [2] 重新配置当前版本（仅修改 values）                           │
│   [3] 查看当前配置                                               │
│   [4] 取消                                                      │
└─────────────────────────────────────────────────────────────────┘
```

### Manual Chart Input UI

Triggered by selecting `[0]` or when Artifact Hub search fails:

```
┌─────────────────────────────────────────────────────────────────┐
│ 手动指定 Helm Chart                                              │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│ Repo URL:                                                       │
│ ┌─────────────────────────────────────────────────────────────┐ │
│ │ https://argoproj.github.io/argo-helm_                       │ │
│ └─────────────────────────────────────────────────────────────┘ │
│                                                                 │
│ Chart 名称:                                                      │
│ ┌─────────────────────────────────────────────────────────────┐ │
│ │ argo-cd_                                                    │ │
│ └─────────────────────────────────────────────────────────────┘ │
│                                                                 │
│ 💡 提示：可在 https://artifacthub.io 搜索 chart 的 repo URL     │
│                                                                 │
│ Tab 切换字段  Enter 确认  Esc 取消                               │
└─────────────────────────────────────────────────────────────────┘
```

After `Enter`, the system validates by running `helm repo add` + `helm show chart`. If invalid, shows error inline and allows re-editing.

---

## LLM Values Generation (Phase 4)

### Strategy

**Full default values as context + minimal override output.**

The LLM receives the complete default `values.yaml` (with comments) as reference documentation, but is instructed to output only the fields that need to be changed.

### System Prompt

```
你是 Helm values 配置专家。

用户意图：{query}
应用：{chart_name}（{chart_description}）
目标命名空间：{namespace}
{extra_notes}

以下是该 chart 的完整默认 values.yaml（带注释，作为参考）：
---
{default_values}
---

请根据用户意图，生成最小化的 override values YAML。

规则：
1. 只包含需要修改的字段，不要重复默认值
2. 保持 YAML 层级结构正确
3. 如果用户没有明确要求，不要修改安全相关配置（密码、证书等）
4. 在每个修改项上方加注释说明修改原因
5. 如果用户意图不需要修改任何值（使用默认配置即可），输出空 YAML 并说明

输出格式：纯 YAML，不要包含 markdown 代码块标记。
```

### Token Budget Consideration

Typical `values.yaml` sizes:
- ArgoCD: ~1500 lines (~15k tokens)
- Prometheus Stack: ~3000 lines (~30k tokens)
- Simple charts (Redis, MySQL): ~200-500 lines (~2-5k tokens)

For very large values files (>2000 lines), consider truncating to the most relevant sections based on user intent. However, for the initial implementation, pass the full values — deployment is not a high-frequency operation, and correctness matters more than token cost.

---

## Human Review UI (Phase 5)

### TUI Component: DeployConfirmModel

```
┌─────────────────────────────────────────────────────────────────┐
│ Helm Deploy 确认                                                 │
│ Chart: argo/argo-cd (v5.46.0)  来源: 内置目录                   │
│ Release: argocd → Namespace: argocd                             │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│ ┌─────────────────────┬─────────────────────────────────────┐  │
│ │ 默认 Values (参考)   │ Override Values (Agent 生成)        │  │
│ ├─────────────────────┼─────────────────────────────────────┤  │
│ │ # Server config     │ # 用户要求暴露 NodePort             │  │
│ │ server:             │ server:                             │  │
│ │   service:          │   service:                          │  │
│ │     type: ClusterIP │     type: NodePort                  │  │
│ │     port: 80        │     nodePort: 30080                 │  │
│ │   replicas: 1       │                                     │  │
│ │   ...               │                                     │  │
│ │ (↑↓ 滚动)           │ (只显示 override 部分)              │  │
│ └─────────────────────┴─────────────────────────────────────┘  │
│                                                                 │
│ [Y] 执行  [E] 编辑 YAML  [C] 自然语言修正  [V] 完整预览  [N] 取消│
└─────────────────────────────────────────────────────────────────┘
```

### Interaction Modes

#### Choice Mode (initial state)

| Key | Action |
|-----|--------|
| `Y` | Execute `helm install/upgrade` with current override values |
| `E` | Enter YAML edit mode — right panel becomes editable textarea |
| `C` | Enter natural language correction mode — input box appears at bottom |
| `V` | Toggle full merged values preview (default values + override merged) |
| `N` / `Esc` | Cancel deployment |

#### YAML Edit Mode (`E`)

Right panel becomes an editable `textarea`:
- `Ctrl+S`: Save and return to choice mode (validates YAML syntax first)
- `Esc`: Discard changes and return to choice mode
- If YAML is invalid on save, show error message inline, stay in edit mode

#### Natural Language Correction Mode (`C`)

Bottom input box appears:
- User types correction instruction (e.g., "把 NodePort 改成 30090，副本数改成 3")
- `Enter`: Submit to LLM for re-generation, then return to choice mode with updated values
- `Esc`: Cancel and return to choice mode
- Supports multiple rounds of correction

### State Machine

```
confirmModeView (initial)
    │
    ├─ Y → [helm install/upgrade]
    ├─ N/Esc → [cancel]
    │
    ├─ E → confirmModeEditYAML
    │         ├─ Ctrl+S (valid) → confirmModeView (updated values)
    │         ├─ Ctrl+S (invalid) → show error, stay
    │         └─ Esc → confirmModeView (discard)
    │
    ├─ C → confirmModeEditNL
    │         ├─ Enter → [LLM regenerate] → confirmModeView (updated values)
    │         └─ Esc → confirmModeView
    │
    └─ V → confirmModeFullPreview
              └─ Esc → confirmModeView
```

### DeployConfirmationHandler Interface

```go
// pkg/agent/deploy/agent.go

type DeployConfirmationHandler interface {
    // ConfirmDeploy presents the deploy plan and waits for user decision.
    ConfirmDeploy(ctx context.Context, plan DeployPlan) (DeployDecision, error)
}

type DeployPlan struct {
    ChartInfo     *catalog.ChartInfo
    DefaultValues string // full default values.yaml with comments
    CustomValues  string // LLM-generated override values
    ReleaseName   string
    Namespace     string
    IsUpgrade     bool   // true if release already exists
}

type DeployDecision struct {
    Action     string // "execute" | "cancel"
    Values     string // final override values (may be edited by user)
    Correction string // if user used natural language correction
}
```

---

## Helm Client (`pkg/helm/`)

### Client Interface

```go
// pkg/helm/client.go

type Client struct {
    kubeConfig  string
    helmDriver  string // "secrets" (default)
}

// AddRepo adds a Helm repository.
func (c *Client) AddRepo(ctx context.Context, name, url string) error

// FetchDefaultValues runs `helm show values` and returns the full values.yaml string.
func (c *Client) FetchDefaultValues(ctx context.Context, repoName, chartName string) (string, error)

// InstallOrUpgrade installs a new release or upgrades an existing one.
func (c *Client) InstallOrUpgrade(ctx context.Context, opts InstallOptions) (*Release, error)

// Uninstall removes a release.
func (c *Client) Uninstall(ctx context.Context, releaseName, namespace string) error

// Status returns the status of a release.
func (c *Client) Status(ctx context.Context, releaseName, namespace string) (*Release, error)

// ListReleases returns all releases across all namespaces.
func (c *Client) ListReleases(ctx context.Context) ([]Release, error)

type InstallOptions struct {
    ReleaseName string
    RepoName    string
    ChartName   string
    Namespace   string
    Values      string // override values YAML
    CreateNS    bool   // --create-namespace
    Wait        bool   // --wait
    Timeout     time.Duration
}

type Release struct {
    Name      string
    Namespace string
    Chart     string
    Version   string
    Status    string // "deployed", "failed", "pending-install", etc.
    Updated   time.Time
}
```

### Implementation: Helm Go SDK vs. exec

Use the **Helm Go SDK** (`helm.sh/helm/v3`) directly, not `exec("helm ...")`:

| Dimension | Go SDK | exec |
|-----------|--------|------|
| Dependency | Go library, compiled in | Requires `helm` binary on PATH |
| Error handling | Structured Go errors | Parse stderr strings |
| Performance | In-process | Fork + exec overhead |
| Portability | Works everywhere Go runs | Requires helm installed |

The Go SDK is the clear choice for a compiled binary distribution.

---

## Deploy Agent Implementation

```go
// pkg/agent/deploy/agent.go

type Agent struct {
    llmClient      *llm.Client
    helmClient     *helm.Client
    chartResolver  *catalog.ChainResolver
    confirmHandler DeployConfirmationHandler
    eventCh        chan<- events.TUIEvent
    queryID        string
}

func New(llmClient *llm.Client, helmClient *helm.Client, chartResolver *catalog.ChainResolver, opts ...Option) *Agent

func (a *Agent) HandleQuery(ctx context.Context, query string, entities types.Entities) (string, error) {
    // Phase 1: Extract app name from entities or query
    appName := a.extractAppName(entities, query)

    // Phase 2: Resolve chart
    chartInfo, err := a.chartResolver.Resolve(ctx, appName)
    if chartInfo == nil {
        // All resolvers failed → trigger manual input or ArtifactHub interactive selection
        // (communicated via TUI events)
    }

    // Phase 2.5: Check if already deployed
    existingRelease, _ := a.helmClient.Status(ctx, appName, chartInfo.DefaultNamespace)
    if existingRelease != nil {
        // Handle already-deployed scenario (upgrade prompt)
    }

    // Phase 3: Fetch default values
    if err := a.helmClient.AddRepo(ctx, chartInfo.RepoName, chartInfo.RepoURL); err != nil {
        return "", fmt.Errorf("添加 Helm 仓库失败: %w", err)
    }
    defaultValues, err := a.helmClient.FetchDefaultValues(ctx, chartInfo.RepoName, chartInfo.ChartName)

    // Phase 4: LLM generates override values
    customValues, err := a.generateValues(ctx, query, chartInfo, defaultValues)

    // Phase 5: Human review
    decision, err := a.confirmHandler.ConfirmDeploy(ctx, DeployPlan{
        ChartInfo:     chartInfo,
        DefaultValues: defaultValues,
        CustomValues:  customValues,
        ReleaseName:   appName,
        Namespace:     chartInfo.DefaultNamespace,
        IsUpgrade:     existingRelease != nil,
    })
    if decision.Action == "cancel" {
        return "部署已取消", nil
    }

    // Handle natural language correction loop
    finalValues := decision.Values
    if decision.Correction != "" {
        finalValues, err = a.regenerateValues(ctx, query, chartInfo, defaultValues, decision.Correction)
        // Re-confirm...
    }

    // Phase 6: Execute helm install/upgrade
    release, err := a.helmClient.InstallOrUpgrade(ctx, helm.InstallOptions{
        ReleaseName: appName,
        RepoName:    chartInfo.RepoName,
        ChartName:   chartInfo.ChartName,
        Namespace:   chartInfo.DefaultNamespace,
        Values:      finalValues,
        CreateNS:    true,
        Wait:        true,
        Timeout:     5 * time.Minute,
    })

    return a.buildReport(release, chartInfo), nil
}
```

---

## Configuration

```yaml
# config.yaml additions
deploy:
  artifact_hub:
    enabled: true
    timeout: 5s              # search timeout
    selection_timeout: 10s   # auto-select countdown
  helm:
    wait_timeout: 5m         # helm install --wait timeout
```

---

## Summary of Design Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Agent placement | Independent Deploy Agent | Fundamentally different workflow from Operation Agent (single helm install vs. multi-step atomic ops) |
| Chart discovery | Catalog + Local + ArtifactHub chain | Catalog for reliability, ArtifactHub for coverage, Local for extensibility |
| ArtifactHub selection | Interactive list with auto-timeout | Transparent (user sees what they're installing), safe (user chooses), efficient (auto-timeout for common case) |
| LLM role | Generate override values only | Minimizes hallucination — LLM reads documented values.yaml, outputs only changes |
| Values review | Left-right diff panel | User can compare override against defaults with full context |
| Edit modes | YAML edit + natural language correction | YAML for precision, NL for convenience |
| Helm integration | Go SDK (helm.sh/helm/v3) | No external binary dependency, structured error handling |
| Already-deployed handling | Smart prompt (upgrade/reconfigure/view) | Prevents accidental re-install, enables upgrade workflow |
