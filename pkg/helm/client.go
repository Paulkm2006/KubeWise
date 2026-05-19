// pkg/helm/client.go
package helm

import (
	"context"
	"fmt"
	"strings"
	"time"

	"helm.sh/helm/v4/pkg/action"
	"helm.sh/helm/v4/pkg/chart"
	"helm.sh/helm/v4/pkg/chart/loader"
	"helm.sh/helm/v4/pkg/cli"
	"helm.sh/helm/v4/pkg/getter"
	"helm.sh/helm/v4/pkg/kube"
	ri "helm.sh/helm/v4/pkg/release"
	repov1 "helm.sh/helm/v4/pkg/repo/v1"
	"go.uber.org/zap"
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
	RepoURL     string // Helm 仓库 URL，用于 LocateChart 下载
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
	log        *zap.Logger
}

// SetLogger injects a logger for debug output.
func (c *Client) SetLogger(l *zap.Logger) { c.log = l }

func (c *Client) logger() *zap.Logger {
	if c.log == nil {
		return zap.NewNop()
	}
	return c.log
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
		log:        zap.NewNop(),
	}
}

// actionConfig 创建 Helm action 配置（每次操作创建新实例以支持不同 namespace）。
func (c *Client) actionConfig(namespace string) (*action.Configuration, error) {
	cfg := action.NewConfiguration()
	if err := cfg.Init(c.settings.RESTClientGetter(), namespace, "secrets"); err != nil {
		c.logger().Error("helm action config init failed", zap.Error(err), zap.String("namespace", namespace))
		return nil, fmt.Errorf("初始化 helm action 配置失败: %w", err)
	}
	return cfg, nil
}

// AddRepo 添加 Helm 仓库，写入 repositories.yaml 并下载索引。
// LocateChart 需要已缓存的 repo index 才能解析 repo/chart 引用。
func (c *Client) AddRepo(ctx context.Context, name, repoURL string) error {
	repoFile := c.settings.RepositoryConfig
	f, err := repov1.LoadFile(repoFile)
	if err != nil {
		f = repov1.NewFile()
	}

	// 检查仓库是否已存在且 URL 未变更
	for _, e := range f.Repositories {
		if e != nil && e.Name == name && e.URL == repoURL {
			c.logger().Debug("helm repo already added, skipping", zap.String("name", name))
			return nil
		}
	}

	c.logger().Debug("adding helm repo", zap.String("name", name), zap.String("url", repoURL))

	entry := &repov1.Entry{
		Name: name,
		URL:  repoURL,
	}

	// 下载 repo index 到缓存（LocateChart 后续读取）
	r, err := repov1.NewChartRepository(entry, getter.All(c.settings))
	if err != nil {
		c.logger().Error("create chart repository failed", zap.Error(err), zap.String("name", name))
		return fmt.Errorf("创建 chart 仓库失败: %w", err)
	}
	if _, err := r.DownloadIndexFile(); err != nil {
		c.logger().Error("download repo index failed", zap.Error(err), zap.String("name", name))
		return fmt.Errorf("下载仓库索引失败: %w", err)
	}

	f.Update(entry)
	if err := f.WriteFile(repoFile, 0644); err != nil {
		c.logger().Error("write repo file failed", zap.Error(err))
		return err
	}
	return nil
}

// FetchDefaultValues 下载 chart 并返回完整的 values.yaml 字符串（含注释）。
func (c *Client) FetchDefaultValues(ctx context.Context, repoName, repoURL, chartName string) (string, error) {
	c.logger().Debug("fetching default values",
		zap.String("repo", repoName), zap.String("chart", chartName),
	)
	cp, err := c.resolveChart(ctx, repoName, repoURL, chartName)
	if err != nil {
		c.logger().Error("resolve chart failed", zap.Error(err), zap.String("chart", chartName))
		return "", err
	}

	cfg, err := c.actionConfig("")
	if err != nil {
		return "", err
	}
	showClient := action.NewShow(action.ShowValues, cfg)
	output, err := showClient.Run(cp)
	if err != nil {
		c.logger().Error("show values failed", zap.Error(err), zap.String("chart", chartName))
		return "", fmt.Errorf("获取 chart 默认 values 失败: %w", err)
	}
	return output, nil
}

// resolveChart 下载并缓存 chart，返回本地文件路径。
// 通过 ChartPathOptions.LocateChart（标准 Helm v4 方式）定位 chart。
// 使用 repo/chart 格式，LocateChart 会从 repositories.yaml 读取 repo URL。
func (c *Client) resolveChart(ctx context.Context, repoName, repoURL, chartName string) (string, error) {
	chartRef := fmt.Sprintf("%s/%s", repoName, chartName)
	cpo := &action.ChartPathOptions{}
	cp, err := cpo.LocateChart(chartRef, c.settings)
	if err != nil {
		c.logger().Error("locate chart failed", zap.Error(err), zap.String("chart", chartRef))
		return "", fmt.Errorf("定位 chart 失败: %w", err)
	}
	return cp, nil
}

