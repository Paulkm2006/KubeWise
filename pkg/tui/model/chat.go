package model

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/kubewise/kubewise/pkg/tui/events"
	"github.com/kubewise/kubewise/pkg/tui/session"
	"github.com/kubewise/kubewise/pkg/tui/styles"
)

// toolLine tracks a single tool call within a progress card.
type toolLine struct {
	name    string
	step    int
	done    bool
	elapsed time.Duration
}

// progressCard tracks an in-flight agent execution.
type progressCard struct {
	queryID   string
	agentName string
	tools     []toolLine
	done      bool
	failed    bool
	errMsg    string
	duration  time.Duration
	inTokens  int
	outTokens int
}

// pendingMsg accumulates rendered output blocks for a query in progress.
type pendingMsg struct {
	blocks   []session.Block
	rendered []string // pre-rendered strings corresponding to each block
}

// chatEntry is a completed message (user or assistant) ready for display.
type chatEntry struct {
	role      string // "user" | "assistant" | "error"
	content   string // raw content for session persistence
	lines     []string
	blocks    []session.Block
	timestamp time.Time
	inTokens  int
	outTokens int
	durationS float64
}

// ChatModel manages the chat display, progress cards, and pending message assembly.
type ChatModel struct {
	messages []chatEntry
	pending  map[string]*pendingMsg
	cards    map[string]*progressCard
	renderer *Renderer
	width    int
	height   int
}

// NewChatModel creates an empty ChatModel sized to the given terminal dimensions.
func NewChatModel(width, height int) ChatModel {
	return ChatModel{
		messages: make([]chatEntry, 0),
		pending:  make(map[string]*pendingMsg),
		cards:    make(map[string]*progressCard),
		renderer: NewRenderer(width - 2),
		width:    width,
		height:   height,
	}
}

// AddUserMessage appends a user-authored message to the display list.
func (m *ChatModel) AddUserMessage(text string) {
	m.messages = append(m.messages, chatEntry{
		role:      "user",
		content:   text,
		lines:     []string{styles.UserBubble.Render("You: ") + text},
		timestamp: time.Now(),
	})
}

// SetSize updates the terminal dimensions and propagates the width to the renderer.
func (m *ChatModel) SetSize(width, height int) {
	m.width = width
	m.height = height
	m.renderer.SetWidth(width - 2)
}

// CompletedMessages returns session.Message structs for all completed assistant messages.
func (m *ChatModel) CompletedMessages() []session.Message {
	var out []session.Message
	for _, e := range m.messages {
		if e.role != "assistant" {
			continue
		}
		content := e.content
		if content == "" {
			content = strings.Join(e.lines, "\n")
		}
		out = append(out, session.Message{
			Role:      "assistant",
			Content:   content,
			Blocks:    e.blocks,
			Timestamp: e.timestamp,
			InTokens:  e.inTokens,
			OutTokens: e.outTokens,
			DurationS: e.durationS,
		})
	}
	return out
}

// AllMessages returns session.Message structs for all messages (user + assistant).
func (m *ChatModel) AllMessages() []session.Message {
	var out []session.Message
	for _, e := range m.messages {
		switch e.role {
		case "user":
			out = append(out, session.Message{
				Role:      "user",
				Content:   e.content,
				Timestamp: e.timestamp,
			})
		case "assistant":
			content := e.content
			if content == "" {
				content = strings.Join(e.lines, "\n")
			}
			out = append(out, session.Message{
				Role:      "assistant",
				Content:   content,
				Blocks:    e.blocks,
				Timestamp: e.timestamp,
				InTokens:  e.inTokens,
				OutTokens: e.outTokens,
				DurationS: e.durationS,
			})
		}
	}
	return out
}

