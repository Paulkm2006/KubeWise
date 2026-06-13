package application

import (
	"encoding/json"

	"github.com/kubewise/kubewise/internal/config"
	"github.com/kubewise/kubewise/internal/diagnosis/domain"
	"go.uber.org/zap"
)

func logDiagnosisProgressEvent(diagnosisID string, target domain.Target, ev domain.EventAppend) {
	fields := []zap.Field{
		zap.String("diagnosis_id", diagnosisID),
		zap.String("event_type", ev.EventType),
		zap.String("cluster", target.Cluster),
		zap.String("namespace", target.Namespace),
		zap.String("pod", target.Pod),
	}
	if ev.Message != "" {
		fields = append(fields, zap.String("message", ev.Message))
	}
	if ev.Summary != "" {
		fields = append(fields, zap.String("summary", ev.Summary))
	}
	if ev.Detail != "" {
		fields = append(fields, zap.String("detail", ev.Detail))
	}
	if ev.PayloadKind != "" {
		fields = append(fields, zap.String("payload_kind", ev.PayloadKind))
	}
	if ev.PayloadJSON != "" {
		fields = append(fields, zap.String("payload_json", ev.PayloadJSON))
	}

	switch ev.EventType {
	case "llm_step_degraded":
		config.L().Warn("diagnosis llm step degraded", append(fields, parseLLMStepFields(ev.PayloadJSON)...)...)
	case "stream_err", "tool_fail":
		config.L().Error("diagnosis progress error", fields...)
	case "phase":
		if ev.PayloadKind == "diagnosis_enrichment" {
			config.L().Warn("diagnosis completed with degraded enrichment", fields...)
		}
	}
}

func parseLLMStepFields(payloadJSON string) []zap.Field {
	if payloadJSON == "" {
		return nil
	}
	var payload struct {
		Step      string `json:"step"`
		Phase     string `json:"phase"`
		Error     string `json:"error"`
		Transient bool   `json:"transient"`
		Fallback  string `json:"fallback"`
	}
	if err := json.Unmarshal([]byte(payloadJSON), &payload); err != nil {
		return nil
	}
	return []zap.Field{
		zap.String("llm_step", payload.Step),
		zap.String("llm_phase", payload.Phase),
		zap.String("llm_error", payload.Error),
		zap.Bool("llm_transient", payload.Transient),
		zap.String("llm_fallback", payload.Fallback),
	}
}
