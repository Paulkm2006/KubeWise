# TUI Multi-Round Chat Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add an interactive TUI mode (`kubewise tui`) supporting multi-round conversation, session persistence, Agent work visualization, and operation confirmation — without changing the existing `chat` subcommand.

**Architecture:** A bubbletea-based App model composes four sub-models (sidebar, chat, input, confirm) and listens for `TUIEvent`s pushed from agents via a shared `chan events.TUIEvent`. Agents gain a `WithEventCh` functional option; the router gains `HandleQueryStream` that spawns sub-agents with the shared channel and runs a bridge goroutine for operation confirmations. Sessions are saved as JSON files under `~/.kubewise/sessions/`.

**Tech Stack:** `github.com/charmbracelet/bubbletea`, `github.com/charmbracelet/lipgloss`, `github.com/charmbracelet/bubbles` (textarea), `github.com/alecthomas/chroma/v2` (syntax highlight)

---

## File Map

**New files:**
- `pkg/tui/events/events.go` — TUIEvent interface + all event types
- `pkg/tui/session/session.go` — Session / Message / Block data types
- `pkg/tui/session/store.go` — JSON persistence to `~/.kubewise/sessions/`
- `pkg/tui/session/store_test.go`
- `pkg/tui/styles/styles.go` — lipgloss colour + border constants
- `pkg/tui/model/renderer.go` — table / code / kv / list → styled string
- `pkg/tui/model/renderer_test.go`
- `pkg/tui/model/input.go` — bubbletea input box sub-model
- `pkg/tui/model/confirm.go` — modal confirmation overlay
- `pkg/tui/model/sidebar.go` — session list panel
- `pkg/tui/model/chat.go` — chat area: message history + progress cards
- `pkg/tui/app.go` — root bubbletea App model

**Modified files:**
- `pkg/llm/types.go` — add `Usage` struct + `Usage *Usage` on `Message`
- `pkg/llm/client.go` — populate `Usage` from openai response
- `pkg/agent/query/agent.go` — add `Option` type + `WithEventCh` + emit events
- `pkg/agent/troubleshooting/agent.go` — same pattern
- `pkg/agent/security/agent.go` — same pattern
- `pkg/agent/operation/agent.go` — add `WithEventCh` + emit events
- `pkg/agent/router/agent.go` — add `HandleQueryStream` + result-to-render logic
- `cmd/main.go` — add `tui` subcommand

---

## Task 1: Add TUI dependencies

**Files:**
- Modify: `go.mod`, `go.sum`

- [ ] **Step 1: Install packages**

```bash
go get github.com/charmbracelet/bubbletea@latest
go get github.com/charmbracelet/lipgloss@latest
go get github.com/charmbracelet/bubbles@latest
go get github.com/alecthomas/chroma/v2@latest
go mod tidy
```

Expected: all packages added to `go.mod` without error.

- [ ] **Step 2: Verify build still passes**

```bash
make build
```

Expected: binary compiles with exit code 0.

- [ ] **Step 3: Commit**

```bash
git add go.mod go.sum
git commit -m "chore(deps): add bubbletea, lipgloss, bubbles, chroma for TUI"
```

---

## Task 2: Add llm.Usage

**Files:**
- Modify: `pkg/llm/types.go`
- Modify: `pkg/llm/client.go`

- [ ] **Step 1: Add Usage to types.go**

In `pkg/llm/types.go`, add after the `Config` type:

```go
// Usage holds token counts returned by the LLM provider.
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}
```

Also add `Usage *Usage` field to `Message`:

```go
type Message struct {
	Role       string     `json:"role"`
	Content    string     `json:"content,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
	Usage      *Usage     `json:"usage,omitempty"`
}
```

- [ ] **Step 2: Populate usage in client.go**

In `pkg/llm/client.go`, after the line `result := &Message{...}` and before the JSON tool-call parsing block, add:

```go
	if resp.Usage.TotalTokens > 0 {
		result.Usage = &Usage{
			PromptTokens:     int(resp.Usage.PromptTokens),
			CompletionTokens: int(resp.Usage.CompletionTokens),
			TotalTokens:      int(resp.Usage.TotalTokens),
		}
	}
```

- [ ] **Step 3: Build to verify**

```bash
make build
```

Expected: clean build.

- [ ] **Step 4: Commit**

```bash
git add pkg/llm/types.go pkg/llm/client.go
git commit -m "feat(llm): expose token usage from chat completion response"
```

---

## Task 3: Create events package

**Files:**
- Create: `pkg/tui/events/events.go`

- [ ] **Step 1: Write the events file**

```go
package events

import "time"

// TUIEvent is the sealed interface for all events flowing from agents to the TUI.
type TUIEvent interface{ isTUIEvent() }

// AgentStartEvent fires when an agent begins processing a query.
type AgentStartEvent struct {
	QueryID   string
	AgentName string
}

func (AgentStartEvent) isTUIEvent() {}

// AgentDoneEvent fires when an agent finishes.
type AgentDoneEvent struct {
	QueryID   string
	Duration  time.Duration
	InTokens  int
	OutTokens int
}

func (AgentDoneEvent) isTUIEvent() {}

// ToolCallEvent fires immediately before a tool is invoked.
type ToolCallEvent struct {
	QueryID  string
	ToolName string
	Step     int
}

func (ToolCallEvent) isTUIEvent() {}

// ToolDoneEvent fires after a tool returns.
type ToolDoneEvent struct {
	QueryID  string
	ToolName string
	Step     int
	Elapsed  time.Duration
}

func (ToolDoneEvent) isTUIEvent() {}

// RenderTextEvent carries a plain-text reply.
type RenderTextEvent struct {
	QueryID string
	Text    string
}

func (RenderTextEvent) isTUIEvent() {}

// RenderTableEvent carries a pipe-delimited table reply.
type RenderTableEvent struct {
	QueryID string
	Headers []string
	Rows    [][]string
}

func (RenderTableEvent) isTUIEvent() {}

// RenderCodeEvent carries a fenced code block reply.
type RenderCodeEvent struct {
	QueryID  string
	Language string
	Content  string
}

func (RenderCodeEvent) isTUIEvent() {}

// KVPair is a single key-value entry.
type KVPair struct {
	Key   string
	Value string
}

// RenderKVEvent carries a key-value list reply.
type RenderKVEvent struct {
	QueryID string
	Pairs   []KVPair
}

func (RenderKVEvent) isTUIEvent() {}

// ListItem is a single status-bearing line.
type ListItem struct {
	Status string // "ok" | "warn" | "error" | "info"
	Text   string
}

// RenderListEvent carries a status-list reply.
type RenderListEvent struct {
	QueryID string
	Items   []ListItem
}

func (RenderListEvent) isTUIEvent() {}

// ConfirmRequestEvent is sent when an operation step needs user approval.
// Step is operation.OperationStep (typed as any to avoid import cycle).
// RespCh must receive one operation.ConfirmResponse to unblock the agent.
type ConfirmRequestEvent struct {
	QueryID    string
	Step       any      // cast to operation.OperationStep in app.go
	TotalSteps int
	RespCh     chan<- any // send operation.ConfirmResponse here
}

func (ConfirmRequestEvent) isTUIEvent() {}

// StreamDoneEvent carries the final result string after a full query completes.
type StreamDoneEvent struct {
	QueryID string
	Result  string
}

func (StreamDoneEvent) isTUIEvent() {}

// StreamErrEvent carries an unrecoverable error.
type StreamErrEvent struct {
	QueryID string
	Err     error
}

func (StreamErrEvent) isTUIEvent() {}
```

- [ ] **Step 2: Build**

```bash
make build
```

Expected: clean build (package is declaration-only, no logic).

- [ ] **Step 3: Commit**

```bash
git add pkg/tui/events/events.go
git commit -m "feat(tui/events): define TUIEvent interface and all event types"
```

---

## Task 4: Create session types

**Files:**
- Create: `pkg/tui/session/session.go`

- [ ] **Step 1: Write session.go**

```go
package session

import (
	"encoding/json"
	"fmt"
	"time"
)

// Session represents one conversation thread.
type Session struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Messages  []Message `json:"messages"`

	InterruptedQuery string `json:"interrupted_query,omitempty"`
}

// Message is a single turn in the conversation.
type Message struct {
	Role      string    `json:"role"` // "user" | "assistant"
	Content   string    `json:"content"`
	Blocks    []Block   `json:"blocks,omitempty"`
	Timestamp time.Time `json:"timestamp"`
	InTokens  int       `json:"in_tokens,omitempty"`
	OutTokens int       `json:"out_tokens,omitempty"`
	DurationS float64   `json:"duration_s,omitempty"`
}

// Block is a typed payload inside an assistant message.
type Block struct {
	Type    string          `json:"type"` // "table"|"code"|"kv"|"list"|"text"
	Payload json.RawMessage `json:"payload"`
}

// TablePayload is the payload for a "table" block.
type TablePayload struct {
	Headers []string   `json:"headers"`
	Rows    [][]string `json:"rows"`
}

