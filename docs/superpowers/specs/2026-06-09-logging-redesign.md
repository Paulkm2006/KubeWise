# 后端全链路日志改造设计

> 日期: 2026-06-09
> 状态: 已批准待实施

## 1. 问题陈述

当前后端日志存在三个核心问题:

1. **API Handler 无日志** — 所有 handler 在 `return c.JSON(err)` 之前不记录任何服务器端日志，错误信息仅随 HTTP response 返回，出问题无从查起
2. **HTTP 请求日志质量低** — 使用 Echo 内置 `middleware.RequestLogger()`，与 zap 无集成，字段结构单一（method/uri/status/latency），无 Trace ID
3. **业务层无事件日志** — diagnosis、activity、agent 路由层的关键事件（诊断开始/完成、意图分类、子 agent 派发）均未记录

## 2. 设计目标

1. **可追踪** — 每个请求有唯一 Trace ID，贯穿 middleware → handler → service → agent 全链路
2. **可诊断** — 错误日志带完整上下文（Trace ID、操作名、输入参数、错误详情），grep Trace ID 即可拉全链路
3. **可观测** — 关键业务事件（诊断开始/完成、Agent 路由、集群连接异常）均有结构化日志，用 `event` 字段标记
4. **渐进式** — 不破坏现有接口，不改外部 API，仅在现有 zap 基础设施上增强

## 3. 整体架构

```
Client Request
     │
     ▼
┌─────────────────────────────────────────┐
│  ZapLogger Middleware                    │
│  (替换 Echo RequestLogger)              │
│  • 生成/提取 TraceID → context          │
│  • request start: debug 级别            │
│  • request end:   info/warn/error       │
│  • 慢请求检测: >1s → warn 级别          │
└─────────────┬───────────────────────────┘
              │ context.WithValue(trace_id)
              ▼
┌─────────────────────────────────────────┐
│  API Handler                             │
│  • logger.Ctx(ctx).Error(msg, fields...) │
│  • logger.Ctx(ctx).Info(msg, fields...)  │
│  • 所有错误先记录再返回                    │
└─────────────┬───────────────────────────┘
              │ context 层层传递
              ▼
┌─────────────────────────────────────────┐
│  Router Agent → Sub-agent               │
│  Diagnosis Runner                        │
│  Activity Service                        │
│  Cluster Client Manager                  │
│  • 同样的 Ctx logger                     │
│  • 业务事件: event="diagnosis.started"   │
└─────────────────────────────────────────┘
```

## 4. 组件详细设计

### 4.1 Context Logger — `internal/utils/log/context.go`

核心辅助函数，从 `context.Context` 提取 Trace ID 并附加到 logger。全局 logger 通过 `config.L()` 获取，该函数在 `internal/config/config.go` 中定义。

```go
package log

import (
    "context"
    "go.uber.org/zap"
)

type ctxKey string

// TraceIDKey 是存储在 context 中的 Trace ID 的键名。
// 导出以便 middleware 和其他包用同一键写入/读取。
const TraceIDKey ctxKey = "trace_id"

// Ctx 返回一个带 context 中 Trace ID 的 logger。
// 如果 context 中没有 Trace ID，则返回全局 logger（不退化为无日志）。
// 内部调用 config.L() 获取全局 zap.Logger 实例。
func Ctx(ctx context.Context) *zap.Logger {
    if ctx == nil {
        return config.L()
    }
    if id, ok := ctx.Value(TraceIDKey).(string); ok && id != "" {
        return config.L().With(zap.String("trace_id", id))
    }
    return config.L()
}
```

设计决策:
- 函数名 `Ctx` 而非 `WithContext`，更简洁，高频调用不啰嗦
- context 中无 TraceID 时返回全局 logger，不退化为 `Nop`
- `TraceIDKey` 导出，middleware 和其他包用同一键写入/读取 context
- 内部通过 `config.L()` 获取全局 logger，不维护自己的 logger 状态

### 4.2 HTTP 中间件 — `internal/api/middlewares/logger.go`

