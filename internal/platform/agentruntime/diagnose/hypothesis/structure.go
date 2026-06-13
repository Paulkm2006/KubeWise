package hypothesis

import (
	"github.com/kubewise/kubewise/internal/platform/agentruntime/diagnose/casefile"
)

func structuralFromEvidence(evs []casefile.Evidence, target casefile.Target, obs casefile.Observation) []Hypothesis {
	out := make([]Hypothesis, 0, len(evs))
	for _, ev := range evs {
		switch ev.Type {
		case casefile.TypeOOMKilled:
			out = append(out, Hypothesis{
				ID: "hp-oom", Category: "oom_killed", Title: "Memory limit too low",
				Reasoning:   "container was terminated by OOMKilled, usually indicating memory pressure or limit too low",
				EvidenceIDs: []string{ev.ID}, Confidence: "high",
				VerifySteps: []VerifyStep{{
					Tool:        "get_pod_resource_usage",
					Args:        map[string]any{"namespace": target.Namespace, "podName": target.Pod},
					MustContain: []string{"memory", "limit"},
				}},
			})
		case casefile.TypeImagePullBackOff:
			out = append(out, Hypothesis{
				ID: "hp-image-pull", Category: "image_pull", Title: "Image pull authorization/tag issue",
				Reasoning:   "container image pull is backing off, likely due to bad image ref or auth problem",
				EvidenceIDs: []string{ev.ID}, Confidence: "high",
			})
		case casefile.TypeCrashLoopBackOff:
			container := containerFromEvidenceID(ev.ID, "ev-crashloop-")
			out = append(out, Hypothesis{
				ID: "hp-crashloop", Category: "app_crash", Title: "Application startup crash",
				Reasoning:   "container restarts repeatedly and remains in CrashLoopBackOff",
				EvidenceIDs: []string{ev.ID}, Confidence: "medium",
				VerifySteps: crashLoopVerifySteps(target, container),
			})
		case casefile.TypeFailedScheduling:
			out = append(out, Hypothesis{
				ID: "hp-scheduling", Category: "scheduling", Title: "Insufficient schedulable resources/constraints",
				Reasoning:   "scheduler reports failed scheduling for this pod",
				EvidenceIDs: []string{ev.ID}, Confidence: "high",
				VerifySteps: schedulingVerifySteps(target, obs),
			})
		case casefile.TypeFailedMount:
			out = append(out, Hypothesis{
				ID: "hp-mount", Category: "volume_mount", Title: "Volume mounting issue",
				Reasoning:   "kubelet reports volume mount/setup failure",
				EvidenceIDs: []string{ev.ID}, Confidence: "high",
				VerifySteps: []VerifyStep{{
					Tool: "get_resource_events",
					Args: map[string]any{
						"namespace": target.Namespace, "resourceName": target.Pod,
					},
					MustContain: []string{"FailedMount"},
				}},
			})
		case casefile.TypeProbeFailure:
			out = append(out, Hypothesis{
				ID: "hp-probe", Category: "probe_failure", Title: "Health probe misconfigured or app unhealthy",
				Reasoning:   "probe failures indicate endpoint/timeout/startup mismatch or unstable service",
				EvidenceIDs: []string{ev.ID}, Confidence: "medium",
			})
		}
	}
	return uniqueByID(out)
}
