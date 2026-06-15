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
				ID: "hp-oom", Category: "oom_killed", Title: "内存限制过低",
				Reasoning:   "容器因 OOMKilled 被终止，通常表示内存不足或限制过低",
				EvidenceIDs: []string{ev.ID}, Confidence: "high",
				VerifySteps: []VerifyStep{{
					Tool:        "get_pod_resource_usage",
					Args:        map[string]any{"namespace": target.Namespace, "podName": target.Pod},
					MustContain: []string{"memory", "limit"},
				}},
			})
		case casefile.TypeImagePullBackOff:
			out = append(out, Hypothesis{
				ID: "hp-image-pull", Category: "image_pull", Title: "镜像拉取授权/标签问题",
				Reasoning:   "容器镜像拉取正在回退，可能是镜像引用错误或认证问题",
				EvidenceIDs: []string{ev.ID}, Confidence: "high",
			})
		case casefile.TypeCrashLoopBackOff:
			container := containerFromEvidenceID(ev.ID, "ev-crashloop-")
			out = append(out, Hypothesis{
				ID: "hp-crashloop", Category: "app_crash", Title: "应用启动崩溃",
				Reasoning:   "容器反复重启并处于 CrashLoopBackOff 状态",
				EvidenceIDs: []string{ev.ID}, Confidence: "medium",
				VerifySteps: crashLoopVerifySteps(target, container),
			})
		case casefile.TypeFailedScheduling:
			out = append(out, Hypothesis{
				ID: "hp-scheduling", Category: "scheduling", Title: "可调度资源/约束不足",
				Reasoning:   "调度器报告该 Pod 调度失败",
				EvidenceIDs: []string{ev.ID}, Confidence: "high",
				VerifySteps: schedulingVerifySteps(target, obs),
			})
		case casefile.TypeFailedMount:
			out = append(out, Hypothesis{
				ID: "hp-mount", Category: "volume_mount", Title: "卷挂载问题",
				Reasoning:   "kubelet 报告卷挂载/设置失败",
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
				ID: "hp-probe", Category: "probe_failure", Title: "健康探针配置错误或应用不健康",
				Reasoning:   "探针失败表明端点/超时/启动不匹配或服务不稳定",
				EvidenceIDs: []string{ev.ID}, Confidence: "medium",
			})
		}
	}
	return uniqueByID(out)
}
