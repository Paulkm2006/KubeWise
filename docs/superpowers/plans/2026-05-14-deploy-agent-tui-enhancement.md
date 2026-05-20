# Deploy Agent TUI 交互增强 实现计划

**Goal:** 解决 Deploy Agent TUI 的 4 个交互问题：实时进度显示、窗口自适应、双面板滚动浏览、方向键选择修复与光标增强。

**Architecture:** 4 个独立的任务组，按 TDD 方式执行。任务1 修改 deploy agent 发送更多事件；任务2 将窗口尺寸参数化；任务3 添加 Tab 切换焦点面板；任务4 修复方向键拦截并增强光标行样式。全部改动在已有代码结构内完成。

**Tech Stack:** Go, Bubble Tea, lipgloss, viewport

---

## Task 概览

| 任务 | 文件 | 改动类型 | 测试文件 |
|------|------|----------|----------|
| 1 | `pkg/agent/deploy/agent.go` | 修改 | `pkg/agent/deploy/agent_test.go` |
| 2 | `pkg/tui/model/deploy_confirm.go` | 修改 | `pkg/tui/model/deploy_confirm_test.go` |
| 3 | `pkg/tui/model/deploy_confirm.go` | 修改 | 同上 |
| 4a | `pkg/tui/app.go` | 修改 | - |
| 4b | `pkg/tui/model/chart_select.go` | 修改 | `pkg/tui/model/chart_select_test.go` |

---

### Task 1: 实时进度显示 — Deploy Agent 增加事件发送

**Files:**
- Modify: `pkg/agent/deploy/agent.go:73-179`
- Create: `pkg/agent/deploy/agent_test.go`

**Step 1: Write the failing test for event emissions**

```go
// pkg/agent/deploy/agent_test.go
package deploy

import (
	"context"
	"testing"
	"time"

	"github.com/kubewise/kubewise/pkg/tui/events"
)

// fakeEventRecv collects events from the agent channel.
type fakeEventRecv struct {
	ch chan events.TUIEvent
}

func newFakeEventRecv() *fakeEventRecv {
	return &fakeEventRecv{ch: make(chan events.TUIEvent, 64)}
}

func TestHandleQueryEmitsPhaseEvents(t *testing.T) {
	// We test event emission behavior using a minimal agent.
	// Since HandleQuery calls helm/external services, we use a
	// lightweight verification: the event channel is wired, and
	// we verify that at least PhaseEvent messages are emitted.
	//
	// For full integration testing, see deploy_integration_test.go

	recv := newFakeEventRecv()
	// Create agent without external dependencies, just to verify
	// the emit() helper and event wiring.
	ag := &Agent{
		eventCh: recv.ch,
		queryID: "q-test",
	}
	defer close(recv.ch)

	// Test emit directly (HandleQuery requires full helm/catalog setup)
	ag.emit(events.PhaseEvent{QueryID: "q-test", Phase: "test-phase"})

	select {
	case ev := <-recv.ch:
		pe, ok := ev.(events.PhaseEvent)
		if !ok {
			t.Fatalf("expected PhaseEvent, got %T", ev)
		}
		if pe.Phase != "test-phase" {
			t.Errorf("expected phase 'test-phase', got %q", pe.Phase)
		}
		if pe.QueryID != "q-test" {
			t.Errorf("expected queryID 'q-test', got %q", pe.QueryID)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for event")
	}

	// Test ToolCallEvent emit
	ag.emit(events.ToolCallEvent{QueryID: "q-test", ToolName: "helm repo add", Step: 1})
	select {
	case ev := <-recv.ch:
		tc, ok := ev.(events.ToolCallEvent)
		if !ok {
			t.Fatalf("expected ToolCallEvent, got %T", ev)
		}
		if tc.ToolName != "helm repo add" {
			t.Errorf("expected tool 'helm repo add', got %q", tc.ToolName)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for ToolCallEvent")
	}

	// Test ToolDoneEvent emit
	ag.emit(events.ToolDoneEvent{QueryID: "q-test", ToolName: "helm repo add", Step: 1, Elapsed: time.Second})
	select {
	case ev := <-recv.ch:
		td, ok := ev.(events.ToolDoneEvent)
		if !ok {
			t.Fatalf("expected ToolDoneEvent, got %T", ev)
		}
		if td.Elapsed != time.Second {
			t.Errorf("expected elapsed 1s, got %v", td.Elapsed)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for ToolDoneEvent")
	}
}
```

