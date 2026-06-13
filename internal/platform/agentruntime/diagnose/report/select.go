package report

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/kubewise/kubewise/internal/platform/agentruntime/diagnose/casefile"
	"github.com/kubewise/kubewise/internal/platform/agentruntime/diagnose/hypothesis"
	"github.com/kubewise/kubewise/internal/platform/agentruntime/diagnose/prompt"
	"github.com/kubewise/kubewise/internal/platform/agentruntime/diagnose/verify"
	"github.com/kubewise/kubewise/internal/utils/llm"
)

type rootPick struct {
	HypothesisID string  `json:"hypothesis_id"`
	Summary      string  `json:"summary"`
	Confidence   float64 `json:"confidence"`
	Reason       string  `json:"reason"`
}

type rootSelection struct {
	Root rootPick `json:"root"`
}

func pickRootCause(ctx context.Context, llmClient llm.ClientPort, file *casefile.CaseFile, hs []hypothesis.Hypothesis, checks []verify.Result, strength map[string]string) (hypothesis.Hypothesis, string, bool) {
	supported := supportedHypotheses(hs, checks)
	if len(supported) == 0 {
		return hypothesis.Hypothesis{}, "", false
	}
	if len(supported) == 1 {
		return supported[0], supported[0].Reasoning, true
	}
	if llmClient != nil {
		if picked, summary, ok := pickRootWithLLM(ctx, llmClient, file, supported, checks); ok {
			return picked, summary, true
		}
	}
	return selectRootCauseDeterministic(supported, checks, strength)
}

func supportedHypotheses(hs []hypothesis.Hypothesis, checks []verify.Result) []hypothesis.Hypothesis {
	statusByID := statusMap(checks)
	var out []hypothesis.Hypothesis
	for _, h := range hs {
		if len(h.EvidenceIDs) == 0 {
			continue
		}
		if statusByID[h.ID] == "supported" {
			out = append(out, h)
		}
	}
	return out
}

func pickRootWithLLM(ctx context.Context, llmClient llm.ClientPort, file *casefile.CaseFile, supported []hypothesis.Hypothesis, checks []verify.Result) (hypothesis.Hypothesis, string, bool) {
	payload := map[string]any{
		"target":        file.Target,
		"evidence":      file.Catalog.All(),
		"hypotheses":    supported,
		"verifications": checks,
	}
	raw, _ := json.Marshal(payload)
	var out rootSelection
	_, err := llm.CompleteJSON(ctx, llmClient, llm.CompletionRequest{
		Messages: []llm.Message{
			{Role: "system", Content: prompt.RootSelectSystem()},
			{Role: "user", Content: string(raw)},
		},
		Temperature: floatPtr(0.1),
	}, rootSchema(), &out)
	if err != nil || out.Root.HypothesisID == "" {
		return hypothesis.Hypothesis{}, "", false
	}
	for _, h := range supported {
		if h.ID != out.Root.HypothesisID {
			continue
		}
		summary := strings.TrimSpace(out.Root.Summary)
		if summary == "" {
			summary = h.Reasoning
		}
		return h, summary, true
	}
	return hypothesis.Hypothesis{}, "", false
}

func selectRootCauseDeterministic(hs []hypothesis.Hypothesis, checks []verify.Result, strength map[string]string) (hypothesis.Hypothesis, string, bool) {
	statusByID := statusMap(checks)
	bestIdx := -1
	bestScore := -1.0
	for i, h := range hs {
		if statusByID[h.ID] == "refuted" || len(h.EvidenceIDs) == 0 {
			continue
		}
		score := baseConfidence(h.Confidence)
		if hasStrongEvidence(h, strength) {
			score += 0.1
		}
		if statusByID[h.ID] == "supported" {
			score += 0.15
		}
		score += 0.05 * float64(len(h.EvidenceIDs))
		if h.ID != "hp-crashloop" {
			score += 0.08
		}
		if citesLogEvidence(h, strength) {
			score += 0.07
		}
		if score > bestScore {
			bestScore = score
			bestIdx = i
		}
	}
	if bestIdx < 0 {
		return hypothesis.Hypothesis{}, "", false
	}
	h := hs[bestIdx]
	return h, h.Reasoning, true
}

func statusMap(checks []verify.Result) map[string]string {
	out := make(map[string]string, len(checks))
	for _, check := range checks {
		out[check.HypothesisID] = check.Status
	}
	return out
}

func citesLogEvidence(h hypothesis.Hypothesis, strength map[string]string) bool {
	for _, id := range h.EvidenceIDs {
		if strings.HasPrefix(id, "ev-log") {
			return true
		}
		if strength[id] == casefile.StrengthModerate && strings.Contains(id, "log") {
			return true
		}
	}
	return false
}

func rootSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"root": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"hypothesis_id": map[string]any{"type": "string"},
					"summary":       map[string]any{"type": "string"},
					"confidence":    map[string]any{"type": "number"},
					"reason":        map[string]any{"type": "string"},
				},
				"required": []string{"hypothesis_id", "summary", "reason"},
			},
		},
		"required": []string{"root"},
	}
}

func floatPtr(v float64) *float64 { return &v }
