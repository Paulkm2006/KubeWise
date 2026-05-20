package recovery

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/kubewise/kubewise/pkg/agent/deploy/plan"
	"github.com/kubewise/kubewise/pkg/agent/deploy/workflow"
	helmtools "github.com/kubewise/kubewise/pkg/agent/deploy/workflow/helm"
	"github.com/kubewise/kubewise/pkg/agent/troubleshooting"
	"github.com/kubewise/kubewise/pkg/catalog"
	"github.com/kubewise/kubewise/pkg/helm"
	"github.com/kubewise/kubewise/pkg/k8s"
	"github.com/kubewise/kubewise/pkg/llm"
	"github.com/kubewise/kubewise/pkg/tool"
	"github.com/kubewise/kubewise/pkg/tui/events"
)

const (
	maxRecoverSteps          = 20
	maxRecoverDeployAttempts = 2
	recoveryLoopRepeatsAbort = 4
)

// Action indicates the outcome of a recovery attempt.
type Action int

const (
	ActionAbort Action = iota
	ActionRecovered
)

// Result carries the outcome of recovery.
type Result struct {
	Action  Action
	Reason  string
	Details string
	YAML    string
	Summary string
}

// LLMClient is the minimal LLM interface for recovery.
type LLMClient interface {
	ChatCompletion(ctx context.Context, messages []llm.Message, functions []llm.FunctionDefinition) (*llm.Message, error)
}

// HelmClient performs install/status for recovery redeploy.
type HelmClient interface {
	helmtools.Client
	Status(ctx context.Context, releaseName, namespace string) (*helm.Release, error)
}

// ConfirmFunc presents a deploy plan and returns the user decision.
type ConfirmFunc func(ctx context.Context, plan events.DeployPlan) (events.DeployDecision, error)

// ReportFunc builds a post-deploy success report.
type ReportFunc func(ctx context.Context, rel *helm.Release, chart *catalog.ChartInfo, namespace, releaseName string) string

// CriticalEmitter sends blocking TUI events (code preview, text).
type CriticalEmitter func(e events.TUIEvent)

// PhaseEmitter sends phase progress events.
type PhaseEmitter func(e events.TUIEvent)

// Logger provides structured logging for recovery.
type Logger interface {
	Info(msg string, fields ...zap.Field)
	Debug(msg string, fields ...zap.Field)
	Warn(msg string, fields ...zap.Field)
	Error(msg string, fields ...zap.Field)
}

// Runner executes the recovery ReAct loop.
type Runner struct {
	QueryID      string
	LLM          LLMClient
	Helm         HelmClient
	Tools        *tool.Registry
	Workflow     *workflow.Runner
	Confirm      ConfirmFunc
	BuildReport  ReportFunc
	EmitPhase    PhaseEmitter
	EmitCritical CriticalEmitter
	Log          Logger
	K8s          *k8s.Client
}

// RunInput is input for the recovery loop.
type RunInput struct {
	DeployErr         error
	Query             string
	CorrectionHistory []string
	Chart             *catalog.ChartInfo
	DefaultValues     string
	CurrentValues     string
	TargetNS          string
	AppName           string
}

