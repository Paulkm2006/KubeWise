package runtime

import (
	"github.com/kubewise/kubewise/internal/platform/agentruntime/diagnose/casefile"
	"github.com/kubewise/kubewise/internal/platform/agentruntime/event"
	"github.com/kubewise/kubewise/internal/utils/llm"
)

func RecordLLMStart(st *State, stepKey, phase, summary string) {
	if st == nil || stepKey == "" {
		return
	}
	st.EmitPhase(summary, &event.Payload{
		Kind: event.PayloadKindDiagnosisLLMStep,
		Data: map[string]any{"step": stepKey, "phase": phase, "status": "running"},
	})
}

func RecordLLMFailure(st *State, file *casefile.CaseFile, stepKey, phase, fallback string, err error) {
	if st == nil || file == nil || err == nil {
		return
	}
	file.AddMissing(stepKey, err.Error())
	st.EmitLLMDegraded(stepKey, phase, err.Error(), fallback, llm.IsTransientError(err))
}
