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
				ID: "hp-oom", Category: "oom_killed", Title: "容器内存不足或 limit 偏低",
				Reasoning:   "容器被 OOMKilled，通常说明运行时内存压力过高，或 memory limit 设置不足",
				EvidenceIDs: []string{ev.ID}, Confidence: "high",
				VerifySteps: []VerifyStep{{
					Tool:        "get_pod_resource_usage",
					Args:        map[string]any{"namespace": target.Namespace, "podName": target.Pod},
					MustContain: []string{"memory", "limit"},
				}},
			})
		case casefile.TypeImagePullBackOff:
			out = append(out, Hypothesis{
				ID: "hp-image-pull", Category: "image_pull", Title: "镜像引用、标签或拉取凭据异常",
				Reasoning:   "容器进入 ImagePullBackOff/ErrImagePull，常见原因是 image 引用错误、tag 不存在或 registry 认证失败",
				EvidenceIDs: []string{ev.ID}, Confidence: "high",
			})
		case casefile.TypeCrashLoopBackOff:
			container := containerFromEvidenceID(ev.ID, "ev-crashloop-")
			out = append(out, Hypothesis{
				ID: "hp-crashloop", Category: "app_crash", Title: "应用启动后崩溃或快速退出",
				Reasoning:   "容器反复重启并进入 CrashLoopBackOff，需要结合启动日志确认应用异常",
				EvidenceIDs: []string{ev.ID}, Confidence: "medium",
				VerifySteps: crashLoopVerifySteps(target, container),
			})
		case casefile.TypeFailedScheduling:
			out = append(out, Hypothesis{
				ID: "hp-scheduling", Category: "scheduling", Title: "资源不足或调度约束无法满足",
				Reasoning:   "scheduler 对该 Pod 报告 FailedScheduling，说明当前节点资源或调度约束无法满足",
				EvidenceIDs: []string{ev.ID}, Confidence: "high",
				VerifySteps: schedulingVerifySteps(target, obs),
			})
		case casefile.TypeFailedMount:
			out = append(out, Hypothesis{
				ID: "hp-mount", Category: "volume_mount", Title: "Volume 挂载失败",
				Reasoning:   "kubelet 报告 volume mount 失败，需检查 Secret、ConfigMap、PVC 或挂载路径相关事件",
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
				ID: "hp-probe", Category: "probe_failure", Title: "健康检查探针配置不匹配或应用未就绪",
				Reasoning:   "探针失败通常来自 endpoint、timeout、startupProbe 时序或应用健康状态不匹配",
				EvidenceIDs: []string{ev.ID}, Confidence: "medium",
			})
		}
	}
	return uniqueByID(out)
}
