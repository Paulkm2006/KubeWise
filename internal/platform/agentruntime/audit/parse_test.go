package audit

import (
	"testing"

	"github.com/kubewise/kubewise/internal/audit/domain"
)

func TestParseFindings(t *testing.T) {
	text := `=== RBAC安全审计结果 ===

[HIGH] ClusterRole "admin": 动词(*) 和 资源(*)
[LOW] ServiceAccount prod/legacy: 未绑定任何角色

共发现 2 个安全问题`

	findings := ParseFindings("RBAC", text)
	if len(findings) != 2 {
		t.Fatalf("expected 2 findings, got %d", len(findings))
	}
	if findings[0].Severity != domain.SeverityHigh {
		t.Fatalf("expected high severity, got %s", findings[0].Severity)
	}
	if findings[0].Category != "RBAC" {
		t.Fatalf("expected RBAC category, got %s", findings[0].Category)
	}
	if findings[0].Suggestion == "" {
		t.Fatal("expected non-empty suggestion")
	}
}
