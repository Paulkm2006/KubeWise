package plan

import (
	"strings"
	"testing"

	"github.com/kubewise/kubewise/pkg/catalog"
)

func TestSanitizeReleaseName(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"nginx", "nginx"},
		{"Nginx Ingress", "nginx-ingress"},
		{"", "release"},
		{strings.Repeat("a", 60), strings.Repeat("a", 53)},
	}
	for _, tc := range tests {
		got := SanitizeReleaseName(tc.in)
		if got != tc.want {
			t.Errorf("SanitizeReleaseName(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestValidateNamespace_Blocked(t *testing.T) {
	for _, ns := range []string{"kube-system", "kube-public", "kube-node-lease"} {
		if err := ValidateNamespace(ns); err == nil {
			t.Fatalf("expected error for %q", ns)
		}
	}
}

func TestValidateNamespace_Invalid(t *testing.T) {
	cases := []string{"", "Bad_NS", "-bad", "toolong" + strings.Repeat("x", 60)}
	for _, ns := range cases {
		if err := ValidateNamespace(ns); err == nil {
			t.Fatalf("expected error for %q", ns)
		}
	}
}

func TestValidateNamespace_Valid(t *testing.T) {
	if err := ValidateNamespace("monitoring"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestScanPolicyWarnings_Privileged(t *testing.T) {
	yaml := "securityContext:\n  privileged: true\n"
	warnings := ScanPolicyWarnings(yaml)
	if len(warnings) == 0 {
		t.Fatal("expected policy warnings")
	}
}

func TestScanPolicyWarnings_ClusterAdmin(t *testing.T) {
	yaml := "rbac:\n  clusterAdmin: true\n"
	warnings := ScanPolicyWarnings(yaml)
	found := false
	for _, w := range warnings {
		if w.Severity == "error" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected blocking policy warning")
	}
}

func TestValidateDeployPlan_BlocksInvalidNamespace(t *testing.T) {
	p := DeployPlan{
		ReleaseName:  "nginx",
		Namespace:    "kube-system",
		Chart:        &catalog.ChartInfo{ChartName: "nginx"},
		CustomValues: "replicas: 1\n",
	}
	result := ValidateDeployPlan(p)
	if !result.HasBlockingErrors() {
		t.Fatal("expected blocking errors")
	}
}
