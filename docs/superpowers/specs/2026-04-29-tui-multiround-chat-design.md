# TUI 多轮对话模式设计

**日期**: 2026-04-29  
**状态**: 已批准

## Context

KubeWise 当前只支持单次命令行查询（`kubewise chat "..."`），每次调用都是无状态的一次性请求。这个设计添加一个交互式 TUI 模式（`kubewise tui`），支持多轮对话、会话管理、Agent 工作可视化和操作确认。

目标：让用户在终端里像使用 ChatGPT 桌面端一样与 KubeWise 交互，同时保留 CLI 模式不受影响。

---

## 架构概览

```
kubewise tui
    └── pkg/tui/app.go (bubbletea v2 App Model)
            ├── SidebarModel        — 左侧会话列表（��定 24 列）
            ├── ChatModel           — 右侧对话区 + 进度卡片 + 渲染块
            ├── InputModel          — 底部输入框
            └── ConfirmModel        — 模态确认覆盖层（Operation 专用）

用户提交消息
    └── goroutine: router.HandleQueryStream(ctx, query, eventCh)
            ├── 向 eventCh 推送 TUIEvent
            └── ConfirmRequestEvent 通过 ChannelConfirmationHandler 桥接

bubbletea Update loop
    └── tea.WaitForActivity(eventCh) → 接收事件 → 更新 Model ��� 触发 View 重绘
```

现有 `chat` 子命令**不受影响**。Agent 层通过 `WithEventCh(nil)` 保持原有行为。

---

## 目录结构

```
pkg/tui/
  app.go               # 根 Model，组合所有子组件
  events/
    events.go          # TUIEvent 接口 + 所有事件类型
  model/
    sidebar.go         # 会话列表组件
    chat.go            # 对话区（消息历史 + 进度卡片）
    input.go           # 输入框组件（bubbles/textarea）
    confirm.go         # 模态确认覆盖层
    renderer.go        # 格式化渲染（表格/代码块/KV/列表）
  session/
    session.go         # Session / Message / Block struct
    store.go           # JSON 文件读写
  styles/
    styles.go          # lipgloss 样式常量
cmd/main.go            # 新增 tui 子命令
```

---

## 事件系统

文件：`pkg/tui/events/events.go`

```go
type TUIEvent interface{ isTUIEvent() }

// Agent 生命周期
type AgentStartEvent struct { AgentName string; QueryID string }
type AgentDoneEvent  struct { QueryID string; Duration time.Duration; InTokens, OutTokens int }
type ToolCallEvent   struct { QueryID string; ToolName string }
type ToolDoneEvent   struct { QueryID string; ToolName string; Elapsed time.Duration }

// 格式化渲染
type RenderTextEvent  struct { QueryID string; Text string }
type RenderTableEvent struct { QueryID string; Headers []string; Rows [][]string }
type RenderCodeEvent  struct { QueryID string; Language, Content string }
type RenderKVEvent    struct { QueryID string; Pairs []KVPair }
type RenderListEvent  struct { QueryID string; Items []ListItem }

// Operation 确认
type ConfirmRequestEvent struct { QueryID string; Req operation.ConfirmRequest }

// 结束 / 错误
type StreamDoneEvent struct { QueryID string }
type StreamErrEvent  struct { QueryID string; Err error }
```

### 渲染类型选择规则

Router Agent 解析子 Agent 返回的字符串结果后，按以下规则决定发哪种 `RenderXxxEvent`（不依赖 LLM 选择）：

- 含 `|` 分隔的表格行 → `RenderTableEvent`
- 含 `apiVersion:` / `kind:` / YAML 缩进结构 → `RenderCodeEvent{Language: "yaml"}`
- 含 ` ` 缩进的 JSON → `RenderCodeEvent{Language: "json"}`
- 含 `key: value` 对为主 → `RenderKVEvent`
- 多行列表（含状态词 Running/Pending/Error 等）→ `RenderListEvent`
- 其他 → `RenderTextEvent`

---

## Agent 层修改

每个 Agent 加 `WithEventCh(ch chan<- TUIEvent)` 函数选项：

```go
// 示例：query.Agent
type Agent struct {
    // ... 现有字段
    eventCh chan<- events.TUIEvent  // nil = CLI 模式，不发事件
}

func WithEventCh(ch chan<- events.TUIEvent) Option {
    return func(a *Agent) { a.eventCh = ch }
}

func (a *Agent) emit(e events.TUIEvent) {
    if a.eventCh != nil {
        a.eventCh <- e
    }
}
```

Router Agent 创建子 Agent 时透传同一个 `eventCh`。

涉及文件：
- `pkg/agent/router/agent.go` — 新增 `HandleQueryStream(ctx, query string, eventCh chan<- TUIEvent)`
- `pkg/agent/query/agent.go` — 加 `WithEventCh` 选项，在工具调用前后 emit 事件
- `pkg/agent/operation/agent.go` — 加 `WithEventCh` 选项，在每个步骤前后 emit 事件
- `pkg/agent/troubleshooting/agent.go` — 同上
- `pkg/agent/security/agent.go` — 同上

---

## bubbletea v2 App Model