// Run diagnoses and attempts to fix a failed deployment.
func (r *Runner) Run(ctx context.Context, in RunInput) (*Result, error) {
	if r.Tools == nil {
		return &Result{
			Action:  ActionAbort,
			Reason:  "诊断工具不可用",
			Details: "诊断工具不可用，无法进行故障排查。请手动检查集群状态。",
		}, nil
	}

	snapshot := BuildDiagnosticsSnapshot(ctx, in.DeployErr, in.AppName, in.TargetNS, in.Chart, r.Helm, r.K8s)
	correctionText := formatCorrectionHistory(in.CorrectionHistory)

	sysPrompt := fmt.Sprintf(`你是 Kubernetes 部署故障诊断与修复助手。

部署失败，请根据自动采集的快照和诊断工具分析根因并尝试修复。

你可以：
1. 调用诊断工具补充信息
2. 调用 submit_values 提交修复后的 values（系统会校验、预检、用户确认后重新部署，最多 %d 次）
3. 调用 submit_report 结束诊断

## 用户需求
%s

## 部署信息
Chart: %s
Release: %s
Namespace: %s
错误: %s

## 当前 override values
%s

## 配置修正历史
%s

## 自动诊断快照
%s

修复成功后系统会自动结束诊断，无需再调用 submit_report。`,
		maxRecoverDeployAttempts,
		in.Query, in.Chart.ChartName, plan.SanitizeReleaseName(in.AppName), in.TargetNS,
		in.DeployErr.Error(), in.CurrentValues, correctionText, snapshot,
	)

	recoveryCtx := NewContext([]llm.Message{
		{Role: "system", Content: sysPrompt},
		{Role: "user", Content: fmt.Sprintf("请诊断并修复 %s 部署失败: %s", in.Chart.ChartName, in.DeployErr.Error())},
	}, maxRecoveryToolOutput)

	tools := troubleshooting.RecoveryToolDefinitions(r.Tools)
	tools = append(tools, submitReportFn, submitValuesFn)

	deployAttempts := 0
	currentValues := in.CurrentValues
	lastToolBatchSig := ""
	toolRepeatStreak := 0

	r.logInfo("recovery loop started",
		zap.String("app", in.AppName),
		zap.String("chart", in.Chart.ChartName),
		zap.String("namespace", in.TargetNS),
		zap.Error(in.DeployErr),
	)

	for step := 0; step < maxRecoverSteps; step++ {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		if r.EmitPhase != nil {
			r.EmitPhase(events.PhaseEvent{QueryID: r.QueryID, Phase: "诊断修复中"})
		}
		r.logDebug("recovery step", zap.Int("step", step+1), zap.Int("messages", recoveryCtx.Len()))

		resp, err := r.LLM.ChatCompletion(ctx, recoveryCtx.Messages(), tools)
		if err != nil {
			r.logError("recover LLM call failed", zap.Int("step", step+1), zap.Error(err))
			return nil, fmt.Errorf("诊断修复过程出错: %w", err)
		}

		if len(resp.ToolCalls) == 0 {
			r.logDebug("recovery LLM returned text only", zap.Int("step", step+1), zap.Int("content_len", len(resp.Content)))
			recoveryCtx.AppendAssistant(llm.Message{Role: "assistant", Content: resp.Content})
			continue
		}

		sig := summarizeToolCallsForLoopDetect(resp.ToolCalls)
		if sig != "" && sig == lastToolBatchSig {
			toolRepeatStreak++
			if toolRepeatStreak >= recoveryLoopRepeatsAbort {
				r.logWarn("recovery repeated identical tool batches", zap.Int("steps", toolRepeatStreak))
				return &Result{
					Action:  ActionAbort,
					Reason:  "诊断循环卡住",
					Details: fmt.Sprintf("连续 %d 次模型发起了相同的工具调用组合，可能存在循环；诊断已中止。请换一种方式描述问题或手动检查集群。", toolRepeatStreak),
				}, nil
			}
		} else if sig != "" {
			lastToolBatchSig = sig
			toolRepeatStreak = 1
		}

		recoveryCtx.AppendAssistant(llm.Message{Role: "assistant", Content: resp.Content, ToolCalls: resp.ToolCalls})

		for _, funcCall := range resp.ToolCalls {
			r.logInfo("recovery tool call", zap.String("tool", funcCall.Function.Name), zap.Int("step", step+1))
			toolResult, done, result, err := r.handleToolCall(
				ctx, funcCall, in.Chart, in.DefaultValues, &currentValues,
				in.TargetNS, in.AppName, &deployAttempts,
			)
			if err != nil {
				return nil, err
			}
			if done {
				return result, nil
			}
			recoveryCtx.AppendToolResult(funcCall.ID, toolResult)
		}
	}

	r.logWarn("recovery loop exhausted", zap.String("app", in.AppName))
	return &Result{
		Action:  ActionAbort,
		Reason:  "诊断达到最大步数",
		Details: fmt.Sprintf("诊断修复已达到最大步数（%d步），未能完成修复。请手动检查集群状态。", maxRecoverSteps),
	}, nil
}

