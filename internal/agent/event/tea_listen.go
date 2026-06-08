package event

import (
	tea "github.com/charmbracelet/bubbletea"
)

// TeaMsg wraps Stream Event inside bubbletea.
type TeaMsg struct {
	Event Event
}

// Listen waits for one event from ch (blocking).
func Listen(ch <-chan Event) tea.Cmd {
	return func() tea.Msg {
		ev, ok := <-ch
		if !ok {
			return TeaMsg{Event: StreamErr{Err: ErrChannelClosed}}
		}
		return TeaMsg{Event: ev}
	}
}

// ErrChannelClosed is returned when the stream channel closes before stream_done.
var ErrChannelClosed = errClosed{}

type errClosed struct{}

func (errClosed) Error() string { return "stream closed" }
