package status

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/Pankaj3112/labelr/internal/config"
	"github.com/Pankaj3112/labelr/internal/db"
	"github.com/Pankaj3112/labelr/internal/service"
	"github.com/Pankaj3112/labelr/internal/tui"
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
	activity []db.ActivityEntry
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
		d.activity = msg.activity
		d.lastPoll = msg.lastPoll
		d.err = msg.err
	}
	return d, nil
}

func (d *Dashboard) View() string {
	if d.err != nil {
		return tui.ErrorStyle.Render("  Error: " + d.err.Error())
	}

	var sb strings.Builder

	// Status line: ● Running   12 pending · 347 labeled · 3 failed
	statusDot := lipgloss.NewStyle().Foreground(tui.ColorGreen).Render("●")
	statusText := "Running"
	if !d.running {
		statusDot = lipgloss.NewStyle().Foreground(tui.ColorRed).Render("●")
		statusText = "Stopped"
	}

	sb.WriteString(fmt.Sprintf("  %s %s", statusDot, statusText))

	if d.stats != nil {
		pending := lipgloss.NewStyle().Foreground(tui.ColorYellow).Bold(true).Render(fmt.Sprintf("%d", d.stats.Pending))
		labeled := lipgloss.NewStyle().Foreground(tui.ColorGreen).Bold(true).Render(fmt.Sprintf("%d", d.stats.Labeled))
		failed := lipgloss.NewStyle().Foreground(tui.ColorRed).Bold(true).Render(fmt.Sprintf("%d", d.stats.Failed))
		sb.WriteString(fmt.Sprintf("   %s pending · %s labeled · %s failed", pending, labeled, failed))
	}

	if d.lastPoll != "" {
		sb.WriteString(tui.DimStyle.Render(fmt.Sprintf("   · polled %s", d.lastPoll)))
	}

	sb.WriteString("\n")

	// Activity feed
	sb.WriteString("\n")
	sb.WriteString(tui.DimStyle.Render("  Recent activity"))
	sb.WriteString("\n")

	// Separator line
	lineWidth := 50
	if d.width > 4 && d.width-4 < lineWidth {
		lineWidth = d.width - 4
	}
	sb.WriteString("  " + tui.DimStyle.Render(strings.Repeat("─", lineWidth)))
	sb.WriteString("\n")

	if len(d.activity) == 0 {
		sb.WriteString(tui.DimStyle.Render("  No activity yet. Queue emails with 'labelr sync'."))
		sb.WriteString("\n")
	} else {
		for _, entry := range d.activity {
			sb.WriteString(d.renderActivityEntry(entry, lineWidth))
			sb.WriteString("\n")
		}
	}

	// Footer: config info
	sb.WriteString("\n")
	sb.WriteString(tui.DimStyle.Render(fmt.Sprintf("  %s · %s / %s · every %ds",
		d.cfg.Gmail.Email, d.cfg.AI.Provider, d.cfg.AI.Model, d.cfg.PollInterval)))

	return sb.String()
}

func (d *Dashboard) renderActivityEntry(e db.ActivityEntry, maxWidth int) string {
	// Parse time for relative display
	timeStr := "  "
	if t, err := time.Parse("2006-01-02 15:04:05", e.ProcessedAt); err == nil {
		timeStr = relativeTime(t)
	}
	timeCol := tui.DimStyle.Render(fmt.Sprintf("  %-8s", timeStr))

	// Status icon
	var icon string
	if e.Status == "labeled" {
		icon = tui.SuccessStyle.Render("✓")
	} else {
		icon = tui.ErrorStyle.Render("✗")
	}

	// Subject (truncate if needed)
	subject := e.Subject
	if subject == "" {
		subject = "(no subject)"
	}
	maxSubjectLen := maxWidth - 30
	if maxSubjectLen < 10 {
		maxSubjectLen = 10
	}
	if len(subject) > maxSubjectLen {
		subject = subject[:maxSubjectLen-1] + "…"
	}
	subjectStr := fmt.Sprintf("%-*s", maxSubjectLen, subject)

	// Label or (failed)
	var labelStr string
	if e.Status == "labeled" && e.Label != "" {
		labelStr = tui.DimStyle.Render("→ ") + lipgloss.NewStyle().Bold(true).Render(e.Label)
	} else if e.Status == "failed" {
		labelStr = tui.ErrorStyle.Render("(failed)")
	}

	return fmt.Sprintf("%s %s %s  %s", timeCol, icon, subjectStr, labelStr)
}

// Messages

type refreshMsg struct {
	running  bool
	stats    *db.Stats
	activity []db.ActivityEntry
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

		activity, _ := d.store.RecentActivity(10)

		lastPoll := ""
		if lp, err := d.store.GetState("last_poll_time"); err == nil {
			if t, err := time.Parse(time.RFC3339, lp); err == nil {
				lastPoll = relativeTime(t)
			} else {
				lastPoll = lp
			}
		}

		return refreshMsg{running: running, stats: stats, activity: activity, lastPoll: lastPoll}
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
