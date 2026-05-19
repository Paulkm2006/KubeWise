# Deploy Agent TUI 交互增强设计

**日期:** 2026-05-14
**状态:** Draft
**作者:** Brainstorming Session

---

## 问题概述

Deploy Agent 的 TUI 交互存在 4 个问题：

| # | 问题 | 严重度 | 影响 |
|---|------|--------|------|
| 1 | 执行进度无实时可视化展示 | 中 | 用户不知道当前进行到哪个阶段，体验不佳 |
| 2 | Confirm 界面面板尺寸未自适应终端窗口 | 低 | 首次渲染闪烁，但 1 帧后恢复正常 |
| 3 | Value 查看不支持滚动浏览（右面板） | 中 | 大 values.yaml 只能看到前 20 行 |
| 4 | Chart 选择列表方向键被 App 层拦截 | 高 | ↑↓ 完全无法移动光标选择 |

---

## 设计目标

1. **进度可视化** — Deploy Agent 6 个阶段实时显示完成状态
2. **真·窗口自适应** — 创建时即使用正确尺寸，消除硬编码闪烁
3. **双面板可滚动** — Tab 切换左右面板焦点，↑↓ 滚动，高亮指示
4. **方向键选择** — 修复事件拦截，光标行视觉增强

---

## 方案详述

### 1. 实时进度显示

**核心思路**: 复用现有的 [`progressCard`](pkg/tui/model/chat.go:285) 和 `PhaseEvent`/`ToolCallEvent`/`ToolDoneEvent` 事件机制，Deploy Agent 在 6 个阶段各发送一次事件即可。

**Deploy Agent 6 阶段事件流**:

```
[Phase 1: 解析 Chart]        → PhaseEvent{Phase: "解析 Chart: argocd"}
[Phase 2: 获取默认 values]    → PhaseEvent{Phase: "获取 Chart 默认配置"}
  ├─ helm repo add            → ToolCallEvent{ToolName: "helm repo add"}
  │                           → ToolDoneEvent{ToolName: "helm repo add", Elapsed: 0.5s}
  └─ helm show values         → ToolCallEvent{ToolName: "helm show values"}
                              → ToolDoneEvent{ToolName: "helm show values", Elapsed: 0.8s}
[Phase 3: LLM 生成 values]    → PhaseEvent{Phase: "生成配置建议"}
                              → ToolCallEvent{ToolName: "LLM values generation"}
                              → ToolDoneEvent{ToolName: "LLM values generation", Elapsed: 2.1s}
[Phase 4: 人工审查]            → PhaseEvent{Phase: "等待用户确认"}
  ├─ 用户确认 (Y)              → ToolCallEvent{ToolName: "user confirm"}
  │                           → ToolDoneEvent{ToolName: "user confirm", Elapsed: 5.0s}
  └─ (如 NL 修正)              → ToolCallEvent{ToolName: "LLM values regeneration"}
                              → ToolDoneEvent{ToolName: "LLM values regeneration", Elapsed: 1.5s}
[Phase 5: 执行部署]            → PhaseEvent{Phase: "执行 helm install/upgrade"}
                              → ToolCallEvent{ToolName: "helm install/upgrade"}
                              → ToolDoneEvent{ToolName: "helm install/upgrade", Elapsed: 30s}
[Phase 6: 验证]                → PhaseEvent{Phase: "验证部署状态"}
                              → ToolCallEvent{ToolName: "verify deployment"}
                              → ToolDoneEvent{ToolName: "verify deployment", Elapsed: 2s}
```

**渲染效果**（聊天面板中的 progressCard）：

```
┌─ Deploy Agent 执行中 ─────────────────────┐
│ ✓ 解析 Chart: argocd                      │
│ ✓ 获取 Chart 默认配置                      │
│ ⟳ 生成配置建议                            │
│ ○ 等待用户确认                            │
├──────────────────────────────────────────┤
│ ✓ helm repo add        (0.5s)            │
│ ✓ helm show values     (0.8s)            │
│ ⟳ LLM values generation (2.1s...)         │
└──────────────────────────────────────────┘
```

