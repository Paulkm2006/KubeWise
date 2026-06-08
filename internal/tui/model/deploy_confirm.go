// pkg/tui/model/deploy_confirm.go
package model

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	deploytypes "github.com/kubewise/kubewise/internal/agent/subagent/deploy/types"
	"github.com/kubewise/kubewise/internal/utils/helm"
)

// deployConfirmMode 表示 Deploy 确认界面的当前交互模式。
type deployConfirmMode int

const (
	deployConfirmModeView        deployConfirmMode = iota // 初始查看模式
	deployConfirmModeEditYAML                             // YAML 编辑模式
	deployConfirmModeEditNL                               // 自然语言修正模式
	deployConfirmModeFullPreview                          // 完整 values 预览模式
)

// 布局 padding 常量（基于 View() 渲染结构计算）。
const (
	panelHPadding   = 8 // 面板垂直方向：标题(1) + 空行(2) + 边框(2) + 间隔(1) + 操作栏(2)
	panelWPadding   = 4 // 面板水平方向：左右边框各2
	fullPreviewHPad = 6 // 完整预览垂直方向：标题(1) + 边框(2) + 间隔(1) + 操作栏(2)
	fullPreviewWPad = 4 // 完整预览水平方向
)

// 面板边框样式
var (
	panelFocusBorder  = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("12")) // 蓝色
	panelNormalBorder = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("8"))  // 灰色
)

// layoutDimensions 根据窗口尺寸计算各组件布局尺寸，含最小值保护。
func layoutDimensions(width, height int) (panelW, panelH, fullW, fullH int) {
	panelW = max((width-panelWPadding)/2, 1)
	panelH = max(height-panelHPadding, 1)
	fullW = max(width-fullPreviewWPad, 1)
	fullH = max(height-fullPreviewHPad, 1)
	return
}

// DeployConfirmDoneMsg 用户完成确认后发送的消息。
type DeployConfirmDoneMsg struct {
	QueryID  string
	Decision deploytypes.DeployDecision
}

// DeployConfirmModel 是 Deploy 确认界面的 Bubble Tea 模型。
type DeployConfirmModel struct {
	queryID string
	plan    deploytypes.DeployPlan
	mode    deployConfirmMode
	active  bool
	// 左面板：默认 values 滚动视图
	defaultVP viewport.Model
	// 右面板：override values（查看模式）
	overrideVP viewport.Model
	// YAML 编辑模式
	yamlEditor  textarea.Model
	yamlEditErr string
	// 自然语言修正模式
	nlInput textinput.Model
	// 完整预览模式
	fullPreviewVP viewport.Model
	width         int
	height        int
	focusPanel    int // 0 = 左面板（default），1 = 右面板（override）
}

