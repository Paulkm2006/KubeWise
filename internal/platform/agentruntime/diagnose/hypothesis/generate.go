package hypothesis

import (
	"context"

	"github.com/kubewise/kubewise/internal/platform/agentruntime/diagnose/casefile"
	"github.com/kubewise/kubewise/internal/utils/llm"
)

type Hypothesis struct {
	ID          string       `json:"id"`
	Category    string       `json:"category"`
	Title       string       `json:"title"`
	Reasoning   string       `json:"reasoning"`
	EvidenceIDs []string     `json:"evidence_ids"`
	Confidence  string       `json:"confidence"`
	VerifySteps []VerifyStep `json:"verify_steps"`
}

type VerifyStep struct {
	Tool        string         `json:"tool"`
	Args        map[string]any `json:"args"`
	MustContain []string       `json:"must_contain,omitempty"`
	LLMJudge    bool           `json:"llm_judge,omitempty"`
	Expectation string         `json:"expectation,omitempty"`
}

func Generate(ctx context.Context, llmClient llm.ClientPort, file *casefile.CaseFile, evs []casefile.Evidence, target casefile.Target, obs casefile.Observation) []Hypothesis {
	structural := structuralFromEvidence(evs, target, obs)
	proposed, err := Propose(ctx, llmClient, file, structural, target, obs)
	if err != nil && file != nil {
		file.AddMissing("llm_hypothesis_proposal", err.Error())
	}
	return uniqueByID(append(structural, proposed...))
}

func crashLoopVerifySteps(target casefile.Target, container string) []VerifyStep {
	args := map[string]any{"namespace": target.Namespace, "podName": target.Pod, "tailLines": 100}
	if container != "" {
		args["container"] = container
	}
	return []VerifyStep{{
		Tool: "get_pod_logs", Args: args,
		Expectation: "日志应显示重启前的应用错误或崩溃",
		LLMJudge:    true,
	}}
}

func schedulingVerifySteps(target casefile.Target, obs casefile.Observation) []VerifyStep {
	_ = obs
	return []VerifyStep{{
		Tool: "get_resource_events",
		Args: map[string]any{
			"namespace": target.Namespace, "resourceName": target.Pod,
		},
		MustContain: []string{"FailedScheduling"},
	}}
}

func containerFromEvidenceID(id, prefix string) string {
	if len(id) > len(prefix) {
		return id[len(prefix):]
	}
	return ""
}

func uniqueByID(in []Hypothesis) []Hypothesis {
	seen := make(map[string]struct{}, len(in))
	out := make([]Hypothesis, 0, len(in))
	for _, h := range in {
		if _, ok := seen[h.ID]; ok {
			continue
		}
		seen[h.ID] = struct{}{}
		out = append(out, h)
	}
	return out
}
