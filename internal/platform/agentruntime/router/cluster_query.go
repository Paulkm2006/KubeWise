package router

import (
	"context"
	"fmt"
	"strings"

	"github.com/kubewise/kubewise/internal/platform/cluster"
)

func parseClusterFromQuery(userQuery string) (clusterName string, query string) {
	userQuery = strings.TrimSpace(userQuery)
	const prefix = "[cluster: "
	if !strings.HasPrefix(userQuery, prefix) {
		return "", userQuery
	}
	rest := userQuery[len(prefix):]
	end := strings.Index(rest, "]")
	if end < 0 {
		return "", userQuery
	}
	clusterName = strings.TrimSpace(rest[:end])
	query = strings.TrimSpace(rest[end+1:])
	if query == "" {
		return clusterName, strings.TrimSpace(userQuery)
	}
	return clusterName, query
}

func (a *Agent) k8sClientForCluster(ctx context.Context, clusterName string) (*cluster.Client, error) {
	if clusterName == "" || a.clusterManager == nil {
		if a.k8sClient == nil {
			return nil, fmt.Errorf("single-context Kubernetes client is not configured")
		}
		return a.k8sClient, nil
	}
	cc, err := a.clusterManager.GetClient(ctx, clusterName)
	if err != nil {
		return nil, err
	}
	return cluster.NewClientFromClusterClient(cc)
}
