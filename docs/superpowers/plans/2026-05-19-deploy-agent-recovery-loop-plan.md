# Deploy Agent Recovery Loop Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the rigid 3-retry diagnostic loop with a free-form ReAct Agent that lets the LLM freely call tools, decide when to abort (via `submit_report`) or retry (via `submit_values`), and output diagnostic reports to the TUI chat box.

**Architecture:** `recoverDeploy()` in `diagnostics.go` replaces `runDiagnostics()` with an up-to-20-step ReAct loop. Two special tools (`submit_report`, `submit_values`) are intercepted by the loop — not registered in the tool registry. `submit_report.details` goes through format detection (`emitRenderEvent`-like logic) and emits Render*Events to TUI. `submit_values` triggers user confirmation + helm retry inside the loop. Phase 6 in `agent.go` is simplified to a single install attempt + `recoverDeploy()` on failure.

**Tech Stack:** Go 1.26, openai-go v3 (LLM function calling), helm v4 Go SDK, bubbletea TUI

---

### Task 1: Define RecoverResult type and emitDetected helper

**Files:**
- Modify: `pkg/agent/deploy/diagnostics.go` — add types + helper at top

- [ ] **Step 1: Add RecoverResult type and emitDetected helper**

Add to the top of `pkg/agent/deploy/diagnostics.go` (replace the file's current content with types + helper, keeping `package deploy`):

```go
package deploy

import (
    "strings"

    "github.com/kubewise/kubewise/pkg/tui/events"
)

// RecoverAction indicates the outcome of a recovery attempt.
type RecoverAction int

const (
    ActionAbort RecoverAction = iota
)

// RecoverResult carries the outcome of recoverDeploy().
// recoverDeploy always returns ActionAbort — helm retries happen internally via submit_values.
type RecoverResult struct {
    Action  RecoverAction
    Reason  string // submit_report reason
    Details string // submit_report details (goes through format detection → TUI)
    YAML    string // submit_values yaml (RenderCodeEvent → TUI)
    Summary string // submit_values summary (RenderTextEvent → TUI)
}

// emitDetected applies emitRenderEvent-style format detection to content
// and emits the appropriate Render*Event to the TUI via a.emit().
// Mirrors router.emitRenderEvent() logic.
func (a *Agent) emitDetected(content string) {
    trimmed := strings.TrimSpace(content)
    if trimmed == "" {
        return
    }

    // 1. YAML code block
    for line := range strings.SplitSeq(content, "\n") {
        t := strings.TrimSpace(line)
        if strings.HasPrefix(t, "apiVersion:") || strings.HasPrefix(t, "kind:") {
            a.emit(events.RenderCodeEvent{QueryID: a.queryID, Language: "yaml", Content: content})
            return
        }
    }

    // 2. JSON code block
    if strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[") {
        a.emit(events.RenderCodeEvent{QueryID: a.queryID, Language: "json", Content: content})
        return
    }

    // 3. Table (pipe-delimited, >= 3 lines with "|")
    if headers, rows, ok := parseTable(content); ok {
        a.emit(events.RenderTableEvent{QueryID: a.queryID, Headers: headers, Rows: rows})
        return
    }

    // 4. Status list (>= 2 lines matching status keywords)
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
        default:
            status = ""
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

    // 5. KV pairs
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
            pairs = append(pairs, events.KVPair{
                Key:   strings.TrimSpace(key),
                Value: strings.TrimSpace(val),
            })
        }
        a.emit(events.RenderKVEvent{QueryID: a.queryID, Pairs: pairs})
        return
    }

    // 6. Default: plain text
    a.emit(events.RenderTextEvent{QueryID: a.queryID, Text: content})
}

// containsAny reports whether s contains any of the given substrings.
func containsAny(s string, subs ...string) bool {
    for _, sub := range subs {
        if strings.Contains(s, sub) {
            return true
        }
    }
    return false
}

// parseTable tries to parse a pipe-delimited markdown table from result.
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
```

Note: This removes the old `runDiagnostics` function and its imports. The old `diagnostics.go` content (maxDiagnosticSteps, runDiagnostics) will be fully replaced in Task 2 when recoverDeploy is implemented.

- [ ] **Step 2: Run test to verify it compiles**

Run: `go build ./pkg/agent/deploy/`
Expected: compiles successfully

- [ ] **Step 3: Commit**

```bash
git add pkg/agent/deploy/diagnostics.go
git commit -m "feat(deploy): add RecoverResult type and emitDetected format helper"
```

---

### Task 2: Implement recoverDeploy() ReAct loop

**Files:**
- Modify: `pkg/agent/deploy/diagnostics.go` — add recoverDeploy(), function definitions, and imports

- [ ] **Step 1: Add function definition vars and recoverDeploy()**

`pkg/agent/deploy/diagnostics.go` final content — append to Task 1 code (replace the entire file with the complete implementation):

```go
package deploy

import (
    "context"
    "encoding/json"
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

type RecoverAction int

const (
    ActionAbort RecoverAction = iota
)

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
// The LLM can call query/troubleshooting tools to gather info, submit_values to
// propose a fix (triggers user confirmation + helm retry), and submit_report to
// finish with a diagnostic report. Returns RecoverResult (always ActionAbort).
// appName is the Helm release name (not chartInfo.ChartName — they may differ).
func (a *Agent) recoverDeploy(
    ctx context.Context,
    deployErr error,
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

    sysPrompt := fmt.Sprintf(`你是 Kubernetes 部署故障诊断与修复助手。

部署失败，请诊断根因并尝试修复。你可以：
1. 调用诊断工具（查询资源、查看事件和日志）收集信息
2. 调用 submit_values 提交修复后的 values（系统会展示给用户确认并重新部署）
3. 调用 submit_report 结束诊断并输出报告

## 部署信息
Chart: %s
应用名: %s
Namespace: %s
错误: %s

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
        chartInfo.ChartName, appName, targetNS, deployErr.Error(),
    )

    messages := []llm.Message{
        {Role: "system", Content: sysPrompt},
        {Role: "user", Content: fmt.Sprintf("请诊断并修复 %s 部署失败: %s", chartInfo.ChartName, deployErr.Error())},
    }

    tools := a.toolRegistry.GetAllFunctionDefinitions()
    tools = append(tools, submitReportFn, submitValuesFn)

    for step := 0; step < maxRecoverSteps; step++ {
        a.emit(events.PhaseEvent{
            QueryID: a.queryID,
            Phase:   "诊断修复中",
        })

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

        // --- submit_report — terminate loop, emit report to TUI ---
        if name == "submit_report" {
            var args struct {
                Reason  string `json:"reason"`
                Details string `json:"details"`
            }
            if err := json.Unmarshal([]byte(funcCall.Function.Arguments), &args); err != nil {
                a.logger().Error("failed to parse submit_report args", zap.Error(err))
                return nil, fmt.Errorf("解析 submit_report 参数失败: %w", err)
            }
            a.emitDetected(args.Details)
            return &RecoverResult{
                Action:  ActionAbort,
                Reason:  args.Reason,
                Details: args.Details,
            }, nil
        }

        // --- submit_values — user confirm + helm retry inside loop ---
        if name == "submit_values" {
            var args struct {
                YAML    string `json:"yaml"`
                Summary string `json:"summary"`
            }
            if err := json.Unmarshal([]byte(funcCall.Function.Arguments), &args); err != nil {
                a.logger().Error("failed to parse submit_values args", zap.Error(err))
                return nil, fmt.Errorf("解析 submit_values 参数失败: %w", err)
            }

            a.emit(events.RenderCodeEvent{QueryID: a.queryID, Language: "yaml", Content: args.YAML})
            a.emit(events.RenderTextEvent{QueryID: a.queryID, Text: args.Summary})

            plan := events.DeployPlan{
                ChartInfo:     chartInfo,
                DefaultValues: defaultValues,
                CustomValues:  args.YAML,
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

            t0 := time.Now()
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

        a.emit(events.ToolCallEvent{
            QueryID:  a.queryID,
            ToolName: name,
            Step:     step + 1,
        })
        t0 := time.Now()
        toolResult, toolErr := tool.Execute(ctx, funcCall.Function.Arguments)
        elapsed := time.Since(t0)
        a.emit(events.ToolDoneEvent{
            QueryID:  a.queryID,
            ToolName: name,
            Step:     step + 1,
            Elapsed:  elapsed,
        })

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

// emitDetected, containsAny, parseTable, isSeparatorRow — same as Task 1
```

- [ ] **Step 2: Write the failing test for recoverDeploy submit_report path**

Add to `pkg/agent/deploy/agent_test.go`. Uses the existing `mockLLMClient` with `chatCompletionFunc`:

```go
func TestRecoverDeploy_SubmitReport(t *testing.T) {
    llmClient := &mockLLMClient{
        chatCompletionFunc: func(ctx context.Context, messages []llm.Message, functions []llm.FunctionDefinition) (*llm.Message, error) {
            args, _ := json.Marshal(map[string]string{
                "reason":  "镜像拉取失败",
                "details": "检查发现镜像标签 latest 不存在",
            })
            return &llm.Message{
                Role: "assistant",
                ToolCalls: []llm.ToolCall{{
                    ID:   "call_1",
                    Type: "function",
                    Function: llm.FunctionCall{
                        Name:      "submit_report",
                        Arguments: string(args),
                    },
                }},
            }, nil
        },
    }
    agent := &Agent{
        llmClient:    llmClient,
        helmClient:   &mockHelmClient{},
        toolRegistry: tool.NewRegistry(tool.ToolDependency{}),
    }

    result, err := agent.recoverDeploy(context.Background(),
        fmt.Errorf("install timed out"),
        &catalog.ChartInfo{ChartName: "nginx", RepoName: "nginx", RepoURL: "https://helm.nginx.com/stable"},
        "replicas: 1\n", "replicas: 3\n", "default", "nginx",
    )

    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    if result.Action != ActionAbort {
        t.Fatalf("expected ActionAbort, got %v", result.Action)
    }
    if result.Reason != "镜像拉取失败" {
        t.Fatalf("expected reason '镜像拉取失败', got %q", result.Reason)
    }
    if !strings.Contains(result.Details, "镜像标签") {
        t.Fatalf("expected details about image tag, got %q", result.Details)
    }
}
```

Also add `"encoding/json"` to the test file imports.

- [ ] **Step 3: Run test to verify it fails**

Run: `go test -run TestRecoverDeploy_SubmitReport ./pkg/agent/deploy/ -v`
Expected: FAIL — recoverDeploy not defined

- [ ] **Step 4: Run test to verify it passes (after implementing recoverDeploy)**

Run: `go test -run TestRecoverDeploy_SubmitReport ./pkg/agent/deploy/ -v`
Expected: PASS

- [ ] **Step 5: Run all deploy tests to check for regressions**

Run: `go test ./pkg/agent/deploy/ -v`
Expected: all existing tests pass (Task 4 handles any that need updating)

- [ ] **Step 6: Commit**

```bash
git add pkg/agent/deploy/diagnostics.go
git commit -m "feat(deploy): implement recoverDeploy free ReAct loop"
```

---

### Task 3: Simplify Phase 6 in agent.go

**Files:**
- Modify: `pkg/agent/deploy/agent.go:253-346` — replace retry loop with single install + recoverDeploy

- [ ] **Step 1: Replace the retry loop block**

In `pkg/agent/deploy/agent.go`, find the Phase 6 block starting at `// Phase 6:` (line 253) through `if err != nil { return "", fmt.Errorf("部署失败: %w", err) }` (line 346). Replace it with:

```go
// Phase 6: 执行 helm install/upgrade — 失败时进入自由诊断修复循环
a.emit(events.PhaseEvent{QueryID: a.queryID, Phase: "执行部署"})

a.emit(events.ToolCallEvent{QueryID: a.queryID, ToolName: "helm install/upgrade", Step: 6})
t0 = time.Now()
rel, err = a.helmClient.InstallOrUpgrade(ctx, helm.InstallOptions{
    ReleaseName: appName,
    RepoName:    chartInfo.RepoName,
    ChartName:   chartInfo.ChartName,
    RepoURL:     chartInfo.RepoURL,
    Namespace:   targetNS,
    Values:      finalValues,
    CreateNS:    true,
    Wait:        true,
    Timeout:     5 * time.Minute,
})
a.emit(events.ToolDoneEvent{QueryID: a.queryID, ToolName: "helm install/upgrade", Step: 6, Elapsed: time.Since(t0)})

if err != nil {
    a.logger().Warn("helm install/upgrade failed, starting recovery loop",
        zap.Error(err),
        zap.String("release", appName),
    )

    result, recErr := a.recoverDeploy(ctx, err, chartInfo, defaultValues, finalValues, targetNS, appName)
    if recErr != nil {
        return "", fmt.Errorf("诊断修复过程出错: %w", recErr)
    }

    // recoverDeploy already emitted Render*Events to TUI via submit_report
    // Return nil error so Router sends StreamDoneEvent (not StreamErrEvent)
    return result.Details, nil
}
```

No import changes needed in agent.go — recoverDeploy is in the same package.

- [ ] **Step 2: Run test to verify it compiles**

Run: `go build ./pkg/agent/deploy/`
Expected: compiles successfully

- [ ] **Step 3: Commit**

```bash
git add pkg/agent/deploy/agent.go
git commit -m "refactor(deploy): simplify Phase 6, remove retry loop in favor of recoverDeploy"
```

---

### Task 4: Update tests

**Files:**
- Modify: `pkg/agent/deploy/agent_test.go` — add recoverDeploy tests, update existing

- [ ] **Step 1: Add test for recoverDeploy nil registry (early return)**

```go
func TestRecoverDeploy_NilRegistry(t *testing.T) {
    agent := &Agent{}
    result, err := agent.recoverDeploy(context.Background(),
        fmt.Errorf("install timed out"),
        &catalog.ChartInfo{ChartName: "nginx"},
        "", "", "default", "nginx",
    )
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    if result.Action != ActionAbort {
        t.Fatalf("expected ActionAbort, got %v", result.Action)
    }
    if !strings.Contains(result.Reason, "诊断工具不可用") {
        t.Fatalf("expected reason about unavailable tools, got %q", result.Reason)
    }
}
```

- [ ] **Step 2: Add test for recoverDeploy step exhaustion**

```go
func TestRecoverDeploy_StepLimit(t *testing.T) {
    // LLM returns text-only responses each step to exhaust the 20-step limit
    callCount := 0
    llmClient := &mockLLMClient{
        chatCompletionFunc: func(ctx context.Context, messages []llm.Message, functions []llm.FunctionDefinition) (*llm.Message, error) {
            callCount++
            return &llm.Message{Role: "assistant", Content: "still thinking..."}, nil
        },
    }
    agent := &Agent{
        llmClient:    llmClient,
        helmClient:   &mockHelmClient{},
        toolRegistry: tool.NewRegistry(tool.ToolDependency{}),
    }

    result, err := agent.recoverDeploy(context.Background(),
        fmt.Errorf("install timed out"),
        &catalog.ChartInfo{ChartName: "nginx"},
        "", "", "default", "nginx",
    )
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    if result.Action != ActionAbort {
        t.Fatalf("expected ActionAbort, got %v", result.Action)
    }
    if !strings.Contains(result.Reason, "最大步数") {
        t.Fatalf("expected reason about step limit, got %q", result.Reason)
    }
}
```

- [ ] **Step 3: Update existing TestHandleQuery_EmitsAllPhaseAndToolEvents**

This test uses the default success path (helm install returns no error). The flow changes slightly:
- No more Step 5 (LLM values regeneration) in the event sequence
- The test expects events at specific indices — verify the event count still matches (≥ 20 events) since Phase 6 still emits ToolCallEvent + ToolDoneEvent for helm install/upgrade Step 6

Run the test first to see if it still passes. If it fails, check which indices changed and adjust the expected event order. The likely change: the old retry loop emitted Step 5 ToolCallEvent/ToolDoneEvent for values regeneration, and the test might have expected those. In the new code, those are gone since values regeneration happens inside recoverDeploy.

Run: `go test -run TestHandleQuery_EmitsAllPhaseAndToolEvents ./pkg/agent/deploy/ -v`
If FAIL: check the actual event order (the test logs all events) and update the expected indices accordingly.

- [ ] **Step 4: Run all tests**

Run: `go test ./pkg/agent/deploy/ -v`
Expected: all tests pass

- [ ] **Step 5: Commit**

```bash
git add pkg/agent/deploy/agent_test.go
git commit -m "test(deploy): add recoverDeploy tests, update event emission test"
```

---

### Task 5: Run full verification

- [ ] **Step 1: Run go vet**

Run: `go vet ./pkg/agent/deploy/`
Expected: no vet errors

- [ ] **Step 2: Build binary**

Run: `go build -o /dev/null ./cmd/`
Expected: compiles successfully

- [ ] **Step 3: Final commit if needed**

```bash
git add -A
git commit -m "chore(deploy): cleanup after recoverDeploy refactor"
```
