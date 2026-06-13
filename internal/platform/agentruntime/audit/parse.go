package audit

import (
	"regexp"
	"strings"

	"github.com/kubewise/kubewise/internal/audit/domain"
)

var severityRe = regexp.MustCompile(`^\[(CRITICAL|HIGH|MEDIUM|LOW)\]\s*(.+)$`)

func ParseFindings(category, text string) []domain.Finding {
	lines := strings.Split(text, "\n")
	var out []domain.Finding
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "===") || strings.HasPrefix(line, "共发现") {
			continue
		}
		m := severityRe.FindStringSubmatch(line)
		if len(m) != 3 {
			continue
		}
		body := strings.TrimSpace(m[2])
		resource, risk := splitResourceRisk(body)
		out = append(out, domain.Finding{
			Severity:   mapSeverity(m[1]),
			Category:   category,
			Resource:   resource,
			Risk:       risk,
			Impact:     impactFor(body, category),
			Suggestion: suggestFor(body, category),
		})
	}
	return out
}

func splitResourceRisk(body string) (resource, risk string) {
	if idx := strings.Index(body, ": "); idx >= 0 {
		return strings.TrimSpace(body[:idx]), strings.TrimSpace(body[idx+2:])
	}
	return body, body
}

func mapSeverity(raw string) domain.Severity {
	switch strings.ToUpper(raw) {
	case "CRITICAL":
		return domain.SeverityCritical
	case "HIGH":
		return domain.SeverityHigh
	case "MEDIUM":
		return domain.SeverityMedium
	default:
		return domain.SeverityLow
	}
}

func Summarize(findings []domain.Finding) domain.Summary {
	s := domain.Summary{}
	for _, f := range findings {
		s.Total++
		switch f.Severity {
		case domain.SeverityCritical:
			s.Critical++
		case domain.SeverityHigh:
			s.High++
		case domain.SeverityMedium:
			s.Medium++
		case domain.SeverityLow:
			s.Low++
		}
	}
	return s
}
