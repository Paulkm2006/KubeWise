package helm

import (
	"context"
	"fmt"

	"go.uber.org/zap"
	"helm.sh/helm/v4/pkg/action"
	"helm.sh/helm/v4/pkg/chart/loader"
	ri "helm.sh/helm/v4/pkg/release"
	"sigs.k8s.io/yaml"
)

func (c *Client) prepareChart(ctx context.Context, opts ChartOptions) (string, map[string]interface{}, error) {
	_ = ctx
	vals := map[string]interface{}{}
	if opts.Values != "" {
		if err := yaml.Unmarshal([]byte(opts.Values), &vals); err != nil {
			return "", nil, fmt.Errorf("解析 override values 失败: %w", err)
		}
	}
	cp, err := c.resolveChart(ctx, opts.RepoName, opts.RepoURL, opts.ChartName)
	if err != nil {
		return "", nil, err
	}
	return cp, vals, nil
}

// Lint runs helm lint against the chart with the given values.
func (c *Client) Lint(ctx context.Context, opts LintOptions) (*LintResult, error) {
	cp, vals, err := c.prepareChart(ctx, opts.ChartOptions)
	if err != nil {
		return nil, err
	}
	lint := action.NewLint()
	lint.Namespace = opts.Namespace
	lint.Strict = opts.Strict
	lint.WithSubcharts = opts.WithSubcharts
	lint.SkipSchemaValidation = opts.SkipSchemaValidation

	lintPath, cleanup, err := lintChartPath(cp)
	if err != nil {
		return nil, err
	}
	defer cleanup()

	result := lint.Run([]string{lintPath}, vals)
	out := &LintResult{}
	for _, msg := range result.Messages {
		out.Messages = append(out.Messages, msg.Error())
	}
	out.Errors = result.Errors
	return out, nil
}

// Render renders manifests using client-side dry-run (helm template equivalent).
func (c *Client) Render(ctx context.Context, opts RenderOptions) (*RenderResult, error) {
	cp, vals, err := c.prepareChart(ctx, opts.ChartOptions)
	if err != nil {
		return nil, err
	}
	ch, err := loader.Load(cp)
	if err != nil {
		return nil, fmt.Errorf("加载 chart 失败: %w", err)
	}

	cfg, err := c.actionConfig(opts.Namespace)
	if err != nil {
		return nil, err
	}

	statusClient := action.NewStatus(cfg)
	_, statusErr := statusClient.Run(opts.ReleaseName)
	isUpgrade := opts.IsUpgrade || statusErr == nil

	var rel ri.Releaser
	if isUpgrade {
		up := action.NewUpgrade(cfg)
		up.Namespace = opts.Namespace
		up.DryRunStrategy = action.DryRunClient
		rel, err = up.RunWithContext(ctx, opts.ReleaseName, ch, vals)
	} else {
		inst := action.NewInstall(cfg)
		inst.ReleaseName = opts.ReleaseName
		inst.Namespace = opts.Namespace
		inst.DryRunStrategy = action.DryRunClient
		rel, err = inst.RunWithContext(ctx, ch, vals)
	}
	if err != nil {
		return nil, fmt.Errorf("helm render 失败: %w", err)
	}

	acc, err := ri.NewAccessor(rel)
	if err != nil {
		return nil, err
	}
	return &RenderResult{
		Manifest: acc.Manifest(),
		Notes:    acc.Notes(),
	}, nil
}

// Validate runs YAML syntax check, lint, and client dry-run render.
func (c *Client) Validate(ctx context.Context, opts RenderOptions) (*ValidationResult, error) {
	c.logger().Debug("helm validate starting",
		zap.String("release", opts.ReleaseName),
		zap.String("chart", opts.ChartName),
		zap.String("namespace", opts.Namespace),
		zap.Bool("upgrade", opts.IsUpgrade),
	)
	result := &ValidationResult{}
	if opts.Values != "" {
		result.ValuesOK = ValidateYAML(opts.Values)
		if result.ValuesOK != nil {
			c.logger().Warn("override values YAML invalid",
				zap.String("release", opts.ReleaseName),
				zap.Error(result.ValuesOK),
			)
		}
	}
	lintOpts := LintOptions{
		ChartOptions: opts.ChartOptions,
		Strict:       false,
	}
	lintRes, lintErr := c.Lint(ctx, lintOpts)
	if lintErr != nil {
		c.logger().Error("helm lint failed", zap.Error(lintErr), zap.String("chart", opts.ChartName))
		result.Lint = &LintResult{Errors: []error{lintErr}}
	} else {
		result.Lint = lintRes
		if lintRes != nil && len(lintRes.Errors) > 0 {
			c.logger().Warn("helm lint reported errors",
				zap.String("chart", opts.ChartName),
				zap.Int("errors", len(lintRes.Errors)),
			)
		}
	}
	renderRes, renderErr := c.Render(ctx, opts)
	if renderErr != nil {
		c.logger().Error("helm render dry-run failed",
			zap.Error(renderErr),
			zap.String("release", opts.ReleaseName),
		)
		result.RenderErr = renderErr
	} else {
		result.Render = renderRes
	}
	if result.HasErrors() {
		c.logger().Warn("helm validate failed", zap.String("summary", result.ErrorSummary()))
	} else {
		c.logger().Debug("helm validate passed", zap.String("release", opts.ReleaseName))
	}
	return result, nil
}