// CodePayload is the payload for a "code" block.
type CodePayload struct {
	Language string `json:"language"`
	Content  string `json:"content"`
}

// KVPayload is the payload for a "kv" block.
type KVPayload struct {
	Pairs []KVPair `json:"pairs"`
}

// KVPair is a single key-value pair.
type KVPair struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// ListPayload is the payload for a "list" block.
type ListPayload struct {
	Items []ListItem `json:"items"`
}

// ListItem is one status-bearing row in a list block.
type ListItem struct {
	Status string `json:"status"` // "ok"|"warn"|"error"|"info"
	Text   string `json:"text"`
}

// New creates a new session with a generated ID.
func New() *Session {
	now := time.Now()
	id := fmt.Sprintf("%x", now.UnixNano())[:8]
	return &Session{
		ID:        id,
		CreatedAt: now,
		UpdatedAt: now,
	}
}

// TitleFromFirstMessage derives a display title (max 20 chars) from the first user message.
func TitleFromFirstMessage(content string) string {
	runes := []rune(content)
	if len(runes) <= 20 {
		return content
	}
	return string(runes[:20]) + "…"
}
```

- [ ] **Step 2: Build**

```bash
make build
```

- [ ] **Step 3: Commit**

```bash
git add pkg/tui/session/session.go
git commit -m "feat(tui/session): add Session/Message/Block data types"
```

---

## Task 5: Create session store

**Files:**
- Create: `pkg/tui/session/store.go`
- Create: `pkg/tui/session/store_test.go`

- [ ] **Step 1: Write failing test**

```go
// pkg/tui/session/store_test.go
package session_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kubewise/kubewise/pkg/tui/session"
)

func TestStoreSaveAndLoadRecent(t *testing.T) {
	dir := t.TempDir()

	store := &session.Store{}
	session.SetStoreDir(store, dir)

	sess := session.New()
	sess.Title = "test session"
	sess.Messages = []session.Message{
		{Role: "user", Content: "hello", Timestamp: time.Now()},
	}

	if err := store.Save(sess); err != nil {
		t.Fatalf("Save: %v", err)
	}

	results, err := store.LoadRecent(10)
	if err != nil {
		t.Fatalf("LoadRecent: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("want 1 session, got %d", len(results))
	}
	if results[0].Title != "test session" {
		t.Errorf("title mismatch: got %q", results[0].Title)
	}
}

func TestStoreLoadRecentCapsAtN(t *testing.T) {
	dir := t.TempDir()

	store := &session.Store{}
	session.SetStoreDir(store, dir)

	for i := range 25 {
		s := session.New()
		s.Title = filepath.Join("session", string(rune('A'+i)))
		// Ensure distinct file names by sleeping briefly is flaky; use explicit IDs
		s.ID = fmt.Sprintf("%02d", i)
		_ = store.Save(s)
	}

	results, err := store.LoadRecent(20)
	if err != nil {
		t.Fatalf("LoadRecent: %v", err)
	}
	if len(results) > 20 {
		t.Errorf("want ≤20, got %d", len(results))
	}
}
```

Note: the test imports `fmt` — add `"fmt"` to the import block.

- [ ] **Step 2: Run to confirm failure**

```bash
go test ./pkg/tui/session/... -v
```

Expected: compile error — `store.go` does not exist.

- [ ] **Step 3: Write store.go**

```go
// pkg/tui/session/store.go
package session

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// Store persists sessions as JSON files under Dir.
type Store struct {
	Dir string
}

// SetStoreDir is exported for tests to inject a temp directory.
func SetStoreDir(s *Store, dir string) { s.Dir = dir }

// NewStore creates a Store pointed at ~/.kubewise/sessions/, creating it if absent.
func NewStore() (*Store, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("get home dir: %w", err)
	}
	dir := filepath.Join(home, ".kubewise", "sessions")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create sessions dir: %w", err)
	}
	return &Store{Dir: dir}, nil
}

