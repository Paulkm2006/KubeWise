package audit

import (
	"testing"
	"time"

	"github.com/kubewise/kubewise/internal/audit/domain"
)

func TestParseStructuredFindings(t *testing.T) {
	content := "分析完成。\n```json\n{\"findings\":[{\"severity\":\"high\",\"category\":\"RBAC\",\"resource\":\"Role/admin\",\"risk\":\"wildcard\",\"impact\":\"broad access\",\"suggestion\":\"scope it\"}]}\n```"
	findings := ParseStructuredFindings(content)
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(findings))
	}
	if findings[0].Severity != domain.SeverityHigh {
		t.Fatalf("expected high, got %s", findings[0].Severity)
	}
}

func TestBuildResultFallsBackToToolFindings(t *testing.T) {
	tool := []domain.Finding{{
		Severity: domain.SeverityLow, Category: "RBAC", Resource: "sa/test",
		Risk: "orphan", Impact: "x", Suggestion: "y",
	}}
	result := BuildResult("kw-exp-c", "not json", tool, mustTime(), 100)
	if len(result.Findings) != 1 {
		t.Fatalf("expected fallback findings")
	}
}

func mustTime() time.Time {
	return time.Unix(1_700_000_000, 0)
}
