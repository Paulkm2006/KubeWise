package model

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/kubewise/kubewise/pkg/session"
	"github.com/kubewise/kubewise/pkg/stream"
	"github.com/kubewise/kubewise/pkg/tui/styles"
)

// toolLine tracks a single tool call within a progress card.
type toolLine struct {
	name    string
	step    int
	done    bool
	failed  bool
	elapsed time.Duration
}

// phaseGroup tracks a phase with nested tools and reasoning text.
type phaseGroup struct {
	label             string
	start             time.Time
	done              bool
	elapsed           time.Duration
	tools             []toolLine
	reasoningText     strings.Builder
	reasoningExpanded bool
}

// progressCard tracks an in-flight agent execution.
type progressCard struct {
	queryID     string
	agentName   string
	phases      []phaseGroup
	done        bool
	failed      bool
	errMsg      string
	duration    time.Duration
	inTokens    int
	outTokens   int
	finalReport string // from AgentDone.Result
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
	messages     []chatEntry
	cards        map[string]*progressCard
	renderer     *Renderer
	width        int
	height       int
	spinner      spinner.Model
	phase        string
	phaseStart   time.Time
	spinning     bool
	scrollOffset int // 0 = pinned to bottom; >0 = number of lines scrolled up
}

// NewChatModel creates an empty ChatModel sized to the given terminal dimensions.
func NewChatModel(width, height int) ChatModel {
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = styles.CardRunning

	return ChatModel{
		messages: make([]chatEntry, 0),
		cards:    make(map[string]*progressCard),
		renderer: NewRenderer(width - 2),
		width:    width,
		height:   height,
		spinner:  sp,
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
			pairs := make([]session.KVPair, len(p.Pairs))
			for i, kp := range p.Pairs {
				pairs[i] = session.KVPair{Key: kp.Key, Value: kp.Value}
			}
			return m.renderer.RenderKV(pairs)
		}
	case "list":
		var p session.ListPayload
		if err := json.Unmarshal(b.Payload, &p); err == nil {
			items := make([]session.ListItem, len(p.Items))
			for i, li := range p.Items {
				items[i] = session.ListItem{Status: li.Status, Text: li.Text}
			}
			return m.renderer.RenderList(items)
		}
	case "detail":
		var p session.DetailPayload
		if err := json.Unmarshal(b.Payload, &p); err == nil {
			d := session.DetailPayload{
				Kind: p.Kind, Name: p.Name, Namespace: p.Namespace,
				Status: p.Status, RecentLogs: p.RecentLogs, Labels: p.Labels,
			}
			for _, c := range p.Containers {
				d.Containers = append(d.Containers, session.ContainerInfo{
					Name: c.Name, Image: c.Image, Ready: c.Ready,
					RestartCount: c.RestartCount, State: c.State, Resources: c.Resources,
				})
			}
			for _, c := range p.Conditions {
				d.Conditions = append(d.Conditions, session.ConditionInfo{
					Type: c.Type, Status: c.Status, Reason: c.Reason, Message: c.Message,
				})
			}
			for _, e := range p.Events {
				d.Events = append(d.Events, session.EventInfo{
					Type: e.Type, Reason: e.Reason, Message: e.Message, Timestamp: e.Timestamp,
				})
			}
			return m.renderer.RenderDetail(d)
		}
	}
	return m.renderer.RenderText(string(b.Payload))
}

// ScrollUp scrolls the chat view up by n lines.
func (m *ChatModel) ScrollUp(n int) {
	m.scrollOffset += n
	total := m.totalLines()
	maxScroll := total - m.height
	if maxScroll < 0 {
		maxScroll = 0
	}
	if m.scrollOffset > maxScroll {
		m.scrollOffset = maxScroll
	}
}

// ScrollDown scrolls the chat view down by n lines.
func (m *ChatModel) ScrollDown(n int) {
	m.scrollOffset -= n
	if m.scrollOffset < 0 {
		m.scrollOffset = 0
	}
}

// ScrollToBottom resets the scroll offset to pin the view to the bottom.
func (m *ChatModel) ScrollToBottom() {
	m.scrollOffset = 0
}