// SetMessages replaces the display with previously saved session messages.
func (m *ChatModel) SetMessages(msgs []session.Message) {
	m.messages = make([]chatEntry, 0, len(msgs))
	m.pending = make(map[string]*pendingMsg)
	m.cards = make(map[string]*progressCard)
	for _, msg := range msgs {
		switch msg.Role {
		case "user":
			m.messages = append(m.messages, chatEntry{
				role:      "user",
				content:   msg.Content,
				lines:     []string{styles.UserBubble.Render("You: ") + msg.Content},
				timestamp: msg.Timestamp,
			})
		case "assistant":
			var lines []string
			if len(msg.Blocks) > 0 {
				for _, b := range msg.Blocks {
					lines = append(lines, m.renderBlock(b))
				}
			} else {
				lines = []string{m.renderer.RenderText(msg.Content)}
			}
			m.messages = append(m.messages, chatEntry{
				role:      "assistant",
				content:   msg.Content,
				lines:     lines,
				blocks:    msg.Blocks,
				timestamp: msg.Timestamp,
				inTokens:  msg.InTokens,
				outTokens: msg.OutTokens,
				durationS: msg.DurationS,
			})
		}
	}
}

// renderBlock renders a single session.Block to a styled string.
func (m *ChatModel) renderBlock(b session.Block) string {
	switch b.Type {
	case "table":
		var p session.TablePayload
		if err := json.Unmarshal(b.Payload, &p); err == nil {
			return m.renderer.RenderTable(p.Headers, p.Rows)
		}
	case "code":
		var p session.CodePayload
		if err := json.Unmarshal(b.Payload, &p); err == nil {
			return m.renderer.RenderCode(p.Language, p.Content)
		}
	case "kv":
		var p session.KVPayload
		if err := json.Unmarshal(b.Payload, &p); err == nil {
			pairs := make([]events.KVPair, len(p.Pairs))
			for i, kp := range p.Pairs {
				pairs[i] = events.KVPair{Key: kp.Key, Value: kp.Value}
			}
			return m.renderer.RenderKV(pairs)
		}
	case "list":
		var p session.ListPayload
		if err := json.Unmarshal(b.Payload, &p); err == nil {
			items := make([]events.ListItem, len(p.Items))
			for i, li := range p.Items {
				items[i] = events.ListItem{Status: li.Status, Text: li.Text}
			}
			return m.renderer.RenderList(items)
		}
	}
	return m.renderer.RenderText(string(b.Payload))
}

// Update handles TUIEvent messages dispatched from the event channel.
// Returns (ChatModel, tea.Cmd) — sub-model pattern, NOT (tea.Model, tea.Cmd).
func (m ChatModel) Update(msg tea.Msg) (ChatModel, tea.Cmd) {
	switch ev := msg.(type) {

	case events.AgentStartEvent:
		m.cards[ev.QueryID] = &progressCard{
			queryID:   ev.QueryID,
			agentName: ev.AgentName,
		}

	case events.AgentDoneEvent:
		if c, ok := m.cards[ev.QueryID]; ok {
			c.done = true
			c.duration = ev.Duration
			c.inTokens = ev.InTokens
			c.outTokens = ev.OutTokens
		}

	case events.ToolCallEvent:
		if c, ok := m.cards[ev.QueryID]; ok {
			c.tools = append(c.tools, toolLine{name: ev.ToolName, step: ev.Step})
		}

	case events.ToolDoneEvent:
		if c, ok := m.cards[ev.QueryID]; ok {
			for i := range c.tools {
				if c.tools[i].name == ev.ToolName && c.tools[i].step == ev.Step && !c.tools[i].done {
					c.tools[i].done = true
					c.tools[i].elapsed = ev.Elapsed
					break
				}
			}
		}

	case events.RenderTextEvent:
		m.addPending(ev.QueryID, session.Block{Type: "text"}, m.renderer.RenderText(ev.Text))

	case events.RenderTableEvent:
		rendered := m.renderer.RenderTable(ev.Headers, ev.Rows)
		payload, _ := json.Marshal(session.TablePayload{Headers: ev.Headers, Rows: ev.Rows})
		m.addPending(ev.QueryID, session.Block{Type: "table", Payload: payload}, rendered)

	case events.RenderCodeEvent:
		rendered := m.renderer.RenderCode(ev.Language, ev.Content)
		payload, _ := json.Marshal(session.CodePayload{Language: ev.Language, Content: ev.Content})
		m.addPending(ev.QueryID, session.Block{Type: "code", Payload: payload}, rendered)

	case events.RenderKVEvent:
		rendered := m.renderer.RenderKV(ev.Pairs)
		kvPairs := make([]session.KVPair, len(ev.Pairs))
		for i, p := range ev.Pairs {
			kvPairs[i] = session.KVPair{Key: p.Key, Value: p.Value}
		}
		payload, _ := json.Marshal(session.KVPayload{Pairs: kvPairs})
		m.addPending(ev.QueryID, session.Block{Type: "kv", Payload: payload}, rendered)

	case events.RenderListEvent:
		rendered := m.renderer.RenderList(ev.Items)
		listItems := make([]session.ListItem, len(ev.Items))
		for i, it := range ev.Items {
			listItems[i] = session.ListItem{Status: it.Status, Text: it.Text}
		}
		payload, _ := json.Marshal(session.ListPayload{Items: listItems})
		m.addPending(ev.QueryID, session.Block{Type: "list", Payload: payload}, rendered)

	case events.StreamDoneEvent:
		p := m.pending[ev.QueryID]
		var lines []string
		var blocks []session.Block
		if p != nil {
			lines = p.rendered
			blocks = p.blocks
		}
		if len(lines) == 0 && ev.Result != "" {
			lines = []string{m.renderer.RenderText(ev.Result)}
		}
		var card *progressCard
		if c, ok := m.cards[ev.QueryID]; ok {
			card = c
		}
		entry := chatEntry{
			role:      "assistant",
			content:   ev.Result,
			lines:     lines,
			blocks:    blocks,
			timestamp: time.Now(),
		}
		if card != nil {
			entry.inTokens = card.inTokens
			entry.outTokens = card.outTokens
			entry.durationS = card.duration.Seconds()
		}
		m.messages = append(m.messages, entry)
		delete(m.pending, ev.QueryID)
		delete(m.cards, ev.QueryID)

	case events.StreamErrEvent:
		errMsg := fmt.Sprintf("错误：%v", ev.Err)
		m.messages = append(m.messages, chatEntry{
			role:      "error",
			lines:     []string{styles.CardFailed.Render(errMsg)},
			timestamp: time.Now(),
		})
		delete(m.pending, ev.QueryID)
		delete(m.cards, ev.QueryID)
	}

	return m, nil
}

