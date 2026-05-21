# Deploy Agent 错误诊断与修复 — 自由 Agent 循环设计

## 问题

当前 deploy agent 在 Helm 安装失败后的错误恢复流程不合理：

1. **诊断阶段 (`runDiagnostics`)** 被限制在最多 5 步 ReAct，步数耗尽时必须输出结论
2. **结论只有两个枚举值**：`[ACTION: retry]`（重新生成 values）或 `[ACTION: abort]`（放弃）
3. **诊断和修复被强行拆分**：诊断完成后要么放弃、要么调用 `regenerateValues()` 单独修复
4. **模型没有自主权**：不能自己决定何时停止、输出什么格式、是否要反复试验

## 目标

将 Phase 6b（错误恢复）改造成一个统一的自由 Agent 循环：

- 模型在循环内自由调用 query + troubleshooting 工具收集信息
- 模型通过调用特定工具（`submit_report` / `submit_values`）来输出结果
- 模型自己决定何时已经收集了足够的信息、应该输出什么
- 诊断报告通过 TUI 交互框展示，而不是作为错误信息丢失

## 设计

### 整体流程

```
Phase 6: Helm install 失败
    │
    ▼
Phase 6b: recoverDeploy() — 自由 ReAct Agent（最多 20 步）
    │  • 工具: query + troubleshooting
    │  • 输出工具: submit_report / submit_values
    │  • TUI: progress card 显示 "诊断修复中" + 工具调用进度
    │  • 模型自主决定调用什么工具、调多少轮、何时输出结果
    │  • 排查过程的工具调用在 progress card 展示，最终报告通过 submit 工具的参数带到 TUI 交互框
    │
    ├──→ 调用 submit_report(reason, details)
    │     │  系统从 details 提取内容，用 emitRenderEvent() 格式检测
    │     │  发 Render*Event 到 TUI 交互框（pending）
    │     │  recoverDeploy 返回 AbortResult
    │     │  HandleQuery return (reportString, nil) → Router → StreamDoneEvent
    │     │  TUI 交互框显示诊断报告 ✅（零额外工具调用、零额外 token）
    │
    └──→ 调用 submit_values(yaml, summary)
          │  系统从 yaml → RenderCodeEvent(yaml)，从 summary → RenderTextEvent 到 TUI
          │  ▼
          │  DeployConfirmRequestEvent（用户确认弹窗）
          │  ▼
          ├── 用户取消 → recoverDeploy 返回 "部署已取消"
          └── 用户确认 → helm install/upgrade 重试
               │
               ├── 成功 → 结果以 tool response 喂回给模型
               │        → 模型继续 ReAct，可调 submit_report("部署成功") 结束
               │
               └── 再次失败 → 结果喂回给模型
                            → 模型继续诊断、再试、或 submit_report 放弃
```

### recoverDeploy() 函数

```go
type RecoverAction int

const (
    ActionAbort RecoverAction = iota
)

type RecoverResult struct {
    Action  RecoverAction
    Reason  string   // submit_report 的 reason
    Details string   // submit_report 的 details — 经 emitRenderEvent 发到 TUI
    YAML    string   // submit_values 的 yaml — RenderCodeEvent 到 TUI
    Summary string   // submit_values 的 summary — RenderTextEvent 到 TUI
}

func (a *DeployAgent) recoverDeploy(
    ctx context.Context,
    deployErr error,
    chartInfo *catalog.ChartInfo,
    defaultValues string,
    currentValues string,
    targetNS string,
) (*RecoverResult, error)
```

recoverDeploy 只有一条返回路径：`AbortResult`。submit_values 的 helm 重试在循环内部完成，不会从 recoverDeploy 向外返回"我要重试"的信号。

### 输出工具定义

两个由 ReAct 循环拦截的特殊工具（不经过 tool.Registry）：

#### submit_report — 结束诊断

```go
{
    Name: "submit_report",
    Description: "诊断结束，输出最终报告。调用此工具会终止诊断阶段。",
    Parameters: map[string]any{
        "type": "object",
        "properties": map[string]any{
            "reason": map[string]any{
                "type": "string",
                "description": "结论摘要，如 '部署成功' 或 'PVC 不存在'",
            },
            "details": map[string]any{
                "type": "string",
                "description": "详细的诊断过程和结论。支持纯文本、YAML、JSON、表格（|分隔）、键值对、列表等形式，系统会自动识别格式并渲染到界面。",
            },
        },
        "required": []string{"reason", "details"},
    },
}
```

系统收到 submit_report 后，将 `details` 参数字符串传给 `emitRenderEvent()` 同款的格式检测逻辑（YAML → RenderCodeEvent, 表格 → RenderTableEvent, 列表 → RenderListEvent, KV → RenderKVEvent, 纯文本 → RenderTextEvent），把报告内容渲染到 TUI 交互框。

#### submit_values — 提交修复方案

```go
{
    Name: "submit_values",
    Description: "提交修复后的 values 并请求系统重新部署。"
                + "系统会展示变更给用户确认，执行 helm install，"
                + "然后将部署结果返回。如果再次失败可以继续诊断。",
    Parameters: map[string]any{
        "type": "object",
        "properties": map[string]any{
            "yaml": map[string]any{
                "type": "string",
                "description": "完整的新的 override values YAML",
            },
            "summary": map[string]any{
                "type": "string",
                "description": "变更说明，如 '将副本数从 3 改为 1 以适配节点资源'",
            },
        },
        "required": []string{"yaml", "summary"},
    },
}
```

