package cluster

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/kubewise/kubewise/internal/platform/cluster"
)

func listIssues(ctx context.Context, manager *cluster.ClusterClientManager, name string) ([]cluster.Issue, error) {
	if name == "" {
		return nil, ErrNameRequired
	}

	cc, err := manager.GetClient(ctx, name)
	if err != nil {
		return nil, ErrNotFound
	}

	cs := cc.Clientset()
	if cs == nil {
		return nil, ErrOffline
	}

	pods, err := cs.CoreV1().Pods(metav1.NamespaceAll).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list pods: %w", err)
	}

	var issues []cluster.Issue
	for _, p := range pods.Items {
		if p.Status.Phase == corev1.PodRunning || p.Status.Phase == corev1.PodSucceeded {
			continue
		}
		severity := "low"
		if p.Status.Phase == corev1.PodPending {
			severity = "medium"
		}
		for _, cs := range p.Status.ContainerStatuses {
			if cs.State.Waiting != nil && cs.State.Waiting.Reason == "CrashLoopBackOff" {
				severity = "high"
				break
			}
		}

		age := time.Since(p.CreationTimestamp.Time).Round(time.Second).String()
		issues = append(issues, cluster.Issue{
			Severity:  severity,
			Cluster:   name,
			Pod:       p.Name,
			Namespace: p.Namespace,
			Status:    fmt.Sprintf("%s (%d/%d)", p.Status.Phase, countRestarts(p), len(p.Status.ContainerStatuses)),
			Restarts:  countRestarts(p),
			Age:       age,
		})
	}
	return issues, nil
}

func listClusterEvents(ctx context.Context, manager *cluster.ClusterClientManager, name string) ([]corev1.Event, error) {
	if name == "" {
		return nil, ErrNameRequired
	}

	cc, err := manager.GetClient(ctx, name)
	if err != nil {
		return nil, ErrNotFound
	}

	cs := cc.Clientset()
	if cs == nil {
		return nil, ErrOffline
	}

	events, err := cs.CoreV1().Events(metav1.NamespaceAll).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list events: %w", err)
	}
	return events.Items, nil
}

func countRestarts(p corev1.Pod) int32 {
	var total int32
	for _, cs := range p.Status.ContainerStatuses {
		total += cs.RestartCount
	}
	return total
}