// addPending appends a block and its pre-rendered string to the pending message for queryID.
func (m *ChatModel) addPending(queryID string, block session.Block, rendered string) {
	if m.pending[queryID] == nil {
		m.pending[queryID] = &pendingMsg{}
	}
	m.pending[queryID].blocks = append(m.pending[queryID].blocks, block)
	m.pending[queryID].rendered = append(m.pending[queryID].rendered, rendered)
}

// View renders the entire chat area: completed messages followed by any active progress cards.
func (m ChatModel) View() string {
	var sb strings.Builder

	for _, e := range m.messages {
		ts := styles.TimestampStyle.Render(e.timestamp.Format("15:04"))
		switch e.role {
		case "user":
			sb.WriteString(ts + "\n")
		case "assistant":
			sb.WriteString(ts + "\n")
		case "error":
		}
		for _, line := range e.lines {
			sb.WriteString(line + "\n")
		}
		sb.WriteString("\n")
	}

	for _, card := range m.cards {
		sb.WriteString(m.renderCard(card))
		sb.WriteString("\n")
	}

	return sb.String()
}

// renderCard renders a single progress card to a styled string.
func (m ChatModel) renderCard(c *progressCard) string {
	if c.done {
		summary := fmt.Sprintf("✓ %s  %.1fs | ↑ %d ↓ %d tok",
			c.agentName, c.duration.Seconds(), c.inTokens, c.outTokens)
		return styles.CardDone.Render(summary)
	}
	if c.failed {
		summary := fmt.Sprintf("✗ %s: %s", c.agentName, c.errMsg)
		return styles.CardFailed.Render(summary)
	}

	icon := "⚙"
	lines := []string{styles.CardRunning.Render(fmt.Sprintf("%s %s", icon, c.agentName))}

	for _, t := range c.tools {
		if t.done {
			line := fmt.Sprintf("  ✓ %-30s %s", t.name, t.elapsed.Round(time.Millisecond).String())
			lines = append(lines, styles.CardDone.Render(line))
		} else {
			line := fmt.Sprintf("  ⟳ %s", t.name)
			lines = append(lines, styles.CardRunning.Render(line))
		}
	}

	return styles.CardStyle.Width(m.width - 4).Render(strings.Join(lines, "\n"))
}
