package audit

import (
	"encoding/json"
	"regexp"
	"strings"
	"time"

	"github.com/kubewise/kubewise/internal/audit/domain"
)

var jsonFenceRe = regexp.MustCompile("(?s)```(?:json)?\\s*(\\{.*?\\})\\s*```")

type structuredReport struct {
	Findings []domain.Finding `json:"findings"`
}

func BuildResult(cluster, llmContent string, toolFindings []domain.Finding, startedAt time.Time, durationMs int64) *domain.Result {
	findings := ParseStructuredFindings(llmContent)
	if len(findings) == 0 {
		findings = toolFindings
	}
	summary := Summarize(findings)
	markdown := llmContent
	if !strings.Contains(markdown, "# KubeWise Security Audit") {
		markdown = RenderMarkdown(cluster, findings, summary, startedAt, durationMs)
	}
	return &domain.Result{
		Findings:   findings,
		Summary:    summary,
		Markdown:   markdown,
		DurationMs: durationMs,
	}
}

func ParseStructuredFindings(content string) []domain.Finding {
	content = strings.TrimSpace(content)
	if content == "" {
		return nil
	}

	if m := jsonFenceRe.FindStringSubmatch(content); len(m) == 2 {
		if findings := decodeFindings(m[1]); len(findings) > 0 {
			return findings
		}
	}

	if strings.HasPrefix(content, "{") {
		if findings := decodeFindings(content); len(findings) > 0 {
			return findings
		}
	}

	if idx := strings.Index(content, "{"); idx >= 0 {
		if findings := decodeFindings(content[idx:]); len(findings) > 0 {
			return findings
		}
	}
	return nil
}

func decodeFindings(raw string) []domain.Finding {
	var rep structuredReport
	if err := json.Unmarshal([]byte(raw), &rep); err != nil {
		return nil
	}
	return normalizeFindings(rep.Findings)
}

func normalizeFindings(in []domain.Finding) []domain.Finding {
	var out []domain.Finding
	for _, f := range in {
		if f.Resource == "" && f.Risk == "" {
			continue
		}
		switch f.Severity {
		case domain.SeverityCritical, domain.SeverityHigh, domain.SeverityMedium, domain.SeverityLow:
		default:
			f.Severity = domain.SeverityLow
		}
		if f.Impact == "" {
			f.Impact = impactFor(f.Risk, f.Category)
		}
		if f.Suggestion == "" {
			f.Suggestion = suggestFor(f.Risk, f.Category)
		}
		out = append(out, f)
	}
	return out
}

func FindingsFromToolResult(toolName, display string) []domain.Finding {
	phase, ok := PhaseForTool(toolName)
	if !ok {
		return nil
	}
	return ParseFindings(phase.Category, display)
}
