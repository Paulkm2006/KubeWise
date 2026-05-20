package recovery

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	corev1 "k8s.io/api/core/v1"

	"github.com/kubewise/kubewise/pkg/agent/deploy/plan"
	"github.com/kubewise/kubewise/pkg/catalog"
	"github.com/kubewise/kubewise/pkg/helm"
	"github.com/kubewise/kubewise/pkg/k8s"
)

const (
	maxSnapshotPods   = 20
	maxSnapshotEvents = 15
	maxPodsForEvents  = 5
)

// StatusClient reads Helm release status.
type StatusClient interface {
	Status(ctx context.Context, releaseName, namespace string) (*helm.Release, error)
}

// BuildDiagnosticsSnapshot collects deploy failure context for the recovery LLM.
func BuildDiagnosticsSnapshot(
	ctx context.Context,
	deployErr error,
	releaseName, namespace string,
	chart *catalog.ChartInfo,
	helmClient StatusClient,
	k8sClient *k8s.Client,
) string {
	snap := map[string]any{
		"schemaVersion": "1",
		"deployError":   deployErr.Error(),
		"release":       plan.SanitizeReleaseName(releaseName),
		"namespace":     namespace,
		"chart":         chart.ChartName,
	}

	if helmClient != nil {
		if rel, err := helmClient.Status(ctx, plan.SanitizeReleaseName(releaseName), namespace); err == nil && rel != nil {
			snap["helmRelease"] = map[string]any{
				"name":      rel.Name,
				"namespace": rel.Namespace,
				"status":    rel.Status,
				"chart":     rel.Chart,
			}
		}
	}

	if k8sClient != nil && namespace != "" {
		if pods, err := k8sClient.ListPods(ctx, namespace); err == nil {
			snap["pods"] = summarizePods(pods, maxSnapshotPods)
			snap["podEvents"] = collectUnhealthyPodEvents(ctx, namespace, pods, k8sClient)
		}
	}

	b, _ := json.MarshalIndent(snap, "", "  ")
	return string(b)
}

func summarizePods(pods []corev1.Pod, limit int) []map[string]string {
	sort.Slice(pods, func(i, j int) bool {
		return pods[i].Name < pods[j].Name
	})
	if len(pods) > limit {
		pods = pods[:limit]
	}
	out := make([]map[string]string, 0, len(pods))
	for _, p := range pods {
		phase := string(p.Status.Phase)
		ready := "0/0"
		if len(p.Status.ContainerStatuses) > 0 {
			readyCount := 0
			for _, cs := range p.Status.ContainerStatuses {
				if cs.Ready {
					readyCount++
				}
			}
			ready = fmt.Sprintf("%d/%d", readyCount, len(p.Status.ContainerStatuses))
		}
		reason := ""
		if len(p.Status.ContainerStatuses) > 0 && p.Status.ContainerStatuses[0].State.Waiting != nil {
			reason = p.Status.ContainerStatuses[0].State.Waiting.Reason
		}
		out = append(out, map[string]string{
			"name":   p.Name,
			"phase":  phase,
			"ready":  ready,
			"reason": reason,
		})
	}
	return out
}

type podEventLister interface {
	GetEvents(ctx context.Context, namespace, involvedObjectName string) ([]corev1.Event, error)
}

func collectUnhealthyPodEvents(ctx context.Context, namespace string, pods []corev1.Pod, k8s podEventLister) []map[string]string {
	var unhealthy []corev1.Pod
	for _, p := range pods {
		if p.Status.Phase != corev1.PodRunning && p.Status.Phase != corev1.PodSucceeded {
			unhealthy = append(unhealthy, p)
		}
	}
	sort.Slice(unhealthy, func(i, j int) bool { return unhealthy[i].Name < unhealthy[j].Name })
	if len(unhealthy) > maxPodsForEvents {
		unhealthy = unhealthy[:maxPodsForEvents]
	}

	var out []map[string]string
	for _, p := range unhealthy {
		events, err := k8s.GetEvents(ctx, namespace, p.Name)
		if err != nil || len(events) == 0 {
			continue
		}
		sort.Slice(events, func(i, j int) bool {
			return events[i].LastTimestamp.After(events[j].LastTimestamp.Time)
		})
		limit := len(events)
		if limit > maxSnapshotEvents {
			limit = maxSnapshotEvents
		}
		for _, ev := range events[:limit] {
			out = append(out, map[string]string{
				"pod":     p.Name,
				"type":    ev.Type,
				"reason":  ev.Reason,
				"message": truncate(ev.Message, 200),
			})
		}
	}
	return out
}

func truncate(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
