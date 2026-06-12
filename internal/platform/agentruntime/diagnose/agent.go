package diagnose

import (
	"context"
	"fmt"
	"time"

	"github.com/kubewise/kubewise/internal/platform/agentruntime/diagnose/casefile"
	"github.com/kubewise/kubewise/internal/platform/agentruntime/diagnose/collect"
	"github.com/kubewise/kubewise/internal/platform/agentruntime/diagnose/evidence"
	"github.com/kubewise/kubewise/internal/platform/agentruntime/diagnose/hypothesis"
	"github.com/kubewise/kubewise/internal/platform/agentruntime/diagnose/report"
	"github.com/kubewise/kubewise/internal/platform/agentruntime/diagnose/runtime"
	"github.com/kubewise/kubewise/internal/platform/agentruntime/diagnose/verify"
	"github.com/kubewise/kubewise/internal/platform/agentruntime/event"
	"github.com/kubewise/kubewise/internal/platform/agentruntime/router/types"
	toolv2 "github.com/kubewise/kubewise/internal/platform/agentruntime/tool/v2"
	"github.com/kubewise/kubewise/internal/platform/cluster"
	"github.com/kubewise/kubewise/internal/utils/llm"
)

type Agent struct {
	k8s       *cluster.Client
	llmClient *llm.Client
}

func NewAgent(k8s *cluster.Client, llmClient *llm.Client) *Agent {
	return &Agent{k8s: k8s, llmClient: llmClient}
}

func (a *Agent) Run(ctx context.Context, queryID string, target runtime.Target, profile Profile, eventCh chan<- event.Event) (*report.DiagnosisReport, string, error) {
	st := runtime.New(ctx, queryID, target, string(profile.Mode), eventCh)
	st.Emit(event.AgentStart{QueryID: queryID, AgentName: "Diagnosis Agent"})
	start := time.Now()

	rep, markdown, runErr := a.run(st, profile)
	donePayload := &event.Payload{Kind: event.PayloadKindDiagnosisReport}
	if rep != nil {
		donePayload.Data = rep
	}
	st.Emit(event.AgentDone{
		QueryID:  queryID,
		Result:   markdown,
		Duration: time.Since(start),
		Summary:  "diagnosis pipeline finished",
		Payload:  donePayload,
	})
	return rep, markdown, runErr
}

func (a *Agent) HandleQuery(ctx context.Context, userQuery string, entities types.Entities, queryID string, eventCh chan<- event.Event) (string, error) {
	ns, pod := resolvePodTarget(entities)
	if ns == "" || pod == "" {
		return "", fmt.Errorf("troubleshooting requires namespace and pod in the query context")
	}
	_, markdown, err := a.Run(ctx, queryID, runtime.Target{
		Cluster: "", Namespace: ns, Pod: pod,
	}, ConversationProfile(), eventCh)
	_ = userQuery
	return markdown, err
}

func resolvePodTarget(entities types.Entities) (namespace, pod string) {
	if len(entities.Namespace) > 0 {
		namespace = entities.Namespace[0]
	}
	pod = entities.ResourceName
	return namespace, pod
}

