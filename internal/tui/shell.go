// internal/tui/shell.go
package tui

import (
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// View is the interface each command's TUI content must implement.
type View interface {
	Init() tea.Cmd
	Update(msg tea.Msg) (View, tea.Cmd)
	View() string
	HelpKeys() []key.Binding
	Title() string
}

// ShellModel wraps a View with a branded header and help footer.
type ShellModel struct {
	view   View
	width  int
	height int
}

// NewShell creates a shell wrapping the given view.
func NewShell(v View) ShellModel {
	return ShellModel{view: v}
}

func (m ShellModel) Init() tea.Cmd {
	return m.view.Init()
}

func (m ShellModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if key.Matches(msg, KeyForceQuit) {
			return m, tea.Quit
		}
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	}

	var cmd tea.Cmd
	m.view, cmd = m.view.Update(msg)
	return m, cmd
}

func (m ShellModel) View() string {
	if m.width == 0 {
		return "Loading..."
	}

	header := RenderHeader(m.view.Title(), m.width)
	content := m.view.View()
	footer := RenderHelp(m.view.HelpKeys(), m.width)

	return lipgloss.NewStyle().
		Width(m.width).
		Height(m.height).
		Render(lipgloss.JoinVertical(lipgloss.Left,
			header,
			"",
			content,
			"",
			footer,
		))
}

// Run creates and runs a Bubble Tea program with the shell wrapping the view.
// This is the main entry point for TUI commands.
func Run(v View) error {
	p := tea.NewProgram(NewShell(v), tea.WithAltScreen())
	_, err := p.Run()
	return err
}