// NewDeployConfirmModel 创建 Deploy 确认模型。
// width, height 为终端窗口尺寸，用于内部组件布局。
func NewDeployConfirmModel(queryID string, plan deploytypes.DeployPlan, width, height int) DeployConfirmModel {
	panelW, panelH, fullW, fullH := layoutDimensions(width, height)

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
	nlInput.Width = min(panelW, 60)

	// 预计算完整合并 values
	merged, _ := helm.MergeValues(plan.DefaultValues, plan.CustomValues)
	fullPreviewVP := viewport.New(fullW, fullH)
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

// DefaultVPWidth 返回默认 values 面板宽度（测试用）。
func (m DeployConfirmModel) DefaultVPWidth() int { return m.defaultVP.Width }

// DefaultVPHeight 返回默认 values 面板高度（测试用）。
func (m DeployConfirmModel) DefaultVPHeight() int { return m.defaultVP.Height }

// OverrideVPWidth 返回 override values 面板宽度（测试用）。
func (m DeployConfirmModel) OverrideVPWidth() int { return m.overrideVP.Width }

// OverrideVPHeight 返回 override values 面板高度（测试用）。
func (m DeployConfirmModel) OverrideVPHeight() int { return m.overrideVP.Height }

// YAMLEditorWidth 返回 YAML 编辑器宽度（测试用）。
func (m DeployConfirmModel) YAMLEditorWidth() int { return m.yamlEditor.Width() }

// YAMLEditorHeight 返回 YAML 编辑器高度（测试用）。
func (m DeployConfirmModel) YAMLEditorHeight() int { return m.yamlEditor.Height() }

// FullPreviewVPWidth 返回完整预览面板宽度（测试用）。
func (m DeployConfirmModel) FullPreviewVPWidth() int { return m.fullPreviewVP.Width }

// FullPreviewVPHeight 返回完整预览面板高度（测试用）。
func (m DeployConfirmModel) FullPreviewVPHeight() int { return m.fullPreviewVP.Height }

// FocusPanel 返回当前焦点面板（测试用）。
func (m DeployConfirmModel) FocusPanel() int { return m.focusPanel }

func (m DeployConfirmModel) Init() tea.Cmd {
	return nil
}

func (m DeployConfirmModel) Update(msg tea.Msg) (DeployConfirmModel, tea.Cmd) {
	if !m.active {
		return m, nil
	}

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		panelW, panelH, fullW, fullH := layoutDimensions(msg.Width, msg.Height)
		m.defaultVP.Width = panelW
		m.defaultVP.Height = panelH
		m.overrideVP.Width = panelW
		m.overrideVP.Height = panelH
		m.yamlEditor.SetWidth(panelW)
		m.yamlEditor.SetHeight(panelH)
		m.fullPreviewVP.Width = fullW
		m.fullPreviewVP.Height = fullH

	case tea.KeyMsg:
		switch m.mode {
		case deployConfirmModeView:
			return m.handleViewMode(msg)
		case deployConfirmModeEditYAML:
			return m.handleEditYAMLMode(msg)
		case deployConfirmModeEditNL:
			return m.handleEditNLMode(msg)
		case deployConfirmModeFullPreview:
			if msg.String() == "esc" {
				m.mode = deployConfirmModeView
			} else {
				var cmd tea.Cmd
				m.fullPreviewVP, cmd = m.fullPreviewVP.Update(msg)
				return m, cmd
			}
		}

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
	}

	var cmd tea.Cmd
	return m, cmd
}

func (m DeployConfirmModel) handleViewMode(msg tea.KeyMsg) (DeployConfirmModel, tea.Cmd) {
	switch strings.ToLower(msg.String()) {
	case "tab":
		m.focusPanel = 1 - m.focusPanel // 切换 0↔1
	case "y":
		m.active = false
		values := m.plan.CustomValues
		return m, func() tea.Msg {
			return DeployConfirmDoneMsg{
				QueryID:  m.queryID,
				Decision: deploytypes.DeployDecision{Action: "execute", Values: values},
			}
		}
	case "n", "esc":
		m.active = false
		return m, func() tea.Msg {
			return DeployConfirmDoneMsg{
				QueryID:  m.queryID,
				Decision: deploytypes.DeployDecision{Action: "cancel"},
			}
		}
	case "e":
		m.mode = deployConfirmModeEditYAML
		m.yamlEditor.SetValue(m.plan.CustomValues)
		m.yamlEditor.Focus()
		m.yamlEditErr = ""
	case "c":
		m.mode = deployConfirmModeEditNL
		m.nlInput.SetValue("")
		m.nlInput.Focus()
	case "v":
		merged, _ := helm.MergeValues(m.plan.DefaultValues, m.plan.CustomValues)
		m.fullPreviewVP.SetContent(merged)
		m.mode = deployConfirmModeFullPreview
	case "up", "down", "pgup", "pgdown":
		var cmd tea.Cmd
		if m.focusPanel == 0 {
			m.defaultVP, cmd = m.defaultVP.Update(msg)
		} else {
			m.overrideVP, cmd = m.overrideVP.Update(msg)
		}
		return m, cmd
	}
	return m, nil
}

