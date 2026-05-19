package deploy

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/kubewise/kubewise/pkg/catalog"
	"github.com/kubewise/kubewise/pkg/helm"
	"github.com/kubewise/kubewise/pkg/llm"
	"github.com/kubewise/kubewise/pkg/tui/events"
)

const maxRecoverSteps = 20

// --- types ---

// RecoverAction indicates the outcome of a recovery attempt.
type RecoverAction int

const (
	ActionAbort RecoverAction = iota
)

// RecoverResult carries the outcome of recoverDeploy().
type RecoverResult struct {
	Action  RecoverAction
	Reason  string
	Details string
	YAML    string
	Summary string
}

// --- function definitions for LLM ---

var submitReportFn = llm.FunctionDefinition{
	Name:        "submit_report",
	Description: "诊断结束，输出最终报告。调用此工具会终止诊断阶段。",
	Parameters: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"reason": map[string]any{
				"type":        "string",
				"description": "结论摘要",
			},
			"details": map[string]any{
				"type":        "string",
				"description": "详细的诊断过程和结论。支持纯文本、YAML、JSON、表格、键值对、列表等形式，系统自动识别格式。",
			},
		},
		"required": []string{"reason", "details"},
	},
}

var submitValuesFn = llm.FunctionDefinition{
	Name:        "submit_values",
	Description: "提交修复后的 values 并请求系统重新部署。系统会展示变更给用户确认，执行 helm install，然后将部署结果返回。如果再次失败可以继续诊断。",
	Parameters: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"yaml": map[string]any{
				"type":        "string",
				"description": "完整的新的 override values YAML",
			},
			"summary": map[string]any{
				"type":        "string",
				"description": "变更说明",
			},
		},
		"required": []string{"yaml", "summary"},
	},
}

