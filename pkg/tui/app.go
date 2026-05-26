package tui

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"go.uber.org/zap"

	"github.com/kubewise/kubewise/pkg/agent/router"
	"github.com/kubewise/kubewise/pkg/stream"
	"github.com/kubewise/kubewise/pkg/agent/supervisor"
	"github.com/kubewise/kubewise/pkg/catalog"
	"github.com/kubewise/kubewise/pkg/k8s"
	"github.com/kubewise/kubewise/pkg/llm"
	deploytypes "github.com/kubewise/kubewise/pkg/agent/deploy/types"
	"github.com/kubewise/kubewise/pkg/tui/model"
	"github.com/kubewise/kubewise/pkg/tui/session"
	tuistyles "github.com/kubewise/kubewise/pkg/tui/styles"
)

type focusState int

const (
	focusInput focusState = iota
	focusSidebar
)

// App is the root bubbletea model composing sidebar, chat, input, and confirm sub-models.
type App struct {
	sidebar model.SidebarModel
	chat    model.ChatModel
	input   model.InputModel
	confirm *model.ConfirmModel // nil when no modal is shown

	// Deploy TUI 组件（同一时刻最多一个 active）
	chartSelectModel   *model.ChartSelectModel
	manualInputModel   *model.ManualChartInputModel
	deployConfirmModel *model.DeployConfirmModel
	deployConfirmStreamResp chan<- json.RawMessage // deploy_confirm InteractionRequest
	streamChartResp         chan<- json.RawMessage // chart_select InteractionRequest
	streamChartCandidates  []catalog.ChartInfo

	sessions []*session.Session
	active   *session.Session
	store    *session.Store

	eventCh  chan stream.Event
	cancelFn context.CancelFunc
	running  bool
	querySeq int

	routerAgent *router.Agent

	width       int
	height      int
	showSidebar bool
	focus       focusState
}

// NewApp creates the App, loading recent sessions from disk.
func NewApp(k8sClient *k8s.Client, llmClient *llm.Client, log *zap.Logger, maxSteps int, supervisorCfg supervisor.Config) (*App, error) {
	store, err := session.NewStore()
	if err != nil {
		return nil, fmt.Errorf("init session store: %w", err)
	}

	recentSessions, err := store.LoadRecent(20)
	if err != nil {
		recentSessions = nil
	}

	activeSession := session.New()

	routerAgent, err := router.New(k8sClient, llmClient, maxSteps, supervisorCfg)
	if err != nil {
		return nil, err
	}
	routerAgent.SetLogger(log)

	return &App{
		sidebar:     model.NewSidebarModel(),
		sessions:    recentSessions,
		active:      activeSession,
		store:       store,
		routerAgent: routerAgent,
		showSidebar: true,
		focus:       focusInput,
	}, nil
}

// Init initialises the bubbletea program.
func (a App) Init() tea.Cmd {
	return a.input.Init()
}

// listenStream waits for one stream.Event from agents.
func (a App) listenStream() tea.Cmd {
	return stream.Listen(a.eventCh)
}

func chartCandidateIndex(chosen *catalog.ChartInfo, list []catalog.ChartInfo) int {
	if chosen == nil || len(list) == 0 {
		return -1
	}
	for i := range list {
		c := list[i]
		if c.ChartName == chosen.ChartName && c.RepoName == chosen.RepoName && (c.RepoURL == chosen.RepoURL || chosen.RepoURL == "") {
			return i
		}
	}
	return -1
}

