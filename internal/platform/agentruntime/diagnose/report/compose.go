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
			Summary: "Evidence was collected, but no supported root cause candidate was available.",
			Verdict: VerdictInconclusive,
			RootCause: RootCause{
				Category: "unknown", Title: "Inconclusive diagnosis",
				ConfidenceScore: 0.2, ConfidenceLabel: "low",
				Summary: "No supported root cause candidate was produced from collected and verified evidence.",
			},
			Evidence: evidence, Hypotheses: reportHypotheses,
			Actions:     []Action{{Priority: "p2", Description: "Review collected evidence and rerun diagnosis with additional evidence."}},
			Impact:      Impact{Severity: "unknown", Description: "Impact could not be determined from collected evidence."},
			Limitations: limitations, Enrichment: enrichment,
		}
	}

	if rootSummary == "" {
		rootSummary = selected.Reasoning
	}
	score := confidenceScore(selected, checks, strength)
	return &DiagnosisReport{
		Target: target, GeneratedAt: time.Now().UTC(),
		Summary: fmt.Sprintf("Diagnosis selected %s after evidence collection and verification.", selected.Title),
		Verdict: verdictFor(score),
		RootCause: RootCause{
			Category: selected.Category, Title: selected.Title,
			ConfidenceScore: score, ConfidenceLabel: confidenceLabel(score),
			Summary: rootSummary, EvidenceIDs: selected.EvidenceIDs,
		},
		Evidence: evidence, Hypotheses: reportHypotheses,
		Actions:     actionsFor(selected),
		Impact:      Impact{Severity: impactSeverity(selected.Category), Description: "target pod is unhealthy and requires intervention"},
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
		return []Action{{Priority: "p1", Description: "Verify the image reference, registry reachability, and image pull credentials for the affected container."}}
	case "oom_killed":
		return []Action{{Priority: "p1", Description: "Inspect container memory usage and adjust memory limits or application memory behavior."}}
	case "scheduling":
		return []Action{{Priority: "p1", Description: "Review pod resource requests, node capacity, taints, tolerations, affinity, and scheduling constraints."}}
	case "volume_mount":
		return []Action{{Priority: "p1", Description: "Inspect referenced volumes, secrets, config maps, PVCs, and mount events."}}
	default:
		return []Action{{Priority: "p2", Description: "Review the cited evidence and remediate the selected root cause candidate."}}
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
		Message:       "Some AI analysis steps were temporarily unavailable. This report used collected cluster evidence and deterministic fallback logic. Affected steps: " + strings.Join(steps, ", ") + ".",
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