// Save writes sess to a JSON file named <date>-<id>.json, creating the dir if needed.
func (s *Store) Save(sess *Session) error {
	if err := os.MkdirAll(s.Dir, 0o755); err != nil {
		return err
	}
	sess.UpdatedAt = time.Now()
	filename := fmt.Sprintf("%s-%s.json", sess.CreatedAt.Format("2006-01-02-150405"), sess.ID)
	path := filepath.Join(s.Dir, filename)
	data, err := json.MarshalIndent(sess, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// LoadRecent returns up to n sessions sorted by file modification time, newest first.
func (s *Store) LoadRecent(n int) ([]*Session, error) {
	entries, err := os.ReadDir(s.Dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	type entry struct {
		path    string
		modTime time.Time
	}
	var files []entry
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		files = append(files, entry{
			path:    filepath.Join(s.Dir, e.Name()),
			modTime: info.ModTime(),
		})
	}

	sort.Slice(files, func(i, j int) bool {
		return files[i].modTime.After(files[j].modTime)
	})

	if len(files) > n {
		files = files[:n]
	}

	sessions := make([]*Session, 0, len(files))
	for _, f := range files {
		data, err := os.ReadFile(f.path)
		if err != nil {
			continue
		}
		var sess Session
		if err := json.Unmarshal(data, &sess); err != nil {
			continue
		}
		sessions = append(sessions, &sess)
	}
	return sessions, nil
}
```

- [ ] **Step 4: Run tests**

```bash
go test ./pkg/tui/session/... -v
```

Expected: PASS for both test functions.

- [ ] **Step 5: Commit**

```bash
git add pkg/tui/session/
git commit -m "feat(tui/session): add JSON session store with LoadRecent"
```

---

## Task 6: Create styles

**Files:**
- Create: `pkg/tui/styles/styles.go`

- [ ] **Step 1: Write styles.go**

```go
package styles

import "github.com/charmbracelet/lipgloss"

var (
	// Colours
	Primary   = lipgloss.Color("#7C9EFF") // blue
	Secondary = lipgloss.Color("#888888") // muted grey
	Success   = lipgloss.Color("#79C896") // green
	Warning   = lipgloss.Color("#F5A623") // orange
	Error     = lipgloss.Color("#E06C75") // red

	// Sidebar
	SidebarWidth        = 24
	SidebarStyle        = lipgloss.NewStyle().Width(SidebarWidth).BorderRight(true).BorderStyle(lipgloss.NormalBorder()).BorderForeground(Secondary)
	SidebarItemStyle    = lipgloss.NewStyle().PaddingLeft(1)
	SidebarActiveStyle  = lipgloss.NewStyle().PaddingLeft(1).Foreground(Primary).Bold(true)
	SidebarHeaderStyle  = lipgloss.NewStyle().PaddingLeft(1).Foreground(Secondary).Bold(true)

	// Chat
	UserBubble      = lipgloss.NewStyle().Foreground(Primary).Bold(true)
	AssistantBubble = lipgloss.NewStyle()
	TimestampStyle  = lipgloss.NewStyle().Foreground(Secondary).Italic(true)

	// Progress card
	CardStyle    = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(Secondary).Padding(0, 1)
	CardDone     = lipgloss.NewStyle().Foreground(Success)
	CardRunning  = lipgloss.NewStyle().Foreground(Warning)
	CardFailed   = lipgloss.NewStyle().Foreground(Error)

	// Renderer
	TableBorderStyle = lipgloss.NewStyle().BorderStyle(lipgloss.NormalBorder()).BorderForeground(Secondary)
	CodeBlockStyle   = lipgloss.NewStyle().Background(lipgloss.Color("#1E1E2E")).Padding(0, 1)
	CodeLangStyle    = lipgloss.NewStyle().Foreground(Primary).Bold(true)
	KVKeyStyle       = lipgloss.NewStyle().Foreground(Primary).Bold(true)
	ListOKStyle      = lipgloss.NewStyle().Foreground(Success)
	ListWarnStyle    = lipgloss.NewStyle().Foreground(Warning)
	ListErrorStyle   = lipgloss.NewStyle().Foreground(Error)
	ListInfoStyle    = lipgloss.NewStyle().Foreground(Secondary)

	// Confirm modal
	ModalStyle       = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(1, 2).Width(60)
	ModalTitleStyle  = lipgloss.NewStyle().Foreground(Warning).Bold(true)
	ModalHintStyle   = lipgloss.NewStyle().Foreground(Secondary)

	// Input bar
	InputStyle = lipgloss.NewStyle().BorderTop(true).BorderStyle(lipgloss.NormalBorder()).BorderForeground(Secondary)
)
```

- [ ] **Step 2: Build**

```bash
make build
```

- [ ] **Step 3: Commit**

```bash
git add pkg/tui/styles/styles.go
git commit -m "feat(tui/styles): add lipgloss style constants"
```

---

## Task 7: Create renderer

**Files:**
- Create: `pkg/tui/model/renderer.go`
- Create: `pkg/tui/model/renderer_test.go`

- [ ] **Step 1: Write failing test**

```go
// pkg/tui/model/renderer_test.go
package model_test

import (
	"strings"
	"testing"

	"github.com/kubewise/kubewise/pkg/tui/events"
	"github.com/kubewise/kubewise/pkg/tui/model"
)

func TestRenderText(t *testing.T) {
	r := model.NewRenderer(80)
	out := r.RenderText("hello world")
	if !strings.Contains(out, "hello world") {
		t.Errorf("want 'hello world' in output, got: %q", out)
	}
}

func TestRenderKV(t *testing.T) {
	r := model.NewRenderer(80)
	pairs := []events.KVPair{{Key: "namespace", Value: "default"}, {Key: "pods", Value: "5"}}
	out := r.RenderKV(pairs)
	if !strings.Contains(out, "namespace") || !strings.Contains(out, "default") {
		t.Errorf("unexpected KV output: %q", out)
	}
}

func TestRenderTable(t *testing.T) {
	r := model.NewRenderer(80)
	headers := []string{"Name", "Status"}
	rows := [][]string{{"pod-a", "Running"}, {"pod-b", "Pending"}}
	out := r.RenderTable(headers, rows)
	if !strings.Contains(out, "pod-a") || !strings.Contains(out, "Running") {
		t.Errorf("unexpected table output: %q", out)
	}
}

func TestRenderList(t *testing.T) {
	r := model.NewRenderer(80)
	items := []events.ListItem{{Status: "ok", Text: "pod running"}, {Status: "error", Text: "pod crashed"}}
	out := r.RenderList(items)
	if !strings.Contains(out, "pod running") {
		t.Errorf("want 'pod running' in output: %q", out)
	}
}
```

- [ ] **Step 2: Run to confirm failure**

```bash
go test ./pkg/tui/model/... -v 2>&1 | head -20
```

Expected: compile error (renderer.go not found).

- [ ] **Step 3: Write renderer.go**

```go
// pkg/tui/model/renderer.go
package model

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/alecthomas/chroma/v2/formatters"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
	"github.com/charmbracelet/lipgloss"

	"github.com/kubewise/kubewise/pkg/tui/events"
	tuistyles "github.com/kubewise/kubewise/pkg/tui/styles"
)

// Renderer converts structured event payloads to styled terminal strings.
type Renderer struct {
	width int
}

// NewRenderer creates a Renderer with the given terminal width.
func NewRenderer(width int) *Renderer {
	return &Renderer{width: width}
}

// SetWidth updates the terminal width (called on resize).
func (r *Renderer) SetWidth(w int) { r.width = w }

// RenderText returns plain text, word-wrapped to terminal width.
func (r *Renderer) RenderText(text string) string {
	return lipgloss.NewStyle().Width(r.width).Render(text)
}

// RenderTable renders headers + rows as a lipgloss-bordered table.
func (r *Renderer) RenderTable(headers []string, rows [][]string) string {
	// Calculate column widths
	colWidths := make([]int, len(headers))
	for i, h := range headers {
		colWidths[i] = len(h)
	}
	for _, row := range rows {
		for i, cell := range row {
			if i < len(colWidths) && len(cell) > colWidths[i] {
				colWidths[i] = len(cell)
			}
		}
	}

	var sb strings.Builder
	sep := tuistyles.TableBorderStyle.Render(strings.Repeat("─", r.width-2))

	// Header row
	sb.WriteString(sep + "\n")
	for i, h := range headers {
		sb.WriteString(lipgloss.NewStyle().Width(colWidths[i]+2).Bold(true).Render(h))
	}
	sb.WriteString("\n" + sep + "\n")

	// Data rows
	for _, row := range rows {
		for i, cell := range row {
			if i >= len(colWidths) {
				break
			}
			sb.WriteString(lipgloss.NewStyle().Width(colWidths[i]+2).Render(cell))
		}
		sb.WriteString("\n")
	}
	sb.WriteString(sep)
	return sb.String()
}

// RenderCode renders a syntax-highlighted fenced code block.
func (r *Renderer) RenderCode(language, content string) string {
	// Try syntax highlight with chroma
	lexer := lexers.Get(language)
	if lexer == nil {
		lexer = lexers.Fallback
	}
	style := styles.Get("monokai")
	if style == nil {
		style = styles.Fallback
	}
	formatter := formatters.Get("terminal256")
	if formatter == nil {
		// fallback: plain
		header := tuistyles.CodeLangStyle.Render(language)
		return tuistyles.CodeBlockStyle.Width(r.width - 2).Render(header + "\n" + content)
	}

	tokenizer, err := lexer.Tokenise(nil, content)
	if err != nil {
		return content
	}
	var buf bytes.Buffer
	if err := formatter.Format(&buf, style, tokenizer); err != nil {
		return content
	}

	header := tuistyles.CodeLangStyle.Render(language)
	return tuistyles.CodeBlockStyle.Width(r.width-2).Render(header + "\n" + buf.String())
}

// RenderKV renders key-value pairs with right-aligned keys and left-aligned values.
func (r *Renderer) RenderKV(pairs []events.KVPair) string {
	// Find max key width
	maxKey := 0
	for _, p := range pairs {
		if len(p.Key) > maxKey {
			maxKey = len(p.Key)
		}
	}

	var sb strings.Builder
	for _, p := range pairs {
		key := tuistyles.KVKeyStyle.Width(maxKey + 2).Align(lipgloss.Right).Render(p.Key)
		sb.WriteString(fmt.Sprintf("%s  %s\n", key, p.Value))
	}
	return strings.TrimRight(sb.String(), "\n")
}

// RenderList renders status items with coloured status icons.
func (r *Renderer) RenderList(items []events.ListItem) string {
	var sb strings.Builder
	for _, item := range items {
		icon, style := statusIconStyle(item.Status)
		sb.WriteString(style.Render(icon+" "+item.Text) + "\n")
	}
	return strings.TrimRight(sb.String(), "\n")
}

func statusIconStyle(status string) (string, lipgloss.Style) {
	switch status {
	case "ok":
		return "✓", tuistyles.ListOKStyle
	case "warn":
		return "⚠", tuistyles.ListWarnStyle
	case "error":
		return "✗", tuistyles.ListErrorStyle
	default:
		return "•", tuistyles.ListInfoStyle
	}
}
```

- [ ] **Step 4: Run tests**

```bash
go test ./pkg/tui/model/... -v
```

Expected: PASS for all four renderer tests.

- [ ] **Step 5: Commit**

```bash
git add pkg/tui/model/renderer.go pkg/tui/model/renderer_test.go
git commit -m "feat(tui/model): add Renderer for table/code/kv/list output"
```

---

## Task 8: Add WithEventCh to query, troubleshooting, and security agents

All three follow the identical pattern. Apply to each in turn.

**Files:**
- Modify: `pkg/agent/query/agent.go`
- Modify: `pkg/agent/troubleshooting/agent.go`
- Modify: `pkg/agent/security/agent.go`

- [ ] **Step 1: Update query/agent.go**

Add `Option` type, new fields, and `WithEventCh` below the existing imports (add `"time"` to imports):

```go
// Option configures the Agent.
type Option func(*Agent)

// WithEventCh enables TUI event emission.
func WithEventCh(ch chan<- events.TUIEvent, queryID string) Option {
	return func(a *Agent) {
		a.eventCh = ch
		a.queryID = queryID
	}
}
```

Add fields to the `Agent` struct:

```go
type Agent struct {
	k8sClient    *k8s.Client
	llmClient    *llm.Client
	toolRegistry *tool.Registry
	// TUI mode (nil = CLI)
	eventCh chan<- events.TUIEvent
	queryID string
}
```

Change `New` signature to accept options:

```go
func New(k8sClient *k8s.Client, llmClient *llm.Client, opts ...Option) (*Agent, error) {
	// ... existing registry loading ...
	a := &Agent{
		k8sClient:    k8sClient,
		llmClient:    llmClient,
		toolRegistry: registry,
	}
	for _, opt := range opts {
		opt(a)
	}
	return a, nil
}
```

Add `emit` helper:

```go
func (a *Agent) emit(e events.TUIEvent) {
	if a.eventCh != nil {
		a.eventCh <- e
	}
}
```

Modify `HandleQuery` to emit events. Add `"time"` import. At the top of `HandleQuery`, before the messages loop:

```go
start := time.Now()
var inTokens, outTokens int
a.emit(events.AgentStartEvent{QueryID: a.queryID, AgentName: "Query Agent"})
defer func() {
	a.emit(events.AgentDoneEvent{
		QueryID:   a.queryID,
		Duration:  time.Since(start),
		InTokens:  inTokens,
		OutTokens: outTokens,
	})
}()
```

Inside the for loop, replace the `fmt.Printf` calls with event emission. Before `tool.Execute`:

```go
a.emit(events.ToolCallEvent{QueryID: a.queryID, ToolName: funcCall.Name, Step: step + 1})
toolStart := time.Now()
```

After `tool.Execute`:

```go
a.emit(events.ToolDoneEvent{QueryID: a.queryID, ToolName: funcCall.Name, Step: step + 1, Elapsed: time.Since(toolStart)})
```

Accumulate token usage after each LLM call (after `resp, err := a.llmClient.ChatCompletion`):

```go
if resp.Usage != nil {
	inTokens += resp.Usage.PromptTokens
	outTokens += resp.Usage.CompletionTokens
}
```

Also add the import:

```go
"github.com/kubewise/kubewise/pkg/tui/events"
```

- [ ] **Step 2: Apply the same pattern to troubleshooting/agent.go**

Same steps: add `Option` type, `WithEventCh`, `emit`, update `New`, add `AgentStartEvent`/`AgentDoneEvent` to `HandleQuery`, `ToolCallEvent`/`ToolDoneEvent` around tool calls. Use `"Troubleshooting Agent"` for `AgentName`.

- [ ] **Step 3: Apply the same pattern to security/agent.go**

Same steps. Use `"Security Agent"` for `AgentName`.

- [ ] **Step 4: Verify the router still builds (it calls `query.New` without options)**

```bash
make build
```

Expected: clean build (variadic `opts ...Option` is backward-compatible).

- [ ] **Step 5: Run existing tests**

```bash
make test
```

Expected: all existing tests pass.

- [ ] **Step 6: Commit**

```bash
git add pkg/agent/query/agent.go pkg/agent/troubleshooting/agent.go pkg/agent/security/agent.go
git commit -m "feat(agents): add WithEventCh option to query/troubleshooting/security agents"
```

---

## Task 9: Add WithEventCh to operation agent

**Files:**
- Modify: `pkg/agent/operation/agent.go`

- [ ] **Step 1: Add fields, option, and emit**

Add fields to `Agent` struct:

```go
type Agent struct {
	k8sClient      *k8s.Client
	llmClient      *llm.Client
	readRegistry   *tool.Registry
	writeRegistry  writeRegistryI
	confirmHandler ConfirmationHandler
	// TUI mode (nil = CLI)
	eventCh chan<- events.TUIEvent
	queryID string
}
```

Add option (after existing `WithConfirmationHandler`):

```go
// WithEventCh enables TUI event emission for the operation agent.
func WithEventCh(ch chan<- events.TUIEvent, queryID string) Option {
	return func(a *Agent) {
		a.eventCh = ch
		a.queryID = queryID
	}
}
```

Add `emit` helper:

```go
func (a *Agent) emit(e events.TUIEvent) {
	if a.eventCh != nil {
		a.eventCh <- e
	}
}
```

Add `"time"` and `"github.com/kubewise/kubewise/pkg/tui/events"` to imports.

- [ ] **Step 2: Emit events in HandleQuery**

Modify `HandleQuery`:

```go
func (a *Agent) HandleQuery(ctx context.Context, userQuery string, entities types.Entities) (string, error) {
	start := time.Now()
	var inTokens, outTokens int
	a.emit(events.AgentStartEvent{QueryID: a.queryID, AgentName: "Operation Agent"})
	defer func() {
		a.emit(events.AgentDoneEvent{
			QueryID:   a.queryID,
			Duration:  time.Since(start),
			InTokens:  inTokens,
			OutTokens: outTokens,
		})
	}()

	fmt.Println("正在分析操作意图并规划执行步骤...")
	steps, err := a.plan(ctx, userQuery, entities, &inTokens, &outTokens)
	// ...rest unchanged...
}
```

Modify `plan` signature to accept token accumulators, and accumulate after each LLM call:

```go
func (a *Agent) plan(ctx context.Context, userQuery string, _ types.Entities, inTok, outTok *int) ([]OperationStep, error) {
	// ... existing code ...
	// After each: resp, err := a.llmClient.ChatCompletion(...)
	if resp.Usage != nil {
		*inTok += resp.Usage.PromptTokens
		*outTok += resp.Usage.CompletionTokens
	}
	// Before tool call:
	a.emit(events.ToolCallEvent{QueryID: a.queryID, ToolName: funcCall.Name, Step: round + 1})
	toolStart := time.Now()
	// After tool call:
	a.emit(events.ToolDoneEvent{QueryID: a.queryID, ToolName: funcCall.Name, Step: round + 1, Elapsed: time.Since(toolStart)})
}
```

Update `HandleQuery` to pass `&inTokens, &outTokens` to `plan`.

- [ ] **Step 3: Build**

```bash
make build
```

- [ ] **Step 4: Run existing operation tests**

```bash
go test ./pkg/agent/operation/... -v
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/agent/operation/agent.go
git commit -m "feat(agent/operation): add WithEventCh for TUI event emission"
```

---

## Task 10: Add HandleQueryStream to router

**Files:**
- Modify: `pkg/agent/router/agent.go`

- [ ] **Step 1: Add HandleQueryStream method**

Add to `pkg/agent/router/agent.go`. Add imports: `"time"`, `"strings"`, `"github.com/kubewise/kubewise/pkg/tui/events"`.

```go
// HandleQueryStream classifies the query, routes to the matching agent, and
// pushes TUIEvents to eventCh throughout execution. Intended to be called in
// a goroutine by the TUI app. Sends StreamDoneEvent or StreamErrEvent last.
func (a *Agent) HandleQueryStream(ctx context.Context, query string, eventCh chan<- events.TUIEvent) {
	queryID := fmt.Sprintf("%x", time.Now().UnixNano())[:12]

	eventCh <- events.AgentStartEvent{QueryID: queryID, AgentName: "Router"}

	intent, err := a.classifyIntent(ctx, query)
	if err != nil {
		eventCh <- events.StreamErrEvent{QueryID: queryID, Err: fmt.Errorf("意图分类失败: %w", err)}
		return
	}

	switch intent.TaskType {
	case types.TaskTypeQuery:
		qAgent, err := query.New(a.k8sClient, a.llmClient, query.WithEventCh(eventCh, queryID))
		if err != nil {
			eventCh <- events.StreamErrEvent{QueryID: queryID, Err: err}
			return
		}
		result, err := qAgent.HandleQuery(ctx, query, intent.Entities)
		if err != nil {
			eventCh <- events.StreamErrEvent{QueryID: queryID, Err: err}
			return
		}
		emitRenderEvent(queryID, result, eventCh)
		eventCh <- events.StreamDoneEvent{QueryID: queryID, Result: result}

	case types.TaskTypeOperation:
		confirmHandler := operation.NewChannelConfirmationHandler()
		bridgeCtx, bridgeCancel := context.WithCancel(ctx)
		defer bridgeCancel()

		go func() {
			for {
				select {
				case req := <-confirmHandler.Requests:
					respCh := make(chan any, 1)
					eventCh <- events.ConfirmRequestEvent{
						QueryID:    queryID,
						Step:       req.Step,
						TotalSteps: req.TotalSteps,
						RespCh:     respCh,
					}
					select {
					case raw := <-respCh:
						if resp, ok := raw.(operation.ConfirmResponse); ok {
							confirmHandler.Responses <- resp
						}
					case <-bridgeCtx.Done():
						return
					}
				case <-bridgeCtx.Done():
					return
				}
			}
		}()

		opAgent, err := operation.New(a.k8sClient, a.llmClient,
			operation.WithConfirmationHandler(confirmHandler),
			operation.WithEventCh(eventCh, queryID),
		)
		if err != nil {
			eventCh <- events.StreamErrEvent{QueryID: queryID, Err: err}
			return
		}
		result, err := opAgent.HandleQuery(ctx, query, intent.Entities)
		if err != nil {
			eventCh <- events.StreamErrEvent{QueryID: queryID, Err: err}
			return
		}
		emitRenderEvent(queryID, result, eventCh)
		eventCh <- events.StreamDoneEvent{QueryID: queryID, Result: result}

	case types.TaskTypeTroubleshooting:
		tsAgent, err := troubleshooting.New(a.k8sClient, a.llmClient,
			troubleshooting.WithEventCh(eventCh, queryID))
		if err != nil {
			eventCh <- events.StreamErrEvent{QueryID: queryID, Err: err}
			return
		}
		result, err := tsAgent.HandleQuery(ctx, query, intent.Entities)
		if err != nil {
			eventCh <- events.StreamErrEvent{QueryID: queryID, Err: err}
			return
		}
		emitRenderEvent(queryID, result, eventCh)
		eventCh <- events.StreamDoneEvent{QueryID: queryID, Result: result}

	case types.TaskTypeSecurity:
		secAgent, err := security.New(a.k8sClient, a.llmClient,
			security.WithEventCh(eventCh, queryID))
		if err != nil {
			eventCh <- events.StreamErrEvent{QueryID: queryID, Err: err}
			return
		}
		result, err := secAgent.HandleQuery(ctx, query, intent.Entities)
		if err != nil {
			eventCh <- events.StreamErrEvent{QueryID: queryID, Err: err}
			return
		}
		emitRenderEvent(queryID, result, eventCh)
		eventCh <- events.StreamDoneEvent{QueryID: queryID, Result: result}

	default:
		eventCh <- events.StreamErrEvent{QueryID: queryID, Err: fmt.Errorf("不支持的任务类型: %s", intent.TaskType)}
	}
}
```

- [ ] **Step 2: Add emitRenderEvent helper**

```go
// emitRenderEvent inspects result text and sends the most appropriate RenderXxxEvent.
func emitRenderEvent(queryID, result string, eventCh chan<- events.TUIEvent) {
	lines := strings.Split(strings.TrimSpace(result), "\n")

	// YAML detection
	if strings.Contains(result, "apiVersion:") || strings.Contains(result, "kind:") {
		eventCh <- events.RenderCodeEvent{QueryID: queryID, Language: "yaml", Content: result}
		return
	}

	// JSON detection (indented block starting with { or [)
	trimmed := strings.TrimSpace(result)
	if (strings.HasPrefix(trimmed, "{") && strings.HasSuffix(trimmed, "}")) ||
		(strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]")) {
		eventCh <- events.RenderCodeEvent{QueryID: queryID, Language: "json", Content: result}
		return
	}

	// Table detection: majority of non-empty lines contain "|"
	pipeLines := 0
	nonEmpty := 0
	for _, l := range lines {
		l = strings.TrimSpace(l)
		if l == "" {
			continue
		}
		nonEmpty++
		if strings.Contains(l, "|") {
			pipeLines++
		}
	}
	if nonEmpty > 1 && pipeLines >= nonEmpty/2 {
		headers, rows := parseMarkdownTable(lines)
		if len(headers) > 0 {
			eventCh <- events.RenderTableEvent{QueryID: queryID, Headers: headers, Rows: rows}
			return
		}
	}

	// List detection: lines with status keywords Running/Pending/Error/Warning/...
	statusKeywords := []string{"Running", "Pending", "Error", "Warning", "Failed", "CrashLoop", "Terminating", "✓", "✗", "⚠"}
	statusLines := 0
	for _, l := range lines {
		for _, kw := range statusKeywords {
			if strings.Contains(l, kw) {
				statusLines++
				break
			}
		}
	}
	if nonEmpty >= 2 && statusLines >= nonEmpty/2 {
		items := parseListLines(lines)
		eventCh <- events.RenderListEvent{QueryID: queryID, Items: items}
		return
	}

	// KV detection: majority of non-empty lines match "key: value"
	kvLines := 0
	for _, l := range lines {
		if strings.Contains(l, ": ") && !strings.HasPrefix(strings.TrimSpace(l), "#") {
			kvLines++
		}
	}
	if nonEmpty >= 2 && kvLines >= (nonEmpty*2/3) {
		pairs := parseKVLines(lines)
		if len(pairs) > 0 {
			eventCh <- events.RenderKVEvent{QueryID: queryID, Pairs: pairs}
			return
		}
	}

	// Default: plain text
	eventCh <- events.RenderTextEvent{QueryID: queryID, Text: result}
}

