package report

import (
	"fmt"
	"strings"
)

func ToMarkdown(r DiagnosisReport) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("## 诊断：%s/%s\n\n", r.Target.Namespace, r.Target.Pod))
	b.WriteString(fmt.Sprintf("- 集群：`%s`\n", r.Target.Cluster))
	b.WriteString(fmt.Sprintf("- 结论：**%s**\n", verdictText(r.Verdict)))
	b.WriteString(fmt.Sprintf("- 摘要：%s\n\n", r.Summary))

	b.WriteString("### 根因\n\n")
	b.WriteString(fmt.Sprintf("- **%s** (%s)\n", r.RootCause.Title, r.RootCause.Category))
	b.WriteString(fmt.Sprintf("- %s\n", r.RootCause.Summary))
	if r.RootCause.ConfidenceLabel != "" {
		b.WriteString(fmt.Sprintf("- 置信度：%s (%.0f%%)\n\n", confidenceText(r.RootCause.ConfidenceLabel), r.RootCause.ConfidenceScore*100))
	}

	if len(r.Evidence) > 0 {
		b.WriteString("### 证据\n\n")
		for _, ev := range r.Evidence {
			b.WriteString(fmt.Sprintf("- [%s] (%s/%s) %s\n", ev.ID, ev.Source, strengthText(ev.Strength), ev.Summary))
		}
		b.WriteString("\n")
	}

	if len(r.Hypotheses) > 0 {
		b.WriteString("### 已检查的假设\n\n")
		for _, h := range r.Hypotheses {
			b.WriteString(fmt.Sprintf("- %s [%s]: %s\n", h.Title, statusText(h.Status), h.Rationale))
		}
		b.WriteString("\n")
	}

	if len(r.Actions) > 0 {
		b.WriteString("### 推荐操作\n\n")
		for _, a := range r.Actions {
			if a.Command != "" {
				b.WriteString(fmt.Sprintf("- (%s) %s\n  - `%s`\n", a.Priority, a.Description, a.Command))
				continue
			}
			b.WriteString(fmt.Sprintf("- (%s) %s\n", a.Priority, a.Description))
		}
		b.WriteString("\n")
	}

	if len(r.Limitations) > 0 {
		b.WriteString("### 局限性\n\n")
		for _, lim := range r.Limitations {
			b.WriteString(fmt.Sprintf("- %s\n", lim))
		}
		b.WriteString("\n")
	}

	if r.Enrichment.Status == EnrichmentDegraded && r.Enrichment.Message != "" {
		b.WriteString("### 补充说明\n\n")
		b.WriteString(r.Enrichment.Message + "\n")
	}
	return b.String()
}

func verdictText(v Verdict) string {
	switch v {
	case VerdictConfirmed:
		return "已确认"
	case VerdictLikely:
		return "较可能"
	case VerdictInconclusive:
		return "暂无法确认"
	default:
		return string(v)
	}
}

func confidenceText(label string) string {
	switch label {
	case "high":
		return "高"
	case "medium":
		return "中"
	case "low":
		return "低"
	default:
		return label
	}
}

func strengthText(strength string) string {
	switch strength {
	case "strong":
		return "强证据"
	case "moderate":
		return "中等证据"
	case "weak":
		return "弱证据"
	default:
		return strength
	}
}

func statusText(status string) string {
	switch status {
	case "supported":
		return "已支持"
	case "refuted":
		return "已排除"
	case "uncertain":
		return "不确定"
	default:
		return status
	}
}
