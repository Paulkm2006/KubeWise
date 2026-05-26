package plan

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/kubewise/kubewise/pkg/helm"
)

var (
	dnsLabelRegex = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`)
)

var blockedNamespaces = map[string]struct{}{
	"kube-system":     {},
	"kube-public":     {},
	"kube-node-lease": {},
}

// ValidationResult holds blocking errors and non-blocking warnings.
type ValidationResult struct {
	Errors   []string
	Warnings []Warning
}

// HasBlockingErrors returns true when validation must stop before apply.
func (r ValidationResult) HasBlockingErrors() bool {
	return len(r.Errors) > 0
}

// Merge combines another validation result.
func (r *ValidationResult) Merge(other ValidationResult) {
	r.Errors = append(r.Errors, other.Errors...)
	r.Warnings = append(r.Warnings, other.Warnings...)
}

// ValidateDeployPlan runs static checks on a deploy plan before review/apply.
func ValidateDeployPlan(p DeployPlan) ValidationResult {
	var result ValidationResult

	if p.Chart == nil {
		result.Errors = append(result.Errors, "chart 信息缺失")
		return result
	}

	if err := ValidateNamespace(p.Namespace); err != nil {
		result.Errors = append(result.Errors, err.Error())
	} else if p.Namespace == "default" {
		result.Warnings = append(result.Warnings, Warning{
			Severity: "warn",
			Message:  "将部署到 default 命名空间，建议使用专用 namespace",
		})
	}

	if err := ValidateReleaseName(p.ReleaseName); err != nil {
		result.Errors = append(result.Errors, err.Error())
	}

	if p.CustomValues != "" {
		if err := helm.ValidateYAML(p.CustomValues); err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("override values YAML 语法错误: %v", err))
		}
	}

	policy := ScanPolicyWarnings(p.CustomValues)
	result.Warnings = append(result.Warnings, policy...)

	for _, w := range policy {
		if w.Severity == "error" {
			result.Errors = append(result.Errors, w.Message)
		}
	}

	return result
}

// ValidateNamespace checks Kubernetes DNS label rules and blocked namespaces.
func ValidateNamespace(ns string) error {
	ns = strings.TrimSpace(ns)
	if ns == "" {
		return fmt.Errorf("namespace 不能为空")
	}
	if len(ns) > 63 {
		return fmt.Errorf("namespace 长度不能超过 63 字符")
	}
	if !dnsLabelRegex.MatchString(ns) {
		return fmt.Errorf("namespace %q 不符合 DNS label 规则", ns)
	}
	if _, blocked := blockedNamespaces[ns]; blocked {
		return fmt.Errorf("禁止部署到系统 namespace %q", ns)
	}
	return nil
}

// ValidateReleaseName checks Helm release name constraints.
func ValidateReleaseName(name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("release 名称不能为空")
	}
	if len(name) > 53 {
		return fmt.Errorf("release 名称长度不能超过 53 字符")
	}
	if !dnsLabelRegex.MatchString(name) {
		return fmt.Errorf("release 名称 %q 不符合 DNS label 规则", name)
	}
	return nil
}