```go
type App struct {
    sidebar   SidebarModel
    chat      ChatModel
    input     InputModel
    confirm   *ConfirmModel      // 非 nil 时渲染模态层
    sessions  []session.Session
    active    *session.Session
    eventCh   chan events.TUIEvent
    confirmCh chan operation.ConfirmResponse
    cancelFn  context.CancelFunc
    running   bool               // true 时禁用输入
    width     int
    height    int
}
```

### 进度卡片（ProgressCard）

`ChatModel` 维护 `map[string]*ProgressCard`，以 `QueryID` 为键：

```
运行中:
┌─ ⚙ Query Agent ───────────────────────────────┐
│ ⟳ Router: query → Query Agent                  │
│ ✓ list_namespaces                    0.5s      │
│ ⟳ get_pod_resource_usage (3/5)       2.1s…    │
└─────────────────────────────────────────────────┘

完成后折叠为:
  ✓ Query Agent  4.1s | ↑ 429 ↓ 862 tok
```

### 侧边栏

- 固定宽度 24 列
- 终端宽度 < 100 时自动隐藏，`Tab` 可手动切换显示
- 显示会话标题（首条消息截断 20 字符）+ 相对时间

---

## 会话管理

### 存储

路径：`~/.kubewise/sessions/<YYYY-MM-DD-HHMMSS>-<shortid>.json`

```go
type Session struct {
    ID        string    `json:"id"`
    Title     string    `json:"title"`
    CreatedAt time.Time `json:"created_at"`
    UpdatedAt time.Time `json:"updated_at"`
    Messages  []Message `json:"messages"`
}

type Message struct {
    Role      string    `json:"role"`        // "user" | "assistant"
    Content   string    `json:"content"`
    Blocks    []Block   `json:"blocks,omitempty"`
    Timestamp time.Time `json:"timestamp"`
    InTokens  int       `json:"in_tokens,omitempty"`
    OutTokens int       `json:"out_tokens,omitempty"`
    DurationS float64   `json:"duration_s,omitempty"`
}

type Block struct {
    Type    string          `json:"type"`    // "table"|"code"|"kv"|"list"|"text"
    Payload json.RawMessage `json:"payload"`
}
```

- 每次 `StreamDoneEvent` 后自动保存
- 启动时加载最近 20 条会话显示在侧边栏

### 操作

| 操作 | 触发 |
|------|------|
| 新建会话 | `Ctrl+N` |
| 清空当前会话消息 | `Ctrl+L` |
| 自动保存 | 每次回复完成后 |
| 切换历史会话 | 侧边栏 `↑/↓` + `Enter` |

---

## 键盘快捷键

| 按键 | 动作 |
|------|------|
| `Enter` | 发送消息 |
| `Ctrl+N` | 新建会话 |
| `Ctrl+C` | Agent 运行中：中断；空闲：退出确认 |
| `Ctrl+L` | 清空当前会话消息 |
| `Tab` | 焦点侧边栏 ↔ 对话区 |
| `↑/↓` | 侧边栏中切换会话 |
| `/resume` | 重发上次被中断的消息 |
| `Y` / `N` / `E` | 确认模态：确认 / 拒绝 / 修改参数 |
| `Esc` | 关闭确认模态（等同拒绝） |

---

## 中断与恢复

1. `Ctrl+C` 调用 `cancelFn()`，agent goroutine 退出（context cancelled）
2. 进度卡片标记"已中断"，`running = false`，输入框重新激活
3. `active.InterruptedQuery` 记录被中断的原始消息
4. 用户输入 `/resume` → 用 `InterruptedQuery` 重新发起请求

---

## 格式化渲染

文件：`pkg/tui/model/renderer.go`，使用 lipgloss：

- **表格**：`lipgloss` border 样式，列宽按内容自适应，超出终端宽度时截断
- **代码块**：`alecthomas/chroma/v2` 语法高亮，带语言标题行，`lipgloss` 边框
- **键值对**：key 右对齐 + 固定间距 + value 左对齐
- **列表**：状态图标 `✓`(绿) `⚠`(黄) `✗`(红) `•`(灰) + 内容

---

## 新增依赖

```
charm.land/bubbletea/v2
github.com/charmbracelet/lipgloss
github.com/charmbracelet/bubbles
github.com/alecthomas/chroma/v2
```

---

## 验证方式

1. `kubewise tui` 启动 TUI，侧边栏显示历史会话（如有）
2. 输入查询 → 进度卡片显示 Router + 子 Agent 步骤，完成后折叠为 token 统计摘要行
3. 输入 Operation 类请求 → 确认模态弹出，按 Y 执行，N 取消，E 输入修改意见
4. 运行中按 `Ctrl+C` → 卡片标记已中断，输入框恢复；`/resume` 重新执行
5. `Ctrl+N` 新建会话，`Tab` 切换到侧边栏，`↑/↓` 选历史会话
6. `Ctrl+L` 清空消息后会话仍在侧边栏
7. `~/.kubewise/sessions/` 下生成对应 JSON 文件，重启后会话仍可加载
8. 终端宽度缩小到 < 100 列时侧边栏自动隐藏