func (a *Agent) run(st *runtime.State, profile Profile) (*report.DiagnosisReport, string, error) {
	file := casefile.New(st.QueryID, casefile.Target{
		Cluster: st.Target.Cluster, Namespace: st.Target.Namespace, Pod: st.Target.Pod,
	}, st.Profile)

	st.Phase = runtime.PhaseCollect
	st.EmitPhase("collecting baseline pod context", stagePayload(st.Phase, "running", 0))
	collected, err := collect.Run(st.Ctx, a.k8s, a.llmClient, file, collect.Profile{
		MaxSupplementalTools: profile.MaxSupplementalTools,
	}, st)
	if err != nil {
		st.Fail(err)
		return nil, "", err
	}
	st.EmitPhase("collected pod state, events, and supplemental observations", stagePayload(st.Phase, "completed", 0))

	st.Phase = runtime.PhaseEvidence
	st.EmitPhase("building evidence catalog", stagePayload(st.Phase, "running", 0))
	evs := evidence.Build(st.Ctx, file, collected, a.llmClient)
	file.Catalog.Add(evs...)
	st.Evidence = file.Catalog.All()
	st.EmitPhase("built evidence catalog", evidencePayload(st.Evidence))

	st.Phase = runtime.PhaseHypothesis
	runtime.RecordLLMStart(st, "llm_hypothesis_proposal", "hypothesis", "proposing evidence-grounded hypotheses")
	hs := hypothesis.Generate(st.Ctx, a.llmClient, file, st.Evidence, file.Target, collected.Observation)
	st.Hypotheses = hs
	st.Emit(event.ToolDone{
		QueryID: st.QueryID, ToolName: "generate_hypotheses", Step: 1,
		Summary: fmt.Sprintf("generated %d hypotheses", len(hs)),
		Payload: &event.Payload{Kind: event.PayloadKindDiagnosisHypothesis, Data: hs},
	})
	st.EmitPhase(fmt.Sprintf("built %d evidence items and %d hypotheses", len(st.Evidence), len(hs)), stagePayload(st.Phase, "completed", len(st.Evidence)))

	st.Phase = runtime.PhaseVerify
	st.EmitPhase("verifying hypotheses against executable paths", stagePayload(st.Phase, "running", 0))
	reg, err := collect.NewReadRegistry(a.k8s)
	if err != nil {
		st.Fail(err)
		return nil, "", err
	}
	checks := verify.Run(st.Ctx, toolv2.NewExecutor(reg), reg.Names(), file, hs, a.llmClient, st)
	st.EmitPhase("verified hypotheses against evidence", stagePayload(st.Phase, "completed", len(st.Evidence)))
	st.Emit(event.ToolDone{
		QueryID: st.QueryID, ToolName: "verify_hypotheses", Step: 1,
		Summary: fmt.Sprintf("verified %d hypotheses", len(checks)),
		Payload: &event.Payload{Kind: event.PayloadKindDiagnosisVerification, Data: checks},
	})

	st.Phase = runtime.PhaseReport
	st.EmitPhase("composing structured diagnosis report", stagePayload(st.Phase, "running", 0))
	runtime.RecordLLMStart(st, "llm_root_selection", "report", "selecting strongest supported root cause")
	rep := report.Compose(st.Ctx, a.llmClient, file, hs, checks)
	if err := report.Validate(*rep); err != nil {
		st.Fail(err)
		return nil, "", err
	}
	st.Emit(event.ToolDone{
		QueryID: st.QueryID, ToolName: "build_report", Step: 1,
		Summary: "assembled structured diagnosis report",
		Payload: &event.Payload{Kind: event.PayloadKindDiagnosisReport, Data: rep},
	})

	md := report.ToMarkdown(*rep)
	st.Markdown = md
	st.EmitPhase("formatted report to markdown", stagePayload(st.Phase, "completed", len(st.Evidence)))

	st.Phase = runtime.PhaseDone
	st.EmitPhase("diagnosis complete", stagePayload(st.Phase, "completed", len(st.Evidence)))
	return rep, md, nil
}

func stagePayload(phase runtime.Phase, status string, evidenceCount int) *event.Payload {
	return &event.Payload{
		Kind: event.PayloadKindDiagnosisStage,
		Data: map[string]any{
			"stage": phase.Stage(), "status": status, "evidence_count": evidenceCount,
		},
	}
}

func evidencePayload(evs []casefile.Evidence) *event.Payload {
	ids := make([]string, 0, len(evs))
	for _, ev := range evs {
		ids = append(ids, ev.ID)
	}
	return &event.Payload{
		Kind: event.PayloadKindDiagnosisEvidence,
		Data: map[string]any{"evidence_count": len(evs), "evidence_ids": ids},
	}
}