func (a App) dispatchInteractionRequest(ir stream.InteractionRequest) (tea.Model, tea.Cmd) {
	switch ir.Kind {
	case stream.KindOperationStep:
		cm, err := model.NewConfirmModel(ir.QueryID, ir.TotalSteps, ir.Payload, ir.RespCh)
		if err != nil {
			return a, a.listenStream()
		}
		a.confirm = &cm
		return a, a.listenStream()
	case stream.KindChartSelect:
		var p struct {
			AppName    string               `json:"app_name"`
			Candidates []catalog.ChartInfo `json:"candidates"`
		}
		if err := json.Unmarshal(ir.Payload, &p); err != nil {
			return a, a.listenStream()
		}
		a.streamChartResp = ir.RespCh
		a.streamChartCandidates = p.Candidates
		m := model.NewChartSelectModel(ir.QueryID, p.AppName, p.Candidates)
		a.chartSelectModel = &m
		return a, tea.Batch(a.listenStream(), m.Init())
	case stream.KindDeployConfirm:
		var plan deploytypes.DeployPlan
		if err := json.Unmarshal(ir.Payload, &plan); err != nil {
			return a, a.listenStream()
		}
		a.deployConfirmStreamResp = ir.RespCh
		m := model.NewDeployConfirmModel(ir.QueryID, plan, a.width, a.height)
		a.deployConfirmModel = &m
		return a, tea.Batch(a.listenStream(), m.Init())
	default:
		return a, a.listenStream()
	}
}

func (a App) dispatchStreamEvent(ev stream.Event) (tea.Model, tea.Cmd) {
	switch e := ev.(type) {
	case stream.InteractionRequest:
		return a.dispatchInteractionRequest(e)
	case stream.StreamDone:
		a.chat, _ = a.chat.Update(e)
		a.running = false
		a.input.SetEnabled(true)
		a.persistAssistantMessage()
		return a, nil
	case stream.StreamErr:
		err := e.Err
		if err == nil {
			err = fmt.Errorf("stream ended")
		}
		a.chat, _ = a.chat.Update(stream.StreamErr{QueryID: e.QueryID, Err: err})
		a.running = false
		a.input.SetEnabled(true)
		return a, nil
	default:
		a.chat, _ = a.chat.Update(ev)
		return a, a.listenStream()
	}
}

// Update handles all messages.
func (a App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		a.width = msg.Width
		a.height = msg.Height
		a.doLayout()
		return a, nil

	case tea.KeyMsg:
		if cmd, handled := a.handleShortcut(msg); handled {
			return a, cmd
		}
		// Not a shortcut — fall through to sub-model routing below.

	case tea.MouseMsg:
		switch msg.Action {
		case tea.MouseActionPress:
			switch msg.Button {
			case tea.MouseButtonWheelUp:
				a.chat.ScrollUp(3)
				return a, nil
			case tea.MouseButtonWheelDown:
				a.chat.ScrollDown(3)
				return a, nil
			}
		}

	case stream.TeaMsg:
		return a.dispatchStreamEvent(msg.Event)

	case model.ChartSelectedMsg:
		if a.streamChartResp != nil {
			if msg.ChartInfo == nil {
				raw, _ := json.Marshal(stream.ChartSelectResponse{Cancelled: true})
				select {
				case a.streamChartResp <- raw:
				default:
				}
				a.streamChartResp = nil
				a.streamChartCandidates = nil
				a.chartSelectModel = nil
				return a, a.listenStream()
			}
			if msg.ChartInfo.Source == "manual" {
				a.chartSelectModel = nil
				m := model.NewManualChartInputModel(msg.QueryID)
				a.manualInputModel = &m
				return a, tea.Batch(a.listenStream(), m.Init())
			}
			idx := chartCandidateIndex(msg.ChartInfo, a.streamChartCandidates)
			raw, err := json.Marshal(stream.ChartSelectResponse{CandidateIndex: idx})
			if err == nil {
				select {
				case a.streamChartResp <- raw:
				default:
				}
			}
			a.streamChartResp = nil
			a.streamChartCandidates = nil
			a.chartSelectModel = nil
			return a, a.listenStream()
		}
		a.chartSelectModel = nil
		return a, a.listenStream()

	case model.ManualChartInputDoneMsg:
		if a.streamChartResp != nil {
			var r stream.ChartSelectResponse
			if msg.ChartInfo == nil {
				r = stream.ChartSelectResponse{Cancelled: true}
			} else {
				r = stream.ChartSelectResponse{
					UseManualChart:  true,
					ManualRepoURL:   msg.ChartInfo.RepoURL,
					ManualChartName: msg.ChartInfo.ChartName,
				}
			}
			raw, err := json.Marshal(r)
			if err == nil {
				select {
				case a.streamChartResp <- raw:
				default:
				}
			}
			a.streamChartResp = nil
			a.streamChartCandidates = nil
		}
		a.manualInputModel = nil
		return a, a.listenStream()

	case model.DeployConfirmDoneMsg:
		if a.deployConfirmStreamResp != nil {
			raw, err := json.Marshal(stream.DeployConfirmResponse{
				Action:     msg.Decision.Action,
				Values:     msg.Decision.Values,
				Correction: msg.Decision.Correction,
			})
			if err == nil {
				select {
				case a.deployConfirmStreamResp <- raw:
				default:
				}
			}
			a.deployConfirmStreamResp = nil
		}
		a.deployConfirmModel = nil
		return a, a.listenStream()

	case spinner.TickMsg:
		var cmd tea.Cmd
		a.chat, cmd = a.chat.Update(msg)
		if cmd != nil {
			return a, cmd
		}
		return a, nil

	case model.SubmitMsg:
		return a.handleSubmit(msg.Value)

	case model.ConfirmDoneMsg:
		a.confirm = nil
		return a, a.listenStream()
	}

	// Route remaining messages (including unmatched KeyMsg) to sub-models
	var cmd tea.Cmd
	if a.deployConfirmModel != nil {
		updated, dcCmd := a.deployConfirmModel.Update(msg)
		*a.deployConfirmModel = updated
		return a, dcCmd
	}
	if a.chartSelectModel != nil {
		updated, csCmd := a.chartSelectModel.Update(msg)
		*a.chartSelectModel = updated
		return a, csCmd
	}
	if a.manualInputModel != nil {
		updated, miCmd := a.manualInputModel.Update(msg)
		*a.manualInputModel = updated
		return a, miCmd
	}
	if a.confirm != nil {
		*a.confirm, cmd = a.confirm.Update(msg)
		return a, cmd
	}
	switch a.focus {
	case focusSidebar:
		a.sidebar, cmd = a.sidebar.Update(msg)
	case focusInput:
		a.input, cmd = a.input.Update(msg)
	}
	return a, cmd
}

