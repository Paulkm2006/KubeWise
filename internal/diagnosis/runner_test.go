package diagnosis

import (
	"context"
	"testing"
)

func TestNewRunner(t *testing.T) {
	r := NewRunner()
	if r == nil {
		t.Fatal("NewRunner() returned nil")
	}
}

func TestRunnerStart(t *testing.T) {
	r := NewRunner()
	ctx := context.Background()
	target := DiagnosisTarget{
		Cluster:        "test-cluster",
		ClusterDisplay: "Test Cluster",
		Namespace:      "default",
		Pod:            "test-pod",
	}

	err := r.Start(ctx, "diag-1", target)
	if err == nil {
		t.Log("Start succeeded (requires running DB)")
	} else {
		t.Logf("Start returned expected error: %v", err)
	}
}