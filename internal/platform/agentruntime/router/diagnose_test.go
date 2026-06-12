package router

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/kubewise/kubewise/internal/platform/cluster"
)

func TestDiagnosisK8sClientFallsBackToSingleContextWhenClusterEmpty(t *testing.T) {
	fallback := &cluster.Client{}
	agent := &Agent{k8sClient: fallback}

	got, err := agent.diagnosisK8sClient(context.Background(), "")
	if err != nil {
		t.Fatalf("expected fallback client, got error: %v", err)
	}
	if got != fallback {
		t.Fatal("expected empty cluster to use the existing single-context client")
	}
}

func TestDiagnosisK8sClientResolvesNamedClusterThroughManager(t *testing.T) {
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/version" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"major":"1","minor":"30","gitVersion":"v1.30.0"}`))
	}))
	t.Cleanup(api.Close)

	kubeconfig := writeTestKubeconfig(t, api.URL, "kind-a")
	manager, err := cluster.NewClusterClientManager(kubeconfig)
	if err != nil {
		t.Fatalf("new cluster manager: %v", err)
	}
	t.Cleanup(manager.Close)

	fallback := &cluster.Client{}
	agent := &Agent{k8sClient: fallback, clusterManager: manager}

	got, err := agent.diagnosisK8sClient(context.Background(), "kind-a")
	if err != nil {
		t.Fatalf("expected named cluster client, got error: %v", err)
	}
	if got == fallback {
		t.Fatal("expected named cluster to use manager-selected client, not fallback")
	}
	version, err := got.ServerVersion(context.Background())
	if err != nil {
		t.Fatalf("expected selected client to call test API server: %v", err)
	}
	if version != "v1.30.0" {
		t.Fatalf("expected selected cluster version v1.30.0, got %q", version)
	}
}

func writeTestKubeconfig(t *testing.T, serverURL, contextName string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "kubeconfig")
	data := fmt.Sprintf(`apiVersion: v1
kind: Config
clusters:
- name: test-cluster
  cluster:
    server: %s
users:
- name: test-user
  user: {}
contexts:
- name: %s
  context:
    cluster: test-cluster
    user: test-user
current-context: %s
`, serverURL, contextName, contextName)
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatalf("write kubeconfig: %v", err)
	}
	return path
}
