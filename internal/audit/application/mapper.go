package application

import (
	"encoding/json"

	"github.com/kubewise/kubewise/internal/audit/domain"
	"github.com/kubewise/kubewise/internal/platform/agentruntime"
	"github.com/kubewise/kubewise/internal/platform/agentruntime/event"
)

func mapProgressEvent(ev agentruntime.ProgressEvent) domain.EventAppend {
	return domain.EventAppend{
		EventType:   ev.Type,
		Message:     ev.Message,
		Summary:     firstNonEmpty(ev.Summary, ev.Message),
		Detail:      ev.Detail,
		PayloadKind: ev.PayloadKind,
		PayloadJSON: ev.PayloadJSON,
		ElapsedMs:   ev.ElapsedMs,
	}
}

func parseResultFromProgress(final agentruntime.ProgressEvent, durationMs int64) *domain.Result {
	if final.PayloadKind == event.PayloadKindAuditReport && final.PayloadJSON != "" {
		var result domain.Result
		if err := json.Unmarshal([]byte(final.PayloadJSON), &result); err == nil {
			if result.DurationMs == 0 {
				result.DurationMs = durationMs
			}
			return &result
		}
	}
	return nil
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
