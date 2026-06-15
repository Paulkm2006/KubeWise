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
		Summary:  "诊断流程已完成",
		Payload:  donePayload,
	})
	return rep, markdown, runErr
}

func (a *Agent) HandleQuery(ctx context.Context, userQuery string, entities types.Entities, queryID string, eventCh chan<- event.Event) (string, error) {
	ns, pod := resolvePodTarget(entities)
	if ns == "" || pod == "" {
		return "", fmt.Errorf("诊断需要指定 namespace 和 Pod")
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
	st.EmitPhase("正在采集 Pod 基础上下文", stagePayload(st.Phase, "running", 0))
	collected, err := collect.Run(st.Ctx, a.k8s, a.llmClient, file, collect.Profile{
		MaxSupplementalTools: profile.MaxSupplementalTools,
	}, st)
	if err != nil {
		st.Fail(err)
		return nil, "", err
	}
	st.EmitPhase("已采集 Pod 状态、事件和补充观测", stagePayload(st.Phase, "completed", 0))

	st.Phase = runtime.PhaseEvidence
	st.EmitPhase("正在构建证据清单", stagePayload(st.Phase, "running", 0))
	evs := evidence.Build(st.Ctx, file, collected, a.llmClient)
	file.Catalog.Add(evs...)
	st.Evidence = file.Catalog.All()
	st.EmitPhase("已构建证据清单", evidencePayload(st.Evidence))

	st.Phase = runtime.PhaseHypothesis
	runtime.RecordLLMStart(st, "llm_hypothesis_proposal", "hypothesis", "正在基于证据生成候选根因")
	hs := hypothesis.Generate(st.Ctx, a.llmClient, file, st.Evidence, file.Target, collected.Observation)
	st.Hypotheses = hs
	st.Emit(event.ToolDone{
		QueryID: st.QueryID, ToolName: "generate_hypotheses", Step: 1,
		Summary: fmt.Sprintf("已生成 %d 个候选根因", len(hs)),
		Payload: &event.Payload{Kind: event.PayloadKindDiagnosisHypothesis, Data: hs},
	})
	st.EmitPhase(fmt.Sprintf("已生成 %d 条证据和 %d 个候选根因", len(st.Evidence), len(hs)), stagePayload(st.Phase, "completed", len(st.Evidence)))

	st.Phase = runtime.PhaseVerify
	st.EmitPhase("正在验证候选根因", stagePayload(st.Phase, "running", 0))
	reg, err := collect.NewReadRegistry(a.k8s)
	if err != nil {
		st.Fail(err)
		return nil, "", err
	}
	checks := verify.Run(st.Ctx, toolv2.NewExecutor(reg), reg.Names(), file, hs, a.llmClient, st)
	st.EmitPhase("候选根因验证完成", stagePayload(st.Phase, "completed", len(st.Evidence)))
	st.Emit(event.ToolDone{
		QueryID: st.QueryID, ToolName: "verify_hypotheses", Step: 1,
		Summary: fmt.Sprintf("已验证 %d 个候选根因", len(checks)),
		Payload: &event.Payload{Kind: event.PayloadKindDiagnosisVerification, Data: checks},
	})

	st.Phase = runtime.PhaseReport
	st.EmitPhase("正在生成诊断报告", stagePayload(st.Phase, "running", 0))
	runtime.RecordLLMStart(st, "llm_root_selection", "report", "正在选择证据最充分的根因")
	rep := report.Compose(st.Ctx, a.llmClient, file, hs, checks)
	if err := report.Validate(*rep); err != nil {
		st.Fail(err)
		return nil, "", err
	}
	st.Emit(event.ToolDone{
		QueryID: st.QueryID, ToolName: "build_report", Step: 1,
		Summary: "结构化诊断报告已生成",
		Payload: &event.Payload{Kind: event.PayloadKindDiagnosisReport, Data: rep},
	})

	md := report.ToMarkdown(*rep)
	st.Markdown = md
	st.EmitPhase("诊断报告已格式化为 Markdown", stagePayload(st.Phase, "completed", len(st.Evidence)))

	st.Phase = runtime.PhaseDone
	st.EmitPhase("诊断完成", stagePayload(st.Phase, "completed", len(st.Evidence)))
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
