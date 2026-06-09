package cluster

import (
	"sync"
	"time"

	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

type ClusterHealth string

const (
	HealthHealthy  ClusterHealth = "healthy"
	HealthDegraded ClusterHealth = "degraded"
	HealthOffline  ClusterHealth = "offline"
)

type ClusterSummary struct {
	ID          string        `json:"id"`
	Name        string        `json:"name"`
	Health      ClusterHealth `json:"health"`
	PodsReady   int           `json:"pods_ready"`
	PodsTotal   int           `json:"pods_total"`
	IssuesCount int           `json:"issues_count"`
	Nodes       int           `json:"nodes"`
	Namespaces  int           `json:"namespaces"`
	Version     string        `json:"version"`
	Fingerprint string        `json:"fingerprint"`
	LastUpdated int           `json:"last_updated"`
}

type Issue struct {
	Severity  string `json:"severity"`
	Cluster   string `json:"cluster"`
	Pod       string `json:"pod"`
	Namespace string `json:"namespace"`
	Status    string `json:"status"`
	Restarts  int32  `json:"restarts"`
	Age       string `json:"age"`
}

type ResourceRow struct {
	Pod       string `json:"pod"`
	Namespace string `json:"namespace"`
	CPU       string `json:"cpu_usage"`
	Mem       string `json:"mem_usage"`
	CPUReq    string `json:"cpu_request,omitempty"`
	MemLimit  string `json:"mem_limit,omitempty"`
}

// ClusterClient wraps a Client with cluster identity and health tracking.
type ClusterClient struct {
	ContextName string
	clientset   *kubernetes.Clientset
	dynamic     dynamic.Interface
	restConfig  *rest.Config
	Health      ClusterHealth
	Fingerprint string
	LastSeen    time.Time
	mu          sync.RWMutex
}
