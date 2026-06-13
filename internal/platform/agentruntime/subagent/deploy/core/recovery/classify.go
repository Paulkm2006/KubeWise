package recovery

import (
	"fmt"
	"strings"
)

// Class identifies a recovery error category.
type Class string

const (
	ClassUnknown               Class = "unknown"
	ClassHelmOwnershipConflict Class = "helm_ownership_conflict"
)

// Triage is the outcome of classifying a deploy error.
type Triage struct {
	Class         Class
	Deterministic bool
	Reason        string
	Report        string
}

// ClassifyError triages deploy errors before entering the recovery loop.
func ClassifyError(err error) Triage {
	if err == nil {
		return Triage{Class: ClassUnknown}
	}
	msg := err.Error()
	if isHelmOwnershipConflict(msg) {
		return Triage{
			Class:         ClassHelmOwnershipConflict,
			Deterministic: true,
			Reason:        "Helm 资源归属冲突",
			Report:        buildOwnershipConflictReport(msg),
		}
	}
	return Triage{Class: ClassUnknown}
}

func isHelmOwnershipConflict(msg string) bool {
	return strings.Contains(msg, "cannot be imported into the current release") &&
		strings.Contains(msg, "invalid ownership metadata") &&
		strings.Contains(msg, "meta.helm.sh/release-namespace")
}

func buildOwnershipConflictReport(msg string) string {
	return fmt.Sprintf(`部署失败：Helm 发现已有资源属于另一个 release，不能被当前 release 接管。

根因：
%s

这类错误通常不是 values 渲染问题，也不需要继续查询 Pod 或日志。请在以下处置方式中选择一种：
1. 使用现有 release/namespace 继续管理该应用。
2. 确认旧 release 不再需要后，先卸载旧 release 并清理相关 CRD，再重新安装。
3. 如果 chart 支持跳过 CRD 安装，可以在确认风险后关闭 CRD 安装，让新 release 复用已有 CRD。

为避免误删集群级 CRD，KubeWise 不会自动修改 CRD ownership annotation 或删除 CRD。`, msg)
}
