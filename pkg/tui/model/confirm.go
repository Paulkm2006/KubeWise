package model

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/kubewise/kubewise/pkg/agent/operation"
	"github.com/kubewise/kubewise/pkg/stream"
	"github.com/kubewise/kubewise/pkg/tui/styles"
)

type confirmMode int

const (
	confirmModeChoice confirmMode = iota // showing Y/N/E options
	confirmModeEdit                      // typing a correction
)

// ConfirmDoneMsg is sent when the modal has received an answer and closed.
type ConfirmDoneMsg struct{}

// ConfirmModel renders a modal overlay for operation step approval (operation_step interaction).
type ConfirmModel struct {
	step       operation.OperationStep
	queryID    string
	totalSteps int
	mode       confirmMode
	editInput  textinput.Model
	width      int
	respCh     chan<- json.RawMessage
}

// NewConfirmModel builds a confirm modal from stream.InteractionRequest (kind operation_step).
func NewConfirmModel(queryID string, totalSteps int, stepJSON []byte, respCh chan<- json.RawMessage) (ConfirmModel, error) {
	var step operation.OperationStep
	if err := json.Unmarshal(stepJSON, &step); err != nil {
		return ConfirmModel{}, err
	}
	ti := textinput.New()
	ti.Placeholder = "Describe your correction..."
	ti.CharLimit = 512
	ti.Width = 54 // fits inside ModalStyle width=60 with padding
	return ConfirmModel{
		queryID:    queryID,
		totalSteps: totalSteps,
		step:       step,
		mode:       confirmModeChoice,
		editInput:  ti,
		width:      60,
		respCh:     respCh,
	}, nil
}

func (m ConfirmModel) respond(confirmed bool, correction string) tea.Cmd {
	return func() tea.Msg {
		raw, err := json.Marshal(stream.OperationConfirmResponse{
			Confirmed:  confirmed,
			Correction: correction,
		})
		if err == nil {
			select {
			case m.respCh <- raw:
			default:
			}
		}
		return ConfirmDoneMsg{}
	}
}

// Update handles key messages and delegates to the text input in edit mode.
func (m ConfirmModel) Update(msg tea.Msg) (ConfirmModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch m.mode {
		case confirmModeChoice:
			switch strings.ToLower(msg.String()) {
			case "y":
				return m, m.respond(true, "")
			case "n", "esc":
				return m, m.respond(false, "")
			case "e":
				m.mode = confirmModeEdit
				m.editInput.Focus()
				return m, textinput.Blink
			}
		case confirmModeEdit:
			switch msg.Type {
			case tea.KeyEnter:
				correction := strings.TrimSpace(m.editInput.Value())
				m.editInput.Reset()
				return m, m.respond(false, correction)
			case tea.KeyEsc:
				m.mode = confirmModeChoice
				m.editInput.Blur()
				m.editInput.Reset()
				return m, nil
			}
			var cmd tea.Cmd
			m.editInput, cmd = m.editInput.Update(msg)
			return m, cmd
		}
	}
	if m.mode == confirmModeEdit {
		var cmd tea.Cmd
		m.editInput, cmd = m.editInput.Update(msg)
		return m, cmd
	}
	return m, nil
}

// View renders the confirm modal overlay.
func (m ConfirmModel) View() string {
	total := m.totalSteps
	if total == 0 {
		total = 1
	}
	title := styles.ModalTitleStyle.Render(
		fmt.Sprintf("步骤 %d/%d：%s", m.step.StepIndex, total, m.step.OperationType),
	)
	resource := fmt.Sprintf("  资源：%s/%s", m.step.ResourceKind, m.step.ResourceName)
	if m.step.Namespace != "" {
		resource += fmt.Sprintf(" (ns: %s)", m.step.Namespace)
	}
	desc := fmt.Sprintf("  %s", m.step.Description)

	var hint string
	if m.mode == confirmModeChoice {
		hint = styles.ModalHintStyle.Render("[Y] 执行  [N] 跳过  [E] 修改")
	} else {
		hint = m.editInput.View() + "\n" + styles.ModalHintStyle.Render("Enter 确认  Esc 返回")
	}

	content := strings.Join([]string{title, resource, desc, "", hint}, "\n")
	return styles.ModalStyle.Render(content)
}

// QueryID returns the query ID this confirm modal is responding to.
func (m ConfirmModel) QueryID() string {
	return m.queryID
}
