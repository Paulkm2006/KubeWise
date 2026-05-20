package values

import (
	"context"
	"fmt"
	"strings"

	"github.com/kubewise/kubewise/pkg/agent/deploy/core/plan"
	"github.com/kubewise/kubewise/pkg/catalog"
	"github.com/kubewise/kubewise/pkg/helm"
	"github.com/kubewise/kubewise/pkg/llm"
)

const (
	ToolGenerate   = "LLM values generation"
	ToolRegenerate = "LLM values regeneration"
)

const maxDefaultValuesLines = 2000

const submitGeneratedValuesFnName = "submit_generated_values"

var submitGeneratedValuesFn = llm.FunctionDefinition{
	Name:        submitGeneratedValuesFnName,
	Description: "提交生成的 Helm override values 与目标 namespace。完成分析后必须调用此工具。",
	Parameters: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"namespace": map[string]any{
				"type":        "string",
				"description": "推荐的目标 Kubernetes namespace（DNS label，如 monitoring、ingress-nginx）",
			},
			"values_yaml": map[string]any{
				"type":        "string",
				"description": "最小化 override values YAML，仅包含需要修改的字段；无需修改时传空字符串",
			},
			"explanation": map[string]any{
				"type":        "string",
				"description": "变更说明，面向用户",
			},
			"risk_level": map[string]any{
				"type":        "string",
				"enum":        []string{"low", "medium", "high"},
				"description": "配置变更风险等级",
			},
		},
		"required": []string{"namespace", "values_yaml", "explanation", "risk_level"},
	},
}

const valuesGenSystemPrompt = `你是 Helm values 配置专家。

规则：
1. 分析完成后必须调用 submit_generated_values，不要用纯文本回复
2. 只包含需要修改的 YAML 字段，不要重复默认值
3. 保持 YAML 层级结构正确
4. 如果用户没有明确要求，不要修改安全相关配置（密码、证书、privileged 等）
5. 在每个修改项上方加注释说明修改原因
6. 如果用户意图不需要修改任何值，values_yaml 传空字符串
7. namespace 必须符合 DNS label 规则，禁止 kube-system、kube-public、kube-node-lease
8. 用户若明确指定 namespace（如「部署到 dev」「放到 staging 命名空间」），必须使用用户指定的 namespace，不要用惯例覆盖
9. 根据风险设置 risk_level：仅常规副本/资源调整为 low；涉及 NodePort/LoadBalancer 为 medium；涉及特权/主机网络/RBAC 通配符为 high`

// LLMClient is the minimal LLM interface for values generation.
type LLMClient interface {
	ChatCompletion(ctx context.Context, messages []llm.Message, functions []llm.FunctionDefinition) (*llm.Message, error)
}

// Result holds structured LLM output for values generation.
type Result struct {
	Values      string
	Namespace   string
	Explanation string
	RiskLevel   string
}

// GenerateInput is input for initial values generation.
type GenerateInput struct {
	Query         string
	Chart         *catalog.ChartInfo
	DefaultValues string
}

// RegenerateInput is input for values regeneration after user correction.
type RegenerateInput struct {
	Query         string
	Chart         *catalog.ChartInfo
	DefaultValues string
	CurrentValues string
	Correction    string
}

// Generate calls the LLM to produce override values.
func Generate(ctx context.Context, llmClient LLMClient, in GenerateInput) (*Result, error) {
	truncated := helm.TruncateValues(in.DefaultValues, maxDefaultValuesLines)
	extraNotes := buildChartNotes(in.Chart)
	prompt := fmt.Sprintf(`用户意图：%s
应用：%s（%s）
%s

默认 values.yaml（参考）：
---
%s
---`,
		in.Query, in.Chart.ChartName, in.Chart.Description, extraNotes, truncated,
	)
	return callValuesLLM(ctx, llmClient, valuesGenSystemPrompt, prompt, in.Chart)
}

// Regenerate re-generates values from a user correction.
func Regenerate(ctx context.Context, llmClient LLMClient, in RegenerateInput) (*Result, error) {
	truncated := helm.TruncateValues(in.DefaultValues, maxDefaultValuesLines)
	prompt := fmt.Sprintf(`原始用户意图：%s
应用：%s（%s）

当前 override values：
---
%s
---

默认 values（参考）：
---
%s
---

用户修正指令：%s

根据修正指令更新配置。若修正涉及 namespace，在 submit_generated_values 中更新 namespace。`,
		in.Query, in.Chart.ChartName, in.Chart.Description,
		in.CurrentValues, truncated, in.Correction,
	)
	return callValuesLLM(ctx, llmClient, valuesGenSystemPrompt, prompt, in.Chart)
}