系统收到 submit_values 后：
1. `yaml` → 用 RenderCodeEvent(yaml) 展示新配置到 TUI
2. `summary` → 用 RenderTextEvent 展示变更说明到 TUI
3. 走 DeployConfirmRequestEvent 用户确认
4. 执行 helm install/upgrade
5. 结果作为 tool response 喂回给模型继续

### 上下文维护

遵循标准 ReAct 模式：每次 LLM 调用后，所有 tool call + tool result 都追加到 messages 数组。模型每一步都看到完整历史：

```
system: [诊断助手 system prompt + 部署信息]
user: "诊断 nginx 部署失败: timed out waiting for condition"
  ─── 第 1 轮 ───
assistant: tool_call get_pods(namespace=default)
tool: "Pod nginx-xxx CrashLoopBackOff"
  ─── 第 2 轮 ───
assistant: tool_call get_pod_events(namespace=default, pod=nginx-xxx)
tool: "Failed to pull image nginx:latest"
  ─── 第 3 轮 ───
assistant: tool_call submit_values(yaml="...", summary="修正镜像标签")
...
```

排查进度在 progress card 展示（ToolCallEvent / ToolDoneEvent），最终报告通过 submit 工具参数带到 TUI 交互框。

### ReAct 循环细节

```
输入:
  - 部署错误信息 + 当前 values + chart 信息
  - system prompt + tool definitions + user message

循环 (最多 20 步):
  1. emit PhaseEvent("诊断修复中")
  2. LLM ChatCompletion (with tools = query + troubleshooting + submit_report + submit_values)
  3. 检查 LLM 响应:
     a. 无 tool_calls → 文本追加到 messages，继续
     b. 调用了 submit_report(reason, details) →
        - 系统用 emitRenderEvent() 同款逻辑检测 details 格式
        - emit 对应的 Render*Event 到 TUI
        - recoverDeploy 返回 AbortResult{Reason, Details}
     c. 调用了 submit_values(yaml, summary) →
        - 系统用 yaml → RenderCodeEvent 到 TUI
        - 系统用 summary → RenderTextEvent 到 TUI
        - 走 DeployConfirmRequestEvent bridge（用户确认弹窗）
        - 用户取消 → 返回 AbortResult{"已取消", ""}
        - 用户确认 → helm install/upgrade
        - 结果作为 tool response 喂回给模型
        - 继续循环
     d. 调用了其他工具 →
        - emit ToolCallEvent / ToolDoneEvent（progress card）
        - 执行工具，结果追加到 messages
        - 继续循环

  4. 步数耗尽 → 返回 AbortResult{"诊断达到最大步数", "请手动检查集群状态"}

输出: RecoverResult (Always ActionAbort)
```

### HandleQuery 集成

Phase 6 的 retry 循环（当前行 257-346）被替换为：

```go
err = a.helmClient.InstallOrUpgrade(...)
if err != nil {
    result, recErr := a.recoverDeploy(ctx, err, chartInfo, defaultValues, finalValues, targetNS)
    if recErr != nil {
        return "", fmt.Errorf("诊断修复过程出错: %w", recErr)
    }
    // Render*Events already emitted to TUI inside recoverDeploy
    // Return nil error → Router → StreamDoneEvent → TUI chat box
    return result.Details, nil
}
```

### TUI 事件流

```
AgentStartEvent("Deploy Agent")
  PhaseEvent("搜索 Chart: nginx")
  ...
  PhaseEvent("执行部署")
  ToolCallEvent("helm install/upgrade", Step 6)
  ToolDoneEvent("helm install/upgrade", Step 6, fail)
  ↓
  // recoverDeploy 开始
  PhaseEvent("诊断修复中")
  ToolCallEvent("get_pods", Step 1)
  ToolDoneEvent("get_pods", Step 1)
  ToolCallEvent("get_resource_events", Step 2)
  ToolDoneEvent("get_resource_events", Step 2)
  ...
  // 模型决定 submit_report，details = "...Pod 事件...\n...原因..."
  // 系统检测 details 格式，发 RenderListEvent + RenderCodeEvent 到 TUI（pending）
  ↓
AgentDoneEvent
StreamDoneEvent → 所有 pending 的 Render* 合并成一个 chatEntry 显示在交互框
```

### 对 diagnostics.go 的影响

整个文件被重写为 `recoverDeploy()`。旧的 `runDiagnostics()` 函数被删除。

### 对 agent.go 的影响

- Phase 6 的 retry for 循环（行 257-346）被大幅简化
- `maxRetries = 3` 被移除——重试决策完全由模型控制
- `confirmDeploy()` 仍然用于 submit_values 流程中的用户确认

### 对 events.go 的影响

无需新增事件类型——现有的 `RenderTextEvent`、`RenderCodeEvent`、`RenderListEvent`、`RenderTableEvent`、`RenderKVEvent`、`StreamDoneEvent` 完全够用。

### 边界情况

| 场景 | 行为 |
|------|------|
| ReAct 20 步耗尽 | 返回 AbortResult{"诊断达到最大步数", "请手动检查"} |
| LLM token 耗尽 | recoverDeploy 返回 error，HandleQuery 返回 "诊断修复过程出错" |
| 模型一直调用工具不输出 | 20 步上限兜底 |
| 模型多次调用 submit_values | 每次走用户确认 + helm 重试，模型看到结果后继续 |
| 用户反复取消 submit_values | 每次取消后模型继续 ReAct，直到 submit_report 或步数耗尽 |
| 工具全部失败（集群不可达） | 工具返回 error 作为 tool response，模型据此决定 submit_report |
| submit_values 后 helm 成功 | 结果喂回模型，模型调 submit_report("成功") 结束 |
| submit_values 后 helm 再次失败 | 结果喂回模型，模型继续诊断或 submit_report |