// totalLines counts the total number of rendered lines across all messages and cards.
func (m ChatModel) totalLines() int {
	var count int
	for _, e := range m.messages {
		count += len(e.lines) + 2 // lines + timestamp + blank separator
	}
	for _, card := range m.cards {
		rendered := m.renderCard(card)
		count += strings.Count(rendered, "\n") + 2 // card lines + trailing newline
	}
	return count
}

// Update handles TUIEvent messages dispatched from the event channel.
// Returns (ChatModel, tea.Cmd) — sub-model pattern, NOT (tea.Model, tea.Cmd).
func (m ChatModel) Update(msg tea.Msg) (ChatModel, tea.Cmd) {
	switch ev := msg.(type) {

	case stream.AgentStart:
		now := time.Now()
		m.cards[ev.QueryID] = &progressCard{
			queryID:   ev.QueryID,
			agentName: ev.AgentName,
			phases:    []phaseGroup{{label: ev.AgentName, start: now}},
		}
		m.phase = ev.AgentName
		m.phaseStart = now
		m.spinning = true
		return m, m.spinner.Tick

	case stream.AgentDone:
		if c, ok := m.cards[ev.QueryID]; ok {
			c.done = true
			c.duration = ev.Duration
			c.inTokens = ev.InTokens
			c.outTokens = ev.OutTokens
			c.finalReport = ev.Result
			if len(c.phases) > 0 {
				last := &c.phases[len(c.phases)-1]
				if !last.done {
					last.done = true
					last.elapsed = time.Since(last.start)
				}
			}
		}
		m.spinning = false
		m.phase = ""
		m.scrollOffset = 0

	// LLMTextDelta writes to latest phase's reasoningText.
	case stream.LLMTextDelta:
		if c, ok := m.cards[ev.QueryID]; ok {
			if len(c.phases) > 0 {
				last := &c.phases[len(c.phases)-1]
				last.reasoningText.WriteString(ev.Delta)
			}
		}

	case stream.ToolCall:
		if c, ok := m.cards[ev.QueryID]; ok {
			if len(c.phases) > 0 {
				last := &c.phases[len(c.phases)-1]
				last.tools = append(last.tools, toolLine{name: ev.ToolName, step: ev.Step})
			}
		}

	case stream.ToolDone:
		if c, ok := m.cards[ev.QueryID]; ok {
			if len(c.phases) > 0 {
				for i := range c.phases[len(c.phases)-1].tools {
					t := &c.phases[len(c.phases)-1].tools[i]
					if t.name == ev.ToolName && t.step == ev.Step && !t.done {
						t.done = true
						t.elapsed = ev.Elapsed
						break
					}
				}
			}
		}

	case stream.ToolFail:
		if c, ok := m.cards[ev.QueryID]; ok {
			if len(c.phases) > 0 {
				for i := range c.phases[len(c.phases)-1].tools {
					t := &c.phases[len(c.phases)-1].tools[i]
					if t.name == ev.ToolName && t.step == ev.Step && !t.done {
						t.done = true
						t.failed = true
						t.elapsed = ev.Elapsed
						break
					}
				}
			}
		}

	case stream.StreamDone:
		if c, ok := m.cards[ev.QueryID]; ok {
			cardView := m.renderCompletedCard(c)
			m.messages = append(m.messages, chatEntry{
				role:      "assistant",
				content:   c.finalReport,
				lines:     []string{cardView},
				timestamp: time.Now(),
				inTokens:  c.inTokens,
				outTokens: c.outTokens,
				durationS: c.duration.Seconds(),
			})
			delete(m.cards, ev.QueryID)
		}
		m.spinning = false
		m.phase = ""
		m.scrollOffset = 0

	case stream.StreamErr:
		errMsg := fmt.Sprintf("错误：%v", ev.Err)
		m.messages = append(m.messages, chatEntry{
			role:      "error",
			lines:     []string{styles.CardFailed.Render(errMsg)},
			timestamp: time.Now(),
		})
		m.spinning = false
		m.phase = ""
		m.scrollOffset = 0
		delete(m.cards, ev.QueryID)

	case stream.Phase:
		if c, ok := m.cards[ev.QueryID]; ok {
			now := time.Now()
			if len(c.phases) > 0 {
				last := &c.phases[len(c.phases)-1]
				if !last.done {
					last.done = true
					last.elapsed = now.Sub(last.start)
				}
			}
			c.phases = append(c.phases, phaseGroup{label: ev.Phase, start: now})
			m.phase = ev.Phase
			m.phaseStart = now
		}

	case stream.Supervisor:
		if c, ok := m.cards[ev.QueryID]; ok {
			now := time.Now()
			if len(c.phases) > 0 {
				last := &c.phases[len(c.phases)-1]
				if !last.done {
					last.done = true
					last.elapsed = now.Sub(last.start)
				}
			}
			label := fmt.Sprintf("supervisor: %s — %s", ev.Decision, ev.Detail)
			c.phases = append(c.phases, phaseGroup{label: label, start: now})
			m.phase = label
			m.phaseStart = now
		}

	case spinner.TickMsg:
		if m.spinning {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			return m, cmd
		}
	}

	return m, nil
}

