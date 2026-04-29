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
	SidebarWidth       = 24
	SidebarStyle       = lipgloss.NewStyle().Width(SidebarWidth).BorderRight(true).BorderStyle(lipgloss.NormalBorder()).BorderForeground(Secondary)
	SidebarItemStyle   = lipgloss.NewStyle().PaddingLeft(1)
	SidebarActiveStyle = lipgloss.NewStyle().PaddingLeft(1).Foreground(Primary).Bold(true)
	SidebarHeaderStyle = lipgloss.NewStyle().PaddingLeft(1).Foreground(Secondary).Bold(true)

	// Chat
	UserBubble      = lipgloss.NewStyle().Foreground(Primary).Bold(true)
	AssistantBubble = lipgloss.NewStyle()
	TimestampStyle  = lipgloss.NewStyle().Foreground(Secondary).Italic(true)

	// Progress card
	CardStyle   = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(Secondary).Padding(0, 1)
	CardDone    = lipgloss.NewStyle().Foreground(Success)
	CardRunning = lipgloss.NewStyle().Foreground(Warning)
	CardFailed  = lipgloss.NewStyle().Foreground(Error)

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
	ModalStyle      = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(1, 2).Width(60)
	ModalTitleStyle = lipgloss.NewStyle().Foreground(Warning).Bold(true)
	ModalHintStyle  = lipgloss.NewStyle().Foreground(Secondary)

	// Input bar
	InputStyle = lipgloss.NewStyle().BorderTop(true).BorderStyle(lipgloss.NormalBorder()).BorderForeground(Secondary)
)
