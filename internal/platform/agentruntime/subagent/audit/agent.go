package audit

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/kubewise/kubewise/internal/audit/domain"
	"github.com/kubewise/kubewise/internal/config"
	auditreport "github.com/kubewise/kubewise/internal/platform/agentruntime/audit"
	"github.com/kubewise/kubewise/internal/platform/agentruntime/event"
	"github.com/kubewise/kubewise/internal/platform/agentruntime/loop"
	"github.com/kubewise/kubewise/internal/platform/agentruntime/supervisor"
	securitytools "github.com/kubewise/kubewise/internal/platform/agentruntime/tool/security"
	toolv2 "github.com/kubewise/kubewise/internal/platform/agentruntime/tool/v2"
	"github.com/kubewise/kubewise/internal/platform/cluster"
	"github.com/kubewise/kubewise/internal/utils/llm"
	"go.uber.org/zap"
)

const DefaultMaxSteps = 20

type Agent struct {
	k8sClient     *cluster.Client
	llmClient     *llm.Client
	maxSteps      int
	supervisorCfg supervisor.Config
}

func NewAgent(k8sClient *cluster.Client, llmClient *llm.Client, maxSteps int, supervisorCfg supervisor.Config) *Agent {
	if maxSteps <= 0 {
		maxSteps = DefaultMaxSteps
	}
	return &Agent{
		k8sClient:     k8sClient,
		llmClient:     llmClient,
		maxSteps:      maxSteps,
		supervisorCfg: supervisorCfg,
	}
}

func (a *Agent) RunClusterAudit(ctx context.Context, clusterName, queryID string, eventCh chan<- event.Event) error {
	start := time.Now()
	emit := func(ev event.Event) {
		if eventCh == nil {
			return
		}
		select {
		case eventCh <- ev:
		case <-ctx.Done():
		}
	}

	emit(event.AgentStart{QueryID: queryID, AgentName: "Audit Agent"})
	emit(event.Phase{
		QueryID: queryID,
		Phase:   "starting cluster audit",
		Summary: "running security audit agent",
		Payload: &event.Payload{
			Kind: event.PayloadKindTarget,
			Data: map[string]string{"cluster": clusterName},
		},
	})

	v2reg := toolv2.NewRegistry()
	if err := securitytools.RegisterAuditTools(v2reg, a.k8sClient); err != nil {
		return fmt.Errorf("register audit tools: %w", err)
	}
	allowedTools := toolv2.NewBundleSet(securitytools.NewAuditBundle()).Tools(toolv2.BundleSecurityAudit)

	var toolFindings []domain.Finding
	var completedPhases int

	wrappedEmit := func(ev event.Event) {
		switch e := ev.(type) {
		case event.ToolCall:
			if phase, ok := auditreport.PhaseForTool(e.ToolName); ok {
				emit(event.Phase{
					QueryID: queryID,
					Phase:   phase.ID,
					Summary: fmt.Sprintf("Scanning %s…", phase.Label),
				})
			}
			emit(e)
		case event.ToolDone:
			phase, ok := auditreport.PhaseForTool(e.ToolName)
			if !ok {
				emit(e)
				return
			}
			display := toolDisplayFromPayload(e.Payload)
			findings := auditreport.FindingsFromToolResult(e.ToolName, display)
			toolFindings = append(toolFindings, findings...)
			completedPhases++

			payloadBytes, _ := json.Marshal(map[string]any{
				"phase": phase.ID, "label": phase.Label,
				"index": completedPhases, "total": 4,
				"count": len(findings), "findings": findings,
				"elapsed_ms": e.Elapsed.Milliseconds(),
			})
			emit(event.ToolDone{
				QueryID:  queryID,
				ToolName: phase.ID,
				Step:     e.Step,
				Elapsed:  e.Elapsed,
				Summary:  fmt.Sprintf("%s complete — %d findings", phase.Label, len(findings)),
				Payload: &event.Payload{
					Kind: event.PayloadKindAuditPhaseFindings,
					Data: json.RawMessage(payloadBytes),
				},
			})
		case event.ToolFail:
			if phase, ok := auditreport.PhaseForTool(e.ToolName); ok {
				emit(event.ToolFail{
					QueryID: queryID, ToolName: phase.ID, Step: e.Step,
					Elapsed: e.Elapsed, Err: e.Err,
				})
				return
			}
			emit(e)
		default:
			emit(e)
		}
	}

	userMsg := fmt.Sprintf(
		"对集群 %q 执行全面安全审计。依次调用全部四个审计工具（audit_rbac、audit_pod_security、audit_network_policies、audit_image_security），不要跳过任何维度。",
		clusterName,
	)
	loopResult, err := loop.Run(ctx, loop.Config{
		QueryID:   queryID,
		AgentName: "Audit Agent",
		LLM:       a.llmClient,
		Messages: []llm.Message{
			{Role: "system", Content: a.buildDashboardSystemPrompt()},
			{Role: "user", Content: userMsg},
		},
		Tools:    v2reg.Definitions(allowedTools),
		Executor: toolv2.NewExecutor(v2reg),
		Policy: toolv2.ExecutePolicy{
			AllowedCapabilities: []toolv2.Capability{toolv2.CapabilityAudit},
			AllowedTools:        allowedTools,
			MaxOutputBytes:      20_000,
			EmitEvents:          true,
		},
		MaxSteps: a.maxSteps,
		Emit:     wrappedEmit,
	})
	if err != nil {
		config.L().Error("audit agent failed", zap.String("cluster", clusterName), zap.Error(err))
		emit(event.StreamErr{QueryID: queryID, Err: err})
		return err
	}

	durationMs := time.Since(start).Milliseconds()
	result := auditreport.BuildResult(clusterName, loopResult.Content, toolFindings, start, durationMs)

	emit(event.AgentDone{
		QueryID:   queryID,
		Result:    result.Markdown,
		Duration:  time.Since(start),
		InTokens:  loopResult.InTokens,
		OutTokens: loopResult.OutTokens,
		Summary:   fmt.Sprintf("Audit complete — %d findings", result.Summary.Total),
		Payload:   &event.Payload{Kind: event.PayloadKindAuditReport, Data: result},
	})
	return nil
}

