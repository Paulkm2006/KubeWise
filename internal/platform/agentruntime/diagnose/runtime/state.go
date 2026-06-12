package runtime

import (
	"context"
	"time"

	"github.com/kubewise/kubewise/internal/platform/agentruntime/diagnose/casefile"
	"github.com/kubewise/kubewise/internal/platform/agentruntime/diagnose/hypothesis"
	"github.com/kubewise/kubewise/internal/platform/agentruntime/event"
)

type Target struct {
	Cluster   string
	Namespace string
	Pod       string
}

type State struct {
	Ctx        context.Context
	QueryID    string
	Phase      Phase
	Target     Target
	Profile    string
	Evidence   []casefile.Evidence
	Hypotheses []hypothesis.Hypothesis
	Markdown   string
	StartTime  time.Time
	EventCh    chan<- event.Event
}

func New(ctx context.Context, queryID string, target Target, profile string, eventCh chan<- event.Event) *State {
	return &State{
		Ctx:       ctx,
		QueryID:   queryID,
		Phase:     PhaseIntake,
		Target:    target,
		Profile:   profile,
		StartTime: time.Now(),
		EventCh:   eventCh,
	}
}

func (s *State) Emit(ev event.Event) {
	if s.EventCh == nil {
		return
	}
	select {
	case s.EventCh <- ev:
	default:
	}
}

func (s *State) EmitPhase(summary string, payload *event.Payload) {
	s.Emit(event.Phase{
		QueryID: s.QueryID,
		Phase:   s.Phase.String(),
		Summary: summary,
		Payload: payload,
	})
}

func (s *State) EmitLLMDegraded(step, phase, errMsg, fallback string, transient bool) {
	if errMsg == "" {
		return
	}
	s.Emit(event.LLMStepDegraded{
		QueryID:   s.QueryID,
		Step:      step,
		Phase:     phase,
		Err:       errMsg,
		Transient: transient,
		Fallback:  fallback,
	})
}

func (s *State) Fail(err error) {
	s.Phase = PhaseFailed
	detail := ""
	if err != nil {
		detail = err.Error()
	}
	s.Emit(event.StreamErr{QueryID: s.QueryID, Err: err})
	s.Emit(event.Phase{
		QueryID: s.QueryID,
		Phase:   s.Phase.String(),
		Summary: detail,
		Payload: &event.Payload{Kind: event.PayloadKindError, Data: map[string]string{"error": detail}},
	})
}
