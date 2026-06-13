package evidence

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/kubewise/kubewise/internal/platform/agentruntime/diagnose/casefile"
	"github.com/kubewise/kubewise/internal/platform/agentruntime/diagnose/collect"
	"github.com/kubewise/kubewise/internal/platform/agentruntime/diagnose/prompt"
	toolv2 "github.com/kubewise/kubewise/internal/platform/agentruntime/tool/v2"
	"github.com/kubewise/kubewise/internal/utils/llm"
)

func Build(ctx context.Context, file *casefile.CaseFile, collected collect.Result, llmClient llm.ClientPort) []casefile.Evidence {
	if file == nil {
		return nil
	}
	file.Observation = collected.Observation
	out := deterministic(collected.Observation)
	out = append(out, supplemental(collected.Supplemental)...)
	if len(collected.Observation.Logs) == 0 {
		file.AddMissing("container_logs", "container logs were unavailable or empty")
	} else {
		logEvs, err := fromLogs(ctx, collected.Observation.Logs, llmClient)
		if err != nil {
			file.AddMissing("llm_log_evidence", err.Error())
		} else {
			out = append(out, logEvs...)
		}
	}
	file.AddMissing("metrics", "metrics-server usage data was not verified in this pass")
	return dedup(out)
}

func deterministic(obs casefile.Observation) []casefile.Evidence {
	if obs.Pod == nil {
		return nil
	}
	out := make([]casefile.Evidence, 0, 6)
	add := func(ev casefile.Evidence) { out = append(out, ev) }

	for _, cs := range obs.Pod.Status.ContainerStatuses {
		if cs.LastTerminationState.Terminated != nil && cs.LastTerminationState.Terminated.Reason == "OOMKilled" {
			term := cs.LastTerminationState.Terminated
			add(casefile.Evidence{
				ID: "ev-oom-" + cs.Name, Type: casefile.TypeOOMKilled, Source: "container_status", Signal: "OOMKilled",
				Strength: casefile.StrengthStrong, Title: "Container terminated with OOMKilled",
				Summary: fmt.Sprintf("container %s was OOMKilled", cs.Name),
				Detail:  fmt.Sprintf("exitCode=%d", term.ExitCode),
				Refs:    []string{"pod.status.containerStatuses.lastTerminationState"},
			})
		}
		if cs.State.Waiting != nil && cs.State.Waiting.Reason == "CrashLoopBackOff" {
			add(casefile.Evidence{
				ID: "ev-crashloop-" + cs.Name, Type: casefile.TypeCrashLoopBackOff, Source: "container_status", Signal: "CrashLoopBackOff",
				Strength: casefile.StrengthStrong, Title: "Container in CrashLoopBackOff",
				Summary: fmt.Sprintf("container %s is restarting repeatedly", cs.Name),
				Detail:  cs.State.Waiting.Message,
				Refs:    []string{"pod.status.containerStatuses.state.waiting"},
			})
		}
		if cs.State.Waiting != nil && cs.State.Waiting.Reason == "ImagePullBackOff" {
			add(casefile.Evidence{
				ID: "ev-imagepull-" + cs.Name, Type: casefile.TypeImagePullBackOff, Source: "container_status", Signal: "ImagePullBackOff",
				Strength: casefile.StrengthStrong, Title: "Container image pull failed",
				Summary: fmt.Sprintf("container %s is in ImagePullBackOff", cs.Name),
				Detail:  cs.State.Waiting.Message,
				Refs:    []string{"pod.status.containerStatuses.state.waiting"},
			})
		}
	}

	for _, ev := range obs.Events {
		switch ev.Reason {
		case "FailedScheduling":
			add(casefile.Evidence{
				ID: "ev-failed-scheduling", Type: casefile.TypeFailedScheduling, Source: "kubernetes_event", Signal: "FailedScheduling",
				Strength: casefile.StrengthStrong, Title: "Pod scheduling failed",
				Summary: "scheduler cannot place this pod", Detail: ev.Message,
				Refs: []string{"events.reason=FailedScheduling"},
			})
		case "FailedMount":
			add(casefile.Evidence{
				ID: "ev-failed-mount", Type: casefile.TypeFailedMount, Source: "kubernetes_event", Signal: "FailedMount",
				Strength: casefile.StrengthStrong, Title: "Pod volume mount failed",
				Summary: "volume mount/setup failed", Detail: ev.Message,
				Refs: []string{"events.reason=FailedMount"},
			})
		case "Unhealthy":
			if strings.Contains(strings.ToLower(ev.Message), "probe") {
				add(casefile.Evidence{
					ID: "ev-probe-failure", Type: casefile.TypeProbeFailure, Source: "kubernetes_event", Signal: "Unhealthy",
					Strength: casefile.StrengthModerate, Title: "Probe check failing",
					Summary: "liveness/readiness probe failure detected", Detail: ev.Message,
					Refs: []string{"events.reason=Unhealthy"},
				})
			}
		}
	}
	return out
}