替换 `router.go` 中的 `middleware.RequestLogger()`:

```go
package middlewares

import (
    "context"
    "time"
    "github.com/google/uuid"
    "go.uber.org/zap"
    "github.com/labstack/echo/v5"
    "github.com/kubewise/kubewise/internal/utils/log"
)

// ZapLogger 返回一个 Echo 中间件，用 zap 记录每个 HTTP 请求。
// 替换 Echo 内置的 middleware.RequestLogger()。
func ZapLogger() echo.MiddlewareFunc {
    return func(next echo.HandlerFunc) echo.HandlerFunc {
        return func(c echo.Context) error {
            start := time.Now()
            req := c.Request()

            // 1. Trace ID: 优先复用 X-Request-ID header，否则生成 UUID
            traceID := req.Header.Get("X-Request-ID")
            if traceID == "" {
                traceID = uuid.NewString()
            }

            // 2. Trace ID 存入 context（统一使用 log.TraceIDKey）
            ctx := context.WithValue(req.Context(), log.TraceIDKey, traceID)
            c.SetRequest(req.WithContext(ctx))
            c.Response().Header().Set("X-Request-ID", traceID)

            // 3. request start (debug 级别)
            log.Ctx(ctx).Debug("request started",
                zap.String("method", req.Method),
                zap.String("path", req.URL.Path),
                zap.String("query", req.URL.RawQuery),
            )

            // 4. 执行 handler chain
            err := next(c)

            // 5. request end — Echo v5 的 Response 已自动追踪 Status 和 Size
            status := c.Response().Status
            latency := time.Since(start)
            fields := []zap.Field{
                zap.String("method", req.Method),
                zap.String("path", req.URL.Path),
                zap.Int("status", status),
                zap.Duration("latency", latency),
                zap.Int64("bytes_out", c.Response().Size),
            }

            // 6. 级别判断: 2xx/3xx → info, 4xx → warn, 5xx → error
            //    慢请求 >1s 升级为 warn
            logFn := logFuncFor(status, latency)
            if err != nil {
                fields = append(fields, zap.Error(err))
            }
            logFn(ctx, "request completed", fields...)

            return err
        }
    }
}

func logFuncFor(status int, latency time.Duration) func(ctx context.Context, msg string, fields ...zap.Field) {
    if latency > 1*time.Second {
        return func(ctx context.Context, msg string, fields ...zap.Field) {
            log.Ctx(ctx).Warn(msg, append(fields, zap.Bool("slow", true))...)
        }
    }
    switch {
    case status >= 500:
        return func(ctx context.Context, msg string, fields ...zap.Field) {
            log.Ctx(ctx).Error(msg, fields...)
        }
    case status >= 400:
        return func(ctx context.Context, msg string, fields ...zap.Field) {
            log.Ctx(ctx).Warn(msg, fields...)
        }
    default:
        return func(ctx context.Context, msg string, fields ...zap.Field) {
            log.Ctx(ctx).Info(msg, fields...)
        }
    }
}
```

设计决策:
- 利用 Echo v5 `Response` 内置的 `Status`（int）和 `Size`（int64）字段，无需自写 ResponseRecorder
- `logFuncFor` 闭包按 status + latency 自动选择日志级别，业务意义清晰
- 慢请求阈值 1s（硬编码，后续可配）
- 不记录请求/响应 body，避免敏感信息泄漏

### 4.3 Router 注册变更 — `internal/api/router/router.go`

变更:
```diff
- e.Use(middleware.RequestLogger())
+ e.Use(middlewares.ZapLogger())
```

### 4.4 Handler 日志模式

所有 handler 遵循统一模式:

```go
func (h *Handler) ListIssues(c echo.Context) error {
    ctx := c.Request().Context()
    name := c.Param("name")

    // 日志：操作开始（debug）
    log.Ctx(ctx).Debug("list cluster issues",
        zap.String("cluster", name),
    )

    issues, err := h.clusterMgr.ListIssues(ctx, name)
    if err != nil {
        // 日志：先记录完整错误，再返回 HTTP response
        log.Ctx(ctx).Error("failed to list cluster issues",
            zap.String("cluster", name),
            zap.Error(err),
        )
        return c.JSON(http.StatusInternalServerError, ErrorResponse{
            Error: "query failed", Detail: err.Error(),
        })
    }

    return c.JSON(http.StatusOK, issues)
}
```