func (a *Agent) buildDashboardSystemPrompt() string {
	return `你是 KubeWise Audit Agent，专门服务于 Dashboard 安全审计页面。

你有四个审计工具：
- audit_rbac
- audit_pod_security
- audit_network_policies
- audit_image_security

## 任务要求
1. 对用户发起的「全面集群安全审计」，必须调用全部四个工具（可在一轮或多轮中完成）。
2. 工具返回后，综合所有发现，输出结构化审计报告。
3. 不要输出闲聊或过程描述；最终回复必须是 JSON。

## 最终输出格式（严格遵守）
完成全部工具调用后，你的最后一条消息必须只包含一个 JSON 代码块：

` + "```json\n" + `{
  "findings": [
    {
      "severity": "critical|high|medium|low",
      "category": "RBAC|Pod Security|Network Policy|Image Security",
      "resource": "受影响资源",
      "risk": "风险描述",
      "impact": "影响",
      "suggestion": "修复建议"
    }
  ]
}
` + "```\n" + `
severity 必须小写。findings 应覆盖工具发现的所有问题，按严重度排序。`
}

func toolDisplayFromPayload(p *event.Payload) string {
	if p == nil || p.Data == nil {
		return ""
	}
	switch data := p.Data.(type) {
	case toolv2.ToolResult:
		return data.Display
	case *toolv2.ToolResult:
		if data != nil {
			return data.Display
		}
	}
	bs, err := json.Marshal(p.Data)
	if err != nil {
		return ""
	}
	var result toolv2.ToolResult
	if err := json.Unmarshal(bs, &result); err != nil {
		return ""
	}
	return result.Display
}
