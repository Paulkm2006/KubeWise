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
%s

以下是该 chart 的完整默认 values.yaml（带注释，作为参考）：
---
%s
---

请根据用户意图，生成最小化的 override values YAML。

输出格式：
## namespace: <推荐的目标 namespace>
<override values YAML>

规则：
1. ## namespace 行指定目标 namespace，根据应用类型推荐（如 nginx → ingress-nginx，prometheus → monitoring，argocd → argocd）
2. 只包含需要修改的 YAML 字段，不要重复默认值
3. 保持 YAML 层级结构正确
4. 如果用户没有明确要求，不要修改安全相关配置（密码、证书等）
5. 在每个修改项上方加注释说明修改原因
6. 如果用户意图不需要修改任何值（使用默认配置即可），输出空 YAML 并说明`

const maxDefaultValuesLines = 2000

// valuesResult holds the LLM-generated values and recommended namespace.
type valuesResult struct {
	values    string
	namespace string
}

// generateValues calls the LLM to generate override values and recommend a namespace.
func generateValues(ctx context.Context, llmClient llmClient, query string, chartInfo *catalog.ChartInfo, defaultValues string) (*valuesResult, error) {
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
		extraNotes,
		truncated,
	)

	messages := []llm.Message{
		{Role: "user", Content: prompt},
	}

	resp, err := llmClient.ChatCompletion(ctx, messages, nil)
	if err != nil {
		return nil, fmt.Errorf("LLM values 生成失败: %w", err)
	}

	return parseValuesResult(resp.Content)
}

// regenerateValues re-generates values based on user correction, also supporting namespace changes.
func regenerateValues(ctx context.Context, llmClient llmClient, query string, chartInfo *catalog.ChartInfo, defaultValues, currentValues, correction string) (*valuesResult, error) {
	truncated := helm.TruncateValues(defaultValues, maxDefaultValuesLines)

	prompt := fmt.Sprintf(`你是 Helm values 配置专家。

原始用户意图：%s
应用：%s（%s）
当前 override values：
---
%s
---

完整默认 values（参考）：
---
%s
---

用户修正指令：%s

请根据修正指令更新 override values YAML。

输出格式：
## namespace: <推荐或修正的 namespace>
<override values YAML>

保持最小化原则，只包含需要修改的字段。
如果修正指令中提到了 namespace 变更（如"部署到 xxx namespace"），更新 ## namespace 行。`,
		query,
		chartInfo.ChartName,
		chartInfo.Description,
		currentValues,
		truncated,
		correction,
	)

	messages := []llm.Message{
		{Role: "user", Content: prompt},
	}

	resp, err := llmClient.ChatCompletion(ctx, messages, nil)
	if err != nil {
		return nil, fmt.Errorf("LLM values 重新生成失败: %w", err)
	}

	return parseValuesResult(resp.Content)
}

// parseValuesResult extracts the namespace and values from an LLM response.
// Expected format:
//
//	## namespace: <name>
//	<values YAML>
func parseValuesResult(raw string) (*valuesResult, error) {
	result := &valuesResult{}
	lines := strings.Split(raw, "\n")

	if len(lines) > 0 && strings.HasPrefix(strings.TrimSpace(lines[0]), "## namespace:") {
		result.namespace = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(lines[0]), "## namespace:"))
		raw = strings.Join(lines[1:], "\n")
	}

	result.values = cleanLLMOutput(raw)
	if result.namespace == "" {
		result.namespace = "default"
	}

	return result, nil
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