// detailToPayload converts an session.DetailPayload to a session.DetailPayload.
func detailToPayload(d session.DetailPayload) session.DetailPayload {
	dp := session.DetailPayload{
		Kind:       d.Kind,
		Name:       d.Name,
		Namespace:  d.Namespace,
		Status:     d.Status,
		RecentLogs: d.RecentLogs,
		Labels:     d.Labels,
	}
	for _, c := range d.Containers {
		dp.Containers = append(dp.Containers, session.ContainerInfo{
			Name: c.Name, Image: c.Image, Ready: c.Ready,
			RestartCount: c.RestartCount, State: c.State, Resources: c.Resources,
		})
	}
	for _, c := range d.Conditions {
		dp.Conditions = append(dp.Conditions, session.ConditionInfo{
			Type: c.Type, Status: c.Status, Reason: c.Reason, Message: c.Message,
		})
	}
	for _, e := range d.Events {
		dp.Events = append(dp.Events, session.EventInfo{
			Type: e.Type, Reason: e.Reason, Message: e.Message, Timestamp: e.Timestamp,
		})
	}
	return dp
}

// View renders the entire chat area: completed messages followed by any active progress cards.
// Output is clipped to m.height lines so the input prompt below stays visible.
// When scrollOffset > 0 the view shifts up from the bottom, allowing the user to scroll
// through history. A subtle scroll indicator is shown when the view is not at the bottom.
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

	result := sb.String()
	if m.height <= 0 {
		return result
	}

	lines := strings.Split(result, "\n")
	total := len(lines)
	visible := m.height

	if total <= visible {
		return result
	}

	// Bottom of the visible window (0-indexed from end).
	// scrollOffset=0 means pinned to bottom (show last `visible` lines).
	bottomFromEnd := m.scrollOffset
	topFromEnd := bottomFromEnd + visible

	if topFromEnd > total {
		topFromEnd = total
	}
	if bottomFromEnd > total-visible {
		bottomFromEnd = total - visible
	}

	start := total - topFromEnd
	end := total - bottomFromEnd
	view := strings.Join(lines[start:end], "\n")

	// Show scroll indicator when not at bottom.
	if m.scrollOffset > 0 {
		indicator := styles.ScrollIndicatorStyle.Render(fmt.Sprintf(" ↑ %d lines above (↓/PgDn to scroll down) ", m.scrollOffset))
		view = indicator + "\n" + view
	}

	return view
}