func supplemental(results []toolv2.ToolResult) []casefile.Evidence {
	out := make([]casefile.Evidence, 0, len(results))
	for _, result := range results {
		summary := strings.TrimSpace(result.Display)
		if summary == "" {
			summary = fmt.Sprintf("%s returned data", result.Meta.ToolName)
		}
		if len(summary) > 240 {
			summary = summary[:240] + "..."
		}
		out = append(out, casefile.Evidence{
			ID:         "ev_tool_" + result.Meta.ToolName,
			Type:       "tool_observation",
			Source:     "tool:" + result.Meta.ToolName,
			Strength:   casefile.StrengthWeak,
			Title:      result.Meta.ToolName + " observation",
			Summary:    summary,
			Detail:     strings.TrimSpace(result.Display),
			RawExcerpt: strings.TrimSpace(result.Display),
			Refs:       []string{"tool:" + result.Meta.ToolName},
		})
	}
	return out
}

type logEvidenceItem struct {
	ID         string `json:"id"`
	Container  string `json:"container"`
	Signal     string `json:"signal"`
	Summary    string `json:"summary"`
	RawExcerpt string `json:"raw_excerpt"`
}

type logEvidenceOutput struct {
	Items []logEvidenceItem `json:"items"`
}

func fromLogs(ctx context.Context, logs map[string]string, llmClient llm.ClientPort) ([]casefile.Evidence, error) {
	if len(logs) == 0 {
		return nil, nil
	}
	if llmClient == nil {
		return heuristicLogs(logs), nil
	}
	payload := map[string]any{"logs": logs}
	raw, _ := json.Marshal(payload)
	var out logEvidenceOutput
	_, err := llm.CompleteJSON(ctx, llmClient, llm.CompletionRequest{
		Messages: []llm.Message{
			{Role: "system", Content: prompt.LogEvidenceSystem()},
			{Role: "user", Content: string(raw)},
		},
		Temperature: floatPtr(0.1),
	}, logSchema(), &out)
	if err != nil {
		return heuristicLogs(logs), fmt.Errorf("extract log evidence: %w", err)
	}
	evs := make([]casefile.Evidence, 0, len(out.Items))
	for i, item := range out.Items {
		id := item.ID
		if id == "" {
			id = fmt.Sprintf("ev-log-%d", i+1)
		}
		if item.RawExcerpt == "" || !logContains(logs, item.RawExcerpt) {
			continue
		}
		evs = append(evs, casefile.Evidence{
			ID: id, Type: casefile.TypeLogSignal, Source: "container_log", Signal: item.Signal,
			Strength: casefile.StrengthModerate, Title: "Log signal in " + item.Container,
			Summary: item.Summary, Detail: item.RawExcerpt, RawExcerpt: item.RawExcerpt,
			Refs: []string{"logs:" + item.Container},
		})
	}
	if len(evs) == 0 {
		return heuristicLogs(logs), nil
	}
	return evs, nil
}

func heuristicLogs(logs map[string]string) []casefile.Evidence {
	patterns := []struct {
		signal  string
		needles []string
		title   string
	}{
		{"oom", []string{"out of memory", "oom", "killed"}, "Log indicates memory pressure"},
		{"connection_refused", []string{"connection refused", "connect: connection refused"}, "Log indicates connection refused"},
		{"panic", []string{"panic:", "fatal error"}, "Log indicates application panic/fatal error"},
		{"class_not_found", []string{"classnotfoundexception", "no such file or directory"}, "Log indicates missing dependency or file"},
	}
	var out []casefile.Evidence
	idx := 0
	for container, text := range logs {
		lower := strings.ToLower(text)
		for _, p := range patterns {
			for _, needle := range p.needles {
				if !strings.Contains(lower, needle) {
					continue
				}
				excerpt := excerptAround(text, needle)
				idx++
				out = append(out, casefile.Evidence{
					ID: fmt.Sprintf("ev-log-heuristic-%d", idx), Type: casefile.TypeLogSignal,
					Source: "container_log", Signal: p.signal, Strength: casefile.StrengthModerate,
					Title: p.title, Summary: p.title + " in " + container,
					Detail: excerpt, RawExcerpt: excerpt, Refs: []string{"logs:" + container},
				})
				break
			}
		}
	}
	return out
}

func logContains(logs map[string]string, excerpt string) bool {
	excerpt = strings.TrimSpace(excerpt)
	if excerpt == "" {
		return false
	}
	for _, text := range logs {
		if strings.Contains(text, excerpt) {
			return true
		}
	}
	return false
}

func excerptAround(text, needle string) string {
	lower := strings.ToLower(text)
	idx := strings.Index(lower, strings.ToLower(needle))
	if idx < 0 {
		if len(text) > 200 {
			return text[:200] + "..."
		}
		return text
	}
	start := idx - 40
	if start < 0 {
		start = 0
	}
	end := idx + len(needle) + 120
	if end > len(text) {
		end = len(text)
	}
	return strings.TrimSpace(text[start:end])
}

func logSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"items": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"id":          map[string]any{"type": "string"},
						"container":   map[string]any{"type": "string"},
						"signal":      map[string]any{"type": "string"},
						"summary":     map[string]any{"type": "string"},
						"raw_excerpt": map[string]any{"type": "string"},
					},
					"required": []string{"container", "summary", "raw_excerpt"},
				},
			},
		},
		"required": []string{"items"},
	}
}

func dedup(in []casefile.Evidence) []casefile.Evidence {
	seen := make(map[string]struct{}, len(in))
	out := make([]casefile.Evidence, 0, len(in))
	for _, ev := range in {
		if _, ok := seen[ev.ID]; ok {
			continue
		}
		seen[ev.ID] = struct{}{}
		out = append(out, ev)
	}
	return out
}

func floatPtr(v float64) *float64 { return &v }
