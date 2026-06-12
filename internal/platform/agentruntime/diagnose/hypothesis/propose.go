package hypothesis

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/kubewise/kubewise/internal/platform/agentruntime/diagnose/casefile"
	"github.com/kubewise/kubewise/internal/platform/agentruntime/diagnose/prompt"
	"github.com/kubewise/kubewise/internal/utils/llm"
)

type proposeOutput struct {
	Hypotheses []Hypothesis `json:"hypotheses"`
}

// Propose asks the LLM for additional falsifiable hypotheses grounded in catalog evidence.
func Propose(ctx context.Context, llmClient llm.ClientPort, file *casefile.CaseFile, structural []Hypothesis, target casefile.Target, obs casefile.Observation) ([]Hypothesis, error) {
	if llmClient == nil || file == nil || file.Catalog == nil {
		return nil, nil
	}
	payload := map[string]any{
		"target":                file.Target,
		"evidence":              file.Catalog.All(),
		"structural_candidates": structural,
	}
	raw, _ := json.Marshal(payload)
	var out proposeOutput
	_, err := llm.CompleteJSON(ctx, llmClient, llm.CompletionRequest{
		Messages: []llm.Message{
			{Role: "system", Content: prompt.HypothesisProposeSystem()},
			{Role: "user", Content: string(raw)},
		},
		Temperature: floatPtr(0.2),
	}, proposeSchema(), &out)
	if err != nil {
		return nil, fmt.Errorf("propose hypotheses: %w", err)
	}
	return sanitizeProposed(out.Hypotheses, file, structural, target, obs), nil
}

func sanitizeProposed(in []Hypothesis, file *casefile.CaseFile, structural []Hypothesis, target casefile.Target, obs casefile.Observation) []Hypothesis {
	existing := make(map[string]struct{}, len(structural))
	for _, h := range structural {
		existing[h.ID] = struct{}{}
	}
	evByID := evidenceByID(file)
	out := make([]Hypothesis, 0, len(in))
	for i, h := range in {
		h.EvidenceIDs = knownEvidenceIDs(file, h.EvidenceIDs)
		if len(h.EvidenceIDs) == 0 {
			continue
		}
		if h.Title == "" && h.Reasoning == "" {
			continue
		}
		if h.ID == "" {
			h.ID = slugID(h.Title, i+1)
		}
		if _, ok := existing[h.ID]; ok {
			h.ID = h.ID + "-llm"
		}
		if _, ok := existing[h.ID]; ok {
			continue
		}
		if overlapsStructural(h, structural, evByID) {
			continue
		}
		if h.Category == "" {
			h.Category = inferCategory(h, evByID)
		}
		if h.Confidence == "" {
			h.Confidence = "medium"
		}
		h = enrichVerifySteps(h, target, obs, evByID)
		out = append(out, h)
		existing[h.ID] = struct{}{}
	}
	return out
}

func overlapsStructural(h Hypothesis, structural []Hypothesis, evByID map[string]casefile.Evidence) bool {
	if citesOnlyToolObservations(h, evByID) {
		return true
	}
	for _, s := range structural {
		if s.Category != "" && h.Category == s.Category && sameEvidenceSet(h.EvidenceIDs, s.EvidenceIDs) {
			return true
		}
		if sameEvidenceSet(h.EvidenceIDs, s.EvidenceIDs) {
			return true
		}
	}
	return false
}

func citesOnlyToolObservations(h Hypothesis, evByID map[string]casefile.Evidence) bool {
	if len(h.EvidenceIDs) == 0 {
		return true
	}
	for _, id := range h.EvidenceIDs {
		ev, ok := evByID[id]
		if !ok || ev.Type != "tool_observation" {
			return false
		}
	}
	return true
}

func sameEvidenceSet(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	seen := make(map[string]struct{}, len(a))
	for _, id := range a {
		seen[id] = struct{}{}
	}
	for _, id := range b {
		if _, ok := seen[id]; !ok {
			return false
		}
	}
	return true
}

func knownEvidenceIDs(file *casefile.CaseFile, ids []string) []string {
	if file == nil || file.Catalog == nil {
		return nil
	}
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if file.Catalog.Exists(id) {
			out = append(out, id)
		}
	}
	return out
}

func evidenceByID(file *casefile.CaseFile) map[string]casefile.Evidence {
	out := make(map[string]casefile.Evidence)
	if file == nil || file.Catalog == nil {
		return out
	}
	for _, ev := range file.Catalog.All() {
		out[ev.ID] = ev
	}
	return out
}

func inferCategory(h Hypothesis, evByID map[string]casefile.Evidence) string {
	for _, id := range h.EvidenceIDs {
		switch evByID[id].Type {
		case casefile.TypeOOMKilled:
			return "oom_killed"
		case casefile.TypeImagePullBackOff:
			return "image_pull"
		case casefile.TypeCrashLoopBackOff, casefile.TypeLogSignal:
			return "app_crash"
		case casefile.TypeFailedScheduling:
			return "scheduling"
		case casefile.TypeFailedMount:
			return "volume_mount"
		case casefile.TypeProbeFailure:
			return "probe_failure"
		}
	}
	return "unknown"
}

func enrichVerifySteps(h Hypothesis, target casefile.Target, obs casefile.Observation, evByID map[string]casefile.Evidence) Hypothesis {
	if len(h.VerifySteps) > 0 {
		return h
	}
	for _, id := range h.EvidenceIDs {
		ev, ok := evByID[id]
		if !ok {
			continue
		}
		switch ev.Type {
		case casefile.TypeCrashLoopBackOff:
			return Hypothesis{ID: h.ID, Category: h.Category, Title: h.Title, Reasoning: h.Reasoning, EvidenceIDs: h.EvidenceIDs, Confidence: h.Confidence, VerifySteps: crashLoopVerifySteps(target, containerFromEvidenceID(id, "ev-crashloop-"))}
		case casefile.TypeLogSignal:
			return Hypothesis{ID: h.ID, Category: h.Category, Title: h.Title, Reasoning: h.Reasoning, EvidenceIDs: h.EvidenceIDs, Confidence: h.Confidence, VerifySteps: crashLoopVerifySteps(target, containerFromLogRef(ev))}
		}
	}
	return h
}

func containerFromLogRef(ev casefile.Evidence) string {
	for _, ref := range ev.Refs {
		if strings.HasPrefix(ref, "logs:") {
			return strings.TrimPrefix(ref, "logs:")
		}
	}
	return ""
}

var slugRe = regexp.MustCompile(`[^a-z0-9]+`)

func slugID(title string, n int) string {
	s := strings.ToLower(strings.TrimSpace(title))
	s = slugRe.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	if s == "" {
		return fmt.Sprintf("hp-llm-%d", n)
	}
	if len(s) > 32 {
		s = s[:32]
	}
	return "hp-llm-" + s
}

func proposeSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"hypotheses": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"id":           map[string]any{"type": "string"},
						"category":     map[string]any{"type": "string"},
						"title":        map[string]any{"type": "string"},
						"reasoning":    map[string]any{"type": "string"},
						"evidence_ids": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
						"confidence":   map[string]any{"type": "string", "enum": []string{"high", "medium", "low"}},
					},
					"required": []string{"title", "reasoning", "evidence_ids"},
				},
			},
		},
		"required": []string{"hypotheses"},
	}
}

func floatPtr(v float64) *float64 { return &v }
