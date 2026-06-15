package report

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/kubewise/kubewise/internal/platform/agentruntime/diagnose/casefile"
	"github.com/kubewise/kubewise/internal/platform/agentruntime/diagnose/hypothesis"
	"github.com/kubewise/kubewise/internal/platform/agentruntime/diagnose/verify"
	"github.com/kubewise/kubewise/internal/utils/llm"
)

func Compose(ctx context.Context, llmClient llm.ClientPort, file *casefile.CaseFile, hs []hypothesis.Hypothesis, checks []verify.Result) *DiagnosisReport {
	target := Target{}
	evidence := make([]Evidence, 0)
	strength := make(map[string]string)
	if file != nil {
		target = Target{Cluster: file.Target.Cluster, Namespace: file.Target.Namespace, Pod: file.Target.Pod}
		for _, ev := range file.Catalog.All() {
			evidence = append(evidence, mapEvidence(ev))
			strength[ev.ID] = ev.Strength
		}
	}
	statusByID := make(map[string]string, len(checks))
	for _, check := range checks {
		statusByID[check.HypothesisID] = check.Status
	}
	reportHypotheses := make([]Hypothesis, 0, len(hs))
	for _, h := range hs {
		status := statusByID[h.ID]
		if status == "" {
			status = "uncertain"
		}
		reportHypotheses = append(reportHypotheses, Hypothesis{
			ID: h.ID, Category: h.Category, Title: h.Title, Status: status,
			SupportingEvidence: h.EvidenceIDs, Rationale: h.Reasoning,
		})
	}

	selected, rootSummary, found := pickRootCause(ctx, llmClient, file, hs, checks, strength)
	if !found {
		selected, rootSummary, found = selectRootCauseDeterministic(hs, checks, strength)
	}
	limitations := casefile.UserFacingLimitations(nil)
	enrichment := EnrichmentInfo{Status: EnrichmentFull}
	if file != nil {
		limitations = casefile.UserFacingLimitations(file.MissingData)
		enrichment = BuildEnrichment(file)
	}
	if !found {
		return &DiagnosisReport{
			Target: target, GeneratedAt: time.Now().UTC(),
			Summary: "已收集证据，但没有支持的根因候选。" ,
			Verdict: VerdictInconclusive,
			RootCause: RootCause{
				Category: "unknown", Title: "诊断无法确定",
				ConfidenceScore: 0.2, ConfidenceLabel: "low",
				Summary: "根据收集并验证的证据，未产生支持的根因候选。" ,
			},
			Evidence: evidence, Hypotheses: reportHypotheses,
			Actions:     []Action{{Priority: "p2", Description: "审查已收集的证据，添加更多证据后重新运行诊断。" }},
			Impact:      Impact{Severity: "unknown", Description: "无法根据已收集的证据确定影响。" },
			Limitations: limitations, Enrichment: enrichment,
		}
	}

	if rootSummary == "" {
		rootSummary = selected.Reasoning
	}
	score := confidenceScore(selected, checks, strength)
	return &DiagnosisReport{
		Target: target, GeneratedAt: time.Now().UTC(),
		Summary: fmt.Sprintf("诊断在证据收集和验证后选择：%s", selected.Title),
		Verdict: verdictFor(score),
		RootCause: RootCause{
			Category: selected.Category, Title: selected.Title,
			ConfidenceScore: score, ConfidenceLabel: confidenceLabel(score),
			Summary: rootSummary, EvidenceIDs: selected.EvidenceIDs,
		},
		Evidence: evidence, Hypotheses: reportHypotheses,
		Actions:     actionsFor(selected),
		Impact:      Impact{Severity: impactSeverity(selected.Category), Description: "目标 Pod 不健康，需要干预"},
		Limitations: limitations, Enrichment: enrichment,
	}
}

func mapEvidence(ev casefile.Evidence) Evidence {
	return Evidence{
		ID: ev.ID, Source: ev.Source, Signal: ev.Signal, Strength: ev.Strength,
		Summary: ev.Summary, Detail: ev.Detail, RawExcerpt: ev.RawExcerpt, Refs: ev.Refs,
	}
}

func confidenceScore(h hypothesis.Hypothesis, checks []verify.Result, strength map[string]string) float64 {
	score := baseConfidence(h.Confidence)
	if hasStrongEvidence(h, strength) {
		score += 0.05
	}
	for _, check := range checks {
		if check.HypothesisID == h.ID && check.Status == "supported" {
			score += 0.05
		}
	}
	if score > 0.95 {
		return 0.95
	}
	return score
}

func baseConfidence(label string) float64 {
	switch strings.ToLower(label) {
	case "high":
		return 0.88
	case "medium":
		return 0.7
	default:
		return 0.45
	}
}

func verdictFor(score float64) Verdict {
	if score >= 0.85 {
		return VerdictConfirmed
	}
	if score >= 0.6 {
		return VerdictLikely
	}
	return VerdictInconclusive
}

func confidenceLabel(score float64) string {
	switch {
	case score >= 0.85:
		return "high"
	case score >= 0.6:
		return "medium"
	default:
		return "low"
	}
}

func hasStrongEvidence(h hypothesis.Hypothesis, strength map[string]string) bool {
	for _, id := range h.EvidenceIDs {
		if strength[id] == casefile.StrengthStrong {
			return true
		}
	}
	return false
}

func actionsFor(h hypothesis.Hypothesis) []Action {
	switch h.Category {
	case "image_pull":
		return []Action{{Priority: "p1", Description: "检查受影响容器的镜像引用、仓库可达性和镜像拉取凭证。" }}
	case "oom_killed":
		return []Action{{Priority: "p1", Description: "检查容器内存使用情况，调整内存限制或应用内存行为。" }}
	case "scheduling":
		return []Action{{Priority: "p1", Description: "审查 Pod 资源请求、节点容量、污点、容忍度、亲和性和调度约束。" }}
	case "volume_mount":
		return []Action{{Priority: "p1", Description: "检查引用的卷、Secret、ConfigMap、PVC 和挂载事件。" }}
	default:
		return []Action{{Priority: "p2", Description: "审查引用的证据，处理选定的根因候选。" }}
	}
}

func impactSeverity(category string) string {
	switch category {
	case "oom_killed", "image_pull", "app_crash":
		return "high"
	default:
		return "medium"
	}
}

func BuildEnrichment(file *casefile.CaseFile) EnrichmentInfo {
	if file == nil {
		return EnrichmentInfo{Status: EnrichmentFull}
	}
	steps := degradedSteps(file.MissingData)
	if len(steps) == 0 {
		return EnrichmentInfo{Status: EnrichmentFull}
	}
	return EnrichmentInfo{
		Status:        EnrichmentDegraded,
		DegradedSteps: steps,
		Message:       "部分 AI 分析步骤暂时不可用。本报告使用已收集的集群证据和确定性回退逻辑。受影响的步骤：" + strings.Join(steps, "，") + "。" ,
	}
}

func degradedSteps(missing []casefile.MissingData) []string {
	seen := make(map[string]struct{})
	var out []string
	for _, item := range missing {
		if !strings.HasPrefix(item.Key, "llm_") {
			continue
		}
		label := strings.TrimPrefix(item.Key, "llm_")
		if _, ok := seen[label]; ok {
			continue
		}
		seen[label] = struct{}{}
		out = append(out, label)
	}
	return out
}
