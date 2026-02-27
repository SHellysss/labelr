package setup

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
	"github.com/pankajbeniwal/labelr/internal/config"
	"github.com/pankajbeniwal/labelr/internal/tui"
)

// ReconfigureView shows current config and a menu to change settings.
// When an option is selected, it returns a chosen action for the caller
// to execute outside the TUI (since sub-flows use blocking huh prompts).
type ReconfigureView struct {
	cfg    *config.Config
	form   *huh.Form
	choice string
	done   bool
	width  int
	height int
}

func NewReconfigureView(cfg *config.Config) *ReconfigureView {
	v := &ReconfigureView{cfg: cfg}
	v.buildForm()
	return v
}

func (v *ReconfigureView) buildForm() {
	v.choice = ""
	v.form = huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("What would you like to change?").
				Options(
					huh.NewOption("Nothing, exit", "none"),
					huh.NewOption("Gmail account", "gmail"),
					huh.NewOption("AI provider / model", "ai"),
					huh.NewOption("Just the model", "model"),
					huh.NewOption("Labels", "labels"),
					huh.NewOption("Poll interval", "poll"),
				).
				Value(&v.choice),
		),
	).WithShowHelp(true)
}

func (v *ReconfigureView) Title() string { return "setup" }

func (v *ReconfigureView) HelpKeys() []key.Binding {
	return []key.Binding{tui.KeyUp, tui.KeyDown, tui.KeyEnter, tui.KeyQuit}
}

func (v *ReconfigureView) Init() tea.Cmd {
	return v.form.Init()
}

func (v *ReconfigureView) Update(msg tea.Msg) (tui.View, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		v.width = msg.Width
		v.height = msg.Height
	case tea.KeyMsg:
		if key.Matches(msg, tui.KeyQuit) {
			return v, tea.Quit
		}
	}

	form, cmd := v.form.Update(msg)
	if f, ok := form.(*huh.Form); ok {
		v.form = f
	}

	if v.form.State == huh.StateCompleted {
		v.done = true
		return v, tea.Quit
	}

	return v, cmd
}

func (v *ReconfigureView) View() string {
	bold := lipgloss.NewStyle().Bold(true)

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("  %-12s %s\n", bold.Render("Gmail"), v.cfg.Gmail.Email))
	sb.WriteString(fmt.Sprintf("  %-12s %s / %s\n", bold.Render("Provider"), v.cfg.AI.Provider, v.cfg.AI.Model))
	sb.WriteString(fmt.Sprintf("  %-12s %d labels\n", bold.Render("Labels"), len(v.cfg.Labels)))
	sb.WriteString(fmt.Sprintf("  %-12s every %ds\n", bold.Render("Polling"), v.cfg.PollInterval))
	sb.WriteString("\n")
	sb.WriteString(v.form.View())

	return sb.String()
}

// Choice returns the selected menu option after the view exits.
func (v *ReconfigureView) Choice() string {
	return v.choice
}