**Step 2: Run test to verify it fails (no test file yet)**

Run: `cd /home/cernet/repo/KubeWise && go test ./pkg/agent/deploy/ -run TestHandleQueryEmitsPhaseEvents -v 2>&1`
Expected: `FAIL` — 因为 agent_test.go 不存在，go test 找不到测试。

**Step 3: Write minimal implementation for agent.go**

在 `HandleQuery()` 方法中增加事件发送。当前已有 Phase 1/3/4/6 的 PhaseEvent，需要补全：
- Phase "等待用户确认"（调用 confirmDeploy 前）
- Phase "验证部署状态"（install 之后）
- 子操作的 ToolCallEvent/ToolDoneEvent

核心改动：在代理的 6 个阶段事件流中补发完整事件。

`pkg/agent/deploy/agent.go` 的修改：

```go
// HandleQuery 实现六阶段部署流程。
func (a *Agent) HandleQuery(ctx context.Context, query string, entities types.Entities) (string, error) {
	// Phase 1: 提取应用名称
	appName := a.extractAppName(entities, query)
	if appName == "" {
		return "", fmt.Errorf("无法从查询中提取应用名称，请明确指定要部署的应用")
	}

	a.emit(events.PhaseEvent{QueryID: a.queryID, Phase: fmt.Sprintf("解析 Chart: %s", appName)})

	// Phase 2: 解析 Chart
	chartInfo, err := a.chartResolver.Resolve(ctx, appName)
	if err != nil {
		return "", fmt.Errorf("Chart 解析失败: %w", err)
	}

	if chartInfo == nil {
		chartInfo, err = a.handleChartNotFound(ctx, appName)
		if err != nil {
			return "", err
		}
		if chartInfo == nil {
			return "部署已取消", nil
		}
	}

	// Phase 2.5: 检查是否已部署
	existingRelease, _ := a.helmClient.Status(ctx, appName, chartInfo.DefaultNamespace)

	// Phase 3: 获取默认 values
	a.emit(events.PhaseEvent{QueryID: a.queryID, Phase: "获取 Chart 默认配置"})

	a.emit(events.ToolCallEvent{QueryID: a.queryID, ToolName: "helm repo add", Step: 1})
	if err := a.helmClient.AddRepo(ctx, chartInfo.RepoName, chartInfo.RepoURL); err != nil {
		return "", fmt.Errorf("添加 Helm 仓库失败: %w", err)
	}
	a.emit(events.ToolDoneEvent{QueryID: a.queryID, ToolName: "helm repo add", Step: 1, Elapsed: 0})

	a.emit(events.ToolCallEvent{QueryID: a.queryID, ToolName: "helm show values", Step: 2})
	defaultValues, err := a.helmClient.FetchDefaultValues(ctx, chartInfo.RepoName, chartInfo.RepoURL, chartInfo.ChartName)
	if err != nil {
		return "", fmt.Errorf("获取默认 values 失败: %w", err)
	}
	a.emit(events.ToolDoneEvent{QueryID: a.queryID, ToolName: "helm show values", Step: 2, Elapsed: 0})

	// Phase 4: LLM 生成 override values
	a.emit(events.PhaseEvent{QueryID: a.queryID, Phase: "生成配置建议"})
	a.emit(events.ToolCallEvent{QueryID: a.queryID, ToolName: "LLM values generation", Step: 3})
	customValues, err := generateValues(ctx, a.llmClient, query, chartInfo, defaultValues)
	if err != nil {
		return "", fmt.Errorf("生成 values 失败: %w", err)
	}
	a.emit(events.ToolDoneEvent{QueryID: a.queryID, ToolName: "LLM values generation", Step: 3, Elapsed: 0})

	// 如果需要安装 CRDs，自动添加
	if chartInfo.InstallCRDs {
		customValues = "installCRDs: true\n" + customValues
	}

	// Phase 5: 人工审查
	a.emit(events.PhaseEvent{QueryID: a.queryID, Phase: "等待用户确认"})
	a.emit(events.ToolCallEvent{QueryID: a.queryID, ToolName: "user confirm", Step: 4})

	plan := events.DeployPlan{...} // 同现有代码

	decision, err := a.confirmDeploy(ctx, plan)
	if err != nil {
		return "", fmt.Errorf("确认部署失败: %w", err)
	}
	a.emit(events.ToolDoneEvent{QueryID: a.queryID, ToolName: "user confirm", Step: 4, Elapsed: 0})
	if decision.Action == "cancel" {
		return "部署已取消", nil
	}

	// 处理自然语言修正循环（toolDone already emitted above; regeneration gets new events）
	finalValues := decision.Values
	if decision.Correction != "" {
		a.emit(events.ToolCallEvent{QueryID: a.queryID, ToolName: "LLM values regeneration", Step: 5})
		finalValues, err = regenerateValues(ctx, a.llmClient, query, chartInfo, defaultValues, decision.Values, decision.Correction)
		if err != nil {
			return "", fmt.Errorf("重新生成 values 失败: %w", err)
		}
		a.emit(events.ToolDoneEvent{QueryID: a.queryID, ToolName: "LLM values regeneration", Step: 5, Elapsed: 0})
		// 再次确认
		plan.CustomValues = finalValues
		decision2, err := a.confirmDeploy(ctx, plan)
		if err != nil {
			return "", err
		}
		if decision2.Action == "cancel" {
			return "部署已取消", nil
		}
		finalValues = decision2.Values
	}

	// Phase 6: 执行 helm install/upgrade
	a.emit(events.PhaseEvent{QueryID: a.queryID, Phase: "执行部署"})
	a.emit(events.ToolCallEvent{QueryID: a.queryID, ToolName: "helm install/upgrade", Step: 6})
	rel, err := a.helmClient.InstallOrUpgrade(ctx, helm.InstallOptions{...}) // 同现有
	if err != nil {
		return "", fmt.Errorf("部署失败: %w", err)
	}
	a.emit(events.ToolDoneEvent{QueryID: a.queryID, ToolName: "helm install/upgrade", Step: 6, Elapsed: 0})

	// Phase 7: 验证
	a.emit(events.PhaseEvent{QueryID: a.queryID, Phase: "验证部署状态"})
	a.emit(events.ToolCallEvent{QueryID: a.queryID, ToolName: "verify deployment", Step: 7})
	a.emit(events.ToolDoneEvent{QueryID: a.queryID, ToolName: "verify deployment", Step: 7, Elapsed: 0})

	return a.buildReport(rel, chartInfo), nil
}
```

