package verify

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/kubewise/kubewise/internal/platform/agentruntime/diagnose/casefile"
	"github.com/kubewise/kubewise/internal/platform/agentruntime/diagnose/hypothesis"
	"github.com/kubewise/kubewise/internal/platform/agentruntime/diagnose/prompt"
	"github.com/kubewise/kubewise/internal/platform/agentruntime/diagnose/runtime"
	"github.com/kubewise/kubewise/internal/platform/agentruntime/event"
	toolv2 "github.com/kubewise/kubewise/internal/platform/agentruntime/tool/v2"
	"github.com/kubewise/kubewise/internal/utils/llm"
)

type StepResult struct {
	Tool    string `json:"tool"`
	Passed  bool   `json:"passed"`
	Reason  string `json:"reason"`
	Summary string `json:"summary,omitempty"`
}

type Result struct {
	HypothesisID string       `json:"hypothesis_id"`
	Status       string       `json:"status"`
	Steps        []StepResult `json:"steps"`
	Reason       string       `json:"reason"`
}

func Run(ctx context.Context, exec *toolv2.Executor, allowed []string, file *casefile.CaseFile, hs []hypothesis.Hypothesis, llmClient llm.ClientPort, st *runtime.State) []Result {
	out := make([]Result, 0, len(hs))
	stepNum := 1
	for _, h := range hs {
		res := verifyOne(ctx, exec, allowed, h, llmClient, st, &stepNum)
		out = append(out, res)
	}
	return out
}

func verifyOne(ctx context.Context, exec *toolv2.Executor, allowed []string, h hypothesis.Hypothesis, llmClient llm.ClientPort, st *runtime.State, stepNum *int) Result {
	if len(h.VerifySteps) == 0 {
		if len(h.EvidenceIDs) > 0 {
			return Result{
				HypothesisID: h.ID, Status: "supported",
				Reason: "deterministic evidence sufficient; no additional verification path required",
			}
		}
		return Result{HypothesisID: h.ID, Status: "uncertain", Reason: "no evidence and no verification path"}
	}

	res := Result{HypothesisID: h.ID, Status: "supported"}
	for _, step := range h.VerifySteps {
		st.Emit(event.ToolCall{QueryID: st.QueryID, ToolName: step.Tool, Step: *stepNum})
		start := time.Now()
		toolResult, err := exec.Execute(ctx, step.Tool, step.Args, toolv2.ExecutePolicy{
			AllowedCapabilities: []toolv2.Capability{toolv2.CapabilityRead},
			AllowedTools:        allowed,
			MaxOutputBytes:      12_000,
		})
		if err != nil {
			st.Emit(event.ToolFail{
				QueryID: st.QueryID, ToolName: step.Tool, Step: *stepNum,
				Elapsed: time.Since(start), Err: err.Error(),
			})
			res.Steps = append(res.Steps, StepResult{Tool: step.Tool, Passed: false, Reason: err.Error()})
			res.Status = "uncertain"
			res.Reason = "verification tool failed"
			*stepNum++
			continue
		}
		output := strings.TrimSpace(toolResult.Display)
		passed, reason := evaluateStep(output, step, llmClient, ctx)
		res.Steps = append(res.Steps, StepResult{Tool: step.Tool, Passed: passed, Reason: reason, Summary: truncate(output, 180)})
		st.Emit(event.ToolDone{
			QueryID: st.QueryID, ToolName: step.Tool, Step: *stepNum,
			Elapsed: time.Since(start), Summary: reason,
		})
		*stepNum++
		if !passed {
			res.Status = "refuted"
			res.Reason = reason
			return res
		}
	}
	if res.Status == "supported" {
		res.Reason = "verification path observations matched expectations"
	}
	return res
}

func evaluateStep(output string, step hypothesis.VerifyStep, llmClient llm.ClientPort, ctx context.Context) (bool, string) {
	lower := strings.ToLower(output)
	for _, needle := range step.MustContain {
		if !strings.Contains(lower, strings.ToLower(needle)) {
			return false, fmt.Sprintf("expected output to contain %q", needle)
		}
	}
	if step.LLMJudge && step.Expectation != "" {
		if llmClient == nil {
			if output == "" {
				return false, "expected non-empty tool output for LLM judgment"
			}
			return true, "LLM unavailable; accepted non-empty verification output"
		}
		ok, reason, err := llmJudge(ctx, llmClient, output, step.Expectation)
		if err != nil {
			return output != "", "LLM judgment failed; accepted non-empty output: " + err.Error()
		}
		return ok, reason
	}
	if len(step.MustContain) == 0 && output == "" {
		return false, "verification tool returned empty output"
	}
	return true, "verification expectations met"
}

type judgeOutput struct {
	Matches bool   `json:"matches"`
	Reason  string `json:"reason"`
}

func llmJudge(ctx context.Context, llmClient llm.ClientPort, output, expectation string) (bool, string, error) {
	payload := map[string]any{"tool_output": output, "expectation": expectation}
	raw, _ := json.Marshal(payload)
	var out judgeOutput
	_, err := llm.CompleteJSON(ctx, llmClient, llm.CompletionRequest{
		Messages: []llm.Message{
			{Role: "system", Content: prompt.VerifyJudgeSystem()},
			{Role: "user", Content: string(raw)},
		},
		Temperature: floatPtr(0.05),
	}, map[string]any{
		"type": "object",
		"properties": map[string]any{
			"matches": map[string]any{"type": "boolean"},
			"reason":  map[string]any{"type": "string"},
		},
		"required": []string{"matches", "reason"},
	}, &out)
	if err != nil {
		return false, "", err
	}
	return out.Matches, out.Reason, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

func floatPtr(v float64) *float64 { return &v }