func (m DeployConfirmModel) handleEditYAMLMode(msg tea.KeyMsg) (DeployConfirmModel, tea.Cmd) {
	switch msg.String() {
	case "ctrl+s":
		newValues := m.yamlEditor.Value()
		if err := helm.ValidateYAML(newValues); err != nil {
			m.yamlEditErr = "YAML 语法错误: " + err.Error()
			return m, nil
		}
		m.plan.CustomValues = newValues
		m.overrideVP.SetContent(newValues)
		m.mode = deployConfirmModeView
		m.yamlEditErr = ""
	case "esc":
		m.mode = deployConfirmModeView
		m.yamlEditErr = ""
	default:
		var cmd tea.Cmd
		m.yamlEditor, cmd = m.yamlEditor.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m DeployConfirmModel) handleEditNLMode(msg tea.KeyMsg) (DeployConfirmModel, tea.Cmd) {
	switch msg.String() {
	case "enter":
		correction := strings.TrimSpace(m.nlInput.Value())
		if correction == "" {
			m.mode = deployConfirmModeView
			return m, nil
		}
		m.active = false
		values := m.plan.CustomValues
		return m, func() tea.Msg {
			return DeployConfirmDoneMsg{
				QueryID: m.queryID,
				Decision: deploytypes.DeployDecision{
					Action:     "execute",
					Values:     values,
					Correction: correction,
				},
			}
		}
	case "esc":
		m.mode = deployConfirmModeView
	default:
		var cmd tea.Cmd
		m.nlInput, cmd = m.nlInput.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m DeployConfirmModel) View() string {
	if !m.active {
		return ""
	}

	var sb strings.Builder

	// 标题栏
	chartInfo := m.plan.ChartInfo
	action := "安装"
	if m.plan.IsUpgrade {
		action = "升级"
	}
	header := lipgloss.NewStyle().Bold(true).Render(
		fmt.Sprintf("Helm Deploy 确认 [%s]  Chart: %s/%s  来源: %s  Release: %s → Namespace: %s",
			action,
			chartInfo.RepoName, chartInfo.ChartName,
			chartInfo.Source,
			m.plan.ReleaseName, m.plan.Namespace,
		),
	)
	sb.WriteString(header + "\n\n")

	if len(m.plan.Warnings) > 0 {
		warnStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("11"))
		errStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
		sb.WriteString(lipgloss.NewStyle().Bold(true).Render("策略与校验提示:") + "\n")
		for _, w := range m.plan.Warnings {
			line := "• " + w.Message
			if w.Severity == "error" {
				sb.WriteString(errStyle.Render(line) + "\n")
			} else {
				sb.WriteString(warnStyle.Render(line) + "\n")
			}
		}
		sb.WriteString("\n")
	}

	switch m.mode {
	case deployConfirmModeView:
		// 左右双面板（焦点面板蓝色高亮边框）
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

	case deployConfirmModeEditYAML:
		sb.WriteString(lipgloss.NewStyle().Bold(true).Render("编辑 Override Values (Ctrl+S 保存, Esc 放弃)") + "\n")
		sb.WriteString(m.yamlEditor.View() + "\n")
		if m.yamlEditErr != "" {
			sb.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Render("❌ "+m.yamlEditErr) + "\n")
		}

	case deployConfirmModeEditNL:
		sb.WriteString(lipgloss.NewStyle().Bold(true).Render("自然语言修正 (Enter 提交, Esc 取消)") + "\n")
		sb.WriteString(m.nlInput.View() + "\n")

	case deployConfirmModeFullPreview:
		sb.WriteString(lipgloss.NewStyle().Bold(true).Render("完整合并 Values 预览 (Esc 返回)") + "\n")
		sb.WriteString(m.fullPreviewVP.View() + "\n")
	}

	return sb.String()
}