// renderCard renders a single in-progress progress card to a styled string.
// Has 3 zones: header (failed state), phase list with nested tools and reasoning.
func (m ChatModel) renderCard(c *progressCard) string {
	// Zone 1: Failed state — short summary, no phase list
	if c.failed {
		summary := fmt.Sprintf("✗ %s: %s", c.agentName, c.errMsg)
		return styles.CardFailed.Render(summary)
	}

	var lines []string

	// Zone 2: Phase list with nested tools and reasoning
	for i, p := range c.phases {
		isLast := i == len(c.phases)-1
		if isLast && !p.done {
			// Active phase — show spinner
			elapsed := time.Since(p.start).Round(time.Second)
			line := fmt.Sprintf("%s %s... %s", m.spinner.View(), p.label, elapsed)
			lines = append(lines, styles.StepActiveStyle.Render(line))
		} else {
			// Completed phase
			elapsed := p.elapsed.Round(time.Millisecond)
			line := fmt.Sprintf("  ✓ %s %s", p.label, elapsed)
			lines = append(lines, styles.StepDoneStyle.Render(line))
		}

		// Reasoning text (if any)
		if p.reasoningText.Len() > 0 {
			if p.reasoningExpanded {
				full := p.reasoningText.String()
				divider := strings.Repeat("─", m.width-8)
				lines = append(lines, styles.ReasoningExpanded.Render("  ▼ 推理过程\n  "+divider+"\n"+indentText(full, "  ")+"\n  "+divider))
			} else if !p.done {
				// Running, collapsed: show last 3 lines
				full := p.reasoningText.String()
				tail := lastNLines(full, 3)
				divider := strings.Repeat("─", m.width-8)
				lines = append(lines, styles.ReasoningPreview.Render("  "+divider))
				for _, line := range tail {
					lines = append(lines, styles.ReasoningPreview.Render("  "+line))
				}
			} else {
				// Done, collapsed: just show hint
				lines = append(lines, styles.ReasoningCollapsed.Render("  ▶ 推理过程"))
			}
		}

		// Tools nested under this phase
		for _, t := range p.tools {
			indent := "    "
			switch {
			case t.done && t.failed:
				line := fmt.Sprintf("%s✗ %-30s %s", indent, t.name, t.elapsed.Round(time.Millisecond).String())
				lines = append(lines, styles.StepFailedStyle.Render(line))
			case t.done:
				line := fmt.Sprintf("%s✓ %-30s %s", indent, t.name, t.elapsed.Round(time.Millisecond).String())
				lines = append(lines, styles.StepDoneStyle.Render(line))
			default:
				line := fmt.Sprintf("%s⟳ %s", indent, t.name)
				lines = append(lines, styles.StepActiveStyle.Render(line))
			}
		}
	}

	return styles.CardStyle.Width(m.width - 4).Render(strings.Join(lines, "\n"))
}

// renderCompletedCard renders a summary for a completed card.
func (m ChatModel) renderCompletedCard(c *progressCard) string {
	var sb strings.Builder
	summary := fmt.Sprintf("✓ %s  %.1fs | ↑ %d ↓ %d tok",
		c.agentName, c.duration.Seconds(), c.inTokens, c.outTokens)
	sb.WriteString(styles.CardDone.Render(summary) + "\n")
	if c.finalReport != "" {
		sb.WriteString(m.renderer.RenderText(c.finalReport))
	}
	return sb.String()
}

// TogglePhaseReasoning toggles expanded/collapsed state for the latest active phase's reasoning text.
func (m *ChatModel) TogglePhaseReasoning() bool {
	var latestPhase *phaseGroup
	var latestTime time.Time
	for _, card := range m.cards {
		if card.done {
			continue
		}
		for i := range card.phases {
			p := &card.phases[i]
			if !p.done && p.reasoningText.Len() > 0 {
				if p.start.After(latestTime) {
					latestPhase = p
					latestTime = p.start
				}
			}
		}
	}
	if latestPhase == nil {
		return false
	}
	latestPhase.reasoningExpanded = !latestPhase.reasoningExpanded
	return true
}

// Phase returns the current phase label for the thinking indicator.
func (m ChatModel) Phase() string {
	return m.phase
}

// IsSpinning reports whether the spinner is currently active.
func (m ChatModel) IsSpinning() bool {
	return m.spinning
}

// lastNLines returns the last n lines of s.
func lastNLines(s string, n int) []string {
	all := strings.Split(s, "\n")
	if len(all) <= n {
		return all
	}
	return all[len(all)-n:]
}

// indentText prepends a prefix to every line of s.
func indentText(s, prefix string) string {
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		lines[i] = prefix + line
	}
	return strings.Join(lines, "\n")
}