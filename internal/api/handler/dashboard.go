package handler

import (
	"fmt"
	"net/http"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/kubewise/kubewise/internal/cluster"
	"github.com/labstack/echo/v5"
)

func (h *Handler) ListClusters(c *echo.Context) error {
	if h.clusterManager == nil {
		return c.JSON(http.StatusOK, []any{})
	}
	ctx := c.Request().Context()
	summaries := h.clusterManager.ListClusters(ctx)
	return c.JSON(http.StatusOK, summaries)
}

func (h *Handler) ListIssues(c *echo.Context) error {
	name := c.Param("name")
	if name == "" {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "cluster name required"})
	}

	ctx := c.Request().Context()
	cc, err := h.clusterManager.GetClient(ctx, name)
	if err != nil {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: fmt.Sprintf("cluster %q not found", name)})
	}

	cs := cc.Clientset()
	if cs == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: fmt.Sprintf("cluster %q is offline", name)})
	}

	pods, err := cs.CoreV1().Pods(metav1.NamespaceAll).List(ctx, metav1.ListOptions{})
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
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
			Status:    fmt.Sprintf("%s (%d/%d)", p.Status.Phase, countRestarts(p), countContainers(p)),
			Restarts:  countRestarts(p),
			Age:       age,
		})
	}
	return c.JSON(http.StatusOK, issues)
}

func countRestarts(p corev1.Pod) int32 {
	var total int32
	for _, cs := range p.Status.ContainerStatuses {
		total += cs.RestartCount
	}
	return total
}

func countContainers(p corev1.Pod) int {
	return len(p.Status.ContainerStatuses)
}

func (h *Handler) ListClusterEvents(c *echo.Context) error {
	name := c.Param("name")
	ctx := c.Request().Context()
	cc, err := h.clusterManager.GetClient(ctx, name)
	if err != nil {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: "cluster not found"})
	}
	cs := cc.Clientset()
	if cs == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "cluster offline"})
	}
	events, err := cs.CoreV1().Events(metav1.NamespaceAll).List(ctx, metav1.ListOptions{})
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}
	return c.JSON(http.StatusOK, events.Items)
}
