# 诊断真实流设计

> 日期: 2026-06-10
> 状态: 已批准待实施

## 1. 问题陈述

当前诊断流程完全是 mock 的：

- `POST /api/v1/diagnose` 只注册了 runner，不执行实际诊断工作
- `GET /api/v1/diagnose/stream` 不是真正的 SSE 流（先 Drain 再返回，不保持连接）
- 前端 `DiagnosisOverlay` 用 `setTimeout` 模拟步骤，`mockDiagnosisResult` 硬编码 OOMKilled
- `frontend/src/api/sse.ts` 的 `subscribeDiagnosis()` 已写好但从未被 import

ChatStream 已经走通了同样的模式（`routerAgent.HandleQueryStream` → `eventCh` → `bridgeStreamEvent` → SSE），诊断只需照着做。

## 2. 设计目标

1. **诊断有真实输出** — 点击 Diagnose 后真正触发 troubleshooting Agent，走完整 ReAct 循环
2. **SSE 实时流** — 诊断步骤和中间结果实时推送到前端
3. **前端显示真实数据** — DiagnosisOverlay 从 SSE 流接收事件更新进度，最终从 API 获取诊断报告
4. **数据可持久化** — 诊断结果入 SQLite，跨会话可查

## 3. 数据流

```
1. 用户点 Diagnose →
2. POST /api/v1/diagnose {cluster, namespace, pod}
3. 后端 StartDiagnose:
   a. runner.Start(ctx, id)
   b. 启动 goroutine：
      - 创建 eventCh (缓冲 64)
      - query = "Diagnose pod {pod} in namespace {ns} on cluster {cluster}"
      - h.querier.HandleQueryStream(ctx, query, queryID, eventCh)
        → classifyIntent → troubleshooting agent → ReAct 循环
      - for ev := range eventCh:
          bridgeAgentEventToDiagnosis(ev) → runner.PushEvent(ctx, id, streamEvent)
      - 完成后 runner.Finish(ctx, id)
4. 同时 GET /api/v1/diagnose/stream?id=xxx
   - 新建 SSE 连接
   - 轮询 runner.GetBuffer(id) 是否有新事件
   - 有则写出 SSE（diagnosis_event 类型）
   - StreamDone/连接断开时关闭
5. 前端 DiagnosisOverlay:
   - api.diagnoses.create({cluster, namespace, pod}) → 得到 {diagnosis_id, status}
   - subscribeDiagnosis(diagnosis_id, onEvent, onDone)
   - onEvent: 更新步骤进度
   - onDone: api.diagnoses.get(diagnosis_id) → 渲染报告
```

## 4. 后端设计

### 4.1 StartDiagnose — 发起到 troubleshooting Agent 的异步诊断

```go
func (h *Handler) StartDiagnose(c *echo.Context) error {
    ctx := c.Request().Context()
    // ... bind + validate req ...

    diagID := uuid.New().String()
    h.diagnosisRunner.Start(ctx, diagID)

    // 异步执行诊断
    go func() {
        eventCh := make(chan event.Event, 64)
        queryID := fmt.Sprintf("diag-%s", uuid.New().String()[:8])

        // 构造诊断查询
        query := fmt.Sprintf("Diagnose pod '%s' in namespace '%s' on cluster '%s'",
            req.Pod, req.Namespace, req.Cluster)

        // 触发 Agent 流
        err := h.querier.HandleQueryStream(ctx, query, queryID, eventCh)
        if err != nil {
            h.diagnosisRunner.PushEvent(ctx, diagID, diagnosis.StreamEvent{
                Type: "error", Message: "Diagnosis failed", Detail: err.Error(),
            })
        }

        // 消费事件通道
        for ev := range eventCh {
            if se := bridgeAgentEventToDiagnosis(ev); se != nil {
                h.diagnosisRunner.PushEvent(ctx, diagID, *se)
            }
        }

        h.diagnosisRunner.Finish(ctx, diagID)
    }()

    return c.JSON(http.StatusAccepted, DiagnoseResponse{
        DiagnosisID: diagID, Status: "running",
    })
}
```

### 4.2 Agent Event → StreamEvent 桥接

```go
// bridgeAgentEventToDiagnosis converts agent event.Event types to
// diagnosis.StreamEvent for SSE streaming to the frontend.
func bridgeAgentEventToDiagnosis(ev event.Event) *diagnosis.StreamEvent {
    switch e := ev.(type) {
    case event.Phase:
        return &diagnosis.StreamEvent{Type: "phase", Message: e.Phase}
    case event.AgentStart:
        return &diagnosis.StreamEvent{Type: "phase", Message: e.AgentName + " started"}
    case event.AgentDone:
        return &diagnosis.StreamEvent{Type: "done", Message: e.Result}
    case event.ToolCall:
        return &diagnosis.StreamEvent{Type: "tool", Message: e.ToolName}
    case event.ToolDone:
        return &diagnosis.StreamEvent{Type: "tool_done", Message: e.ToolName, Detail: fmt.Sprintf("%.0fms", e.Elapsed)}
    case event.ToolFail:
        return &diagnosis.StreamEvent{Type: "tool_fail", Message: e.ToolName, Detail: e.Err}
    case event.LLMTextDelta:
        return &diagnosis.StreamEvent{Type: "thought", Message: e.Delta}
    case event.StreamDone:
        return &diagnosis.StreamEvent{Type: "stream_done", Message: "completed"}
    case event.StreamErr:
        msg := ""
        if e.Err != nil { msg = e.Err.Error() }
        return &diagnosis.StreamEvent{Type: "error", Message: msg}
    default:
        return nil // skip unknown event types
    }
}
```