func parseMarkdownTable(lines []string) (headers []string, rows [][]string) {
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.TrimLeft(line, "|-: ") == "" {
			continue
		}
		cells := strings.Split(line, "|")
		var row []string
		for _, c := range cells {
			c = strings.TrimSpace(c)
			if c != "" {
				row = append(row, c)
			}
		}
		if len(row) == 0 {
			continue
		}
		if headers == nil {
			headers = row
		} else {
			rows = append(rows, row)
		}
	}
	return
}

func parseListLines(lines []string) []events.ListItem {
	var items []events.ListItem
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		status := "info"
		if strings.Contains(line, "Running") || strings.Contains(line, "✓") {
			status = "ok"
		} else if strings.Contains(line, "Error") || strings.Contains(line, "Failed") || strings.Contains(line, "CrashLoop") || strings.Contains(line, "✗") {
			status = "error"
		} else if strings.Contains(line, "Warning") || strings.Contains(line, "Pending") || strings.Contains(line, "⚠") {
			status = "warn"
		}
		items = append(items, events.ListItem{Status: status, Text: line})
	}
	return items
}

func parseKVLines(lines []string) []events.KVPair {
	var pairs []events.KVPair
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		idx := strings.Index(line, ": ")
		if idx < 0 {
			continue
		}
		pairs = append(pairs, events.KVPair{
			Key:   strings.TrimSpace(line[:idx]),
			Value: strings.TrimSpace(line[idx+2:]),
		})
	}
	return pairs
}
```

- [ ] **Step 3: Build**

```bash
make build
```

Expected: clean build.

- [ ] **Step 4: Run tests**

```bash
make test
```

Expected: all existing tests pass.

- [ ] **Step 5: Commit**

```bash
git add pkg/agent/router/agent.go
git commit -m "feat(router): add HandleQueryStream with event emission and result rendering"
```

---

## Task 11: Create input model

**Files:**
- Create: `pkg/tui/model/input.go`

- [ ] **Step 1: Write input.go**

```go
// pkg/tui/model/input.go
package model