// handleShortcut checks for global keyboard shortcuts. Returns (cmd, true) if
// the key was handled, (nil, false) otherwise so the key can be forwarded to
// the focused sub-model.
func (a App) handleShortcut(msg tea.KeyMsg) (tea.Cmd, bool) {
	// 如果 deploy TUI 组件 active，不拦截任何方向键/快捷键，
	// 让按键事件透传到子模型的 Update() 方法。
	if a.chartSelectModel != nil || a.deployConfirmModel != nil || a.manualInputModel != nil || a.confirm != nil {
		return nil, false
	}

	switch msg.Type {
	case tea.KeyCtrlC:
		if a.running {
			if a.cancelFn != nil {
				a.cancelFn()
			}
			a.running = false
			a.active.InterruptedQuery = a.lastUserMessage()
			a.input.SetEnabled(true)
			return nil, true
		}
		return tea.Quit, true

	case tea.KeyCtrlN:
		a.newSession()
		return nil, true

	case tea.KeyCtrlL:
		a.active.Messages = nil
		a.chat = model.NewChatModel(a.width-tuistyles.SidebarWidth, a.height-5)
		return nil, true

	case tea.KeyTab:
		if a.showSidebar {
			if a.focus == focusInput {
				a.focus = focusSidebar
				a.input.SetEnabled(false)
			} else {
				a.focus = focusInput
				a.input.SetEnabled(true)
			}
		}
		return nil, true

	case tea.KeyUp:
		if a.focus == focusInput {
			a.chat.ScrollUp(1)
			return nil, true
		}
	case tea.KeyDown:
		if a.focus == focusInput {
			a.chat.ScrollDown(1)
			return nil, true
		}
	case tea.KeyPgUp:
		if a.focus == focusInput {
			a.chat.ScrollUp(a.height - 2)
			return nil, true
		}
	case tea.KeyPgDown:
		if a.focus == focusInput {
			a.chat.ScrollDown(a.height - 2)
			return nil, true
		}

	}

	return nil, false
}

