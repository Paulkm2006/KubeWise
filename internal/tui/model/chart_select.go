// pkg/tui/model/chart_select.go
package model

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/kubewise/kubewise/internal/agent/subagent/deploy/core/catalog"
)

var (
	cursorStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("12")).  // 蓝色字
		Background(lipgloss.Color("153")). // 浅蓝底色
		Bold(true)
)

// ChartSelectedMsg 用户选择了 Chart 后发送的消息。
type ChartSelectedMsg struct {
	QueryID   string
	ChartInfo *catalog.ChartInfo // nil 表示取消；Source="manual" 表示手动输入
}

// chartSelectTickMsg 倒计时 tick 消息。
type chartSelectTickMsg struct{}

// ChartSelectModel 是 Chart 选择界面的 Bubble Tea 模型。
type ChartSelectModel struct {
	queryID    string
	appName    string
	candidates []catalog.ChartInfo
	cursor     int
	countdown  int  // 剩余秒数，-1 表示已禁用
	active     bool // 是否正在显示
}

// NewChartSelectModel 创建 Chart 选择模型。
func NewChartSelectModel(queryID, appName string, candidates []catalog.ChartInfo) ChartSelectModel {
	return ChartSelectModel{
		queryID:    queryID,
		appName:    appName,
		candidates: candidates,
		cursor:     0,
		countdown:  10,
		active:     true,
	}
}

func (m ChartSelectModel) Init() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg {
		return chartSelectTickMsg{}
	})
}

func (m ChartSelectModel) Update(msg tea.Msg) (ChartSelectModel, tea.Cmd) {
	if !m.active {
		return m, nil
	}

	switch msg := msg.(type) {
	case chartSelectTickMsg:
		if m.countdown > 0 {
			m.countdown--
			if m.countdown == 0 {
				// 自动选择第一项
				m.active = false
				if len(m.candidates) > 0 {
					selected := m.candidates[0]
					return m, func() tea.Msg {
						return ChartSelectedMsg{QueryID: m.queryID, ChartInfo: &selected}
					}
				}
			}
			return m, tea.Tick(time.Second, func(t time.Time) tea.Msg {
				return chartSelectTickMsg{}
			})
		}

	case tea.KeyMsg:
		m.countdown = -1 // 任意按键取消倒计时
		switch msg.String() {
		case "esc":
			m.active = false
			return m, func() tea.Msg {
				return ChartSelectedMsg{QueryID: m.queryID, ChartInfo: nil}
			}
		case "0":
			m.active = false
			return m, func() tea.Msg {
				return ChartSelectedMsg{QueryID: m.queryID, ChartInfo: &catalog.ChartInfo{Source: "manual"}}
			}
		case "1", "2", "3", "4", "5", "6", "7", "8", "9":
			idx := int(msg.String()[0] - '1')
			if idx < len(m.candidates) {
				m.active = false
				selected := m.candidates[idx]
				return m, func() tea.Msg {
					return ChartSelectedMsg{QueryID: m.queryID, ChartInfo: &selected}
				}
			}
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.candidates)-1 {
				m.cursor++
			}
		case "enter":
			if len(m.candidates) > 0 {
				m.active = false
				selected := m.candidates[m.cursor]
				return m, func() tea.Msg {
					return ChartSelectedMsg{QueryID: m.queryID, ChartInfo: &selected}
				}
			}
		}
	}
	return m, nil
}

func (m ChartSelectModel) View() string {
	if !m.active {
		return ""
	}

	var sb strings.Builder

	// 标题
	title := fmt.Sprintf("找到 %d 个 Helm Chart，请选择", len(m.candidates))
	if m.countdown > 0 {
		title += fmt.Sprintf("（%d 秒后自动选择推荐项 #1）", m.countdown)
	}
	sb.WriteString(lipgloss.NewStyle().Bold(true).Render(title) + "\n\n")

	// 候选列表
	for i, c := range m.candidates {
		cursor := "  "
		if i == m.cursor {
			cursor = "> "
		}
		stars := ""
		if c.Stars > 0 {
			stars = fmt.Sprintf("⭐ %d", c.Stars)
		}
		recommended := ""
		switch {
		case c.CuratedPick:
			recommended = " [常用·推荐]"
		case i == 0:
			recommended = " [推荐]"
		}
		trust := formatChartTrustBadges(c)
		line := fmt.Sprintf("%s[%d] %s/%s%s  %s%s\n    %s\n    %s\n",
			cursor, i+1,
			c.RepoName, c.ChartName, recommended,
			stars, trust,
			c.Description,
			c.RepoURL,
		)
		if i == m.cursor {
			sb.WriteString(cursorStyle.Render(line))
		} else {
			sb.WriteString(line)
		}
		sb.WriteString("\n")
	}

	// 手动输入选项
	sb.WriteString("  [0] 手动指定 repo URL 和 chart 名称\n\n")
	sb.WriteString("↑↓ 选择  Enter/数字键 确认  Esc 取消\n")

	return sb.String()
}

func formatChartTrustBadges(c catalog.ChartInfo) string {
	var badges []string
	if c.Official {
		badges = append(badges, "official")
	}
	if c.CNCF {
		badges = append(badges, "cncf")
	}
	if c.VerifiedPublisher {
		badges = append(badges, "verified")
	}
	if c.Signed {
		badges = append(badges, "signed")
	}
	if c.Deprecated {
		badges = append(badges, "deprecated")
	}
	if len(badges) == 0 {
		return ""
	}
	return "  [" + strings.Join(badges, " · ") + "]"
}