**改动点**:
- [`pkg/agent/deploy/agent.go`](pkg/agent/deploy/agent.go): 在 `HandleQuery()` 的 6 个阶段各发送 `PhaseEvent`，子操作发送 `ToolCallEvent`/`ToolDoneEvent`
- [`pkg/tui/events/events.go`](pkg/tui/events/events.go): 无需新增事件类型，复用已有事件
- [`pkg/tui/model/chat.go`](pkg/tui/model/chat.go): 无需改动，现有 progressCard 渲染逻辑自动处理

---

### 2. 窗口自适应（消除硬编码）

**问题根因**: [`NewDeployConfirmModel()`](pkg/tui/model/deploy_confirm.go:56-74) 中全部是硬编码尺寸：
```go
defaultVP := viewport.New(40, 20)     // 40×20
overrideVP := viewport.New(40, 20)    // 40×20
yamlEditor.SetWidth(40)               // 40
fullPreviewVP := viewport.New(80, 30) // 80×30
```

虽然 `WindowSizeMsg` 会修正，但第一个帧有闪烁。

**方案**: 构造函数接收窗口尺寸参数，从外部传入。

**签名变更**:
```go
// 旧
func NewDeployConfirmModel(queryID string, plan events.DeployPlan) DeployConfirmModel

// 新
func NewDeployConfirmModel(queryID string, plan events.DeployPlan, width, height int) DeployConfirmModel
```

**内部计算**:
```go
panelW := (width - 4) / 2   // 左右面板各占一半
panelH := height - 8        // 减去标题、操作栏高度
defaultVP := viewport.New(panelW, panelH)
overrideVP := viewport.New(panelW, panelH)
yamlEditor.SetWidth(panelW)
yamlEditor.SetHeight(panelH)
fullPreviewVP := viewport.New(width-4, height-6)
```

**调用方变更** ([`pkg/tui/app.go`](pkg/tui/app.go:157)):
```go
// 旧
m := model.NewDeployConfirmModel(msg.QueryID, msg.Plan)

// 新
m := model.NewDeployConfirmModel(msg.QueryID, msg.Plan, a.width, a.height)
```

**改动点**:
- [`pkg/tui/model/deploy_confirm.go`](pkg/tui/model/deploy_confirm.go): `NewDeployConfirmModel` 签名 + 内部计算
- [`pkg/tui/app.go`](pkg/tui/app.go): 调用处传参

---

### 3. 双面板滚动浏览

**问题根因**: [`deploy_confirm.go:Update()`](pkg/tui/model/deploy_confirm.go:132-137) 只把消息转发给 `defaultVP`，`overrideVP` 完全收不到键盘事件。

**方案**: 引入 `focusPanel` 状态字段，Tab 切换当前焦点面板。

**新增字段**:
```go
type DeployConfirmModel struct {
    // ... 现有字段
    focusPanel int  // 0 = 左面板（default），1 = 右面板（override）
}
```

**交互逻辑**:

| 按键 | 效果 |
|------|------|
| `Tab` | 切换焦点面板（左 ↔ 右） |
| `↑`/`↓` | 滚动当前焦点面板 |
| `PgUp`/`PgDn` | 大幅度滚动当前焦点面板 |
| 鼠标滚轮 | 滚动当前焦点面板 |

**边框高亮表示焦点**:

```
┌──────────────────────┬──────────────────────┐
│ 默认 Values (参考)    │ Override Values (焦点) │ ← 蓝色高亮边框
│                      │                      │
│ # Server config      │ # 用户要求暴露 NodePort│
│ server:              │ server:              │
│   service:           │   service:           │
│     type: ClusterIP  │     type: NodePort   │
│     port: 80         │     nodePort: 30080  │
│   replicas: 1        │                      │
│   ...                │                      │
│                      │                      │
│ ↑↓  Tab 切换         │                      │
└──────────────────────┴──────────────────────┘
```

**lipgloss 样式定义**:
```go
var (
    panelFocusBorder   = lipgloss.Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("12"))  // 蓝色
    panelNormalBorder  = lipgloss.Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("8"))   // 灰色
)
```

**Update 逻辑变更**:
```go
case deployConfirmModeView:
    switch msg.String() {
    case "tab":
        m.focusPanel = 1 - m.focusPanel  // 切换 0↔1
    default:
        // 将消息转发给焦点面板
        if m.focusPanel == 0 {
            m.defaultVP, cmd = m.defaultVP.Update(msg)
        } else {
            m.overrideVP, cmd = m.overrideVP.Update(msg)
        }
    }
```

