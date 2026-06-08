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
