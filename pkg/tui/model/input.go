package model

import (
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/kubewise/kubewise/pkg/tui/styles"
)

// maxHistoryLength limits the number of entries retained for Up/Down history navigation.
const maxHistoryLength = 500

// SubmitMsg is sent when the user presses Enter in the input box.
type SubmitMsg struct{ Value string }

type InputModel struct {
	textarea    textarea.Model
	enabled     bool
	width       int
	history     []string // messages submitted by user
	historyIdx  int      // -1 = fresh input, 0..len-1 = browsing
	savedBuffer string   // temp storage when user starts browsing with content
}

func NewInputModel() InputModel {
	ta := textarea.New()
	ta.Placeholder = "Type a message... (Enter to send)"
	ta.CharLimit = 2048
	ta.SetWidth(80)
	ta.SetHeight(3)
	ta.ShowLineNumbers = false
	ta.Focus()
	return InputModel{
		textarea: ta,
		enabled:  true,
		width:    80,
	}
}

// Init returns the textarea blink command.
func (m InputModel) Init() tea.Cmd {
	return textarea.Blink
}

// Update handles input. Enter submits the current value (if non-empty) and
// adds it to history. Up/Down navigate through history. All other keys
// are forwarded to the underlying textarea when enabled.
func (m InputModel) Update(msg tea.Msg) (InputModel, tea.Cmd) {
	if !m.enabled {
		return m, nil
	}
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyEnter:
			val := strings.TrimSpace(m.textarea.Value())
			if val == "" {
				return m, nil
			}
			// Add to history
			if len(m.history) == 0 || m.history[len(m.history)-1] != val {
				m.history = append(m.history, val)
				if len(m.history) > maxHistoryLength {
					m.history = m.history[len(m.history)-maxHistoryLength:]
				}
			}
			m.historyIdx = -1
			m.savedBuffer = ""
			m.textarea.Reset()
			return m, func() tea.Msg { return SubmitMsg{Value: val} }

		case tea.KeyUp:
			if len(m.history) == 0 {
				return m, nil
			}
			if m.historyIdx == -1 {
				// Save current input before browsing
				m.savedBuffer = m.textarea.Value()
			}
			if m.historyIdx < len(m.history)-1 {
				m.historyIdx++
			}
			m.textarea.SetValue(m.history[len(m.history)-1-m.historyIdx])
			m.textarea.SetCursor(len(m.textarea.Value()))
			return m, nil

		case tea.KeyDown:
			if m.historyIdx == -1 {
				return m, nil // already at fresh input
			}
			m.historyIdx--
			if m.historyIdx == -1 {
				m.textarea.SetValue(m.savedBuffer)
				m.savedBuffer = ""
			} else {
				m.textarea.SetValue(m.history[len(m.history)-1-m.historyIdx])
			}
			m.textarea.SetCursor(len(m.textarea.Value()))
			return m, nil
		}
	}
	var cmd tea.Cmd
	m.textarea, cmd = m.textarea.Update(msg)
	return m, cmd
}

// View renders the input bar with a top border.
func (m InputModel) View() string {
	return styles.InputStyle.Width(m.width).Render(m.textarea.View())
}

// SetWidth resizes the input bar. Subtracts 4 for border and padding.
func (m *InputModel) SetWidth(w int) {
	m.width = w
	m.textarea.SetWidth(w - 4)
}

// SetEnabled enables or disables input. Disabling blurs the textarea.
func (m *InputModel) SetEnabled(enabled bool) {
	m.enabled = enabled
	if enabled {
		m.textarea.Focus()
	} else {
		m.textarea.Blur()
	}
}

func (m InputModel) Enabled() bool { return m.enabled }