func summarizeToolCallsForLoopDetect(calls []llm.ToolCall) string {
	if len(calls) == 0 {
		return ""
	}
	var b strings.Builder
	for _, c := range calls {
		args, err := json.Marshal(c.Function.Arguments)
		if err != nil {
			args = []byte("{}")
		}
		fmt.Fprintf(&b, "%s:%s;", c.Function.Name, args)
	}
	return b.String()
}

var submitReportFn = llm.FunctionDefinition{
	Name:        "submit_report",
	Description: "诊断结束，输出最终报告。调用此工具会终止诊断阶段。",
	Parameters: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"reason":  map[string]any{"type": "string", "description": "结论摘要"},
			"details": map[string]any{"type": "string", "description": "详细的诊断过程和结论"},
		},
		"required": []string{"reason", "details"},
	},
}

var submitValuesFn = llm.FunctionDefinition{
	Name:        "submit_values",
	Description: "提交修复后的 values 并请求系统重新部署。",
	Parameters: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"yaml":    map[string]any{"type": "string", "description": "完整的新的 override values YAML"},
			"summary": map[string]any{"type": "string", "description": "变更说明"},
		},
		"required": []string{"yaml", "summary"},
	},
}

func (r *Runner) handleToolCall(
	ctx context.Context,
	funcCall llm.ToolCall,
	chartInfo *catalog.ChartInfo,
	defaultValues string,
	currentValues *string,
	targetNS, appName string,
	deployAttempts *int,
) (toolResult string, done bool, result *Result, err error) {
	name := funcCall.Function.Name

	switch name {
	case "submit_report":
		args := funcCall.Function.Arguments
		reason, _ := args["reason"].(string)
		details, _ := args["details"].(string)
		return "", true, &Result{Action: ActionAbort, Reason: reason, Details: details}, nil

	case "submit_values":
		return r.handleSubmitValues(ctx, funcCall, chartInfo, defaultValues, currentValues, targetNS, appName, deployAttempts)

	default:
		t, ok := r.Tools.GetTool(name)
		if !ok {
			r.logWarn("recovery unknown tool", zap.String("tool", name))
			return fmt.Sprintf("未知工具: %s（仅允许诊断类只读工具）", name), false, nil, nil
		}
		out, execErr := r.runDiagnosticTool(ctx, name, func(ctx context.Context) (string, error) {
			return t.Execute(ctx, funcCall.Function.Arguments)
		})
		if execErr != nil {
			r.logWarn("recovery diagnostic tool failed", zap.String("tool", name), zap.Error(execErr))
			return fmt.Sprintf("工具调用错误: %v", execErr), false, nil, nil
		}
		r.logDebug("recovery diagnostic tool ok", zap.String("tool", name), zap.Int("output_len", len(out)))
		return truncateRecoveryToolOutput(out), false, nil, nil
	}
}

func (r *Runner) runDiagnosticTool(ctx context.Context, name string, fn func(context.Context) (string, error)) (string, error) {
	if r.Workflow == nil {
		return fn(ctx)
	}
	return workflow.RunWithResult(r.Workflow, ctx, workflow.Meta{Name: name, Step: 0}, fn)
}