具体变更清单:

| 文件 | 新增日志点 | 额外字段 |
|------|-----------|---------|
| `chat.go` | `ChatSync` 调用后成功/失败 | `query_preview` (前80字符) |
| `stream.go` | SSE 连接建立、完成、异常断开 | `query_preview` |
| `interaction.go` | 交互请求收到 | `interaction_id` |
| `dashboard.go` | `ListClusters` / `ListIssues` / `ListClusterEvents` 成功/失败 | `cluster` |
| `diagnosis.go` | `StartDiagnose` / `StreamDiagnosisEvents` 调用 | `cluster`, `namespace`, `pod`, `diagnosis_id` |
| `activities.go` | `ListActivities` 调用 | `limit`, `offset` |
| `session.go` | CRUD 操作 | `session_id` |
| `cluster.go` | `ClusterStatus` 调用 | — |
| `health.go` | 健康检查 | — |

**不做日志的点**: 请求体 body 内容（除非 debug 级别且用户显式开启），避免敏感信息泄漏到日志。

### 4.5 业务事件日志

在业务层的关键节点记录结构化事件，使用 `event` 字段标记:

#### Diagnosis Runner — `internal/diagnosis/runner.go`

| 位置 | 日志语句 | 级别 |
|------|---------|------|
| `Start()` | `Ctx(ctx).Info("diagnosis started", zap.String("event", "diagnosis.started"), zap.String("diagnosis_id", id))` | info |
| `Finish()` | `Ctx(ctx).Info("diagnosis completed", zap.String("event", "diagnosis.completed"), zap.String("diagnosis_id", id), zap.Int("events", n))` | info |
| `PushEvent()` | `Ctx(ctx).Debug("diagnosis event", zap.String("diagnosis_id", id), zap.String("event_type", ev.Type))` | debug |
| buffer 满溢出 | `Ctx(ctx).Warn("ring buffer overflow, dropping event", zap.String("diagnosis_id", id))` | warn |

#### Router Agent — `internal/agent/router/agent.go`

| 位置 | 日志语句 | 级别 |
|------|---------|------|
| `HandleQuery` 中 classifyIntent 后 | `Ctx(ctx).Info("intent classified", zap.String("event", "agent.intent"), zap.String("task_type", intent.TaskType), zap.Float64("confidence", intent.Confidence))` | info |
| `HandleQueryStream` 中 switch 前 | `Ctx(ctx).Info("routing to sub-agent", zap.String("event", "agent.routed"), zap.String("task_type", intent.TaskType))` | info |
| `HandleQuery` 返回错误 | `Ctx(ctx).Error("agent query failed", zap.String("event", "agent.error"), zap.String("task_type", intent.TaskType), zap.Error(err))` | error |

#### Activity Service — `internal/activity/service.go`

| 位置 | 日志语句 | 级别 |
|------|---------|------|
| `Add()` | `Ctx(ctx).Info("activity recorded", zap.String("event", "activity.created"), zap.String("activity_type", string(typ)))` | info |
| `List()` | `Ctx(ctx).Debug("activity list queried")` | debug |

注意: activity 本身是持久化的审计记录，所以日志是对应的补充而非替代。

#### Cluster Client Manager — `internal/cluster/`

假设包路径为 `internal/cluster/`（通过代码探索确认），需要在连接/断开/错误时加日志:

| 位置 | 日志语句 | 级别 |
|------|---------|------|
| `GetClient()` 成功 | `Ctx(ctx).Info("cluster connected", zap.String("event", "cluster.connected"), zap.String("cluster", name))` | info |
| `GetClient()` 失败 | `Ctx(ctx).Error("cluster connection failed", zap.String("event", "cluster.error"), zap.String("cluster", name), zap.Error(err))` | error |
| 连接被主动关闭 | `Ctx(ctx).Warn("cluster disconnected", zap.String("event", "cluster.disconnected"), zap.String("cluster", name))` | warn |