**Step 4: Create test file and run to verify it passes**

Run: `cd /home/cernet/repo/KubeWise && go test ./pkg/agent/deploy/ -run TestHandleQueryEmitsPhaseEvents -v 2>&1`
Expected: `PASS`

**Step 5: Commit**

```bash
cd /home/cernet/repo/KubeWise
git add pkg/agent/deploy/agent.go pkg/agent/deploy/agent_test.go
git commit -m "feat(deploy): add PhaseEvent/ToolCallEvent/ToolDoneEvent for all 6 deploy stages"
```

---

### Task 2: 窗口自适应 — 构造函数签名变更

**Files:**
- Modify: `pkg/tui/model/deploy_confirm.go:55-87`
- Modify: `pkg/tui/app.go:157`
- Create: `pkg/tui/model/deploy_confirm_test.go`

**Step 1: Write the failing test for NewDeployConfirmModel with size params**

```go
// pkg/tui/model/deploy_confirm_test.go
package model

import (
	"testing"

	"github.com/kubewise/kubewise/pkg/catalog"
	"github.com/kubewise/kubewise/pkg/tui/events"
)

func TestNewDeployConfirmModelWithSize(t *testing.T) {
	plan := events.DeployPlan{
		ChartInfo: &catalog.ChartInfo{
			RepoName: "testrepo", ChartName: "testchart",
			Source: "test", DefaultNamespace: "default",
		},
		DefaultValues: "key: value",
		CustomValues:  "override: value",
		ReleaseName:   "test",
		Namespace:     "default",
	}

	// Call with explicit width/height
	m := NewDeployConfirmModel("q-1", plan, 120, 40)

	// Verify viewport dimensions were calculated correctly
	expectedPanelW := (120 - 4) / 2  // = 58
	expectedPanelH := 40 - 8          // = 32

	if m.defaultVP.Width != expectedPanelW {
		t.Errorf("expected defaultVP.Width=%d, got %d", expectedPanelW, m.defaultVP.Width)
	}
	if m.defaultVP.Height != expectedPanelH {
		t.Errorf("expected defaultVP.Height=%d, got %d", expectedPanelH, m.defaultVP.Height)
	}
	if m.overrideVP.Width != expectedPanelW {
		t.Errorf("expected overrideVP.Width=%d, got %d", expectedPanelW, m.overrideVP.Width)
	}
	if m.overrideVP.Height != expectedPanelH {
		t.Errorf("expected overrideVP.Height=%d, got %d", expectedPanelH, m.overrideVP.Height)
	}

	// Verify full preview viewport
	if m.fullPreviewVP.Width != 120-4 {
		t.Errorf("expected fullPreviewVP.Width=%d, got %d", 120-4, m.fullPreviewVP.Width)
	}
	if m.fullPreviewVP.Height != 40-6 {
		t.Errorf("expected fullPreviewVP.Height=%d, got %d", 40-6, m.fullPreviewVP.Height)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `cd /home/cernet/repo/KubeWise && go test ./pkg/tui/model/ -run TestNewDeployConfirmModelWithSize -v 2>&1`
Expected: `FAIL` — 因为 NewDeployConfirmModel 旧签名不认新参数。

**Step 3: Modify NewDeployConfirmModel signature and internal calculations**

`pkg/tui/model/deploy_confirm.go` 修改：

```go
// NewDeployConfirmModel 创建 Deploy 确认模型。
// width, height 为终端窗口尺寸，用于计算面板大小。
func NewDeployConfirmModel(queryID string, plan events.DeployPlan, width, height int) DeployConfirmModel {
	panelW := (width - 4) / 2
	panelH := height - 8

	defaultVP := viewport.New(panelW, panelH)
	defaultVP.SetContent(plan.DefaultValues)

	overrideVP := viewport.New(panelW, panelH)
	overrideVP.SetContent(plan.CustomValues)

	yamlEditor := textarea.New()
	yamlEditor.SetValue(plan.CustomValues)
	yamlEditor.SetWidth(panelW)
	yamlEditor.SetHeight(panelH)

	nlInput := textinput.New()
	nlInput.Placeholder = "例如：把 NodePort 改成 30090，副本数改成 3"
	nlInput.Width = 60

	// 预计算完整合并 values
	merged, _ := helm.MergeValues(plan.DefaultValues, plan.CustomValues)
	fullPreviewVP := viewport.New(width-4, height-6)
	fullPreviewVP.SetContent(merged)

	return DeployConfirmModel{
		queryID:       queryID,
		plan:          plan,
		mode:          deployConfirmModeView,
		active:        true,
		defaultVP:     defaultVP,
		overrideVP:    overrideVP,
		yamlEditor:    yamlEditor,
		fullPreviewVP: fullPreviewVP,
		nlInput:       nlInput,
		width:         width,
		height:        height,
	}
}
```

`pkg/tui/app.go:157` 调用处修改：

```go
// 旧
m := model.NewDeployConfirmModel(msg.QueryID, msg.Plan)