**View 逻辑变更**:
```go
leftStyle := panelNormalBorder
rightStyle := panelNormalBorder
if m.focusPanel == 0 {
    leftStyle = panelFocusBorder
} else {
    rightStyle = panelFocusBorder
}
leftPanel := leftStyle.Render("默认 Values (参考)\n" + m.defaultVP.View())
rightPanel := rightStyle.Render("Override Values (Agent 生成)\n" + m.overrideVP.View())
```

**改动点**:
- [`pkg/tui/model/deploy_confirm.go`](pkg/tui/model/deploy_confirm.go): 新增 `focusPanel` 字段、Tab 切换、边框样式、Update 路由、View 高亮渲染

---

### 4. 方向键选择 + 光标视觉增强

**问题根因**: [`app.go:handleShortcut()`](pkg/tui/app.go:285-303) 在 `focus == focusInput` 时吃掉了 `KeyUp`/`KeyDown`，[`chartSelectModel`](pkg/tui/model/chart_select.go:97-104) 永远收不到方向键事件。

**修复方案**:

在 `handleShortcut()` 中增加前置判断：当 `chartSelectModel` 或 `deployConfirmModel` 或 `manualInputModel` active 时，不拦截方向键：

```go
func (a App) handleShortcut(msg tea.KeyMsg) (tea.Cmd, bool) {
    // 如果 deploy TUI 组件 active，不拦截方向键
    if a.chartSelectModel != nil || a.deployConfirmModel != nil || a.manualInputModel != nil {
        return nil, false
    }
    // ... 原有逻辑
}
```

**视觉增强**（在 [`chartSelectModel.View()`](pkg/tui/model/chart_select.go:118) 中）：

```
当前样式（普通行）:
  [2] bitnami/argo-cd  ⭐ 300
      description...

新样式（光标行 = 蓝字 + 浅蓝底色）:
> [1] argo/argo-cd  ⭐ 1,200         ← 蓝色字 + #E0E8FF 底色
      description...

新样式（非光标行）:
  [2] bitnami/argo-cd  ⭐ 300         ← 白色字，无底色
      description...
```

**样式定义**:
```go
var (
    cursorStyle = lipgloss.NewStyle().
        Foreground(lipgloss.Color("12")).        // 蓝色字
        Background(lipgloss.Color("153")).        // 浅蓝底色 (#E0E8FF)
        Bold(true)
)
```

**改动点**:
- [`pkg/tui/app.go`](pkg/tui/app.go): `handleShortcut()` 增加前置判断
- [`pkg/tui/model/chart_select.go`](pkg/tui/model/chart_select.go): 光标行用 `cursorStyle` 渲染

---

## 改动文件清单

| 文件 | 改动类型 | 内容 |
|------|----------|------|
| `pkg/agent/deploy/agent.go` | 修改 | 6 个阶段增加 PhaseEvent + ToolCallEvent/ToolDoneEvent |
| `pkg/tui/model/deploy_confirm.go` | 修改 | 窗口尺寸参数化、Tab 切换焦点面板、边框高亮 |
| `pkg/tui/app.go` | 修改 | 创建 ConfirmModel 传入尺寸、handleShortcut 方向键不拦截 |
| `pkg/tui/model/chart_select.go` | 修改 | 光标行浅蓝底色 + 蓝色字 |

---

## 不涉及改动的文件

- `pkg/tui/events/events.go` — 复用现有事件类型，无需新增
- `pkg/tui/model/chat.go` — progressCard 渲染逻辑已支持，无需改动
- `pkg/tui/model/manual_chart_input.go` — 不涉及
- `pkg/tui/styles/` — 如果必要可新增样式常量

---

## 设计决策总结

| 决策 | 选择 | 理由 |
|------|------|------|
| 进度显示 | 复用已有 `progressCard` 和事件 | 零新增复杂度，Agent 仅需多发事件 |
| 窗口自适应 | 构造函数传参 | 消除首次渲染闪烁，简单可靠 |
| 面板切换 | Tab + 边框高亮 | 不增加键位冲突，视觉清晰 |
| focusPanel | 2 态（左/右） | 当前仅有两个面板，无需更多状态 |
| 方向键拦截 | handleShortcut 提前返回 | 最小侵入改动，不改变其他功能 |
| 光标效果 | 蓝字 + 浅蓝底色 | 符合终端 TUI 常用交互模式 |
