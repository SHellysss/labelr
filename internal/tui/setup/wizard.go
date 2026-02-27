package setup

import (
	"fmt"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/pankajbeniwal/labelr/internal/config"
	"github.com/pankajbeniwal/labelr/internal/db"
	"github.com/pankajbeniwal/labelr/internal/service"
	"github.com/pankajbeniwal/labelr/internal/tui"
)

// stepID identifies a step in the wizard.
type stepID int

const (
	stepGmail stepID = iota
	stepAI
	stepValidate
	stepLabels
	stepFinish
	stepCount // sentinel — total number of steps
)

var stepNames = map[stepID]string{
	stepGmail:    "Gmail",
	stepAI:       "AI Provider",
	stepValidate: "Validate",
	stepLabels:   "Labels",
	stepFinish:   "Finish",
}

// Wizard manages the multi-step setup flow.
type Wizard struct {
	current stepID
	steps   map[stepID]Step
	cfg     *config.Config
	store   *db.Store
	mgr     service.Manager
	width   int
	height  int
}

// Step is the interface each wizard step implements.
type Step interface {
	Init() tea.Cmd
	Update(msg tea.Msg) (Step, tea.Cmd)
	View() string
	HelpKeys() []key.Binding
	CanGoBack() bool
	Done() bool
}

// Deps holds shared dependencies passed to each step.
type Deps struct {
	Cfg   *config.Config
	Store *db.Store
	Mgr   service.Manager
}

func NewWizard() (*Wizard, error) {
	cfgPath := config.DefaultPath()
	cfg, err := config.Load(cfgPath)
	if err != nil {
		// First time — create empty config
		cfg = &config.Config{
			PollInterval: 60,
		}
	}

	store, err := db.Open(config.DBPath())
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}

	mgr := service.Detect()

	deps := &Deps{Cfg: cfg, Store: store, Mgr: mgr}

	w := &Wizard{
		current: stepGmail,
		cfg:     cfg,
		store:   store,
		mgr:     mgr,
		steps:   make(map[stepID]Step),
	}

	// Initialize all steps
	w.steps[stepGmail] = newGmailStep(deps)
	w.steps[stepAI] = newAIStep(deps)
	w.steps[stepValidate] = newValidateStep(deps)
	w.steps[stepLabels] = newLabelsStep(deps)
	w.steps[stepFinish] = newFinishStep(deps)

	return w, nil
}

func (w *Wizard) Title() string { return "setup" }

func (w *Wizard) HelpKeys() []key.Binding {
	step := w.steps[w.current]
	keys := step.HelpKeys()
	if step.CanGoBack() {
		keys = append([]key.Binding{tui.KeyBack}, keys...)
	}
	return keys
}

func (w *Wizard) Init() tea.Cmd {
	return w.steps[w.current].Init()
}

func (w *Wizard) Update(msg tea.Msg) (tui.View, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		w.width = msg.Width
		w.height = msg.Height

	case tea.KeyMsg:
		// Handle back navigation
		if key.Matches(msg, tui.KeyBack) {
			step := w.steps[w.current]
			if step.CanGoBack() && w.current > 0 {
				w.current--
				return w, w.steps[w.current].Init()
			}
		}
	}

	// Delegate to current step
	step := w.steps[w.current]
	updatedStep, cmd := step.Update(msg)
	w.steps[w.current] = updatedStep

	// Check if step completed — advance to next
	if updatedStep.Done() {
		if w.current < stepCount-1 {
			w.current++
			return w, tea.Batch(cmd, w.steps[w.current].Init())
		}
		// Last step done — save config and quit
		config.Save(config.DefaultPath(), w.cfg)
		w.store.Close()
		return w, tea.Quit
	}

	return w, cmd
}

func (w *Wizard) View() string {
	// Step progress indicator
	progress := w.renderProgress()

	// Current step content
	content := w.steps[w.current].View()

	return progress + "\n\n" + content
}

func (w *Wizard) renderProgress() string {
	stepNum := int(w.current) + 1
	total := int(stepCount)
	stepName := stepNames[w.current]

	// "Step 2 of 5 · AI Provider"
	header := fmt.Sprintf("  Step %d of %d · %s", stepNum, total, stepName)
	headerStyled := lipgloss.NewStyle().Bold(true).Render(header)

	// Progress bar
	barWidth := 40
	if w.width > 0 && w.width-6 < barWidth {
		barWidth = w.width - 6
	}
	filled := barWidth * stepNum / total
	bar := ""
	for i := 0; i < barWidth; i++ {
		if i < filled {
			bar += tui.SuccessStyle.Render("━")
		} else {
			bar += tui.DimStyle.Render("─")
		}
	}

	return headerStyled + "\n  " + bar
}

// SpinnerStep is a helper for steps that show a spinner during async work.
type SpinnerStep struct {
	spinner spinner.Model
	title   string
	done    bool
	err     error
}

func newSpinnerStep(title string) SpinnerStep {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(tui.ColorGreen)
	return SpinnerStep{spinner: s, title: title}
}

func (s *SpinnerStep) SpinnerView() string {
	if s.err != nil {
		return "  " + tui.ErrorStyle.Render("✗ "+s.err.Error())
	}
	if s.done {
		return "  " + tui.SuccessStyle.Render("✓ "+s.title)
	}
	return fmt.Sprintf("  %s %s", s.spinner.View(), s.title)
}
