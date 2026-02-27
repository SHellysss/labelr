package status

import (
	"fmt"
	"time"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/pankajbeniwal/labelr/internal/config"
	"github.com/pankajbeniwal/labelr/internal/db"
	"github.com/pankajbeniwal/labelr/internal/service"
	"github.com/pankajbeniwal/labelr/internal/tui"
)

const refreshInterval = 5 * time.Second

type tickMsg time.Time

type Dashboard struct {
	cfg      *config.Config
	store    *db.Store
	mgr      service.Manager
	width    int
	height   int
	running  bool
	stats    *db.Stats
	lastPoll string
	err      error
}

func New() (*Dashboard, error) {
	cfg, err := config.Load(config.DefaultPath())
	if err != nil {
		return nil, fmt.Errorf("loading config: %w (run 'labelr setup' first)", err)
	}

	store, err := db.Open(config.DBPath())
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}

	mgr := service.Detect()

	return &Dashboard{
		cfg:   cfg,
		store: store,
		mgr:   mgr,
	}, nil
}

func (d *Dashboard) Title() string { return "status" }

func (d *Dashboard) HelpKeys() []key.Binding {
	return []key.Binding{tui.KeyRefresh, tui.KeyQuit}
}

func (d *Dashboard) Init() tea.Cmd {
	return tea.Batch(d.refresh(), tickCmd())
}

func (d *Dashboard) Update(msg tea.Msg) (tui.View, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		d.width = msg.Width
		d.height = msg.Height
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, tui.KeyQuit):
			d.store.Close()
			return d, tea.Quit
		case key.Matches(msg, tui.KeyRefresh):
			return d, d.refresh()
		}
	case tickMsg:
		return d, tea.Batch(d.refresh(), tickCmd())
	case refreshMsg:
		d.running = msg.running
		d.stats = msg.stats
		d.lastPoll = msg.lastPoll
		d.err = msg.err
	}
	return d, nil
}

func (d *Dashboard) View() string {
	if d.err != nil {
		return tui.ErrorStyle.Render("  Error: " + d.err.Error())
	}

	// Status info section
	statusDot := lipgloss.NewStyle().Foreground(tui.ColorGreen).Render("●")
	statusText := "Running"
	if !d.running {
		statusDot = lipgloss.NewStyle().Foreground(tui.ColorRed).Render("●")
		statusText = "Stopped"
	}

	bold := lipgloss.NewStyle().Bold(true)
	info := fmt.Sprintf("  %-12s %s %s\n", bold.Render("Daemon"), statusDot, statusText)
	info += fmt.Sprintf("  %-12s %s\n", bold.Render("Gmail"), d.cfg.Gmail.Email)
	info += fmt.Sprintf("  %-12s %s / %s\n", bold.Render("Provider"), d.cfg.AI.Provider, d.cfg.AI.Model)
	info += fmt.Sprintf("  %-12s every %ds\n", bold.Render("Polling"), d.cfg.PollInterval)

	// Stats cards
	cards := ""
	if d.stats != nil {
		pendingCard := d.renderCard("Pending", fmt.Sprintf("%d", d.stats.Pending), tui.ColorYellow)
		labeledCard := d.renderCard("Labeled", fmt.Sprintf("%d", d.stats.Labeled), tui.ColorGreen)
		failedCard := d.renderCard("Failed", fmt.Sprintf("%d", d.stats.Failed), tui.ColorRed)
		cards = "\n" + lipgloss.JoinHorizontal(lipgloss.Top, "  ", pendingCard, "  ", labeledCard, "  ", failedCard)
	}

	// Last poll
	pollInfo := ""
	if d.lastPoll != "" {
		pollInfo = fmt.Sprintf("\n\n  Last poll: %s    Refreshes: %ds", d.lastPoll, int(refreshInterval.Seconds()))
	}

	return info + cards + pollInfo
}

func (d *Dashboard) renderCard(label, value string, color lipgloss.Color) string {
	style := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(color).
		Padding(0, 2).
		Width(14).
		Align(lipgloss.Center)

	title := lipgloss.NewStyle().Faint(true).Render(label)
	num := lipgloss.NewStyle().Foreground(color).Bold(true).Render(value)
	return style.Render(title + "\n" + num)
}

// Messages

type refreshMsg struct {
	running  bool
	stats    *db.Stats
	lastPoll string
	err      error
}

func (d *Dashboard) refresh() tea.Cmd {
	return func() tea.Msg {
		running := false
		if d.mgr != nil {
			running, _ = d.mgr.IsRunning()
		}

		stats, err := d.store.Stats()
		if err != nil {
			return refreshMsg{err: err}
		}

		lastPoll := ""
		if lp, err := d.store.GetState("last_poll_time"); err == nil {
			if t, err := time.Parse(time.RFC3339, lp); err == nil {
				lastPoll = relativeTime(t)
			} else {
				lastPoll = lp
			}
		}

		return refreshMsg{running: running, stats: stats, lastPoll: lastPoll}
	}
}

func tickCmd() tea.Cmd {
	return tea.Tick(refreshInterval, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func relativeTime(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
}