### 4.6 配置增强

现有日志配置基础上无需新增配置项。`event` 字段配合 zap 的 JSON encoder 天然支持结构化过滤。

日志级别使用建议:
- **error**: 影响用户请求完成的事件（诊断失败、集群连接失败、查询 panic）
- **warn**: 不影响当前请求但值得关注的事件（慢请求 >1s、buffer 溢出、4xx 错误）
- **info**: 正常业务事件（诊断开始/完成、Agent 路由、活动记录）
- **debug**: 细节跟踪（request start、诊断中间事件）

## 5. 日志字段规范

所有日志行统一使用以下字段:

| 字段 | 类型 | 适用级别 | 必填 | 说明 |
|------|------|---------|------|------|
| `level` | string | 全部 | 是 | debug/info/warn/error (zap 原生) |
| `ts` | float | 全部 | 是 | Unix timestamp (zap 原生) |
| `caller` | string | 全部 | 是 | file:line (zap 原生) |
| `trace_id` | string | handler层 | handler层必填 | 请求追踪 ID |
| `event` | string | 业务日志 | 业务日志必填 | 事件名，如 `diagnosis.started` |
| `msg` | string | 全部 | 是 | 人类可读的描述 |
| `error` | string | error | error 必填 | 错误详情 |
| `status` | int | 中间件 | 中间件 | HTTP status code |
| `latency` | duration | 中间件 | 中间件 | 请求延迟 |

## 6. 影响文件清单

### 新增

| 文件 | 内容 |
|------|------|
| `internal/utils/log/context.go` | `Ctx(ctx)` 辅助函数 |

### 修改

| 文件 | 变更 |
|------|------|
| `internal/api/middlewares/middleware.go` | 新增 `ZapLogger()` 和 `ResponseRecorder` |
| `internal/api/router/router.go` | `RequestLogger()` → `ZapLogger()` |
| `internal/api/handler/chat.go` | +结构化日志 |
| `internal/api/handler/stream.go` | +结构化日志 |
| `internal/api/handler/interaction.go` | +结构化日志 |
| `internal/api/handler/dashboard.go` | +结构化日志 |
| `internal/api/handler/diagnosis.go` | +结构化日志 |
| `internal/api/handler/activities.go` | +结构化日志 |
| `internal/api/handler/session.go` | +结构化日志 |
| `internal/api/handler/cluster.go` | +结构化日志 |
| `internal/api/handler/health.go` | +结构化日志 |
| `internal/diagnosis/runner.go` | +业务事件日志 |
| `internal/agent/router/agent.go` | +业务事件日志 |
| `internal/activity/service.go` | +业务事件日志 |

## 7. 不在此次范围内

- **日志告警/监控集成**: 留待后续对接 Prometheus/Grafana/EKL
- **日志轮转策略**: 由 `--log-file` 配置 + 外部 logrotate 管理，不在本次改动
- **前端请求 ID 生成**: 前端可在 fetch header 中加入 `X-Request-ID`，本次不做强制
- **请求/响应 body 日志**: 敏感信息风险高，除非 debug 级别手动开启，不做默认记录
- **性能采样**: 不做基于概率的采样，全量日志（zap 性能足够高）

## 8. 验收标准

1. 启动后端，发出任意 API 请求，日志中出现 `trace_id` 字段
2. 每条日志按 status 自动选择 info/warn/error 级别
3. 慢请求 >1s 自动升级为 warn 并标记 `slow: true`
4. 调用诊断接口后日志中出现 `event="diagnosis.started"` 和 `event="diagnosis.completed"`
5. 向 agent 发送查询后日志中出现 `event="agent.intent"` 和 `event="agent.routed"`
6. 集群连接失败时日志中出现 `event="cluster.error"` 带 error 详情
7. 通过 `grep <trace_id> kubewise.log` 可串联一次请求的全链路日志