import (
	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	tuistyles "github.com/kubewise/kubewise/pkg/tui/styles"
)

// InputModel wraps a bubbles/textarea for the bottom input bar.
type InputModel struct {
	textarea textarea.Model
	width    int
	disabled bool
}

// SubmitMsg is sent when the user presses Enter.
type SubmitMsg struct{ Text string }

// NewInputModel creates the input bar.
func NewInputModel(width int) InputModel {
	ta := textarea.New()
	ta.Placeholder = "输入消息，按 Enter 发送..."
	ta.Focus()
	ta.CharLimit = 4000
	ta.SetWidth(width)
	ta.SetHeight(3)
	ta.ShowLineNumbers = false
	return InputModel{textarea: ta, width: width}
}

// SetWidth updates width on terminal resize.
func (m InputModel) SetWidth(w int) InputModel {
	m.width = w
	m.textarea.SetWidth(w)
	return m
}

// SetDisabled disables input while an agent is running.
func (m InputModel) SetDisabled(d bool) InputModel {
	m.disabled = d
	if d {
		m.textarea.Blur()
	} else {
		m.textarea.Focus()
	}
	return m
}

// Update handles key events.
func (m InputModel) Update(msg tea.Msg) (InputModel, tea.Cmd) {
	if m.disabled {
		return m, nil
	}
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.Type == tea.KeyEnter && !msg.Alt {
			text := m.textarea.Value()
			if text == "" {
				return m, nil
			}
			m.textarea.Reset()
			return m, func() tea.Msg { return SubmitMsg{Text: text} }
		}
	}
	var cmd tea.Cmd
	m.textarea, cmd = m.textarea.Update(msg)
	return m, cmd
}

// View renders the input bar.
func (m InputModel) View() string {
	style := tuistyles.InputStyle.Width(m.width)
	if m.disabled {
		return style.Foreground(lipgloss.Color("#555")).Render(m.textarea.View())
	}
	return style.Render(m.textarea.View())
}
```

- [ ] **Step 2: Build**

```bash
make build
```

- [ ] **Step 3: Commit**

```bash
git add pkg/tui/model/input.go
git commit -m "feat(tui/model): add InputModel wrapping bubbles/textarea"
```

---

## Task 12: Create confirm model

**Files:**
- Create: `pkg/tui/model/confirm.go`

- [ ] **Step 1: Write confirm.go**

```go
// pkg/tui/model/confirm.go
package model

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/kubewise/kubewise/pkg/agent/operation"
	"github.com/kubewise/kubewise/pkg/tui/events"
	tuistyles "github.com/kubewise/kubewise/pkg/tui/styles"
)

// ConfirmDoneMsg is emitted when the user responds to a confirmation request.
type ConfirmDoneMsg struct{}

// ConfirmModel is the modal overlay shown during operation confirmation.
type ConfirmModel struct {
	event      events.ConfirmRequestEvent
	step       operation.OperationStep
	correction string
	mode       confirmMode // "choose" | "edit"
}

type confirmMode int

const (
	confirmModeChoose confirmMode = iota
	confirmModeEdit
)

// NewConfirmModel creates a model from a ConfirmRequestEvent.
// Returns nil if the Step field is not an operation.OperationStep.
func NewConfirmModel(e events.ConfirmRequestEvent) *ConfirmModel {
	step, ok := e.Step.(operation.OperationStep)
	if !ok {
		return nil
	}
	return &ConfirmModel{event: e, step: step}
}

// Update handles Y/N/E/Esc key presses.
func (m *ConfirmModel) Update(msg tea.Msg) (*ConfirmModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch m.mode {
		case confirmModeChoose:
			switch strings.ToUpper(msg.String()) {
			case "Y":
				m.event.RespCh <- operation.ConfirmResponse{Confirmed: true}
				return nil, func() tea.Msg { return ConfirmDoneMsg{} }
			case "N", "ESC":
				m.event.RespCh <- operation.ConfirmResponse{Confirmed: false}
				return nil, func() tea.Msg { return ConfirmDoneMsg{} }
			case "E":
				m.mode = confirmModeEdit
				m.correction = ""
			}
		case confirmModeEdit:
			switch msg.Type {
			case tea.KeyEnter:
				m.event.RespCh <- operation.ConfirmResponse{Confirmed: false, Correction: m.correction}
				return nil, func() tea.Msg { return ConfirmDoneMsg{} }
			case tea.KeyEsc:
				m.mode = confirmModeChoose
			case tea.KeyBackspace, tea.KeyDelete:
				if len(m.correction) > 0 {
					m.correction = m.correction[:len(m.correction)-1]
				}
			default:
				if msg.Type == tea.KeyRunes {
					m.correction += string(msg.Runes)
				}
			}
		}
	}
	return m, nil
}