// recoverDeploy enters a free-form ReAct loop to diagnose and fix deployment failures.
// query is the original user request, correctionHistory tracks NL corrections from Phase 5.
func (a *Agent) recoverDeploy(
	ctx context.Context,
	deployErr error,
	query string,
	correctionHistory []string,
	chartInfo *catalog.ChartInfo,
	defaultValues string,
	currentValues string,
	targetNS string,
	appName string,
) (*RecoverResult, error) {
	if a.toolRegistry == nil {
		return &RecoverResult{
			Action:  ActionAbort,
			Reason:  "诊断工具不可用",
			Details: "诊断工具不可用，无法进行故障排查。请手动检查集群状态。",
		}, nil
	}

	// Build correction history text
	correctionText := "无"
	if len(correctionHistory) > 0 {
		var sb strings.Builder
		for i, c := range correctionHistory {
			sb.WriteString(fmt.Sprintf("%d. %s\n", i+1, c))
		}
		correctionText = sb.String()
	}

	sysPrompt := fmt.Sprintf(`你是 Kubernetes 部署故障诊断与修复助手。

部署失败，请诊断根因并尝试修复。你可以：
1. 调用诊断工具（查询资源、查看事件和日志）收集信息
2. 调用 submit_values 提交修复后的 values（系统会展示给用户确认并重新部署）
3. 调用 submit_report 结束诊断并输出报告

## 用户需求
%s

## 部署信息
Chart: %s
应用名: %s
Namespace: %s
错误: %s

## 当前 override values
%s

## 配置修正历史（你在之前已经和用户进行过这些自然语言修正对话）
%s
## 关键 GVR
- Pod: group="", version="v1", resource="pods"
- Deployment: group="apps", version="v1", resource="deployments"
- StatefulSet: group="apps", version="v1", resource="statefulsets"
- Service: group="", version="v1", resource="services"
- Event: group="", version="v1", resource="events"

## 工作流程
1. 先调用诊断工具了解失败原因
2. 如果能修复，调用 submit_values 提交新 values
3. 系统会执行 helm install 并将结果返回给你
4. 如果修复成功，调用 submit_report 报告结果
5. 如果无法修复，在 details 中说明排查过程和根因，调用 submit_report 结束`,
		query, chartInfo.ChartName, appName, targetNS, deployErr.Error(), currentValues, correctionText,
	)

	messages := []llm.Message{
		{Role: "system", Content: sysPrompt},
		{Role: "user", Content: fmt.Sprintf("请诊断并修复 %s 部署失败: %s", chartInfo.ChartName, deployErr.Error())},
	}

	tools := a.toolRegistry.GetAllFunctionDefinitions()
	tools = append(tools, submitReportFn, submitValuesFn)

	for step := 0; step < maxRecoverSteps; step++ {
		a.emit(events.PhaseEvent{QueryID: a.queryID, Phase: "诊断修复中"})

		resp, err := a.llmClient.ChatCompletion(ctx, messages, tools)
		if err != nil {
			a.logger().Error("recover LLM call failed", zap.Error(err))
			return nil, fmt.Errorf("诊断修复过程出错: %w", err)
		}

		if len(resp.ToolCalls) == 0 {
			messages = append(messages, llm.Message{Role: "assistant", Content: resp.Content})
			continue
		}

		funcCall := resp.ToolCalls[0]
		name := funcCall.Function.Name

		// --- submit_report ---
		if name == "submit_report" {
			args := funcCall.Function.Arguments
			reason, _ := args["reason"].(string)
			details, _ := args["details"].(string)
			a.emitDetected(details)
			return &RecoverResult{
				Action:  ActionAbort,
				Reason:  reason,
				Details: details,
			}, nil
		}

		// --- submit_values ---
		if name == "submit_values" {
			args := funcCall.Function.Arguments
			yaml, _ := args["yaml"].(string)
			summary, _ := args["summary"].(string)

			a.emit(events.RenderCodeEvent{QueryID: a.queryID, Language: "yaml", Content: yaml})
			a.emit(events.RenderTextEvent{QueryID: a.queryID, Text: summary})

			plan := events.DeployPlan{
				ChartInfo:     chartInfo,
				DefaultValues: defaultValues,
				CustomValues:  yaml,
				ReleaseName:   appName,
				Namespace:     targetNS,
				IsUpgrade:     true,
			}
			decision, confirmErr := a.confirmDeploy(ctx, plan)
			if confirmErr != nil {
				return nil, confirmErr
			}
			if decision.Action == "cancel" {
				return &RecoverResult{
					Action:  ActionAbort,
					Reason:  "用户取消了部署",
					Details: "用户在修复确认中取消了部署。",
				}, nil
			}

			_, installErr := a.helmClient.InstallOrUpgrade(ctx, helm.InstallOptions{
				ReleaseName: appName,
				ChartName:   chartInfo.ChartName,
				RepoName:    chartInfo.RepoName,
				RepoURL:     chartInfo.RepoURL,
				Namespace:   targetNS,
				Values:      decision.Values,
				CreateNS:    true,
				Wait:        true,
				Timeout:     5 * time.Minute,
			})

			toolResult := "部署成功"
			if installErr != nil {
				toolResult = fmt.Sprintf("部署失败: %s", installErr.Error())
			}
			messages = append(messages,
				llm.Message{Role: "assistant", Content: "", ToolCalls: resp.ToolCalls},
				llm.Message{Role: "tool", Content: toolResult, ToolCallID: funcCall.ID},
			)
			continue
		}

		// --- Regular tool execution ---
		tool, ok := a.toolRegistry.GetTool(name)
		if !ok {
			result := fmt.Sprintf("未知工具: %s", name)
			messages = append(messages,
				llm.Message{Role: "assistant", Content: "", ToolCalls: resp.ToolCalls},
				llm.Message{Role: "tool", Content: result, ToolCallID: funcCall.ID},
			)
			continue
		}

		a.emit(events.ToolCallEvent{QueryID: a.queryID, ToolName: name, Step: step + 1})
		t0 := time.Now()
		toolResult, toolErr := tool.Execute(ctx, funcCall.Function.Arguments)
		elapsed := time.Since(t0)
		a.emit(events.ToolDoneEvent{QueryID: a.queryID, ToolName: name, Step: step + 1, Elapsed: elapsed})

		if toolErr != nil {
			toolResult = fmt.Sprintf("工具调用错误: %v", toolErr)
		}

		messages = append(messages,
			llm.Message{Role: "assistant", Content: "", ToolCalls: resp.ToolCalls},
			llm.Message{Role: "tool", Content: toolResult, ToolCallID: funcCall.ID},
		)
	}

	a.logger().Warn("recoverDeploy ReAct loop exhausted", zap.String("app", appName))
	return &RecoverResult{
		Action:  ActionAbort,
		Reason:  "诊断达到最大步数",
		Details: fmt.Sprintf("诊断修复已达到最大步数（%d步），未能完成修复。请手动检查集群状态。", maxRecoverSteps),
	}, nil
}