func (r *Runner) handleSubmitValues(
	ctx context.Context,
	funcCall llm.ToolCall,
	chartInfo *catalog.ChartInfo,
	defaultValues string,
	currentValues *string,
	targetNS, appName string,
	deployAttempts *int,
) (string, bool, *Result, error) {
	if *deployAttempts >= maxRecoverDeployAttempts {
		r.logWarn("recovery redeploy limit reached", zap.Int("max", maxRecoverDeployAttempts))
		return fmt.Sprintf("已达到最大重新部署次数（%d次），请调用 submit_report 结束诊断", maxRecoverDeployAttempts), false, nil, nil
	}

	args := funcCall.Function.Arguments
	yamlValues, _ := args["yaml"].(string)
	summary, _ := args["summary"].(string)
	r.logInfo("recovery submit_values", zap.Int("attempt", *deployAttempts+1))

	releaseName := plan.SanitizeReleaseName(appName)
	p := plan.NewDeployPlan(appName, chartInfo, defaultValues, yamlValues, targetNS, r.shouldUpgrade(ctx, releaseName, targetNS))
	p.ReleaseName = releaseName

	validation := plan.ValidateDeployPlan(p)
	p.Warnings = append(p.Warnings, validation.Warnings...)
	if validation.HasBlockingErrors() {
		r.logWarn("recovery values validation failed", zap.Strings("errors", validation.Errors))
		return "values 校验失败: " + strings.Join(validation.Errors, "; "), false, nil, nil
	}

	if err := plan.RunRecoveryHelmPreflight(ctx, r.Helm, &p); err != nil {
		r.logWarn("recovery helm preflight failed", zap.Error(err))
		return "Helm 预检失败: " + err.Error(), false, nil, nil
	}

	if r.EmitCritical != nil {
		r.EmitCritical(events.RenderCodeEvent{QueryID: r.QueryID, Language: "yaml", Content: yamlValues})
		r.EmitCritical(events.RenderTextEvent{QueryID: r.QueryID, Text: summary})
	}

	decision, confirmErr := r.Confirm(ctx, p.ToEventPlan())
	if confirmErr != nil {
		return "", false, nil, confirmErr
	}
	if decision.Action == "cancel" {
		r.logInfo("recovery redeploy cancelled by user")
		return "", true, &Result{
			Action:  ActionAbort,
			Reason:  "用户取消了部署",
			Details: "用户在修复确认中取消了部署。",
		}, nil
	}

	*deployAttempts++
	r.logInfo("recovery redeploy starting", zap.Int("attempt", *deployAttempts))
	rel, installErr := r.Helm.InstallOrUpgrade(ctx, helm.InstallOptions{
		ChartOptions: helm.ChartOptions{
			ReleaseName: releaseName,
			ChartName:   chartInfo.ChartName,
			RepoName:    chartInfo.RepoName,
			RepoURL:     chartInfo.RepoURL,
			Namespace:   targetNS,
			Values:      decision.Values,
		},
		CreateNS: true,
		Wait:     true,
		Timeout:  5 * time.Minute,
	})
	if installErr != nil {
		r.logError("recovery redeploy failed", zap.Error(installErr), zap.Int("attempt", *deployAttempts))
		return fmt.Sprintf("部署失败: %s", installErr.Error()), false, nil, nil
	}

	*currentValues = decision.Values
	r.logInfo("recovery redeploy succeeded", zap.Int("attempt", *deployAttempts))
	report := fmt.Sprintf("✅ %s 修复后部署成功", chartInfo.ChartName)
	if r.BuildReport != nil {
		report = r.BuildReport(ctx, rel, chartInfo, targetNS, plan.SanitizeReleaseName(appName))
	}
	return "", true, &Result{
		Action:  ActionRecovered,
		Reason:  "修复后部署成功",
		Details: report,
		YAML:    decision.Values,
		Summary: summary,
	}, nil
}

func (r *Runner) shouldUpgrade(ctx context.Context, releaseName, namespace string) bool {
	if r.Helm == nil {
		return false
	}
	rel, err := r.Helm.Status(ctx, releaseName, namespace)
	return err == nil && rel != nil && rel.Status == "deployed"
}

func formatCorrectionHistory(history []string) string {
	if len(history) == 0 {
		return "无"
	}
	var sb strings.Builder
	for i, c := range history {
		sb.WriteString(fmt.Sprintf("%d. %s\n", i+1, c))
	}
	return sb.String()
}

func (r *Runner) logInfo(msg string, fields ...zap.Field) {
	if r.Log != nil {
		r.Log.Info(msg, fields...)
	}
}

func (r *Runner) logDebug(msg string, fields ...zap.Field) {
	if r.Log != nil {
		r.Log.Debug(msg, fields...)
	}
}

func (r *Runner) logWarn(msg string, fields ...zap.Field) {
	if r.Log != nil {
		r.Log.Warn(msg, fields...)
	}
}

func (r *Runner) logError(msg string, fields ...zap.Field) {
	if r.Log != nil {
		r.Log.Error(msg, fields...)
	}
}
