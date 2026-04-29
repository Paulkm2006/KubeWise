package model

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/kubewise/kubewise/pkg/tui/session"
	"github.com/kubewise/kubewise/pkg/tui/styles"
)

// SidebarModel renders the session list on the left side of the TUI.
type SidebarModel struct {
	sessions []*session.Session
	selected int
	visible  bool
	height   int
}

// NewSidebarModel returns a SidebarModel with sensible defaults.
func NewSidebarModel() SidebarModel {
	return SidebarModel{visible: true, height: 24}
}

// SetSessions replaces the session list and resets the selection to 0.
func (m *SidebarModel) SetSessions(sessions []*session.Session) {
	m.sessions = sessions
	m.selected = 0
}

// SetHeight sets the available height for rendering.
func (m *SidebarModel) SetHeight(h int) { m.height = h }

// SetVisible controls sidebar visibility.
func (m *SidebarModel) SetVisible(v bool) { m.visible = v }

// IsVisible reports whether the sidebar is shown.
func (m SidebarModel) IsVisible() bool { return m.visible }

// SelectedIndex returns the index of the currently highlighted session.
func (m SidebarModel) SelectedIndex() int { return m.selected }

// SelectedSession returns the currently highlighted session, or nil if none.
func (m SidebarModel) SelectedSession() *session.Session {
	if len(m.sessions) == 0 || m.selected >= len(m.sessions) {
		return nil
	}
	return m.sessions[m.selected]
}

// Update handles ↑/↓ navigation and returns the updated model and an optional command.
func (m SidebarModel) Update(msg tea.Msg) (SidebarModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyUp:
			if m.selected > 0 {
				m.selected--
			}
		case tea.KeyDown:
			if m.selected < len(m.sessions)-1 {
				m.selected++
			}
		}
	}
	return m, nil
}

// View renders the sidebar. Returns an empty string when not visible.
func (m SidebarModel) View() string {
	if !m.visible {
		return ""
	}

	lines := []string{
		styles.SidebarHeaderStyle.Render("会话历史"),
	}

	maxItems := max(m.height-2, 0)

	for i, s := range m.sessions {
		if i >= maxItems {
			break
		}

		// Truncate title to at most 12 runes for display safety.
		runes := []rune(s.Title)
		title := string(runes)
		if len(runes) > 12 {
			title = string(runes[:12])
		}

		rel := relativeTime(s.UpdatedAt)
		_ = rel // included in future formatting; kept for completeness

		var line string
		if i == m.selected {
			line = styles.SidebarActiveStyle.Render("> " + title)
		} else {
			line = styles.SidebarItemStyle.Render("  " + title)
		}
		lines = append(lines, line)
	}

	return styles.SidebarStyle.Height(m.height).Render(strings.Join(lines, "\n"))
}

// relativeTime returns a human-readable Chinese relative time string.
func relativeTime(t time.Time) string {
	diff := time.Since(t)
	switch {
	case diff < time.Minute:
		return "刚刚"
	case diff < time.Hour:
		return fmt.Sprintf("%d分钟前", int(diff.Minutes()))
	case diff < 24*time.Hour:
		return fmt.Sprintf("%d小时前", int(diff.Hours()))
	default:
		return t.Format("01-02")
	}
}