// InstallOrUpgrade 安装新 release 或升级已有 release。
func (c *Client) InstallOrUpgrade(ctx context.Context, opts InstallOptions) (*Release, error) {
	c.logger().Info("helm install/upgrade",
		zap.String("release", opts.ReleaseName),
		zap.String("chart", opts.ChartName),
		zap.String("namespace", opts.Namespace),
	)
	cfg, err := c.actionConfig(opts.Namespace)
	if err != nil {
		return nil, err
	}

	// 解析 override values
	vals := map[string]interface{}{}
	if opts.Values != "" {
		if err := yaml.Unmarshal([]byte(opts.Values), &vals); err != nil {
			c.logger().Error("parse override values failed", zap.Error(err))
			return nil, fmt.Errorf("解析 override values 失败: %w", err)
		}
	}

	// 下载并定位 chart 到本地缓存
	cp, err := c.resolveChart(ctx, opts.RepoName, opts.RepoURL, opts.ChartName)
	if err != nil {
		return nil, err
	}
	chart, err := loader.Load(cp)
	if err != nil {
		c.logger().Error("load chart failed", zap.Error(err), zap.String("chart", opts.ChartName))
		return nil, fmt.Errorf("加载 chart 失败: %w", err)
	}

	// 检查 release 是否已存在
	histClient := action.NewHistory(cfg)
	histClient.Max = 1
	_, histErr := histClient.Run(opts.ReleaseName)

	var rel ri.Releaser
	if histErr != nil {
		// 全新安装
		installClient := action.NewInstall(cfg)
		installClient.ReleaseName = opts.ReleaseName
		installClient.Namespace = opts.Namespace
		installClient.CreateNamespace = opts.CreateNS
		if opts.Wait {
			installClient.WaitStrategy = kube.StatusWatcherStrategy
		}
		if opts.Timeout > 0 {
			installClient.Timeout = opts.Timeout
		}
		rel, err = installClient.RunWithContext(ctx, chart, vals)
		if err != nil {
			c.logger().Error("helm install failed",
				zap.Error(err),
				zap.String("release", opts.ReleaseName),
				zap.String("chart", opts.ChartName),
				zap.String("namespace", opts.Namespace),
			)
			return nil, fmt.Errorf("helm install 失败: %w", err)
		}
	} else {
		// 升级已有 release
		upgradeClient := action.NewUpgrade(cfg)
		upgradeClient.Namespace = opts.Namespace
		if opts.Wait {
			upgradeClient.WaitStrategy = kube.StatusWatcherStrategy
		}
		if opts.Timeout > 0 {
			upgradeClient.Timeout = opts.Timeout
		}
		rel, err = upgradeClient.RunWithContext(ctx, opts.ReleaseName, chart, vals)
		if err != nil {
			c.logger().Error("helm upgrade failed",
				zap.Error(err),
				zap.String("release", opts.ReleaseName),
				zap.String("chart", opts.ChartName),
				zap.String("namespace", opts.Namespace),
			)
			return nil, fmt.Errorf("helm upgrade 失败: %w", err)
		}
	}

	return releaserToRelease(rel)
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
	return releaserToRelease(rel)
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
		rel, err := releaserToRelease(r)
		if err != nil {
			continue
		}
		result = append(result, *rel)
	}
	return result, nil
}

// releaserToRelease 将 Helm SDK Releaser 转换为本地 Release 结构。
func releaserToRelease(r ri.Releaser) (*Release, error) {
	if r == nil {
		return nil, nil
	}
	acc, err := ri.NewAccessor(r)
	if err != nil {
		return nil, fmt.Errorf("获取 release accessor 失败: %w", err)
	}

	chartName := ""
	if ch := acc.Chart(); ch != nil {
		chAcc, err := chart.NewAccessor(ch)
		if err == nil {
			meta := chAcc.MetadataAsMap()
			name, _ := meta["name"].(string)
			version, _ := meta["version"].(string)
			if name != "" {
				chartName = fmt.Sprintf("%s-%s", name, version)
			}
		}
	}

	return &Release{
		Name:      acc.Name(),
		Namespace: acc.Namespace(),
		Chart:     chartName,
		Version:   fmt.Sprintf("%d", acc.Version()),
		Status:    acc.Status(),
		Updated:   acc.DeployedAt(),
	}, nil
}