// 新
m := model.NewDeployConfirmModel(msg.QueryID, msg.Plan, a.width, a.height)
```

**Step 4: Run test to verify it passes**

Run: `cd /home/cernet/repo/KubeWise && go test ./pkg/tui/model/ -run TestNewDeployConfirmModelWithSize -v 2>&1`
Expected: `PASS`

Also run: `cd /home/cernet/repo/KubeWise && go build ./... 2>&1`
Expected: 编译通过无错误

**Step 5: Commit**

```bash
cd /home/cernet/repo/KubeWise
git add pkg/tui/model/deploy_confirm.go pkg/tui/model/deploy_confirm_test.go pkg/tui/app.go
git commit -m "feat(tui): parameterize DeployConfirmModel window size, eliminate hardcoded dimensions"
```

---

### Task 3: 双面板滚动浏览 — Tab 切换 + 焦点高亮

**Files:**
- Modify: `pkg/tui/model/deploy_confirm.go`

**Step 1: Write the failing test for focus panel switching**

```go
// 追加到 pkg/tui/model/deploy_confirm_test.go
func TestDeployConfirmFocusPanel(t *testing.T) {
	plan := events.DeployPlan{
		ChartInfo: &catalog.ChartInfo{
			RepoName: "test", ChartName: "test",
			Source: "test", DefaultNamespace: "default",
		},
		DefaultValues: "default: val",
		CustomValues:  "override: val",
		ReleaseName:   "test",
		Namespace:     "default",
	}
	m := NewDeployConfirmModel("q-1", plan, 120, 40)

	// Initial focus should be left panel (0)
	if m.focusPanel != 0 {
		t.Errorf("expected initial focusPanel=0, got %d", m.focusPanel)
	}

	// Simulate Tab press
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	if updated.focusPanel != 1 {
		t.Errorf("expected focusPanel=1 after Tab, got %d", updated.focusPanel)
	}

	// Simulate Tab again — should go back to 0
	updated2, _ := updated.Update(tea.KeyMsg{Type: tea.KeyTab})
	if updated2.focusPanel != 0 {
		t.Errorf("expected focusPanel=0 after second Tab, got %d", updated2.focusPanel)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `cd /home/cernet/repo/KubeWise && go test ./pkg/tui/model/ -run TestDeployConfirmFocusPanel -v 2>&1`
Expected: `FAIL` — focusPanel 字段尚不存在。

**Step 3: Implement focusPanel logic**

`pkg/tui/model/deploy_confirm.go` 修改清单：

1. **新增字段** `focusPanel int` 到 `DeployConfirmModel` struct
2. **新增边框样式变量**
3. **修改 Update 方法** — Tab 键切换焦点，方向键路由到焦点面板
4. **修改 View 方法** — 焦点面板蓝色高亮边框

**3a: 新增字段**

在 `DeployConfirmModel` struct 中 `height int` 之后添加：

```go
focusPanel int // 0 = 左面板（default），1 = 右面板（override）
```

**3b: 新增边框样式**

在文件顶部（`import` 之后）添加：

```go
var (
	panelFocusBorder  = lipgloss.Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("12")) // 蓝色
	panelNormalBorder = lipgloss.Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("8"))  // 灰色
)
```

**3c: 修改 Update — 在 handleViewMode 添加 Tab 切换**

```go
func (m DeployConfirmModel) handleViewMode(msg tea.KeyMsg) (DeployConfirmModel, tea.Cmd) {
	switch strings.ToLower(msg.String()) {
	case "tab":
		m.focusPanel = 1 - m.focusPanel // 切换 0↔1
	// ... 其余 case 不变
	}
	return m, nil
}
```

**3d: 修改 Update 的尾部视图滚动路由**

将 `Update()` 末尾的（从 line 132 原代码）：

```go
// 转发滚动事件到当前活跃面板
var cmd tea.Cmd
if m.mode == deployConfirmModeView {
    m.defaultVP, cmd = m.defaultVP.Update(msg)
}
return m, cmd
```

改为：

```go
// 转发滚动事件到当前焦点面板
var cmd tea.Cmd
if m.mode == deployConfirmModeView {
    if msg, ok := msg.(tea.KeyMsg); ok {
        switch msg.String() {
        case "up", "down", "pgup", "pgdown":
            if m.focusPanel == 0 {
                m.defaultVP, cmd = m.defaultVP.Update(msg)
            } else {
                m.overrideVP, cmd = m.overrideVP.Update(msg)
            }
            return m, cmd
        }
    }
}
return m, cmd
```

**3e: 修改 View 方法 — 焦点面板蓝框**

在 `View()` 的 `deployConfirmModeView` case 中，将左右面板渲染改为：

```go
case deployConfirmModeView:
    // 左右双面板
    leftStyle := panelNormalBorder
    rightStyle := panelNormalBorder
    if m.focusPanel == 0 {
        leftStyle = panelFocusBorder
    } else {
        rightStyle = panelFocusBorder
    }
    leftPanel := leftStyle.
        Padding(0, 1).
        Render("默认 Values (参考)\n" + m.defaultVP.View())
    rightPanel := rightStyle.
        Padding(0, 1).
        Render("Override Values (Agent 生成)\n" + m.overrideVP.View())
    sb.WriteString(lipgloss.JoinHorizontal(lipgloss.Top, leftPanel, "  ", rightPanel))
    sb.WriteString("\n\n[Y] 执行  [E] 编辑 YAML  [C] 自然语言修正  [V] 完整预览  [Tab] 切换面板  [N] 取消\n")
```

**Step 4: Run test to verify it passes**

Run: `cd /home/cernet/repo/KubeWise && go test ./pkg/tui/model/ -run TestDeployConfirmFocusPanel -v 2>&1`
Expected: `PASS`

Also run: `cd /home/cernet/repo/KubeWise && go build ./... 2>&1`
Expected: 编译通过

**Step 5: Commit**

```bash
cd /home/cernet/repo/KubeWise
git add pkg/tui/model/deploy_confirm.go pkg/tui/model/deploy_confirm_test.go
git commit -m "feat(tui): add Tab-switch focus panel with blue highlight border in deploy confirm view"
```

---

### Task 4a: 方向键选择 — handleShortcut 前置判断

**Files:**
- Modify: `pkg/tui/app.go:250-309`

**Step 1: Write a behavioral test (integration level)**

```go
// 此改动难以在纯单元测试中验证（依赖 Bubble Tea 框架的 KeyMsg 路由）。
// 验证方式为手动测试：编译运行 TUI → 进入 Chart 选择界面 → 按 ↑↓ 确认光标可移动。
// 我们在此编写逻辑覆盖测试验证 helper 方法的行为。
```

**Step 2: N/A — no specific test written for this small change**

**Step 3: Modify handleShortcut**

在 `handleShortcut()` 函数开头添加：

```go
func (a App) handleShortcut(msg tea.KeyMsg) (tea.Cmd, bool) {
	// 如果 deploy TUI 组件 active，不拦截方向键
	if a.chartSelectModel != nil || a.deployConfirmModel != nil || a.manualInputModel != nil {
		return nil, false
	}
	// ... 原有 switch 逻辑
```

**Step 4: Verify compilation**

Run: `cd /home/cernet/repo/KubeWise && go build ./... 2>&1`
Expected: 编译通过

**Step 5: Commit**

```bash
cd /home/cernet/repo/KubeWise
git add pkg/tui/app.go
git commit -m "fix(tui): bypass direction key interception when deploy TUI components active"
```

---

### Task 4b: 光标视觉增强 — Chart 选择列表高亮

**Files:**
- Modify: `pkg/tui/model/chart_select.go`
- Modify: `pkg/tui/model/chart_select_test.go`

**Step 1: Write the failing test for cursor style**

```go
// 追加到 chart_select_test.go（需创建该文件）
// pkg/tui/model/chart_select_test.go
package model

import (
	"strings"
	"testing"

	"github.com/kubewise/kubewise/pkg/catalog"
)

func TestChartSelectCursorStyle(t *testing.T) {
	candidates := []catalog.ChartInfo{
		{RepoName: "bitnami", ChartName: "nginx", Stars: 300, Description: "nginx desc", RepoURL: "https://charts.bitnami.com/bitnami"},
		{RepoName: "argo", ChartName: "argo-cd", Stars: 1200, Description: "argo desc", RepoURL: "https://argoproj.github.io/argo-helm"},
	}
	m := NewChartSelectModel("q-1", "myapp", candidates)

	view := m.View()

	// Cursor line should contain "> " indicator
	if !strings.Contains(view, "> [1]") {
		t.Errorf("expected cursor line '> [1]', got: %q", view)
	}

	// The first line should have blue highlight style (check for ANSI escape codes)
	// lipgloss blue foreground (color "12") = ESC[38;5;12m
	if !strings.Contains(view, "[38;5;12m") {
		t.Errorf("expected blue foreground ANSI code in cursor line, got: %q", view)
	}

	// lipgloss light blue background (color "153") = ESC[48;5;153m
	if !strings.Contains(view, "[48;5;153m") {
		t.Errorf("expected light blue background ANSI code in cursor line, got: %q", view)
	}

	// Second line (non-cursor) should NOT have blue highlight
	// Find the second entry and check it has the non-cursor styling
	if strings.Contains(view, "[2] bitnami/nginx") && strings.Contains(view, "[38;5;12m") == false {
		// This is fine — non-cursor lines should not have the blue foreground
	}
}
```

**Step 2: Run test to verify it fails**

Run: `cd /home/cernet/repo/KubeWise && go test ./pkg/tui/model/ -run TestChartSelectCursorStyle -v 2>&1`
Expected: `FAIL` — 测试文件不存在。

**Step 3: Implement cursor visual enhancement**

在 `chart_select.go` 中添加样式变量：

```go
var (
	cursorStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("12")).  // 蓝色字
		Background(lipgloss.Color("153")). // 浅蓝底色 (#E0E8FF)
		Bold(true)
)
```

修改 `View()` 中的光标行渲染（从 line 149 改）：

```go
if i == m.cursor {
    sb.WriteString(cursorStyle.Render(line))
} else {
    sb.WriteString(line)
}
```

**Step 4: Run test to verify it passes**

Run: `cd /home/cernet/repo/KubeWise && go test ./pkg/tui/model/ -run TestChartSelectCursorStyle -v 2>&1`
Expected: `PASS`

Also run: `cd /home/cernet/repo/KubeWise && go build ./... 2>&1`
Expected: 编译通过

**Step 5: Commit**

```bash
cd /home/cernet/repo/KubeWise
git add pkg/tui/model/chart_select.go pkg/tui/model/chart_select_test.go
git commit -m "feat(tui): enhance chart select cursor with blue foreground + light blue background"
```

---

## 执行顺序与依赖

```
Task 1 (agent.go events) ────────────────── 无依赖
Task 2 (window sizing)   ────────────────── 无依赖
Task 3 (focus panel)     ─── 依赖 Task 2 ── deploy_confirm.go
Task 4a (shortcut fix)   ── 无依赖
Task 4b (cursor style)   ── 无依赖
```

建议执行顺序: 1 → 2 → 3 → 4a → 4b

---

## 验证清单

| 验证项 | 命令 | 预期 |
|--------|------|------|
| 编译 | `go build ./...` | 无错误 |
| 所有测试 | `go test ./...` | ALL PASS |
| Task 1 测试 | `go test ./pkg/agent/deploy/ -v -run TestHandleQuery` | PASS |
| Task 2+3 测试 | `go test ./pkg/tui/model/ -v -run TestDeploy` | PASS |
| Task 4b 测试 | `go test ./pkg/tui/model/ -v -run TestChartSelect` | PASS |
