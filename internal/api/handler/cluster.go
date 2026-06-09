package handler

import (
	"context"
	"net/http"
	"time"

	"go.uber.org/zap"
	"github.com/labstack/echo/v5"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/kubewise/kubewise/internal/utils/log"
)

// ClusterStatusResponse is the JSON body for GET /api/v1/cluster/status.
type ClusterStatusResponse struct {
	Version    string     `json:"version"`
	Nodes      NodeCounts `json:"nodes"`
	Pods       PodCounts  `json:"pods"`
	Namespaces int        `json:"namespaces"`
	Health     string     `json:"health"`
}

type NodeCounts struct {
	Total    int `json:"total"`
	Ready    int `json:"ready"`
	NotReady int `json:"not_ready"`
}

type PodCounts struct {
	Total     int `json:"total"`
	Running   int `json:"running"`
	Pending   int `json:"pending"`
	Failed    int `json:"failed"`
	Succeeded int `json:"succeeded"`
}

func (h *Handler) ClusterStatus(c *echo.Context) error {
	ctx, cancel := context.WithTimeout(c.Request().Context(), 10*time.Second)
	defer cancel()

	version := "unknown"
	nodes := NodeCounts{}
	pods := PodCounts{}
	nsCount := 0

	log.Ctx(ctx).Debug("cluster status requested")

	if h.k8sClient == nil {
		log.Ctx(ctx).Info("cluster status: no k8s client available, reporting offline")
		return c.JSON(http.StatusOK, ClusterStatusResponse{
			Version: version, Health: "offline",
		})
	}

	if v, err := h.k8sClient.ServerVersion(ctx); err == nil {
		version = v
	} else {
		log.Ctx(ctx).Error("cluster status: failed to get server version", zap.Error(err))
	}
	if nodeList, err := h.k8sClient.ListNodes(ctx); err == nil {
		nodes.Total = len(nodeList)
		for _, n := range nodeList {
			if isNodeReady(n) {
				nodes.Ready++
			} else {
				nodes.NotReady++
			}
		}
	} else {
		log.Ctx(ctx).Error("cluster status: failed to list nodes", zap.Error(err))
	}
	if podList, err := h.k8sClient.ListPods(ctx, metav1.NamespaceAll); err == nil {
		pods.Total = len(podList)
		for _, p := range podList {
			switch p.Status.Phase {
			case corev1.PodRunning:
				pods.Running++
			case corev1.PodPending:
				pods.Pending++
			case corev1.PodFailed:
				pods.Failed++
			case corev1.PodSucceeded:
				pods.Succeeded++
			}
		}
	} else {
		log.Ctx(ctx).Error("cluster status: failed to list pods", zap.Error(err))
	}
	if nsList, err := h.k8sClient.ListNamespaces(ctx); err == nil {
		nsCount = len(nsList)
	} else {
		log.Ctx(ctx).Error("cluster status: failed to list namespaces", zap.Error(err))
	}

	health := "critical"
	switch {
	case nodes.Total == 0:
		health = "critical"
	case nodes.Ready == nodes.Total:
		health = "healthy"
	case nodes.Ready < nodes.Total:
		health = "degraded"
	}

	resp := ClusterStatusResponse{
		Version: version, Nodes: nodes, Pods: pods,
		Namespaces: nsCount, Health: health,
	}
	log.Ctx(ctx).Info("cluster status retrieved",
		zap.Any("summary", resp),
	)
	return c.JSON(http.StatusOK, resp)
}

func isNodeReady(node corev1.Node) bool {
	for _, cond := range node.Status.Conditions {
		if cond.Type == corev1.NodeReady {
			return cond.Status == corev1.ConditionTrue
		}
	}
	return false
}
