package model

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/kubewise/kubewise/pkg/agent/stream"
	"github.com/kubewise/kubewise/pkg/agent/operation"
	"github.com/kubewise/kubewise/pkg/tui/events"
	"github.com/kubewise/kubewise/pkg/tui/styles"
)

type confirmMode int

const (
	confirmModeChoice confirmMode = iota // showing Y/N/E options
	confirmModeEdit                      // typing a correction
)

// ConfirmDoneMsg is sent when the modal has received an answer and closed.
type ConfirmDoneMsg struct{}

// ConfirmModel renders a modal overlay for operation step approval.
type ConfirmModel struct {
	event      events.ConfirmRequestEvent
	step       operation.OperationStep
	queryID    string
	totalSteps int
	mode       confirmMode
	editInput  textinput.Model
	width      int
	streamResp chan<- json.RawMessage
}

// NewConfirmModel creates a ConfirmModel from a ConfirmRequestEvent.
func NewConfirmModel(ev events.ConfirmRequestEvent) ConfirmModel {
	step, _ := ev.Step.(operation.OperationStep)
	ti := textinput.New()
	ti.Placeholder = "Describe your correction..."
	ti.CharLimit = 512
	ti.Width = 54 // fits inside ModalStyle width=60 with padding
	return ConfirmModel{
		event:      ev,
		step:       step,
		queryID:    ev.QueryID,
		totalSteps: ev.TotalSteps,
		mode:       confirmModeChoice,
		editInput:  ti,
		width:      60,
	}
}

// NewConfirmModelFromStream builds a confirm modal for stream.InteractionRequest (operation_step).
func NewConfirmModelFromStream(queryID string, totalSteps int, stepJSON []byte, respCh chan<- json.RawMessage) (ConfirmModel, error) {
	var step operation.OperationStep
	if err := json.Unmarshal(stepJSON, &step); err != nil {
		return ConfirmModel{}, err
	}
	ti := textinput.New()
	ti.Placeholder = "Describe your correction..."
	ti.CharLimit = 512
	ti.Width = 54
	return ConfirmModel{
		queryID:    queryID,
		totalSteps: totalSteps,
		step:       step,
		mode:       confirmModeChoice,
		editInput:  ti,
		width:      60,
		streamResp: respCh,
	}, nil
}

// respond sends the confirmation response (non-blocking) and returns a Cmd
// that emits ConfirmDoneMsg to signal the parent to close the modal.
func (m ConfirmModel) respond(confirmed bool, correction string) tea.Cmd {
	return func() tea.Msg {
		if m.streamResp != nil {
			raw, err := json.Marshal(stream.OperationConfirmResponse{
				Confirmed:  confirmed,
				Correction: correction,
			})
			if err == nil {
				select {
				case m.streamResp <- raw:
				default:
				}
			}
		} else {
			select {
			case m.event.RespCh <- operation.ConfirmResponse{Confirmed: confirmed, Correction: correction}:
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
	// Forward non-key messages to editInput when in edit mode
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
		total = m.event.TotalSteps
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
	if m.streamResp != nil && m.queryID != "" {
		return m.queryID
	}
	return m.event.QueryID
}
