package report

import (
	"fmt"
	"strings"
)

func ToMarkdown(r DiagnosisReport) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("## 诊断：%s/%s\n\n", r.Target.Namespace, r.Target.Pod))
	b.WriteString(fmt.Sprintf("- 集群：`%s`\n", r.Target.Cluster))
	b.WriteString(fmt.Sprintf("- 结论：**%s**\n", r.Verdict))
	b.WriteString(fmt.Sprintf("- 摘要：%s\n\n", r.Summary))

	b.WriteString("### 根因\n\n")
	b.WriteString(fmt.Sprintf("- **%s** (%s)\n", r.RootCause.Title, r.RootCause.Category))
	b.WriteString(fmt.Sprintf("- %s\n", r.RootCause.Summary))
	if r.RootCause.ConfidenceLabel != "" {
		b.WriteString(fmt.Sprintf("- 置信度：%s (%.0f%%)\n\n", r.RootCause.ConfidenceLabel, r.RootCause.ConfidenceScore*100))
	}

	if len(r.Evidence) > 0 {
		b.WriteString("### 证据\n\n")
		for _, ev := range r.Evidence {
			b.WriteString(fmt.Sprintf("- [%s] (%s/%s) %s\n", ev.ID, ev.Source, ev.Strength, ev.Summary))
		}
		b.WriteString("\n")
	}

	if len(r.Hypotheses) > 0 {
		b.WriteString("### 已检查的假设\n\n")
		for _, h := range r.Hypotheses {
			b.WriteString(fmt.Sprintf("- %s [%s]: %s\n", h.Title, h.Status, h.Rationale))
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
