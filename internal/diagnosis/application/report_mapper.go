package application

import (
	"encoding/json"

	"github.com/kubewise/kubewise/internal/diagnosis/domain"
	"github.com/kubewise/kubewise/internal/platform/agentruntime"
	"github.com/kubewise/kubewise/internal/platform/agentruntime/diagnose/report"
	"github.com/kubewise/kubewise/internal/platform/agentruntime/event"
)

func mapProgressEvent(ev agentruntime.ProgressEvent) domain.EventAppend {
	detail := ev.Detail
	switch ev.Type {
	case "agent_done":
		// Structured report lives in payload_json; avoid duplicating markdown in run-log detail.
		if ev.PayloadKind == event.PayloadKindDiagnosisReport {
			detail = ""
		} else if detail == "" && ev.Result != "" {
			detail = truncate(ev.Result, 240)
		}
	default:
		if detail == "" {
			detail = ev.Result
		}
	}
	return domain.EventAppend{
		EventType:   ev.Type,
		Message:     ev.Message,
		Summary:     firstNonEmpty(ev.Summary, ev.Message),
		Detail:      detail,
		PayloadKind: ev.PayloadKind,
		PayloadJSON: ev.PayloadJSON,
		TokenIn:     ev.TokenIn,
		TokenOut:    ev.TokenOut,
		ElapsedMs:   ev.ElapsedMs,
	}
}

func parseResultFromProgress(final agentruntime.ProgressEvent, durationMs int64) *domain.Result {
	if final.PayloadKind == event.PayloadKindDiagnosisReport && final.PayloadJSON != "" {
		var rep report.DiagnosisReport
		if err := json.Unmarshal([]byte(final.PayloadJSON), &rep); err == nil {
			return reportToDomain(&rep, firstNonEmpty(final.Result, final.Detail), durationMs)
		}
	}
	if final.Result != "" {
		return &domain.Result{
			Verdict: domain.VerdictInconclusive,
			RootCause: domain.RootCause{
				Title:   truncate(final.Result, 500),
				Summary: truncate(final.Result, 500),
			},
			Markdown:   final.Result,
			DurationMs: durationMs,
		}
	}
	return nil
}

func reportToDomain(rep *report.DiagnosisReport, markdown string, durationMs int64) *domain.Result {
	if rep == nil {
		return nil
	}
	out := &domain.Result{
		Verdict: domain.Verdict(rep.Verdict),
		RootCause: domain.RootCause{
			Category:        rep.RootCause.Category,
			Title:           rep.RootCause.Title,
			ConfidenceScore: rep.RootCause.ConfidenceScore,
			ConfidenceLabel: rep.RootCause.ConfidenceLabel,
			Summary:         rep.RootCause.Summary,
		},
		Limitations: rep.Limitations,
		Enrichment: domain.EnrichmentInfo{
			Status:        rep.Enrichment.Status,
			DegradedSteps: rep.Enrichment.DegradedSteps,
			Message:       rep.Enrichment.Message,
		},
		Markdown:   markdown,
		DurationMs: durationMs,
		Impact:     domain.Impact(rep.Impact),
	}
	for _, ev := range rep.Evidence {
		out.Evidence = append(out.Evidence, domain.Evidence{
			ID: ev.ID, Source: ev.Source, Signal: ev.Signal, Strength: ev.Strength,
			Summary: ev.Summary, Detail: ev.Detail, RawExcerpt: ev.RawExcerpt,
		})
	}
	for _, h := range rep.Hypotheses {
		out.Hypotheses = append(out.Hypotheses, domain.Hypothesis{
			ID: h.ID, Category: h.Category, Title: h.Title, Status: h.Status,
			ConfidenceDelta: h.ConfidenceDelta, SupportingEvidence: h.SupportingEvidence,
			RefutingEvidence: h.RefutingEvidence, Rationale: h.Rationale,
		})
	}
	for _, a := range rep.Actions {
		out.Actions = append(out.Actions, domain.FixAction{
			Priority: a.Priority, Description: a.Description, Command: a.Command, Risk: a.Risk,
		})
	}
	return out
}

func populateDiagnosisLegacyFields(d *domain.Diagnosis, result *domain.Result) {
	if d == nil || result == nil {
		return
	}
	d.Report = result
	d.RootCause = result.RootCause.Summary
	if result.RootCause.Title != "" && d.RootCause == "" {
		d.RootCause = result.RootCause.Title
	}
	d.Confidence = result.RootCause.ConfidenceLabel
	d.DurationMs = result.DurationMs
	d.Impact = result.Impact.Description
	d.FixActions = result.Actions
	for i, ev := range result.Evidence {
		legacy := ev
		legacy.Num = i + 1
		legacy.Text = ev.Summary
		d.Evidence = append(d.Evidence, legacy)
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
