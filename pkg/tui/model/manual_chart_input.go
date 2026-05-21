// pkg/tui/model/manual_chart_input.go
package model

import (
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/kubewise/kubewise/pkg/catalog"
)

// ManualChartInputDoneMsg 用户完成手动输入后发送的消息。
type ManualChartInputDoneMsg struct {
	QueryID   string
	ChartInfo *catalog.ChartInfo // nil 表示取消
	Error     string             // 验证错误信息
}

// ManualChartInputModel 是手动 Chart 输入界面的 Bubble Tea 模型。
type ManualChartInputModel struct {
	queryID    string
	repoInput  textinput.Model
	chartInput textinput.Model
	focusIdx   int // 0=repoURL, 1=chartName
	active     bool
	errMsg     string
}

// NewManualChartInputModel 创建手动输入模型。
func NewManualChartInputModel(queryID string) ManualChartInputModel {
	repoInput := textinput.New()
	repoInput.Placeholder = "https://argoproj.github.io/argo-helm"
	repoInput.Focus()
	repoInput.Width = 60

	chartInput := textinput.New()
	chartInput.Placeholder = "argo-cd"
	chartInput.Width = 60

	return ManualChartInputModel{
		queryID:    queryID,
		repoInput:  repoInput,
		chartInput: chartInput,
		focusIdx:   0,
		active:     true,
	}
}

func (m ManualChartInputModel) Init() tea.Cmd {
	return textinput.Blink
}

func (m ManualChartInputModel) Update(msg tea.Msg) (ManualChartInputModel, tea.Cmd) {
	if !m.active {
		return m, nil
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			m.active = false
			return m, func() tea.Msg {
				return ManualChartInputDoneMsg{QueryID: m.queryID, ChartInfo: nil}
			}
		case "tab", "shift+tab":
			m.focusIdx = 1 - m.focusIdx
			if m.focusIdx == 0 {
				m.repoInput.Focus()
				m.chartInput.Blur()
			} else {
				m.chartInput.Focus()
				m.repoInput.Blur()
			}
		case "enter":
			repoURL := strings.TrimSpace(m.repoInput.Value())
			chartName := strings.TrimSpace(m.chartInput.Value())
			if repoURL == "" || chartName == "" {
				m.errMsg = "Repo URL 和 Chart 名称不能为空"
				return m, nil
			}
			m.active = false
			// 从 URL 中提取 repo 名称（取最后一段路径）
			parts := strings.Split(strings.TrimRight(repoURL, "/"), "/")
			repoName := parts[len(parts)-1]
			return m, func() tea.Msg {
				return ManualChartInputDoneMsg{
					QueryID: m.queryID,
					ChartInfo: &catalog.ChartInfo{
						RepoName:  repoName,
						RepoURL:   repoURL,
						ChartName: chartName,
						Source:    "manual",
					},
				}
			}
		}
	}

	var cmd tea.Cmd
	if m.focusIdx == 0 {
		m.repoInput, cmd = m.repoInput.Update(msg)
	} else {
		m.chartInput, cmd = m.chartInput.Update(msg)
	}
	return m, cmd
}

func (m ManualChartInputModel) View() string {
	if !m.active {
		return ""
	}

	var sb strings.Builder
	sb.WriteString(lipgloss.NewStyle().Bold(true).Render("手动指定 Helm Chart") + "\n\n")
	sb.WriteString("Repo URL:\n")
	sb.WriteString(m.repoInput.View() + "\n\n")
	sb.WriteString("Chart 名称:\n")
	sb.WriteString(m.chartInput.View() + "\n\n")
	sb.WriteString("💡 提示：可在 https://artifacthub.io 搜索 chart 的 repo URL\n\n")
	if m.errMsg != "" {
		sb.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Render("❌ "+m.errMsg) + "\n\n")
	}
	sb.WriteString("Tab 切换字段  Enter 确认  Esc 取消\n")
	return sb.String()
}
