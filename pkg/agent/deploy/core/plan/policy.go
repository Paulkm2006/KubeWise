package plan

import (
	"fmt"
	"strings"

	"sigs.k8s.io/yaml"
)

// ScanPolicyWarnings inspects values YAML for high-risk settings.
func ScanPolicyWarnings(valuesYAML string) []PlanWarning {
	if strings.TrimSpace(valuesYAML) == "" {
		return nil
	}
	var root map[string]interface{}
	if err := yaml.Unmarshal([]byte(valuesYAML), &root); err != nil {
		return nil
	}
	var warnings []PlanWarning
	scanPolicyMap("", root, &warnings)
	return warnings
}

func scanPolicyMap(path string, m map[string]interface{}, out *[]PlanWarning) {
	for k, v := range m {
		full := k
		if path != "" {
			full = path + "." + k
		}
		lower := strings.ToLower(k)
		switch lower {
		case "privileged":
			if truthy(v) {
				*out = append(*out, Warn(full, "privileged: true 将授予容器特权模式"))
			}
		case "hostnetwork":
			if truthy(v) {
				*out = append(*out, Warn(full, "hostNetwork: true 将使用主机网络"))
			}
		case "hostpid", "hostipc":
			if truthy(v) {
				*out = append(*out, Warn(full, fmt.Sprintf("%s: true 将共享主机命名空间", k)))
			}
		case "allowprivilegeescalation":
			if truthy(v) {
				*out = append(*out, Warn(full, "allowPrivilegeEscalation: true 允许权限提升"))
			}
		case "runasuser":
			if n, ok := asInt(v); ok && n == 0 {
				*out = append(*out, Warn(full, "runAsUser: 0 将以 root 用户运行"))
			}
		case "type":
			if path != "" && strings.HasSuffix(strings.ToLower(path), "service") {
				if s, ok := v.(string); ok {
					switch strings.ToLower(s) {
					case "loadbalancer":
						*out = append(*out, Warn(full, "Service type LoadBalancer 可能暴露公网入口"))
					case "nodeport":
						*out = append(*out, Warn(full, "Service type NodePort 将在节点上开放端口"))
					}
				}
			}
		case "insecureskipverify":
			if truthy(v) {
				*out = append(*out, Warn(full, "insecureSkipVerify: true 将跳过 TLS 验证"))
			}
		case "clusteradmin":
			if truthy(v) {
				*out = append(*out, Block(full, "clusterAdmin: true 将授予集群管理员权限"))
			}
		}

		if lower == "hostpath" {
			*out = append(*out, Warn(full, "检测到 hostPath 挂载，可能访问主机文件系统"))
		}

		if lower == "rules" {
			if rules, ok := v.([]interface{}); ok {
				for _, rule := range rules {
					if rm, ok := rule.(map[string]interface{}); ok && isWildcardRBAC(rm) {
						*out = append(*out, Block(full, "RBAC rules 包含通配符权限"))
					}
				}
			}
		}

		switch child := v.(type) {
		case map[string]interface{}:
			scanPolicyMap(full, child, out)
		case []interface{}:
			for i, item := range child {
				if cm, ok := item.(map[string]interface{}); ok {
					scanPolicyMap(fmt.Sprintf("%s[%d]", full, i), cm, out)
				}
			}
		}
	}
}

// Warn builds a non-blocking plan warning.
func Warn(path, msg string) PlanWarning {
	if path != "" {
		msg = path + ": " + msg
	}
	return PlanWarning{Severity: "warn", Message: msg}
}

// Block builds a blocking plan warning.
func Block(path, msg string) PlanWarning {
	if path != "" {
		msg = path + ": " + msg
	}
	return PlanWarning{Severity: "error", Message: msg}
}

func truthy(v interface{}) bool {
	switch t := v.(type) {
	case bool:
		return t
	case string:
		return strings.EqualFold(t, "true")
	default:
		return false
	}
}

func asInt(v interface{}) (int, bool) {
	switch t := v.(type) {
	case int:
		return t, true
	case int64:
		return int(t), true
	case float64:
		return int(t), true
	default:
		return 0, false
	}
}

func isWildcardRBAC(rule map[string]interface{}) bool {
	for _, key := range []string{"apiGroups", "resources", "verbs"} {
		if arr, ok := rule[key].([]interface{}); ok {
			for _, item := range arr {
				if s, ok := item.(string); ok && (s == "*" || s == "*/*") {
					return true
				}
			}
		}
	}
	return false
}