// --- emitDetected and helpers ---

// emitDetected applies format detection to content and emits the appropriate Render*Event.
func (a *Agent) emitDetected(content string) {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return
	}

	for line := range strings.SplitSeq(content, "\n") {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "apiVersion:") || strings.HasPrefix(t, "kind:") {
			a.emit(events.RenderCodeEvent{QueryID: a.queryID, Language: "yaml", Content: content})
			return
		}
	}

	if strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[") {
		a.emit(events.RenderCodeEvent{QueryID: a.queryID, Language: "json", Content: content})
		return
	}

	if headers, rows, ok := parseTable(content); ok {
		a.emit(events.RenderTableEvent{QueryID: a.queryID, Headers: headers, Rows: rows})
		return
	}

	lines := strings.Split(content, "\n")
	statusOf := make(map[int]string)
	matchCount := 0
	for i, line := range lines {
		if line == "" {
			continue
		}
		lower := strings.ToLower(line)
		var status string
		switch {
		case containsAny(lower, "error", "failed", "crashloopbackoff", "unhealthy", "critical"):
			status = "error"
		case containsAny(lower, "pending", "terminating", "warning"):
			status = "warn"
		case containsAny(lower, "running", "healthy"):
			status = "ok"
		}
		if status != "" {
			statusOf[i] = status
			matchCount++
		}
	}
	if matchCount >= 2 {
		items := make([]events.ListItem, 0)
		for i, line := range lines {
			if line == "" {
				continue
			}
			s, ok := statusOf[i]
			if !ok {
				s = "info"
			}
			items = append(items, events.ListItem{Status: s, Text: line})
		}
		a.emit(events.RenderListEvent{QueryID: a.queryID, Items: items})
		return
	}

	var kvLines []string
	var nonEmptyCount int
	for _, l := range lines {
		if strings.TrimSpace(l) == "" {
			continue
		}
		nonEmptyCount++
		if idx := strings.Index(l, ": "); idx > 0 {
			before := strings.TrimSpace(l[:idx])
			if before != "" && !strings.Contains(before, " ") {
				kvLines = append(kvLines, l)
			}
		}
	}
	if len(kvLines) >= 2 && nonEmptyCount > 0 && len(kvLines)*2 >= nonEmptyCount {
		pairs := make([]events.KVPair, 0, len(kvLines))
		for _, l := range kvLines {
			key, val, _ := strings.Cut(l, ": ")
			pairs = append(pairs, events.KVPair{Key: strings.TrimSpace(key), Value: strings.TrimSpace(val)})
		}
		a.emit(events.RenderKVEvent{QueryID: a.queryID, Pairs: pairs})
		return
	}

	a.emit(events.RenderTextEvent{QueryID: a.queryID, Text: content})
}

func containsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

func parseTable(content string) (headers []string, rows [][]string, ok bool) {
	lines := strings.Split(content, "\n")
	var tableLines []string
	for _, l := range lines {
		if strings.Contains(l, "|") {
			tableLines = append(tableLines, l)
		}
	}
	if len(tableLines) < 3 {
		return nil, nil, false
	}
	for _, l := range tableLines {
		if isSeparatorRow(l) {
			continue
		}
		if len(headers) == 0 {
			for cell := range strings.SplitSeq(l, "|") {
				cell = strings.TrimSpace(cell)
				if cell != "" {
					headers = append(headers, cell)
				}
			}
		} else {
			var row []string
			for cell := range strings.SplitSeq(l, "|") {
				cell = strings.TrimSpace(cell)
				if cell != "" {
					row = append(row, cell)
				}
			}
			if len(row) > 0 {
				rows = append(rows, row)
			}
		}
	}
	return headers, rows, len(headers) > 0 && len(rows) > 0
}

func isSeparatorRow(line string) bool {
	hasCell := false
	for cell := range strings.SplitSeq(line, "|") {
		cell = strings.TrimSpace(cell)
		if cell == "" {
			continue
		}
		hasCell = true
		for _, ch := range cell {
			if ch != '-' && ch != ':' {
				return false
			}
		}
	}
	return hasCell
}
