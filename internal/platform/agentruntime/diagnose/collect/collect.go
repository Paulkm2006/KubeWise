package collect

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/kubewise/kubewise/internal/platform/agentruntime/diagnose/casefile"
	"github.com/kubewise/kubewise/internal/platform/agentruntime/diagnose/prompt"
	"github.com/kubewise/kubewise/internal/platform/agentruntime/diagnose/runtime"
	"github.com/kubewise/kubewise/internal/platform/agentruntime/event"
	"github.com/kubewise/kubewise/internal/platform/agentruntime/tool/query"
	"github.com/kubewise/kubewise/internal/platform/agentruntime/tool/troubleshooting"
	toolv2 "github.com/kubewise/kubewise/internal/platform/agentruntime/tool/v2"
	"github.com/kubewise/kubewise/internal/platform/cluster"
	"github.com/kubewise/kubewise/internal/utils/llm"
)

type Profile struct {
	MaxSupplementalTools int
}

type Result struct {
	Observation  casefile.Observation
	Supplemental []toolv2.ToolResult
}

func Run(ctx context.Context, k8s *cluster.Client, llmClient llm.ClientPort, file *casefile.CaseFile, profile Profile, st *runtime.State) (Result, error) {
	target := file.Target
	var out Result
	var err error
	out.Observation, err = baseline(ctx, k8s, target.Namespace, target.Pod)
	if err != nil {
		return out, err
	}
	emitBaselineDone(st, target, out.Observation)

	reg, err := NewReadRegistry(k8s)
	if err != nil {
		return out, err
	}
	if profile.MaxSupplementalTools <= 0 {
		return out, nil
	}

	calls, planErr := planSupplemental(ctx, llmClient, target, out.Observation, reg.Names(), profile.MaxSupplementalTools)
	if planErr != nil {
		runtime.RecordLLMFailure(st, file, "llm_supplemental_collect", "collect", "baseline observation only", planErr)
		return out, nil
	}

	exec := toolv2.NewExecutor(reg)
	step := 2
	for _, call := range calls {
		st.Emit(event.ToolCall{QueryID: st.QueryID, ToolName: call.Tool, Step: step})
		start := time.Now()
		result, callErr := exec.Execute(ctx, call.Tool, call.Args, toolv2.ExecutePolicy{
			AllowedCapabilities: []toolv2.Capability{toolv2.CapabilityRead},
			AllowedTools:        reg.Names(),
			MaxOutputBytes:      12_000,
		})
		if callErr != nil {
			st.Emit(event.ToolFail{
				QueryID: st.QueryID, ToolName: call.Tool, Step: step,
				Elapsed: time.Since(start), Err: callErr.Error(),
			})
			step++
			continue
		}
		out.Supplemental = append(out.Supplemental, result)
		st.Emit(event.ToolDone{
			QueryID: st.QueryID, ToolName: call.Tool, Step: step,
			Elapsed: time.Since(start), Summary: call.Reason,
			Payload: &event.Payload{Kind: event.PayloadKindDiagnosisObservations, Data: result},
		})
		step++
	}
	return out, nil
}

func baseline(ctx context.Context, k8s *cluster.Client, namespace, pod string) (casefile.Observation, error) {
	var out casefile.Observation
	p, err := k8s.GetPod(ctx, namespace, pod)
	if err != nil {
		return out, err
	}
	out.Pod = p
	events, err := k8s.GetEvents(ctx, namespace, pod)
	if err == nil {
		out.Events = events
	}
	out.Logs = map[string]string{}
	for _, cn := range p.Spec.Containers {
		logs, lerr := k8s.GetPodLogs(ctx, namespace, pod, cn.Name, 120)
		if lerr != nil {
			continue
		}
		out.Logs[cn.Name] = logs
	}
	return out, nil
}

func emitBaselineDone(st *runtime.State, target casefile.Target, obs casefile.Observation) {
	if st == nil {
		return
	}
	st.Emit(event.ToolDone{
		QueryID: st.QueryID, ToolName: "collect_baseline", Step: 1,
		Summary: summarizeObservation(obs),
		Payload: &event.Payload{
			Kind: event.PayloadKindDiagnosisObservations,
			Data: map[string]any{
				"namespace": target.Namespace, "pod": target.Pod,
				"event_count": len(obs.Events), "log_containers": len(obs.Logs),
			},
		},
	})
}

func summarizeObservation(obs casefile.Observation) string {
	if obs.Pod == nil {
		return "pod observation unavailable"
	}
	return fmt.Sprintf("phase=%s events=%d logs=%d", obs.Pod.Status.Phase, len(obs.Events), len(obs.Logs))
}

func NewReadRegistry(k8s *cluster.Client) (*toolv2.Registry, error) {
	reg := toolv2.NewRegistry()
	if err := troubleshooting.RegisterTools(reg, k8s); err != nil {
		return nil, err
	}
	for _, t := range []toolv2.Tool{
		query.NewGetPodResourceUsageTool(k8s),
		query.NewGetPodDetailTool(k8s),
	} {
		if err := reg.Register(t); err != nil {
			return nil, err
		}
	}
	return reg, nil
}

type toolCall struct {
	Tool   string         `json:"tool"`
	Args   map[string]any `json:"args"`
	Reason string         `json:"reason"`
}

type supplementalPlan struct {
	Calls []toolCall `json:"calls"`
}

func planSupplemental(ctx context.Context, llmClient llm.ClientPort, target casefile.Target, obs casefile.Observation, allowed []string, max int) ([]toolCall, error) {
	if llmClient == nil || max <= 0 {
		return nil, nil
	}
	payload := map[string]any{
		"target":        target,
		"baseline":      summarizeObservation(obs),
		"allowed_tools": allowed,
		"max_calls":     max,
	}
	raw, _ := json.Marshal(payload)
	var out supplementalPlan
	_, err := llm.CompleteJSON(ctx, llmClient, llm.CompletionRequest{
		Messages: []llm.Message{
			{Role: "system", Content: prompt.SupplementalCollectSystem()},
			{Role: "user", Content: string(raw)},
		},
		Temperature: floatPtr(0.1),
	}, supplementalSchema(), &out)
	if err != nil {
		return nil, fmt.Errorf("plan supplemental collection: %w", err)
	}
	calls := make([]toolCall, 0, len(out.Calls))
	for i, call := range out.Calls {
		if i >= max || call.Tool == "" {
			break
		}
		call.Args = enrichArgs(call.Tool, call.Args, target, obs)
		calls = append(calls, call)
	}
	return calls, nil
}

func enrichArgs(tool string, args map[string]any, target casefile.Target, obs casefile.Observation) map[string]any {
	if args == nil {
		args = map[string]any{}
	}
	set := func(key, value string) {
		if v, ok := args[key].(string); !ok || v == "" {
			args[key] = value
		}
	}
	switch tool {
	case "get_pod_logs", "get_pod_resource_usage", "get_pod_detail":
		set("namespace", target.Namespace)
		set("podName", target.Pod)
	case "get_resource_events":
		set("namespace", target.Namespace)
		set("resourceName", target.Pod)
	}
	return args
}

func supplementalSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"calls": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"tool":   map[string]any{"type": "string"},
						"args":   map[string]any{"type": "object"},
						"reason": map[string]any{"type": "string"},
					},
					"required": []string{"tool", "args", "reason"},
				},
			},
		},
		"required": []string{"calls"},
	}
}

func floatPtr(v float64) *float64 { return &v }