// View renders the confirmation modal.
func (m *ConfirmModel) View() string {
	var sb strings.Builder

	title := tuistyles.ModalTitleStyle.Render(
		fmt.Sprintf("步骤 %d/%d: %s", m.step.StepIndex, m.event.TotalSteps, operationDisplay(m.step.OperationType)))
	sb.WriteString(title + "\n\n")

	// Resource info
	if m.step.Namespace != "" {
		sb.WriteString(fmt.Sprintf("资源: %s/%s (ns: %s)\n", m.step.ResourceKind, m.step.ResourceName, m.step.Namespace))
	} else {
		sb.WriteString(fmt.Sprintf("资源: %s/%s\n", m.step.ResourceKind, m.step.ResourceName))
	}
	if m.step.Description != "" {
		sb.WriteString(fmt.Sprintf("说明: %s\n", m.step.Description))
	}
	sb.WriteString("\n")

	switch m.mode {
	case confirmModeChoose:
		hint := tuistyles.ModalHintStyle.Render("[Y] 确认执行  [N] 跳过  [E] 修改参数  [Esc] 取消")
		sb.WriteString(hint)
	case confirmModeEdit:
		sb.WriteString("修正指令: " + m.correction + "█\n")
		hint := tuistyles.ModalHintStyle.Render("[Enter] 提交修正  [Esc] 返回")
		sb.WriteString(hint)
	}

	return tuistyles.ModalStyle.Render(sb.String())
}

func operationDisplay(op string) string {
	m := map[string]string{
		"scale":         "调整副本数",
		"restart":       "滚动重启",
		"delete":        "删除资源",
		"apply":         "Apply YAML",
		"cordon_drain":  "封锁/驱逐节点",
		"label_annotate": "修改标签/注解",
	}
	if v, ok := m[op]; ok {
		return v
	}
	return op
}
```

- [ ] **Step 2: Build**

```bash
make build
```

- [ ] **Step 3: Commit**

```bash
git add pkg/tui/model/confirm.go
git commit -m "feat(tui/model): add ConfirmModel for operation step approval modal"
```

---

## Task 13: Create sidebar model

**Files:**
- Create: `pkg/tui/model/sidebar.go`

- [ ] **Step 1: Write sidebar.go**

```go
// pkg/tui/model/sidebar.go
package model

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/kubewise/kubewise/pkg/tui/session"
	tuistyles "github.com/kubewise/kubewise/pkg/tui/styles"
)

// SelectSessionMsg is emitted when the user selects a history session.
type SelectSessionMsg struct{ Session *session.Session }

// SidebarModel renders the left session-list panel.
type SidebarModel struct {
	sessions []*session.Session
	active   string // active session ID
	cursor   int
	height   int
	focused  bool
}

// NewSidebarModel creates the sidebar.
func NewSidebarModel(sessions []*session.Session, activeID string, height int) SidebarModel {
	return SidebarModel{sessions: sessions, active: activeID, height: height}
}

// SetSessions refreshes the session list.
func (m SidebarModel) SetSessions(s []*session.Session) SidebarModel {
	m.sessions = s
	if m.cursor >= len(s) && len(s) > 0 {
		m.cursor = len(s) - 1
	}
	return m
}

// SetHeight updates height on resize.
func (m SidebarModel) SetHeight(h int) SidebarModel {
	m.height = h
	return m
}

// SetFocused toggles keyboard focus.
func (m SidebarModel) SetFocused(f bool) SidebarModel {
	m.focused = f
	return m
}

// SetActive marks the given session ID as currently active.
func (m SidebarModel) SetActive(id string) SidebarModel {
	m.active = id
	return m
}

// Update handles arrow keys and Enter when focused.
func (m SidebarModel) Update(msg tea.Msg) (SidebarModel, tea.Cmd) {
	if !m.focused {
		return m, nil
	}
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch key.Type {
	case tea.KeyUp:
		if m.cursor > 0 {
			m.cursor--
		}
	case tea.KeyDown:
		if m.cursor < len(m.sessions)-1 {
			m.cursor++
		}
	case tea.KeyEnter:
		if m.cursor < len(m.sessions) {
			sess := m.sessions[m.cursor]
			return m, func() tea.Msg { return SelectSessionMsg{Session: sess} }
		}
	}
	return m, nil
}

// View renders the sidebar panel.
func (m SidebarModel) View() string {
	var sb strings.Builder
	sb.WriteString(tuistyles.SidebarHeaderStyle.Render("会话") + "\n")

	for i, s := range m.sessions {
		isActive := s.ID == m.active
		isCursor := m.focused && i == m.cursor

		title := session.TitleFromFirstMessage(s.Title)
		if title == "" {
			title = "新会话"
		}
		age := relativeTime(s.UpdatedAt)

		line := fmt.Sprintf("%-16s %4s", truncate(title, 16), age)

		switch {
		case isActive:
			sb.WriteString(tuistyles.SidebarActiveStyle.Render(line) + "\n")
		case isCursor:
			sb.WriteString(tuistyles.SidebarItemStyle.
				Background(lipgloss.Color("#2A2A3A")).
				Render(line) + "\n")
		default:
			sb.WriteString(tuistyles.SidebarItemStyle.Render(line) + "\n")
		}
	}

	return tuistyles.SidebarStyle.Height(m.height - 1).Render(sb.String())
}

func truncate(s string, max int) string {
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max-1]) + "…"
}

func relativeTime(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "now"
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
}
```

- [ ] **Step 2: Build**

```bash
make build
```

- [ ] **Step 3: Commit**

```bash
git add pkg/tui/model/sidebar.go
git commit -m "feat(tui/model): add SidebarModel for session list navigation"
```

---

## Task 14: Create chat model

**Files:**
- Create: `pkg/tui/model/chat.go`

- [ ] **Step 1: Write chat.go**

```go
// pkg/tui/model/chat.go
package model

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/kubewise/kubewise/pkg/tui/events"
	"github.com/kubewise/kubewise/pkg/tui/session"
	tuistyles "github.com/kubewise/kubewise/pkg/tui/styles"
)

// progressLine is one line inside a progress card.
type progressLine struct {
	icon    string
	text    string
	elapsed time.Duration
	done    bool
}

// progressCard tracks the live activity for one query.
type progressCard struct {
	queryID   string
	agentName string
	lines     []progressLine
	done      bool
	summary   string
	startTime time.Time
}

// ChatModel displays message history and live progress cards.
type ChatModel struct {
	messages  []session.Message
	cards     map[string]*progressCard // queryID → card
	renderer  *Renderer
	width     int
	height    int
	scrollOff int
}

// NewChatModel creates the chat area.
func NewChatModel(width, height int) ChatModel {
	return ChatModel{
		cards:    make(map[string]*progressCard),
		renderer: NewRenderer(width - 2),
		width:    width,
		height:   height,
	}
}

// SetSize updates dimensions on terminal resize.
func (m ChatModel) SetSize(w, h int) ChatModel {
	m.width = w
	m.height = h
	m.renderer.SetWidth(w - 2)
	return m
}

// SetMessages replaces the message history (used on session switch).
func (m ChatModel) SetMessages(msgs []session.Message) ChatModel {
	m.messages = msgs
	return m
}

// AddUserMessage appends a user message.
func (m ChatModel) AddUserMessage(text string) ChatModel {
	m.messages = append(m.messages, session.Message{
		Role:      "user",
		Content:   text,
		Timestamp: time.Now(),
	})
	return m
}

