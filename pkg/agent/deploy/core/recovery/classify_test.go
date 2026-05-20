package recovery

import (
	"fmt"
	"strings"
	"testing"
)

func TestClassifyError_OwnershipConflict(t *testing.T) {
	err := fmt.Errorf(`helm install 失败: unable to continue with install: CustomResourceDefinition "applications.argoproj.io" in namespace "" exists and cannot be imported into the current release: invalid ownership metadata; annotation validation error: key "meta.helm.sh/release-namespace" must equal "argocd-new": current value is "argocd"`)

	triage := ClassifyError(err)

	if triage.Class != ClassHelmOwnershipConflict {
		t.Fatalf("expected ownership conflict, got %v", triage.Class)
	}
	if !triage.Deterministic {
		t.Fatal("expected deterministic triage")
	}
	if !strings.Contains(triage.Report, "CRD") || !strings.Contains(triage.Report, "argocd-new") {
		t.Fatalf("expected report to mention CRD and target namespace, got %q", triage.Report)
	}
}