这个函数放在 `internal/api/handler/stream.go`（和 `ChatStream` 的 `bridgeStreamEvent` 放一起）。

### 4.3 StreamDiagnosisEvents — 真 SSE 流

修改函数，每 500ms 轮询 runner buffer：

```go
func (h *Handler) StreamDiagnosisEvents(c *echo.Context) error {
    ctx := c.Request().Context()
    id := c.QueryParam("id")
    if id == "" {
        return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "id parameter is required"})
    }

    sse, err := ssestream.NewSSEWriter(c.Response())
    if err != nil {
        return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
    }

    ticker := time.NewTicker(500 * time.Millisecond)
    defer ticker.Stop()

    // 已发送的 event 数量，避免重复
    sent := 0

    for {
        select {
        case <-ctx.Done():
            return nil
        case <-ticker.C:
            buf := h.diagnosisRunner.GetBuffer(id)
            if buf == nil {
                // 诊断已完成或不存在
                return nil
            }
            events := buf.ReadSince(sent)
            for _, ev := range events {
                if err := sse.WriteEvent("diagnosis_event", ev); err != nil {
                    return nil
                }
                sent++
                if ev.Type == "stream_done" || ev.Type == "error" {
                    return nil
                }
            }
        }
    }
}
```

注意：需要 `RingBuffer` 新增 `ReadSince(index int)` 方法，返回从指定位置开始的新事件。

### 4.4 RingBuffer.ReadSince

```go
// ReadSince returns all events appended after position `since`.
// Returns nil if the buffer no longer exists.
func (rb *RingBuffer) ReadSince(since int) []StreamEvent {
    rb.mu.Lock()
    defer rb.mu.Unlock()
    if since >= rb.totalWritten {
        return nil
    }
    // calculate indexes and return new events
    count := rb.totalWritten - since
    result := make([]StreamEvent, 0, count)
    for i := 0; i < count; i++ {
        idx := (since + i) % rb.capacity
        result = append(result, rb.buffer[idx])
    }
    return result
}
```

新增 `totalWritten` 计数器（Push 时递增），用于追踪已写总数。

## 5. 前端设计

### 5.1 DiagnosisOverlay 改造

去掉 mock 的 `startDiagnosis()` 函数和 `mockDiagnosisResult`，改为：

```typescript
const startDiagnosis = async () => {
  setPhase('running');
  onActivity('pending', `Diagnosing ${pod}...`, cluster);

  try {
    // 1. 发起诊断
    const { diagnosis_id } = await api.diagnoses.create(cluster, namespace, pod);

    // 2. 订阅 SSE 流
    const unsubscribe = subscribeDiagnosis(diagnosis_id, (ev) => {
      // 根据 event type 更新步骤
      if (ev.type === 'phase') setStatusText(ev.message ?? '');
      if (ev.type === 'tool') setCurrentTool(ev.message ?? '');
      // 等
    }, async () => {
      // 3. 流结束，获取完整报告
      const detail = await api.diagnoses.get(diagnosis_id);
      setResult(detailToResult(detail));
      setPhase('done');
      onActivity('done', `${pod} diagnosis complete`, cluster);
    });
  } catch (err) {
    setPhase('idle');
    onActivity('issue', `Diagnosis failed: ${err}`, cluster);
  }
};
```

### 5.2 SSE URL 修复

`sse.ts` 的 URL 改为使用 API client 的 BASE：

```typescript
const BASE = 'http://localhost:3000/api/v1';
export function subscribeDiagnosis(...) {
  const es = new EventSource(`${BASE}/diagnose/stream?id=${encodeURIComponent(id)}`);
  // ...
}
```

### 5.3 类型映射

前端 `StreamEvent` 类型（`{type, message?, detail?}`）已经能直接接收后端 `diagnosis.StreamEvent` JSON 序列化结果，无需额外适配。

## 6. 数据持久化

诊断完成后（`HandleQueryStream` 返回），提取结果存入 SQLite：

```go
// 从 AgentDone 事件中提取根因、证据等
// 存为 DiagnosisDetail → SQLite
if h.activityService != nil {
    h.activityService.Add(ctx, activity.TypeDiagnosis,
        fmt.Sprintf("Diagnosis: %s - %s", req.Pod, rootCause),
        req.Cluster, diagID)
}
```

这部分在 Phase 1 可以做简化——先保证 SSE 流能走通，持久化可以后续补。

## 7. 受影响文件

### 后端
| 文件 | 变更 |
|------|------|
| `internal/api/handler/diagnosis.go` | StartDiagnose 加 goroutine 跑 Agent；StreamDiagnosisEvents 改为真 SSE 轮询 |
| `internal/api/handler/stream.go` | 新增 `bridgeAgentEventToDiagnosis()` |
| `internal/diagnosis/buffer.go` | 新增 `totalWritten` 计数器 + `ReadSince()` 方法 |

### 前端
| 文件 | 变更 |
|------|------|
| `frontend/src/components/DiagnosisOverlay.tsx` | 替换 mock → 调 API + SSE |
| `frontend/src/api/sse.ts` | URL 从常量拿 |

## 8. 不做的事情

- **诊断结果的持久化（SQLite）** — 第一阶段只保证流能通，诊断日志先跟随 SSE 生命周期
- **前端缓存** — 去掉 `cacheRef` 的 5 分钟客户端缓存，每次打开都重新诊断（或后续再优化）
- **诊断历史页面** — `ListDiagnoses` 返回 `[]` 的问题留到 Phase 2
- **LATS-RCA 树搜索** — 仍然走 troubleshooting Agent 的 ReAct 循环，不上完整树搜索