// Update applies a TUIEvent to the progress cards.
func (m ChatModel) Update(msg tea.Msg) ChatModel {
	switch e := msg.(type) {
	case events.AgentStartEvent:
		m.cards[e.QueryID] = &progressCard{
			queryID:   e.QueryID,
			agentName: e.AgentName,
			startTime: time.Now(),
		}

	case events.ToolCallEvent:
		if card, ok := m.cards[e.QueryID]; ok {
			card.lines = append(card.lines, progressLine{
				icon: "⟳",
				text: fmt.Sprintf("%s (step %d)", e.ToolName, e.Step),
			})
		}

	case events.ToolDoneEvent:
		if card, ok := m.cards[e.QueryID]; ok {
			for i := len(card.lines) - 1; i >= 0; i-- {
				if strings.HasPrefix(card.lines[i].text, e.ToolName) && !card.lines[i].done {
					card.lines[i].icon = "✓"
					card.lines[i].elapsed = e.Elapsed
					card.lines[i].done = true
					break
				}
			}
		}

	case events.AgentDoneEvent:
		if card, ok := m.cards[e.QueryID]; ok {
			card.done = true
			card.summary = fmt.Sprintf("✓ %s  %.1fs | ↑%d ↓%d tok",
				card.agentName,
				e.Duration.Seconds(),
				e.InTokens, e.OutTokens)
		}

	case events.StreamDoneEvent:
		// Finalise: move render result into messages
		// (actual message appending is done in app.go)
		if card, ok := m.cards[e.QueryID]; ok {
			card.done = true
		}

	case events.StreamErrEvent:
		if card, ok := m.cards[e.QueryID]; ok {
			card.done = true
			card.summary = tuistyles.CardFailed.Render("✗ " + e.Err.Error())
		}

	case events.RenderTextEvent:
		m.messages = append(m.messages, session.Message{
			Role: "assistant", Content: e.Text, Timestamp: time.Now(),
		})
	case events.RenderTableEvent:
		m.messages = append(m.messages, session.Message{
			Role:    "assistant",
			Content: m.renderer.RenderTable(e.Headers, e.Rows),
			Timestamp: time.Now(),
		})
	case events.RenderCodeEvent:
		m.messages = append(m.messages, session.Message{
			Role:    "assistant",
			Content: m.renderer.RenderCode(e.Language, e.Content),
			Timestamp: time.Now(),
		})
	case events.RenderKVEvent:
		m.messages = append(m.messages, session.Message{
			Role:    "assistant",
			Content: m.renderer.RenderKV(e.Pairs),
			Timestamp: time.Now(),
		})
	case events.RenderListEvent:
		m.messages = append(m.messages, session.Message{
			Role:    "assistant",
			Content: m.renderer.RenderList(e.Items),
			Timestamp: time.Now(),
		})
	}
	return m
}

// LastMessages returns all messages (used for session saving).
func (m ChatModel) LastMessages() []session.Message {
	return m.messages
}

// View renders message history followed by any active progress cards.
func (m ChatModel) View() string {
	var sb strings.Builder
	usableWidth := m.width

	// Render message history
	for _, msg := range m.messages {
		switch msg.Role {
		case "user":
			prefix := tuistyles.UserBubble.Render("You > ")
			sb.WriteString(prefix + lipgloss.NewStyle().Width(usableWidth - 6).Render(msg.Content) + "\n\n")
		case "assistant":
			sb.WriteString(msg.Content + "\n\n")
		}
	}

	// Render active progress cards
	for _, card := range m.cards {
		if card.done {
			sb.WriteString(tuistyles.CardDone.Render(card.summary) + "\n")
			continue
		}
		var cardSB strings.Builder
		cardSB.WriteString(tuistyles.CardRunning.Render("⚙ "+card.agentName) + "\n")
		for _, line := range card.lines {
			style := tuistyles.CardRunning
			if line.done {
				style = tuistyles.CardDone
			}
			elapsed := ""
			if line.elapsed > 0 {
				elapsed = fmt.Sprintf("  %.1fs", line.elapsed.Seconds())
			}
			cardSB.WriteString(style.Render(fmt.Sprintf("  %s %s%s", line.icon, line.text, elapsed)) + "\n")
		}
		sb.WriteString(tuistyles.CardStyle.Width(usableWidth-2).Render(cardSB.String()) + "\n")
	}

	return lipgloss.NewStyle().Width(usableWidth).Height(m.height).Render(sb.String())
}
```

- [ ] **Step 2: Build**

```bash
make build
```

- [ ] **Step 3: Commit**

```bash
git add pkg/tui/model/chat.go
git commit -m "feat(tui/model): add ChatModel with message history and live progress cards"
```

---

## Task 15: Create root App model

**Files:**
- Create: `pkg/tui/app.go`

- [ ] **Step 1: Write app.go**

```go
// pkg/tui/app.go
package tui

import (
	"context"
	"fmt"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/kubewise/kubewise/pkg/agent/operation"
	"github.com/kubewise/kubewise/pkg/agent/router"
	"github.com/kubewise/kubewise/pkg/k8s"
	"github.com/kubewise/kubewise/pkg/llm"
	"github.com/kubewise/kubewise/pkg/tui/events"
	"github.com/kubewise/kubewise/pkg/tui/model"
	"github.com/kubewise/kubewise/pkg/tui/session"
	tuistyles "github.com/kubewise/kubewise/pkg/tui/styles"
)

// App is the root bubbletea model.
type App struct {
	// Sub-models
	sidebar model.SidebarModel
	chat    model.ChatModel
	input   model.InputModel
	confirm *model.ConfirmModel // nil = no modal

	// Session management
	sessions []*session.Session
	active   *session.Session
	store    *session.Store

	// Agent streaming
	eventCh  chan events.TUIEvent
	cancelFn context.CancelFunc
	running  bool

	// Agent dependencies
	routerAgent *router.Agent

	// Layout
	width       int
	height      int
	showSidebar bool

	// Focus: "input" | "sidebar"
	focus string
}

// NewApp creates the App, loading recent sessions from disk.
func NewApp(k8sClient *k8s.Client, llmClient *llm.Client) (*App, error) {
	store, err := session.NewStore()
	if err != nil {
		return nil, fmt.Errorf("init session store: %w", err)
	}

	recentSessions, err := store.LoadRecent(20)
	if err != nil {
		recentSessions = nil
	}

	activeSession := session.New()

	routerAgent, err := router.New(k8sClient, llmClient)
	if err != nil {
		return nil, err
	}

	app := &App{
		sessions:    recentSessions,
		active:      activeSession,
		store:       store,
		routerAgent: routerAgent,
		showSidebar: true,
		focus:       "input",
	}
	return app, nil
}

// Init initialises the bubbletea program.
func (a App) Init() tea.Cmd {
	return nil
}

// listenForEvents returns a Cmd that blocks until an event arrives on eventCh.
func listenForEvents(ch <-chan events.TUIEvent) tea.Cmd {
	return func() tea.Msg {
		return <-ch
	}
}

// Update handles all messages.
func (a App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		a.width = msg.Width
		a.height = msg.Height
		return a.applyLayout(), nil

	case tea.KeyMsg:
		return a.handleKey(msg)

	// TUI events from agents
	case events.AgentStartEvent,
		events.AgentDoneEvent,
		events.ToolCallEvent,
		events.ToolDoneEvent,
		events.RenderTextEvent,
		events.RenderTableEvent,
		events.RenderCodeEvent,
		events.RenderKVEvent,
		events.RenderListEvent:
		a.chat = a.chat.Update(msg)
		return a, listenForEvents(a.eventCh)

	case events.ConfirmRequestEvent:
		a.confirm = model.NewConfirmModel(msg)
		return a, listenForEvents(a.eventCh)

	case events.StreamDoneEvent:
		a.chat = a.chat.Update(msg)
		a.running = false
		a.input = a.input.SetDisabled(false)
		a.saveSession()
		return a, nil

	case events.StreamErrEvent:
		a.chat = a.chat.Update(msg)
		a.running = false
		a.input = a.input.SetDisabled(false)
		return a, nil

	case model.SubmitMsg:
		return a.handleSubmit(msg.Text)

	case model.ConfirmDoneMsg:
		a.confirm = nil
		return a, listenForEvents(a.eventCh)

	case model.SelectSessionMsg:
		a.loadSession(msg.Session)
		a.focus = "input"
		a.sidebar = a.sidebar.SetFocused(false)
		return a, nil
	}

	// Route updates to sub-models
	var cmd tea.Cmd
	if a.confirm != nil {
		a.confirm, cmd = a.confirm.Update(msg)
		return a, cmd
	}
	switch a.focus {
	case "sidebar":
		a.sidebar, cmd = a.sidebar.Update(msg)
	case "input":
		a.input, cmd = a.input.Update(msg)
	}
	return a, cmd
}

func (a App) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyCtrlC:
		if a.running {
			if a.cancelFn != nil {
				a.cancelFn()
			}
			a.running = false
			a.active.InterruptedQuery = a.getLastUserMessage()
			a.input = a.input.SetDisabled(false)
			return a, nil
		}
		return a, tea.Quit

	case tea.KeyCtrlN:
		return a.newSession(), nil

	case tea.KeyCtrlL:
		a.active.Messages = nil
		a.chat = a.chat.SetMessages(nil)
		return a, nil

	case tea.KeyTab:
		if a.showSidebar {
			if a.focus == "input" {
				a.focus = "sidebar"
				a.sidebar = a.sidebar.SetFocused(true)
				a.input = a.input.SetDisabled(true)
			} else {
				a.focus = "input"
				a.sidebar = a.sidebar.SetFocused(false)
				a.input = a.input.SetDisabled(false)
			}
		}
		return a, nil
	}
	return a, nil
}