func (a *App) handleSubmit(text string) (tea.Model, tea.Cmd) {
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
	}

	a.chat.AddUserMessage(text)
	a.active.Messages = append(a.active.Messages, session.Message{
		Role:      "user",
		Content:   text,
		Timestamp: a.active.UpdatedAt,
	})

	// Start streaming
	a.running = true
	a.input.SetEnabled(false)

	a.querySeq++
	queryID := fmt.Sprintf("q-%d", a.querySeq)
	a.eventCh = make(chan stream.Event, 64)
	ctx, cancel := context.WithCancel(context.Background())
	a.cancelFn = cancel

	routerAgent := a.routerAgent
	eventCh := a.eventCh
	go func() {
		defer func() {
			if r := recover(); r != nil {
				err := fmt.Errorf("agent panic: %v", r)
				select {
				case eventCh <- stream.StreamErr{QueryID: queryID, Err: err}:
				default:
				}
			}
		}()
		_ = routerAgent.HandleQueryStream(ctx, text, queryID, eventCh)
	}()

	return a, a.listenStream()
}

func (a *App) newSession() {
	a.saveSession()
	a.active = session.New()
	a.chat = model.NewChatModel(a.width-tuistyles.SidebarWidth, a.height-5)
	if !containsSession(a.sessions, a.active.ID) {
		a.sessions = append([]*session.Session{a.active}, a.sessions...)
	}
	a.sidebar.SetSessions(a.sessions)
}

func (a *App) loadSession(s *session.Session) {
	a.saveSession()
	a.active = s
	a.chat.SetMessages(s.Messages)
	a.sidebar.SetSessions(a.sessions)
}

func (a *App) saveSession() {
	if a.store == nil || a.active == nil {
		return
	}
	a.active.Messages = a.chat.AllMessages()
	_ = a.store.Save(a.active)
}

func (a *App) persistAssistantMessage() {
	if a.store == nil || a.active == nil {
		return
	}
	a.active.Messages = a.chat.AllMessages()
	_ = a.store.Save(a.active)

	// Reload sessions list so sidebar shows the update
	recent, err := a.store.LoadRecent(20)
	if err == nil {
		a.sessions = recent
		a.sidebar.SetSessions(a.sessions)
	}
}

func (a *App) lastUserMessage() string {
	msgs := a.chat.AllMessages()
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role == "user" {
			return msgs[i].Content
		}
	}
	return ""
}

func (a *App) doLayout() {
	a.showSidebar = a.width >= 100

	sidebarW := 0
	if a.showSidebar {
		sidebarW = tuistyles.SidebarWidth
	}
	chatW := a.width - sidebarW
	inputH := 5
	chatH := a.height - inputH

	a.sidebar.SetSessions(a.sessions)
	a.sidebar.SetHeight(a.height)
	a.sidebar.SetVisible(a.showSidebar)
	a.chat.SetSize(chatW, chatH)
	a.input.SetWidth(chatW)
}

// View renders the full TUI.
func (a App) View() string {
	// Deploy TUI 组件优先级最高（覆盖主界面）
	if a.deployConfirmModel != nil {
		return a.deployConfirmModel.View()
	}
	if a.chartSelectModel != nil {
		return a.chartSelectModel.View()
	}
	if a.manualInputModel != nil {
		return a.manualInputModel.View()
	}
	if a.confirm != nil {
		return a.confirm.View()
	}

	inputView := a.input.View()
	rightPanel := a.chat.View() + "\n" + inputView

	if a.showSidebar {
		return a.sidebar.View() + rightPanel
	}
	return rightPanel
}

// Run starts the bubbletea program.
func Run(k8sClient *k8s.Client, llmClient *llm.Client, log *zap.Logger, maxSteps int, supervisorCfg supervisor.Config) error {
	app, err := NewApp(k8sClient, llmClient, log, maxSteps, supervisorCfg)
	if err != nil {
		return err
	}

	// Initialise layout defaults before first render
	app.width = 120
	app.height = 40
	app.showSidebar = true
	sidebarW := tuistyles.SidebarWidth
	app.sidebar = model.NewSidebarModel()
	app.sidebar.SetSessions(app.sessions)
	app.sidebar.SetHeight(app.height)
	app.chat = model.NewChatModel(app.width-sidebarW, app.height-5)
	app.input = model.NewInputModel()
	app.input.SetWidth(app.width - sidebarW)

	p := tea.NewProgram(app, tea.WithAltScreen(), tea.WithMouseCellMotion())
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
