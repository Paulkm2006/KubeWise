package cluster

import (
	"testing"
)

func TestDiscoverFromEmptyConfig(t *testing.T) {
	_, err := NewClusterClientManager("/nonexistent/kubeconfig")
	if err != nil {
		t.Fatalf("expected no error on missing kubeconfig, got: %v", err)
	}
}
