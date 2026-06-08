package helm

import (
	"strings"
	"testing"
)

func TestValidateYAML_Valid(t *testing.T) {
	if err := ValidateYAML("replicas: 3\n"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateYAML_Invalid(t *testing.T) {
	if err := ValidateYAML("replicas: [broken"); err == nil {
		t.Fatal("expected error for invalid yaml")
	}
}

func TestMergeValues_OverrideWins(t *testing.T) {
	base := "replicas: 1\nservice:\n  type: ClusterIP\n"
	override := "replicas: 3\n"
	merged, err := MergeValues(base, override)
	if err != nil {
		t.Fatalf("MergeValues failed: %v", err)
	}
	if !strings.Contains(merged, "replicas: 3") {
		t.Fatalf("expected override replicas, got:\n%s", merged)
	}
}