func callValuesLLM(ctx context.Context, llmClient LLMClient, systemPrompt, userPrompt string, chartInfo *catalog.ChartInfo) (*Result, error) {
	messages := []llm.Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userPrompt},
	}

	resp, err := llmClient.ChatCompletion(ctx, messages, []llm.FunctionDefinition{submitGeneratedValuesFn})
	if err != nil {
		return nil, fmt.Errorf("LLM values 生成失败: %w", err)
	}

	result, err := parseGeneratedValuesResponse(resp, chartInfo)
	if err != nil {
		return nil, err
	}
	if err := validateGeneratedValues(result); err != nil {
		return nil, err
	}
	return result, nil
}

func parseGeneratedValuesResponse(resp *llm.Message, chartInfo *catalog.ChartInfo) (*Result, error) {
	if resp == nil {
		return nil, fmt.Errorf("LLM 返回为空")
	}

	for _, tc := range resp.ToolCalls {
		if tc.Function.Name == submitGeneratedValuesFnName {
			return parseGeneratedValuesArgs(tc.Function.Arguments, chartInfo)
		}
	}

	return nil, fmt.Errorf("LLM 未调用 %s，请重试（请确认模型支持 function calling）", submitGeneratedValuesFnName)
}

func parseGeneratedValuesArgs(args map[string]any, chartInfo *catalog.ChartInfo) (*Result, error) {
	if args == nil {
		return nil, fmt.Errorf("submit_generated_values 参数为空")
	}

	ns, _ := args["namespace"].(string)
	valuesYAML, _ := args["values_yaml"].(string)
	explanation, _ := args["explanation"].(string)
	riskLevel, _ := args["risk_level"].(string)

	raw := &Result{
		Namespace:   plan.SanitizeNamespace(ns),
		Values:      cleanLLMOutput(valuesYAML),
		Explanation: strings.TrimSpace(explanation),
		RiskLevel:   strings.TrimSpace(riskLevel),
	}
	return finalizeValuesResult(raw, chartInfo), nil
}

func finalizeValuesResult(raw *Result, chartInfo *catalog.ChartInfo) *Result {
	if raw == nil {
		raw = &Result{}
	}
	raw.Namespace = resolveNamespace(chartInfo, raw.Namespace)
	if raw.Namespace == "" {
		raw.Namespace = "default"
	}
	return raw
}

func resolveNamespace(chartInfo *catalog.ChartInfo, llmNamespace string) string {
	ns := plan.SanitizeNamespace(llmNamespace)
	if ns != "" {
		return ns
	}
	if chartInfo != nil && chartInfo.DefaultNamespace != "" {
		return plan.SanitizeNamespace(chartInfo.DefaultNamespace)
	}
	if chartInfo != nil {
		if hint := conventionNamespaceHint(chartInfo.ChartName); hint != "" {
			return hint
		}
	}
	return "default"
}

func conventionNamespaceHint(chartName string) string {
	hints := map[string]string{
		"prometheus":      "monitoring",
		"kube-prometheus": "monitoring",
		"grafana":         "monitoring",
		"loki":            "monitoring",
		"argocd":          "argocd",
		"argo-cd":         "argocd",
		"ingress-nginx":   "ingress-nginx",
		"cert-manager":    "cert-manager",
	}
	return hints[strings.ToLower(chartName)]
}

func validateGeneratedValues(r *Result) error {
	if err := plan.ValidateNamespace(r.Namespace); err != nil {
		return fmt.Errorf("namespace 无效: %w", err)
	}
	if r.Values != "" {
		if err := helm.ValidateYAML(r.Values); err != nil {
			return fmt.Errorf("values YAML 语法错误: %w", err)
		}
	}
	return nil
}

func buildChartNotes(chartInfo *catalog.ChartInfo) string {
	var extra strings.Builder
	if chartInfo.Notes != "" {
		extra.WriteString("额外提示：" + chartInfo.Notes + "\n")
	}
	if chartInfo.InstallCRDs {
		extra.WriteString("注意：此 chart 可能需要 installCRDs，系统会在适用时自动添加。\n")
	}
	if chartInfo.DefaultNamespace != "" {
		extra.WriteString(fmt.Sprintf("Chart 常用 namespace 参考：%s（用户明确要求其他 namespace 时以用户为准）\n", chartInfo.DefaultNamespace))
	}
	if hint := conventionNamespaceHint(chartInfo.ChartName); hint != "" && hint != chartInfo.DefaultNamespace {
		extra.WriteString(fmt.Sprintf("社区惯例 namespace 参考：%s（仅作建议，用户指定时以用户为准）\n", hint))
	}
	return extra.String()
}

func cleanLLMOutput(s string) string {
	result := strings.TrimSpace(s)
	result = strings.TrimPrefix(result, "```yaml\n")
	result = strings.TrimPrefix(result, "```\n")
	result = strings.TrimPrefix(result, "```yaml")
	result = strings.TrimPrefix(result, "```")
	result = strings.TrimSuffix(result, "\n```")
	result = strings.TrimSuffix(result, "```")
	return strings.TrimSpace(result)
}

// SubmitGeneratedValuesFnName is exported for tests.
const SubmitGeneratedValuesFnName = submitGeneratedValuesFnName
