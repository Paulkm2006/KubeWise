# Deploy Confirm 面板滚动支持

**日期:** 2026-05-18
**状态:** 已批准

## 问题

`DeployConfirmModel` 存在两个滚动缺陷：

1. **View 模式（左右 values 对比面板）**：方向键 `up`/`down`/`pgup`/`pgdown` 和鼠标滚轮均无法滚动 viewport。
   - 方向键：`handleViewMode()` 被 `return m.handleViewMode(msg)` 调用，函数直接返回，导致 Update 底部的 fallback 代码（lines 189-201）永远走不到——是死代码。
   - 鼠标滚轮：`Update()` 中完全没有 `tea.MouseMsg` case，事件被静默忽略。

2. **Full Preview 模式（按 V）**：方向键已正确工作（直接转发给 `fullPreviewVP.Update`），但鼠标滚轮缺失（同无 MouseMsg 处理）。

## 方案

方案 A：最小化修改 `deploy_confirm.go`，在 `handleViewMode` 中内联方向键处理，在主 switch 中新增 MouseMsg 分支，删除死代码。

## 改动

只改一个文件：`pkg/tui/model/deploy_confirm.go`

### 1. `handleViewMode` — 新增方向键转发

```go
case "up", "down", "pgup", "pgdown":
    var cmd tea.Cmd
    if m.focusPanel == 0 {
        m.defaultVP, cmd = m.defaultVP.Update(msg)
    } else {
        m.overrideVP, cmd = m.overrideVP.Update(msg)
    }
    return m, cmd
```

Tab 切换焦点面板后，方向键自动作用于新焦点面板。

### 2. `Update()` — 新增 `tea.MouseMsg` 分支

```go
case tea.MouseMsg:
    switch m.mode {
    case deployConfirmModeView:
        var cmd tea.Cmd
        if m.focusPanel == 0 {
            m.defaultVP, cmd = m.defaultVP.Update(msg)
        } else {
            m.overrideVP, cmd = m.overrideVP.Update(msg)
        }
        return m, cmd
    case deployConfirmModeFullPreview:
        var cmd tea.Cmd
        m.fullPreviewVP, cmd = m.fullPreviewVP.Update(msg)
        return m, cmd
    }
```

- View 模式：滚轮事件转发到当前 Tab 焦点的 viewport
- FullPreview 模式：滚轮事件转发到 fullPreviewVP  
- YAML 编辑模式：跳过（textarea 原生支持）
- NL 修正模式：跳过（单行 textinput）

### 3. 删除死代码

删除 `Update()` lines 189-201——原方向键 fallback，已被 `handleViewMode` 的提前 return 阻断。

## 测试

新增 3 个测试用例：

| 测试 | 场景 |
|------|------|
| `TestDeployConfirmViewScrollKeys` | view 模式 `up`/`down` → 焦点 viewport.YOffset 变化 |
| `TestDeployConfirmViewMouseWheel` | view 模式 `MouseMsg{Action: Press, Button: WheelUp}` → 焦点 viewport 滚动 |
| `TestDeployConfirmFullPreviewMouseWheel` | full preview 模式鼠标滚轮 → fullPreviewVP 滚动 |

## 不处理

- YAML 编辑模式（textarea 原生支持方向键和滚轮）
- NL 修正模式（单行输入，无滚动需求）
- 水平滚动（YAML 内容通常不超宽）