func (a App) handleSubmit(text string) (tea.Model, tea.Cmd) {
	if a.running {
		return a, nil
	}

	// /resume command
	if text == "/resume" && a.active.InterruptedQuery != "" {
		text = a.active.InterruptedQuery
		a.active.InterruptedQuery = ""
	}

	// Set session title from first message
	if len(a.active.Messages) == 0 {
		a.active.Title = session.TitleFromFirstMessage(text)
		a.sidebar = a.sidebar.SetActive(a.active.ID)
	}

	a.chat = a.chat.AddUserMessage(text)

	// Start streaming
	a.running = true
	a.input = a.input.SetDisabled(true)

	a.eventCh = make(chan events.TUIEvent, 100)
	ctx, cancel := context.WithCancel(context.Background())
	a.cancelFn = cancel

	routerAgent := a.routerAgent
	eventCh := a.eventCh
	go routerAgent.HandleQueryStream(ctx, text, eventCh)

	return a, listenForEvents(a.eventCh)
}

func (a App) newSession() App {
	a.saveSession()
	a.active = session.New()
	a.chat = a.chat.SetMessages(nil)
	if !containsSession(a.sessions, a.active.ID) {
		a.sessions = append([]*session.Session{a.active}, a.sessions...)
	}
	a.sidebar = a.sidebar.SetActive(a.active.ID).SetSessions(a.sessions)
	return a
}

func (a *App) loadSession(s *session.Session) {
	a.saveSession()
	a.active = s
	a.chat = a.chat.SetMessages(s.Messages)
	a.sidebar = a.sidebar.SetActive(s.ID)
}

func (a *App) saveSession() {
	if a.store == nil || a.active == nil {
		return
	}
	a.active.Messages = a.chat.LastMessages()
	_ = a.store.Save(a.active)
}

func (a *App) getLastUserMessage() string {
	msgs := a.chat.LastMessages()
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role == "user" {
			return msgs[i].Content
		}
	}
	return ""
}

func (a App) applyLayout() App {
	// Hide sidebar when terminal is narrow
	a.showSidebar = a.width >= 100

	sidebarW := 0
	if a.showSidebar {
		sidebarW = tuistyles.SidebarWidth
	}
	chatW := a.width - sidebarW
	inputH := 5
	chatH := a.height - inputH

	a.sidebar = model.NewSidebarModel(a.sessions, a.active.ID, a.height)
	a.chat = a.chat.SetSize(chatW, chatH)
	a.input = a.input.SetWidth(chatW)
	return a
}

// View renders the full TUI.
func (a App) View() string {
	if a.confirm != nil {
		return a.confirm.View()
	}

	inputH := 5
	chatH := a.height - inputH

	chatView := a.chat.SetSize(a.width-tuistyles.SidebarWidth, chatH).View()
	inputView := a.input.View()
	rightPanel := chatView + "\n" + inputView

	if a.showSidebar {
		return a.sidebar.View() + rightPanel
	}
	return rightPanel
}

// Run starts the bubbletea program.
func Run(k8sClient *k8s.Client, llmClient *llm.Client) error {
	app, err := NewApp(k8sClient, llmClient)
	if err != nil {
		return err
	}

	// Initialise layout defaults before first render
	app.width = 120
	app.height = 40
	app.showSidebar = true
	sidebarW := tuistyles.SidebarWidth
	app.sidebar = model.NewSidebarModel(app.sessions, app.active.ID, app.height)
	app.chat = model.NewChatModel(app.width-sidebarW, app.height-5)
	app.input = model.NewInputModel(app.width - sidebarW)

	p := tea.NewProgram(*app, tea.WithAltScreen())
	_, err = p.Run()
	return err
}

func containsSession(sessions []*session.Session, id string) bool {
	for _, s := range sessions {
		if s.ID == id {
			return true
		}
	}
	return false
}

// Ensure App implements tea.Model
var _ tea.Model = App{}
```

Note: The `operation` import in `app.go` is needed for the `ConfirmDoneMsg` — but `ConfirmDoneMsg` is in the `model` package. The only reason `app.go` imports `operation` is the `NewConfirmModel` function needs `operation.OperationStep` for the type assertion inside `confirm.go`. The import in app.go is **not** needed since `confirm.go` handles that internally. Remove `"github.com/kubewise/kubewise/pkg/agent/operation"` from app.go imports if the build complains.

- [ ] **Step 2: Build**

```bash
make build
```

Fix any compile errors:
- If `App` is not a value receiver type, ensure all `Update` / `View` use value receivers consistently.
- If `tea.WithAltScreen` doesn't exist in your bubbletea version, replace with `tea.WithAltScreen()` or just `tea.NewProgram(*app)`.

- [ ] **Step 3: Commit**

```bash
git add pkg/tui/app.go
git commit -m "feat(tui): add root App bubbletea model wiring all sub-models"
```

---

## Task 16: Add tui subcommand to cmd/main.go

**Files:**
- Modify: `cmd/main.go`

- [ ] **Step 1: Add tuiCmd and register it**

Add the following after `chatCmd` declaration in `cmd/main.go`:

```go
var tuiCmd = &cobra.Command{
	Use:   "tui",
	Short: "启动交互式 TUI 多轮对话模式",
	Long: `启动终端交互界面（TUI），支持多轮对话、会话管理和操作确认。
快捷键：
  Enter     发送消息
  Ctrl+N    新建会话
  Ctrl+C    中断当前查询（空闲时退出）
  Ctrl+L    清空当前会话
  Tab       切换焦点（侧边栏 ↔ 输入框）
  /resume   重发被中断的消息`,
	RunE: func(cmd *cobra.Command, args []string) error {
		kubeconfig := viper.GetString("kubeconfig")
		k8sClient, err := k8s.NewClient(kubeconfig)
		if err != nil {
			return fmt.Errorf("初始化K8s客户端失败: %w", err)
		}

		llmConfig := llm.Config{
			Model:   viper.GetString("llm.model"),
			APIKey:  viper.GetString("llm.api_key"),
			APIBase: viper.GetString("llm.api_base"),
		}
		llmClient, err := llm.NewClient(llmConfig)
		if err != nil {
			return fmt.Errorf("初始化LLM客户端失败: %w", err)
		}

		return tui.Run(k8sClient, llmClient)
	},
}
```

Add import:

```go
"github.com/kubewise/kubewise/pkg/tui"
```

In `init()`, add:

```go
rootCmd.AddCommand(tuiCmd)
```

- [ ] **Step 2: Build**

```bash
make build
```

Expected: binary compiles including the new `tui` subcommand.

- [ ] **Step 3: Smoke test the help output**

```bash
./kubewise tui --help
```

Expected: shows the TUI command description and keyboard shortcuts.

- [ ] **Step 4: Run all tests**

```bash
make test
```

Expected: all tests pass.

- [ ] **Step 5: Commit**

```bash
git add cmd/main.go
git commit -m "feat(cmd): add tui subcommand launching interactive TUI mode"
```

---

## Self-Review

### Spec Coverage

| Spec requirement | Task |
|---|---|
| `kubewise tui` CLI entry point | Task 16 |
| bubbletea App with SidebarModel, ChatModel, InputModel, ConfirmModel | Tasks 11–15 |
| TUIEvent interface + all event types | Task 3 |
| WithEventCh on all 4 agents | Tasks 8, 9 |
| HandleQueryStream on router | Task 10 |
| ConfirmRequestEvent bridge via ChannelConfirmationHandler | Task 10 |
| Progress card (⚙ header, ⟳/✓ lines, folded summary) | Task 14 |
| Sidebar fixed 24 cols, hidden < 100 cols, Tab toggle | Tasks 13, 15 |
| Render type detection rules (table/yaml/json/kv/list/text) | Task 10 |
| Session persistence JSON `~/.kubewise/sessions/` | Tasks 4, 5 |
| Ctrl+N new session, Ctrl+L clear, Tab focus switch | Task 15 |
| Ctrl+C interrupt → running=false, /resume | Task 15 |
| Y/N/E confirm modal | Task 12 |
| Session auto-save on StreamDoneEvent | Task 15 |
| Load recent 20 sessions on startup | Tasks 5, 15 |
| lipgloss styles | Task 6 |
| Renderer (table/code/kv/list) | Task 7 |
| llm.Usage tracking | Task 2 |
| Dependencies | Task 1 |

All spec requirements are covered.

### Type Consistency Check

- `events.TUIEvent` sealed with `isTUIEvent()` — consistent throughout
- `events.ConfirmRequestEvent.Step any` and `RespCh chan<- any` — cast to `operation.OperationStep` / send `operation.ConfirmResponse` in `confirm.go` and bridge goroutine in router
- `model.SubmitMsg`, `model.ConfirmDoneMsg`, `model.SelectSessionMsg` all defined in model package and handled in `app.go`
- `session.Store.Save` / `LoadRecent` — used in app.go, tested in store_test.go
- `renderer.RenderTable(headers []string, rows [][]string)` — matches `events.RenderTableEvent` fields
- `WithEventCh(ch chan<- events.TUIEvent, queryID string) Option` — consistent across all 4 agents
- `router.HandleQueryStream(ctx, query, eventCh)` — called in `app.go` `handleSubmit`
