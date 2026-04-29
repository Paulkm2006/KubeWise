package model

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/kubewise/kubewise/pkg/tui/styles"
)

// SubmitMsg is sent when the user presses Enter in the input box.
type SubmitMsg struct{ Value string }

type InputModel struct {
	textarea textarea.Model
	enabled  bool
	width    int
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

// Update handles input. Enter submits the current value (if non-empty).
// All other keys are forwarded to the underlying textarea when enabled.
func (m InputModel) Update(msg tea.Msg) (InputModel, tea.Cmd) {
	if !m.enabled {
		return m, nil
	}
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.Type == tea.KeyEnter {
			val := strings.TrimSpace(m.textarea.Value())
			if val == "" {
				return m, nil
			}
			m.textarea.Reset()
			return m, func() tea.Msg { return SubmitMsg{Value: val} }
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
