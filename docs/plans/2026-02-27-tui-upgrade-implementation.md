# TUI Upgrade Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Replace labelr's sequential-prompt CLI with a unified Bubble Tea TUI — shared app shell with header/footer, four upgraded commands (setup, status, logs, sync).

**Architecture:** A shared `ShellModel` wraps each command's view with a branded header and context-sensitive help footer. Each command implements a `View` interface. Views handle their own state and delegate to Bubble Tea/Bubbles components (viewport, progress, spinner). The shell handles global keys (Ctrl+C) and window resize.

**Tech Stack:** charmbracelet/bubbletea v1.3.10, charmbracelet/bubbles v1.0.0 (viewport, progress, spinner, help, key), charmbracelet/lipgloss v1.1.0, charmbracelet/huh v0.8.0 (embedded forms in setup wizard). All already in go.mod.

**Implementation order:** Infrastructure → Status (simplest, validates shell) → Sync (simple state machine) → Logs (viewport + file watching) → Setup (most complex).

---

### Task 1: TUI Infrastructure (Shell, Theme, Keys)

**Files:**
- Create: `internal/tui/theme.go`
- Create: `internal/tui/keys.go`
- Create: `internal/tui/shell.go`

**Step 1: Create theme.go**

```go
// internal/tui/theme.go
package tui

import "github.com/charmbracelet/lipgloss"

// Colors — same palette as ui/style.go for consistency.
var (
	ColorGreen  = lipgloss.Color("2")
	ColorRed    = lipgloss.Color("1")
	ColorYellow = lipgloss.Color("3")
	ColorDim    = lipgloss.Color("240")
	ColorAccent = lipgloss.Color("6") // cyan for borders/accents
)

// Shared styles used across all TUI views.
var (
	HeaderStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorGreen)

	FooterStyle = lipgloss.NewStyle().
			Faint(true)

	TitleStyle = lipgloss.NewStyle().
			Bold(true)

	DimStyle = lipgloss.NewStyle().
			Faint(true)

	SuccessStyle = lipgloss.NewStyle().
			Foreground(ColorGreen)

	ErrorStyle = lipgloss.NewStyle().
			Foreground(ColorRed)

	WarnStyle = lipgloss.NewStyle().
			Foreground(ColorYellow)

	CardBorder = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorDim).
			Padding(0, 1)

	ProgressBarColor    = ColorGreen
	ProgressBarBgColor  = lipgloss.Color("237")
)

// ChromeHeight is the vertical space used by the shell header + footer.
// Header: 1 line (● labelr cmd) + 1 blank line = 2
// Footer: 1 blank line + 1 help line = 2
const ChromeHeight = 4

// RenderHeader returns the branded header line for a command.
func RenderHeader(title string, width int) string {
	dot := lipgloss.NewStyle().Foreground(ColorGreen).Render("●")
	text := lipgloss.NewStyle().Bold(true).Render("labelr " + title)
	header := "  " + dot + " " + text
	return lipgloss.NewStyle().Width(width).Render(header)
}
```

**Step 2: Create keys.go**

```go
// internal/tui/keys.go
package tui

import "github.com/charmbracelet/bubbles/key"

// Global key bindings shared across views.
var (
	KeyQuit = key.NewBinding(
		key.WithKeys("q"),
		key.WithHelp("q", "quit"),
	)
	KeyForceQuit = key.NewBinding(
		key.WithKeys("ctrl+c"),
	)
	KeyUp = key.NewBinding(
		key.WithKeys("up", "k"),
		key.WithHelp("↑/k", "up"),
	)
	KeyDown = key.NewBinding(
		key.WithKeys("down", "j"),
		key.WithHelp("↓/j", "down"),
	)
	KeyEnter = key.NewBinding(
		key.WithKeys("enter"),
		key.WithHelp("enter", "select"),
	)
	KeyBack = key.NewBinding(
		key.WithKeys("shift+tab", "esc"),
		key.WithHelp("esc", "back"),
	)
	KeyRefresh = key.NewBinding(
		key.WithKeys("r"),
		key.WithHelp("r", "refresh"),
	)
	KeyFilter = key.NewBinding(
		key.WithKeys("f"),
		key.WithHelp("f", "filter"),
	)
	KeySearch = key.NewBinding(
		key.WithKeys("/"),
		key.WithHelp("/", "search"),
	)
	KeyBottom = key.NewBinding(
		key.WithKeys("G"),
		key.WithHelp("G", "bottom"),
	)
)

// RenderHelp produces a single-line help string from key bindings.
// Format: "↑/k up • enter select • q quit"
func RenderHelp(keys []key.Binding, width int) string {
	var parts []string
	for _, k := range keys {
		h := k.Help()
		if h.Key == "" {
			continue
		}
		parts = append(parts, h.Key+" "+h.Desc)
	}
	joined := ""
	for i, p := range parts {
		if i > 0 {
			joined += " • "
		}
		joined += p
	}
	return FooterStyle.Width(width).Render("  " + joined)
}
```

**Step 3: Create shell.go**

```go
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

	return lipgloss.JoinVertical(lipgloss.Left,
		header,
		"",
		content,
		"",
		footer,
	)
}

// Run creates and runs a Bubble Tea program with the shell wrapping the view.
// This is the main entry point for TUI commands.
func Run(v View) error {
	p := tea.NewProgram(NewShell(v), tea.WithAltScreen())
	_, err := p.Run()
	return err
}
```

**Step 4: Verify it compiles**

Run: `cd /Users/pankajbeniwal/Code/labelr && go build ./internal/tui/...`
Expected: No errors.

**Step 5: Commit**

```bash
git add internal/tui/
git commit -m "feat: add shared TUI infrastructure (shell, theme, keys)"
```

---

### Task 2: Status Dashboard

**Files:**
- Create: `internal/tui/status/dashboard.go`
- Modify: `internal/cli/status.go`

**Step 1: Create dashboard.go**

```go
// internal/tui/status/dashboard.go
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
```

**Step 2: Wire status.go to use TUI**

Replace the contents of `internal/cli/status.go`:

```go
package cli

import (
	"github.com/pankajbeniwal/labelr/internal/tui"
	"github.com/pankajbeniwal/labelr/internal/tui/status"
	"github.com/spf13/cobra"
)

func NewStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show daemon status and queue stats",
		RunE:  runStatus,
	}
}

func runStatus(cmd *cobra.Command, args []string) error {
	dashboard, err := status.New()
	if err != nil {
		return err
	}
	return tui.Run(dashboard)
}
```

**Step 3: Build and verify**

Run: `cd /Users/pankajbeniwal/Code/labelr && go build ./...`
Expected: No errors.

**Step 4: Manual test**

Run: `go run ./cmd/labelr status`
Expected: Full-screen TUI with header "● labelr status", daemon info, stats cards, help footer. Auto-refreshes every 5s. Press 'q' to quit, 'r' to refresh.

**Step 5: Commit**

```bash
git add internal/tui/status/ internal/cli/status.go
git commit -m "feat: replace status command with live TUI dashboard"
```

---

### Task 3: Sync Progress View

**Files:**
- Create: `internal/tui/sync/view.go`
- Modify: `internal/cli/sync.go`

**Step 1: Create sync/view.go**

```go
// internal/tui/sync/view.go
package sync

import (
	"context"
	"fmt"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/pankajbeniwal/labelr/internal/db"
	"github.com/pankajbeniwal/labelr/internal/gmail"
	"github.com/pankajbeniwal/labelr/internal/tui"
)

type phase int

const (
	phaseFetching phase = iota
	phaseConfirm
	phaseQueuing
	phaseDone
)

type fetchDoneMsg struct {
	msgs []struct{ ID, ThreadID string }
	err  error
}

type queueDoneMsg struct {
	count int
	err   error
}

type SyncView struct {
	phase    phase
	lastStr  string
	estimate int64
	client   *gmail.Client
	store    *db.Store
	spinner  spinner.Model
	progress progress.Model
	msgs     []struct{ ID, ThreadID string }
	cursor   int // 0 = Yes, 1 = No
	queued   int
	err      error
	width    int
	height   int
}

func New(lastStr string, estimate int64, client *gmail.Client, store *db.Store) *SyncView {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(tui.ColorGreen)

	p := progress.New(progress.WithDefaultGradient())

	return &SyncView{
		phase:    phaseFetching,
		lastStr:  lastStr,
		estimate: estimate,
		client:   client,
		store:    store,
		spinner:  s,
		progress: p,
	}
}

func (v *SyncView) Title() string { return "sync" }

func (v *SyncView) HelpKeys() []key.Binding {
	switch v.phase {
	case phaseConfirm:
		return []key.Binding{tui.KeyUp, tui.KeyDown, tui.KeyEnter, tui.KeyQuit}
	case phaseDone:
		return []key.Binding{tui.KeyQuit}
	default:
		return []key.Binding{tui.KeyQuit}
	}
}

func (v *SyncView) Init() tea.Cmd {
	return tea.Batch(v.spinner.Tick, v.fetchEmails())
}

func (v *SyncView) Update(msg tea.Msg) (tui.View, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		v.width = msg.Width
		v.height = msg.Height
		v.progress.Width = msg.Width - 8
		if v.progress.Width > 60 {
			v.progress.Width = 60
		}

	case tea.KeyMsg:
		switch {
		case key.Matches(msg, tui.KeyQuit):
			v.store.Close()
			return v, tea.Quit
		}

		if v.phase == phaseConfirm {
			switch msg.String() {
			case "up", "k":
				v.cursor = 0
			case "down", "j":
				v.cursor = 1
			case "enter":
				if v.cursor == 1 {
					// User chose No
					v.store.Close()
					return v, tea.Quit
				}
				// User chose Yes — start queuing
				v.phase = phaseQueuing
				return v, v.queueEmails()
			}
		}

	case spinner.TickMsg:
		var cmd tea.Cmd
		v.spinner, cmd = v.spinner.Update(msg)
		return v, cmd

	case progress.FrameMsg:
		m, cmd := v.progress.Update(msg)
		v.progress = m.(progress.Model)
		return v, cmd

	case fetchDoneMsg:
		if msg.err != nil {
			v.err = msg.err
			v.phase = phaseDone
			return v, nil
		}
		v.msgs = msg.msgs
		if len(v.msgs) == 0 {
			v.phase = phaseDone
			return v, nil
		}
		v.phase = phaseConfirm

	case queueDoneMsg:
		if msg.err != nil {
			v.err = msg.err
		}
		v.queued = msg.count
		v.phase = phaseDone
	}

	return v, nil
}

func (v *SyncView) View() string {
	switch v.phase {
	case phaseFetching:
		return fmt.Sprintf("  %s Fetching emails from the last %s...\n\n  %s",
			v.spinner.View(),
			v.lastStr,
			v.progress.ViewAs(0),
		)

	case phaseConfirm:
		yes := "    Yes"
		no := "    No"
		if v.cursor == 0 {
			yes = "  " + tui.SuccessStyle.Render("● ") + "Yes"
		} else {
			no = "  " + tui.SuccessStyle.Render("● ") + "No"
		}
		return fmt.Sprintf("  Found %s emails.\n\n  Queue these for labeling?\n\n%s\n%s",
			lipgloss.NewStyle().Bold(true).Render(fmt.Sprintf("%d", len(v.msgs))),
			yes, no,
		)

	case phaseQueuing:
		return fmt.Sprintf("  %s Queuing emails for labeling...\n\n  %s",
			v.spinner.View(),
			v.progress.ViewAs(0.5),
		)

	case phaseDone:
		if v.err != nil {
			return "  " + tui.ErrorStyle.Render("✗ "+v.err.Error())
		}
		if len(v.msgs) == 0 {
			return "  " + tui.DimStyle.Render("No emails found in this time range.")
		}
		return fmt.Sprintf("  %s %d emails queued\n\n  %s",
			tui.SuccessStyle.Render("✓"),
			v.queued,
			tui.DimStyle.Render("The daemon will process these. Press q to exit."),
		)
	}
	return ""
}

func (v *SyncView) fetchEmails() tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		msgs, err := v.client.ListRecentMessages(ctx, v.estimate)
		return fetchDoneMsg{msgs: msgs, err: err}
	}
}

func (v *SyncView) queueEmails() tea.Cmd {
	return func() tea.Msg {
		count := 0
		for _, m := range v.msgs {
			if err := v.store.InsertMessage(m.ID, m.ThreadID); err != nil {
				return queueDoneMsg{count: count, err: err}
			}
			count++
		}
		return queueDoneMsg{count: count}
	}
}
```

**Step 2: Modify sync.go to launch TUI**

Replace `internal/cli/sync.go`:

```go
package cli

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/pankajbeniwal/labelr/internal/config"
	"github.com/pankajbeniwal/labelr/internal/db"
	gmailpkg "github.com/pankajbeniwal/labelr/internal/gmail"
	"github.com/pankajbeniwal/labelr/internal/tui"
	tuisync "github.com/pankajbeniwal/labelr/internal/tui/sync"
	"github.com/spf13/cobra"
)

func NewSyncCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "sync",
		Short: "One-time backlog scan",
		Long:  "Fetch and queue recent emails for labeling. Example: labelr sync --last 7d",
		RunE:  runSync,
	}
	cmd.Flags().String("last", "7d", "How far back to sync (e.g., 1d, 7d, 30d)")
	return cmd
}

func runSync(cmd *cobra.Command, args []string) error {
	lastStr, _ := cmd.Flags().GetString("last")
	duration, err := parseDuration(lastStr)
	if err != nil {
		return fmt.Errorf("invalid duration %q: %w", lastStr, err)
	}

	_, err = config.Load(config.DefaultPath())
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	ts, err := gmailpkg.TokenSource(config.CredentialsPath())
	if err != nil {
		return fmt.Errorf("creating token source: %w", err)
	}

	ctx := context.Background()
	client, err := gmailpkg.NewClient(ctx, ts)
	if err != nil {
		return fmt.Errorf("creating Gmail client: %w", err)
	}

	store, err := db.Open(config.DBPath())
	if err != nil {
		return fmt.Errorf("opening database: %w", err)
	}

	// Estimate: ~50 emails per day
	estimate := int64(duration.Hours()/24) * 50
	if estimate < 10 {
		estimate = 10
	}
	if estimate > 500 {
		estimate = 500
	}

	view := tuisync.New(lastStr, estimate, client, store)
	return tui.Run(view)
}

func parseDuration(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)
	if len(s) < 2 {
		return 0, fmt.Errorf("too short")
	}
	unit := s[len(s)-1]
	numStr := s[:len(s)-1]
	num, err := strconv.Atoi(numStr)
	if err != nil {
		return 0, err
	}
	switch unit {
	case 'd':
		return time.Duration(num) * 24 * time.Hour, nil
	case 'h':
		return time.Duration(num) * time.Hour, nil
	default:
		return 0, fmt.Errorf("unknown unit %c (use d or h)", unit)
	}
}
```

**Step 3: Build and verify**

Run: `cd /Users/pankajbeniwal/Code/labelr && go build ./...`
Expected: No errors.

**Step 4: Manual test**

Run: `go run ./cmd/labelr sync --last 1d`
Expected: Full-screen TUI with spinner during fetch, confirmation prompt with arrow-key selection, progress during queuing, success message. Press 'q' at any point to quit.

**Step 5: Commit**

```bash
git add internal/tui/sync/ internal/cli/sync.go
git commit -m "feat: replace sync command with TUI progress view"
```

---

### Task 4: Logs Viewer

**Files:**
- Create: `internal/tui/logs/parser.go`
- Create: `internal/tui/logs/parser_test.go`
- Create: `internal/tui/logs/viewer.go`
- Modify: `internal/cli/logs.go`

**Step 1: Write the failing test for log parser**

```go
// internal/tui/logs/parser_test.go
package logs

import "testing"

func TestParseLine(t *testing.T) {
	tests := []struct {
		input string
		want  LogEntry
	}{
		{
			input: "2026/02/27 14:23:01 INFO  Polling for new emails",
			want:  LogEntry{Time: "14:23:01", Level: "INFO", Message: "Polling for new emails"},
		},
		{
			input: "2026/02/27 14:23:04 ERROR Failed to classify message",
			want:  LogEntry{Time: "14:23:04", Level: "ERROR", Message: "Failed to classify message"},
		},
		{
			input: "2026/02/27 14:23:05 DEBUG Detailed debug info here",
			want:  LogEntry{Time: "14:23:05", Level: "DEBUG", Message: "Detailed debug info here"},
		},
		{
			input: "some unstructured line",
			want:  LogEntry{Time: "", Level: "", Message: "some unstructured line"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := ParseLine(tt.input)
			if got.Time != tt.want.Time {
				t.Errorf("Time: got %q, want %q", got.Time, tt.want.Time)
			}
			if got.Level != tt.want.Level {
				t.Errorf("Level: got %q, want %q", got.Level, tt.want.Level)
			}
			if got.Message != tt.want.Message {
				t.Errorf("Message: got %q, want %q", got.Message, tt.want.Message)
			}
		})
	}
}
```

**Step 2: Run test to verify it fails**

Run: `cd /Users/pankajbeniwal/Code/labelr && go test ./internal/tui/logs/ -v`
Expected: FAIL — `ParseLine` undefined.

**Step 3: Write the log parser**

```go
// internal/tui/logs/parser.go
package logs

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/pankajbeniwal/labelr/internal/tui"
)

// LogEntry is a parsed log line.
type LogEntry struct {
	Time    string
	Level   string
	Message string
}

// ParseLine parses a log line in the format: "2026/02/27 14:23:01 LEVEL  Message"
// The daemon logger uses log.LstdFlags which produces "YYYY/MM/DD HH:MM:SS" prefix,
// followed by the level (INFO/ERROR/DEBUG) added by our logger methods.
func ParseLine(line string) LogEntry {
	// Expected format: "2026/02/27 14:23:01 INFO  message text"
	// Positions:        0         1         2     3...
	parts := strings.Fields(line)
	if len(parts) < 3 {
		return LogEntry{Message: line}
	}

	// Check if first part looks like a date (YYYY/MM/DD)
	if len(parts[0]) != 10 || parts[0][4] != '/' {
		return LogEntry{Message: line}
	}

	timeStr := parts[1] // HH:MM:SS
	level := parts[2]   // INFO, ERROR, DEBUG

	// Validate level
	switch level {
	case "INFO", "ERROR", "DEBUG":
		// valid
	default:
		return LogEntry{Time: timeStr, Message: strings.Join(parts[2:], " ")}
	}

	msg := strings.Join(parts[3:], " ")
	return LogEntry{Time: timeStr, Level: level, Message: msg}
}

// Render returns a styled string for the log entry.
func (e LogEntry) Render(width int) string {
	if e.Time == "" && e.Level == "" {
		return tui.DimStyle.Render(e.Message)
	}

	timeStyle := tui.DimStyle
	var levelStyle lipgloss.Style

	switch e.Level {
	case "ERROR":
		levelStyle = tui.ErrorStyle.Bold(true)
	case "WARN":
		levelStyle = tui.WarnStyle.Bold(true)
	default:
		levelStyle = tui.DimStyle
	}

	timePart := timeStyle.Render(e.Time)
	levelPart := levelStyle.Width(6).Render(e.Level)
	return "  " + timePart + "  " + levelPart + " " + e.Message
}
```

**Step 4: Run test to verify it passes**

Run: `cd /Users/pankajbeniwal/Code/labelr && go test ./internal/tui/logs/ -v`
Expected: PASS — all test cases pass.

**Step 5: Write the log viewer**

```go
// internal/tui/logs/viewer.go
package logs

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/pankajbeniwal/labelr/internal/tui"
	"time"
)

const tailPollInterval = 500 * time.Millisecond

type tailMsg struct {
	lines []string
}

type tailErrMsg struct {
	err error
}

type Viewer struct {
	filePath   string
	viewport   viewport.Model
	entries    []LogEntry
	file       *os.File
	autoScroll bool
	filter     string // "" = all, "WARN", "ERROR"
	width      int
	height     int
	ready      bool
}

func NewViewer(filePath string) *Viewer {
	return &Viewer{
		filePath:   filePath,
		autoScroll: true,
	}
}

func (v *Viewer) Title() string { return "logs" }

func (v *Viewer) HelpKeys() []key.Binding {
	return []key.Binding{tui.KeyUp, tui.KeyDown, tui.KeyFilter, tui.KeyBottom, tui.KeyQuit}
}

func (v *Viewer) Init() tea.Cmd {
	return v.openAndReadFile()
}

func (v *Viewer) Update(msg tea.Msg) (tui.View, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		v.width = msg.Width
		v.height = msg.Height
		contentHeight := msg.Height - tui.ChromeHeight
		if contentHeight < 1 {
			contentHeight = 1
		}
		if !v.ready {
			v.viewport = viewport.New(msg.Width, contentHeight)
			v.ready = true
		} else {
			v.viewport.Width = msg.Width
			v.viewport.Height = contentHeight
		}
		v.rebuildContent()

	case tea.KeyMsg:
		switch {
		case key.Matches(msg, tui.KeyQuit):
			if v.file != nil {
				v.file.Close()
			}
			return v, tea.Quit
		case key.Matches(msg, tui.KeyFilter):
			// Cycle filter: all → ERROR → WARN+ERROR → all
			switch v.filter {
			case "":
				v.filter = "ERROR"
			case "ERROR":
				v.filter = "WARN"
			default:
				v.filter = ""
			}
			v.rebuildContent()
			return v, nil
		case key.Matches(msg, tui.KeyBottom):
			v.viewport.GotoBottom()
			v.autoScroll = true
			return v, nil
		case msg.String() == "up" || msg.String() == "k" || msg.String() == "pgup":
			v.autoScroll = false
		}

	case tailMsg:
		for _, line := range msg.lines {
			v.entries = append(v.entries, ParseLine(line))
		}
		v.rebuildContent()
		if v.autoScroll {
			v.viewport.GotoBottom()
		}
		return v, v.pollTail()

	case tailErrMsg:
		// File might have been rotated, try reopening
		if v.file != nil {
			v.file.Close()
		}
		return v, v.openAndReadFile()
	}

	var cmd tea.Cmd
	v.viewport, cmd = v.viewport.Update(msg)
	return v, cmd
}

func (v *Viewer) View() string {
	if !v.ready {
		return "  Loading logs..."
	}

	filterIndicator := ""
	if v.filter != "" {
		filterIndicator = tui.WarnStyle.Render(fmt.Sprintf("  [filter: %s+]", v.filter))
	}

	scrollInfo := ""
	if !v.autoScroll {
		scrollInfo = tui.DimStyle.Render("  ── paused (press G to resume) ──")
	}

	return v.viewport.View() + filterIndicator + scrollInfo
}

func (v *Viewer) rebuildContent() {
	var lines []string
	for _, e := range v.entries {
		if v.filter != "" {
			// Show entries at or above filter level
			switch v.filter {
			case "ERROR":
				if e.Level != "ERROR" {
					continue
				}
			case "WARN":
				if e.Level != "ERROR" && e.Level != "WARN" {
					continue
				}
			}
		}
		lines = append(lines, e.Render(v.width))
	}
	v.viewport.SetContent(strings.Join(lines, "\n"))
}

func (v *Viewer) openAndReadFile() tea.Cmd {
	return func() tea.Msg {
		f, err := os.Open(v.filePath)
		if err != nil {
			return tailErrMsg{err: err}
		}

		// Read existing content
		var lines []string
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			lines = append(lines, scanner.Text())
		}

		// Keep last 1000 lines to avoid memory issues
		if len(lines) > 1000 {
			lines = lines[len(lines)-1000:]
		}

		v.file = f
		return tailMsg{lines: lines}
	}
}

func (v *Viewer) pollTail() tea.Cmd {
	return tea.Tick(tailPollInterval, func(t time.Time) tea.Msg {
		if v.file == nil {
			return tailErrMsg{err: fmt.Errorf("file closed")}
		}

		var lines []string
		reader := bufio.NewReader(v.file)
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				if err == io.EOF {
					break
				}
				return tailErrMsg{err: err}
			}
			line = strings.TrimRight(line, "\n\r")
			if line != "" {
				lines = append(lines, line)
			}
		}
		return tailMsg{lines: lines}
	})
}
```

**Step 6: Wire logs.go to use TUI**

Replace `internal/cli/logs.go`:

```go
package cli

import (
	"fmt"
	"os"

	"github.com/pankajbeniwal/labelr/internal/config"
	"github.com/pankajbeniwal/labelr/internal/tui"
	"github.com/pankajbeniwal/labelr/internal/tui/logs"
	"github.com/spf13/cobra"
)

func NewLogsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "logs",
		Short: "Tail the daemon log file",
		RunE:  runLogs,
	}
}

func runLogs(cmd *cobra.Command, args []string) error {
	logPath := config.LogPath()
	if _, err := os.Stat(logPath); os.IsNotExist(err) {
		return fmt.Errorf("no log file found at %s", logPath)
	}

	viewer := logs.NewViewer(logPath)
	return tui.Run(viewer)
}
```

**Step 7: Build and run tests**

Run: `cd /Users/pankajbeniwal/Code/labelr && go test ./internal/tui/logs/ -v && go build ./...`
Expected: Tests pass, build succeeds.

**Step 8: Manual test**

Run: `go run ./cmd/labelr logs`
Expected: Full-screen log viewer. Colored log levels. Scrollable with arrow keys. Press 'f' to cycle filter (all → ERROR → WARN+ → all). Press 'G' to jump to bottom. Auto-scrolls when new lines appear. Press 'q' to quit.

**Step 9: Commit**

```bash
git add internal/tui/logs/ internal/cli/logs.go
git commit -m "feat: replace logs command with scrollable TUI viewer"
```

---

### Task 5: Setup Wizard

This is the largest task. The setup wizard converts the existing 683-line sequential setup flow into a multi-step Bubble Tea wizard with step navigation and progress indicator.

**Files:**
- Create: `internal/tui/setup/wizard.go`
- Create: `internal/tui/setup/steps.go`
- Modify: `internal/cli/setup.go`

**Step 1: Create wizard.go — the step manager**

```go
// internal/tui/setup/wizard.go
package setup

import (
	"fmt"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/pankajbeniwal/labelr/internal/config"
	"github.com/pankajbeniwal/labelr/internal/db"
	gmailpkg "github.com/pankajbeniwal/labelr/internal/gmail"
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
```

**Step 2: Create steps.go — individual step implementations**

This file contains the implementation for each wizard step. The steps use huh forms embedded in Bubble Tea for interactive input, and spinner patterns for async operations.

```go
// internal/tui/setup/steps.go
package setup

import (
	"context"
	"fmt"
	"os"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
	"github.com/pankajbeniwal/labelr/internal/ai"
	"github.com/pankajbeniwal/labelr/internal/config"
	gmailpkg "github.com/pankajbeniwal/labelr/internal/gmail"
	"github.com/pankajbeniwal/labelr/internal/tui"
)

// ──────────────────────────────────────────
// Step 1: Gmail OAuth
// ──────────────────────────────────────────

type gmailDoneMsg struct {
	email string
	err   error
}

type gmailStep struct {
	deps    *Deps
	spinner SpinnerStep
	email   string
	done    bool
	err     error
}

func newGmailStep(deps *Deps) *gmailStep {
	return &gmailStep{
		deps:    deps,
		spinner: newSpinnerStep("Connecting to Gmail..."),
	}
}

func (s *gmailStep) CanGoBack() bool { return false }
func (s *gmailStep) Done() bool      { return s.done }

func (s *gmailStep) HelpKeys() []key.Binding {
	return []key.Binding{tui.KeyQuit}
}

func (s *gmailStep) Init() tea.Cmd {
	// Check if already authenticated
	if s.deps.Cfg.Gmail.Email != "" {
		ts, err := gmailpkg.TokenSource(config.CredentialsPath())
		if err == nil {
			ctx := context.Background()
			client, err := gmailpkg.NewClient(ctx, ts)
			if err == nil {
				email, err := client.GetUserEmail(ctx)
				if err == nil {
					s.email = email
					s.done = true
					return nil
				}
			}
		}
	}
	return tea.Batch(s.spinner.spinner.Tick, s.authenticate())
}

func (s *gmailStep) Update(msg tea.Msg) (Step, tea.Cmd) {
	switch msg := msg.(type) {
	case spinner.TickMsg:
		var cmd tea.Cmd
		s.spinner.spinner, cmd = s.spinner.spinner.Update(msg)
		return s, cmd
	case gmailDoneMsg:
		if msg.err != nil {
			s.err = msg.err
			s.spinner.err = msg.err
			return s, nil
		}
		s.email = msg.email
		s.deps.Cfg.Gmail.Email = msg.email
		s.spinner.done = true
		s.done = true
	case tea.KeyMsg:
		if key.Matches(msg, tui.KeyQuit) {
			return s, tea.Quit
		}
	}
	return s, nil
}

func (s *gmailStep) View() string {
	if s.done {
		return fmt.Sprintf("  %s Connected as %s",
			tui.SuccessStyle.Render("✓"),
			lipgloss.NewStyle().Bold(true).Render(s.email))
	}
	return s.spinner.SpinnerView() + "\n\n" + tui.DimStyle.Render("  A browser window will open for Google sign-in...")
}

func (s *gmailStep) authenticate() tea.Cmd {
	return func() tea.Msg {
		ts, err := gmailpkg.Authenticate(config.CredentialsPath())
		if err != nil {
			return gmailDoneMsg{err: fmt.Errorf("authentication failed: %w", err)}
		}

		ctx := context.Background()
		client, err := gmailpkg.NewClient(ctx, ts)
		if err != nil {
			return gmailDoneMsg{err: fmt.Errorf("creating client: %w", err)}
		}

		email, err := client.GetUserEmail(ctx)
		if err != nil {
			return gmailDoneMsg{err: fmt.Errorf("getting email: %w", err)}
		}

		return gmailDoneMsg{email: email}
	}
}

// ──────────────────────────────────────────
// Step 2: AI Provider Selection
// ──────────────────────────────────────────

type modelsFetchedMsg struct {
	models []string
	err    error
}

type aiStep struct {
	deps     *Deps
	phase    int // 0=provider, 1=model-fetch, 2=model-select, 3=apikey
	form     *huh.Form
	spinner  SpinnerStep
	provider string
	model    string
	apiKey   string
	models   []string
	done     bool
}

func newAIStep(deps *Deps) *aiStep {
	return &aiStep{
		deps: deps,
	}
}

func (s *aiStep) CanGoBack() bool { return true }
func (s *aiStep) Done() bool      { return s.done }

func (s *aiStep) HelpKeys() []key.Binding {
	return []key.Binding{tui.KeyUp, tui.KeyDown, tui.KeyEnter}
}

func (s *aiStep) Init() tea.Cmd {
	s.phase = 0
	s.done = false
	providerNames := ai.ProviderNamesOrdered()
	options := make([]huh.Option[string], len(providerNames))
	for i, name := range providerNames {
		options[i] = huh.NewOption(name, name)
	}

	s.form = huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Choose your AI provider").
				Options(options...).
				Value(&s.provider),
		),
	).WithShowHelp(true)

	return s.form.Init()
}

func (s *aiStep) Update(msg tea.Msg) (Step, tea.Cmd) {
	switch s.phase {
	case 0: // Provider selection form
		form, cmd := s.form.Update(msg)
		if f, ok := form.(*huh.Form); ok {
			s.form = f
		}
		if s.form.State == huh.StateCompleted {
			s.phase = 1
			s.spinner = newSpinnerStep("Fetching available models...")
			return s, tea.Batch(s.spinner.spinner.Tick, s.fetchModels())
		}
		return s, cmd

	case 1: // Fetching models
		switch msg := msg.(type) {
		case spinner.TickMsg:
			var cmd tea.Cmd
			s.spinner.spinner, cmd = s.spinner.spinner.Update(msg)
			return s, cmd
		case modelsFetchedMsg:
			if msg.err != nil || len(msg.models) == 0 {
				// Fall back to text input
				s.phase = 2
				s.form = huh.NewForm(
					huh.NewGroup(
						huh.NewInput().
							Title("Enter model name").
							Value(&s.model),
					),
				).WithShowHelp(true)
				return s, s.form.Init()
			}
			s.models = msg.models
			s.phase = 2
			options := make([]huh.Option[string], 0, len(msg.models)+1)
			for _, m := range msg.models {
				options = append(options, huh.NewOption(m, m))
			}
			options = append(options, huh.NewOption("Other (custom)", "__other__"))
			s.form = huh.NewForm(
				huh.NewGroup(
					huh.NewSelect[string]().
						Title("Which model?").
						Options(options...).
						Value(&s.model),
				),
			).WithShowHelp(true)
			return s, s.form.Init()
		}
		return s, nil

	case 2: // Model selection
		form, cmd := s.form.Update(msg)
		if f, ok := form.(*huh.Form); ok {
			s.form = f
		}
		if s.form.State == huh.StateCompleted {
			if s.model == "__other__" {
				s.model = ""
				s.form = huh.NewForm(
					huh.NewGroup(
						huh.NewInput().
							Title("Enter model name").
							Value(&s.model),
					),
				).WithShowHelp(true)
				return s, s.form.Init()
			}
			// Move to API key (skip for ollama)
			if s.provider == "ollama" {
				s.deps.Cfg.AI.Provider = s.provider
				s.deps.Cfg.AI.Model = s.model
				s.deps.Cfg.AI.BaseURL = ai.ProviderBaseURL(s.provider)
				s.done = true
				return s, nil
			}
			s.phase = 3
			// Check for env var
			envKey := ai.EnvKeyForProvider(s.provider)
			if envKey != "" {
				if val := os.Getenv(envKey); val != "" {
					s.apiKey = val
					s.deps.Cfg.AI.Provider = s.provider
					s.deps.Cfg.AI.Model = s.model
					s.deps.Cfg.AI.APIKey = s.apiKey
					s.deps.Cfg.AI.BaseURL = ai.ProviderBaseURL(s.provider)
					s.done = true
					return s, nil
				}
			}
			s.form = huh.NewForm(
				huh.NewGroup(
					huh.NewInput().
						Title("API key").
						EchoMode(huh.EchoModePassword).
						Value(&s.apiKey),
				),
			).WithShowHelp(true)
			return s, s.form.Init()
		}
		return s, cmd

	case 3: // API key input
		form, cmd := s.form.Update(msg)
		if f, ok := form.(*huh.Form); ok {
			s.form = f
		}
		if s.form.State == huh.StateCompleted {
			s.deps.Cfg.AI.Provider = s.provider
			s.deps.Cfg.AI.Model = s.model
			s.deps.Cfg.AI.APIKey = s.apiKey
			s.deps.Cfg.AI.BaseURL = ai.ProviderBaseURL(s.provider)
			s.done = true
		}
		return s, cmd
	}

	return s, nil
}

func (s *aiStep) View() string {
	switch s.phase {
	case 0:
		return s.form.View()
	case 1:
		return s.spinner.SpinnerView()
	case 2, 3:
		providerLine := fmt.Sprintf("  Provider: %s", lipgloss.NewStyle().Bold(true).Render(s.provider))
		if s.phase == 3 && s.model != "" {
			providerLine += fmt.Sprintf("  Model: %s", lipgloss.NewStyle().Bold(true).Render(s.model))
		}
		return providerLine + "\n\n" + s.form.View()
	}
	return ""
}

func (s *aiStep) fetchModels() tea.Cmd {
	return func() tea.Msg {
		if s.provider == "ollama" {
			models, err := ai.FetchOllamaModels()
			return modelsFetchedMsg{models: models, err: err}
		}
		models, err := ai.FetchModelsForProvider(s.provider)
		return modelsFetchedMsg{models: models, err: err}
	}
}

// ──────────────────────────────────────────
// Step 3: Validate AI Connection
// ──────────────────────────────────────────

type validateDoneMsg struct {
	err error
}

type validateStep struct {
	deps    *Deps
	spinner SpinnerStep
	done    bool
	err     error
	retry   bool
}

func newValidateStep(deps *Deps) *validateStep {
	return &validateStep{
		deps:    deps,
		spinner: newSpinnerStep("Validating AI connection..."),
	}
}

func (s *validateStep) CanGoBack() bool { return true }
func (s *validateStep) Done() bool      { return s.done }

func (s *validateStep) HelpKeys() []key.Binding {
	if s.err != nil {
		return []key.Binding{tui.KeyEnter, tui.KeyBack, tui.KeyQuit}
	}
	return []key.Binding{tui.KeyQuit}
}

func (s *validateStep) Init() tea.Cmd {
	s.done = false
	s.err = nil
	s.retry = false
	s.spinner = newSpinnerStep("Validating AI connection...")
	return tea.Batch(s.spinner.spinner.Tick, s.validate())
}

func (s *validateStep) Update(msg tea.Msg) (Step, tea.Cmd) {
	switch msg := msg.(type) {
	case spinner.TickMsg:
		var cmd tea.Cmd
		s.spinner.spinner, cmd = s.spinner.spinner.Update(msg)
		return s, cmd
	case validateDoneMsg:
		if msg.err != nil {
			s.err = msg.err
			s.spinner.err = msg.err
			return s, nil
		}
		s.spinner.done = true
		s.done = true
	case tea.KeyMsg:
		if s.err != nil && key.Matches(msg, tui.KeyEnter) {
			// Retry
			return s, s.Init()
		}
		if key.Matches(msg, tui.KeyQuit) {
			return s, tea.Quit
		}
	}
	return s, nil
}

func (s *validateStep) View() string {
	view := s.spinner.SpinnerView()
	if s.err != nil {
		view += "\n\n" + tui.DimStyle.Render("  Press enter to retry or esc to go back")
	}
	return view
}

func (s *validateStep) validate() tea.Cmd {
	return func() tea.Msg {
		cfg := s.deps.Cfg
		apiKey := cfg.ResolveAPIKey()
		classifier := ai.NewClassifier(apiKey, cfg.AI.BaseURL, cfg.AI.Model, cfg.Labels)
		err := classifier.ValidateConnection()
		return validateDoneMsg{err: err}
	}
}

// ──────────────────────────────────────────
// Step 4: Labels
// ──────────────────────────────────────────

type labelsStep struct {
	deps     *Deps
	form     *huh.Form
	phase    int // 0=multiselect, 1=add-custom-loop, 2=done
	selected []string
	adding   bool
	newLabel string
	done     bool
}

func newLabelsStep(deps *Deps) *labelsStep {
	return &labelsStep{deps: deps}
}

func (s *labelsStep) CanGoBack() bool { return true }
func (s *labelsStep) Done() bool      { return s.done }

func (s *labelsStep) HelpKeys() []key.Binding {
	return []key.Binding{tui.KeyUp, tui.KeyDown, tui.KeyEnter}
}

func (s *labelsStep) Init() tea.Cmd {
	s.phase = 0
	s.done = false

	defaults := config.DefaultLabels()
	options := make([]huh.Option[string], len(defaults))
	for i, l := range defaults {
		options[i] = huh.NewOption(l.Name, l.Name).Selected(true)
	}

	s.form = huh.NewForm(
		huh.NewGroup(
			huh.NewMultiSelect[string]().
				Title("Select labels to use").
				Options(options...).
				Value(&s.selected),
		),
	).WithShowHelp(true)

	return s.form.Init()
}

func (s *labelsStep) Update(msg tea.Msg) (Step, tea.Cmd) {
	switch s.phase {
	case 0: // Multi-select defaults
		form, cmd := s.form.Update(msg)
		if f, ok := form.(*huh.Form); ok {
			s.form = f
		}
		if s.form.State == huh.StateCompleted {
			s.phase = 1
			return s, s.showAddCustom()
		}
		return s, cmd

	case 1: // Add custom labels loop
		form, cmd := s.form.Update(msg)
		if f, ok := form.(*huh.Form); ok {
			s.form = f
		}
		if s.form.State == huh.StateCompleted {
			if s.adding {
				// They want to add a custom label
				s.form = huh.NewForm(
					huh.NewGroup(
						huh.NewInput().
							Title("Label name").
							Value(&s.newLabel),
					),
				).WithShowHelp(true)
				s.adding = false
				return s, s.form.Init()
			}
			if s.newLabel != "" {
				// They just entered a custom label name
				s.selected = append(s.selected, s.newLabel)
				s.newLabel = ""
				return s, s.showAddCustom()
			}
			// They chose not to add more — done
			s.finalize()
		}
		return s, cmd
	}

	return s, nil
}

func (s *labelsStep) showAddCustom() tea.Cmd {
	s.adding = true
	s.form = huh.NewForm(
		huh.NewGroup(
			huh.NewConfirm().
				Title("Add a custom label?").
				Value(&s.adding),
		),
	).WithShowHelp(true)
	return s.form.Init()
}

func (s *labelsStep) finalize() {
	// Build final label list
	defaults := config.DefaultLabels()
	defaultMap := make(map[string]config.Label)
	for _, l := range defaults {
		defaultMap[l.Name] = l
	}

	var labels []config.Label
	for _, name := range s.selected {
		if l, ok := defaultMap[name]; ok {
			labels = append(labels, l)
		} else {
			labels = append(labels, config.Label{Name: name, Description: "Custom label"})
		}
	}
	s.deps.Cfg.Labels = labels
	s.done = true
}

func (s *labelsStep) View() string {
	selectedInfo := ""
	if len(s.selected) > 0 && s.phase == 1 {
		selectedInfo = tui.DimStyle.Render(fmt.Sprintf("  Selected: %d labels", len(s.selected))) + "\n\n"
	}
	return selectedInfo + s.form.View()
}

// ──────────────────────────────────────────
// Step 5: Finish (create labels, test run, start daemon)
// ──────────────────────────────────────────

type labelCreateDoneMsg struct {
	err error
}

type daemonStartDoneMsg struct {
	err error
}

type finishStep struct {
	deps        *Deps
	spinner     SpinnerStep
	phase       int // 0=creating-labels, 1=offer-test, 2=starting-daemon, 3=done
	testConfirm bool
	form        *huh.Form
	done        bool
}

func newFinishStep(deps *Deps) *finishStep {
	return &finishStep{
		deps: deps,
	}
}

func (s *finishStep) CanGoBack() bool { return s.phase == 0 }
func (s *finishStep) Done() bool      { return s.done }

func (s *finishStep) HelpKeys() []key.Binding {
	if s.phase == 1 {
		return []key.Binding{tui.KeyEnter}
	}
	return []key.Binding{tui.KeyQuit}
}

func (s *finishStep) Init() tea.Cmd {
	s.phase = 0
	s.done = false
	s.spinner = newSpinnerStep("Creating labels in Gmail...")
	return tea.Batch(s.spinner.spinner.Tick, s.createLabels())
}

func (s *finishStep) Update(msg tea.Msg) (Step, tea.Cmd) {
	switch msg := msg.(type) {
	case spinner.TickMsg:
		var cmd tea.Cmd
		s.spinner.spinner, cmd = s.spinner.spinner.Update(msg)
		return s, cmd

	case labelCreateDoneMsg:
		if msg.err != nil {
			s.spinner.err = msg.err
			return s, nil
		}
		s.spinner.done = true
		// Save config now
		config.Save(config.DefaultPath(), s.deps.Cfg)

		// Move to start daemon
		s.phase = 2
		s.spinner = newSpinnerStep("Starting background service...")
		return s, tea.Batch(s.spinner.spinner.Tick, s.startDaemon())

	case daemonStartDoneMsg:
		if msg.err != nil {
			s.spinner.err = msg.err
		} else {
			s.spinner.done = true
		}
		s.done = true
		return s, nil

	default:
		// Delegate to form if in offer-test phase
		if s.form != nil && s.phase == 1 {
			form, cmd := s.form.Update(msg)
			if f, ok := form.(*huh.Form); ok {
				s.form = f
			}
			if s.form.State == huh.StateCompleted {
				s.phase = 2
				s.spinner = newSpinnerStep("Starting background service...")
				return s, tea.Batch(s.spinner.spinner.Tick, s.startDaemon())
			}
			return s, cmd
		}
	}
	return s, nil
}

func (s *finishStep) View() string {
	switch s.phase {
	case 0:
		return s.spinner.SpinnerView()
	case 1:
		return "  " + tui.SuccessStyle.Render("✓ Labels created") + "\n\n" + s.form.View()
	case 2:
		labelDone := "  " + tui.SuccessStyle.Render("✓ Labels created")
		return labelDone + "\n" + s.spinner.SpinnerView()
	default:
		lines := "  " + tui.SuccessStyle.Render("✓ Labels created")
		if s.spinner.done {
			lines += "\n  " + tui.SuccessStyle.Render("✓ Background service started")
		} else if s.spinner.err != nil {
			lines += "\n  " + tui.ErrorStyle.Render("✗ "+s.spinner.err.Error())
		}
		lines += "\n\n  " + lipgloss.NewStyle().Bold(true).Render("Setup complete!") +
			"\n  " + tui.DimStyle.Render("Use 'labelr status' to monitor.")
		return lines
	}
}

func (s *finishStep) createLabels() tea.Cmd {
	return func() tea.Msg {
		ts, err := gmailpkg.TokenSource(config.CredentialsPath())
		if err != nil {
			return labelCreateDoneMsg{err: err}
		}

		ctx := context.Background()
		client, err := gmailpkg.NewClient(ctx, ts)
		if err != nil {
			return labelCreateDoneMsg{err: err}
		}

		for i, label := range s.deps.Cfg.Labels {
			bgColor, textColor := gmailpkg.ColorForLabel(label.Name, i)
			gmailID, err := client.CreateLabel(ctx, label.Name, bgColor, textColor)
			if err != nil {
				return labelCreateDoneMsg{err: fmt.Errorf("creating label %q: %w", label.Name, err)}
			}
			s.deps.Store.SetLabelMappingWithColor(label.Name, gmailID, bgColor, textColor)
		}

		// Store initial history ID
		profile, err := client.GetProfile(ctx)
		if err == nil && profile.HistoryId > 0 {
			s.deps.Store.SetState("history_id", fmt.Sprintf("%d", profile.HistoryId))
		}

		return labelCreateDoneMsg{}
	}
}

func (s *finishStep) startDaemon() tea.Cmd {
	return func() tea.Msg {
		mgr := s.deps.Mgr
		if mgr == nil {
			return daemonStartDoneMsg{err: fmt.Errorf("unsupported OS for background service")}
		}

		binaryPath, err := os.Executable()
		if err != nil {
			return daemonStartDoneMsg{err: err}
		}

		if err := mgr.Install(binaryPath); err != nil {
			return daemonStartDoneMsg{err: fmt.Errorf("installing service: %w", err)}
		}

		if err := mgr.Start(); err != nil {
			return daemonStartDoneMsg{err: fmt.Errorf("starting service: %w", err)}
		}

		return daemonStartDoneMsg{}
	}
}
```

**Step 3: Check if ai package exports needed functions**

The steps.go file references `ai.ProviderBaseURL()` and `ai.EnvKeyForProvider()`. Check if these exist in the ai package. If not, add them.

Read `internal/ai/providers.go` and check for these functions. If missing, add:

```go
// In internal/ai/providers.go — add if not present:

// ProviderBaseURL returns the API base URL for a provider.
func ProviderBaseURL(provider string) string {
	p, ok := providers[provider]
	if !ok {
		return ""
	}
	return p.BaseURL
}

// EnvKeyForProvider returns the environment variable name for a provider's API key.
func EnvKeyForProvider(provider string) string {
	p, ok := providers[provider]
	if !ok {
		return ""
	}
	return p.EnvKey
}
```

Also check if `gmailpkg.Authenticate()`, `gmailpkg.ColorForLabel()`, `client.GetProfile()` exist with those signatures. Adapt the step code to match actual function signatures in the codebase.

**Step 4: Wire setup.go to use TUI for first-time setup**

Modify `internal/cli/setup.go` — replace the `runFirstTimeSetup` call path with the TUI wizard, keep `runReconfigure` as-is for now (it can be upgraded later):

```go
// At the top of runSetup, change the first-time path:
func runSetup(cmd *cobra.Command, args []string) error {
	cfgPath := config.DefaultPath()
	cfg, err := config.Load(cfgPath)

	if err != nil || cfg.AI.Provider == "" {
		// First-time setup — launch TUI wizard
		wizard, err := setup.NewWizard()
		if err != nil {
			return err
		}
		return tui.Run(wizard)
	}

	// Reconfigure — keep existing flow for now
	return runReconfigure(cfg, cfgPath)
}
```

Add imports at the top of setup.go:
```go
import (
	"github.com/pankajbeniwal/labelr/internal/tui"
	"github.com/pankajbeniwal/labelr/internal/tui/setup"
)
```

**Step 5: Build and verify**

Run: `cd /Users/pankajbeniwal/Code/labelr && go build ./...`
Expected: No errors. Fix any compilation issues (missing functions, wrong signatures, etc.)

**Step 6: Manual test**

Run: `go run ./cmd/labelr setup`
Expected: Full-screen wizard with step progress bar. Step through Gmail → AI → Validate → Labels → Finish. Each step shows relevant content. Back navigation works (esc/shift+tab). Spinners animate during async work. Final step creates labels and starts daemon.

**Step 7: Commit**

```bash
git add internal/tui/setup/ internal/cli/setup.go
git commit -m "feat: replace first-time setup with multi-step TUI wizard"
```

---

### Task 6: Final Polish and Cleanup

**Files:**
- Modify: `internal/cli/uninstall.go` (add WithShowHelp to confirm)
- Remove: unused imports from modified files
- Verify: all commands work end-to-end

**Step 1: Add help text to uninstall confirmation**

In `internal/cli/uninstall.go`, update the confirm prompt:

```go
huh.NewConfirm().
    Title("Keep your data (~/.labelr/)?").
    Value(&keepData).
    WithShowHelp(true).  // Add this line
    Run()
```

**Step 2: Run full test suite**

Run: `cd /Users/pankajbeniwal/Code/labelr && go test ./... -v`
Expected: All existing tests pass. New log parser tests pass.

**Step 3: Build final binary**

Run: `cd /Users/pankajbeniwal/Code/labelr && go build -o labelr ./cmd/labelr`
Expected: Binary builds successfully.

**Step 4: End-to-end smoke test**

Test each command:
```bash
./labelr status    # Should show live dashboard
./labelr logs      # Should show scrollable log viewer
./labelr sync --last 1d  # Should show fetch progress + confirmation
./labelr start     # Should print start message (no TUI)
./labelr stop      # Should print stop message (no TUI)
```

**Step 5: Clean up and commit**

Run: `go mod tidy` to clean up go.mod (bubbles/bubbletea should now be direct deps).

```bash
go mod tidy
git add -A
git commit -m "chore: final polish — help text on uninstall, tidy deps"
```

---

## Notes for Implementer

1. **API Version Awareness:** The project uses Bubble Tea v1.3.10 (not v2). Key differences:
   - Use `tea.KeyMsg` (not `tea.KeyPressMsg`)
   - Use `msg.String()` for key matching
   - Import from `github.com/charmbracelet/bubbletea` (not `charm.land/bubbletea/v2`)
   - Import bubbles from `github.com/charmbracelet/bubbles/...`

2. **huh Forms in Bubble Tea:** `huh.Form` implements `tea.Model`. You can embed it by calling `form.Init()`, `form.Update(msg)`, and `form.View()`. The `Update` returns `(tea.Model, tea.Cmd)` — type-assert back to `*huh.Form`.

3. **Function Signatures:** The step implementations reference functions from `ai`, `gmail`, and `config` packages. The exact signatures may differ from what's written here. Read the actual source files and adapt. Key functions to verify:
   - `gmailpkg.Authenticate()` — may be `gmailpkg.AuthenticateInteractive()` or similar
   - `gmailpkg.ColorForLabel()` — may be in `gmail/colors.go`
   - `ai.ProviderBaseURL()` — may need to be added
   - `ai.EnvKeyForProvider()` — may need to be added
   - `client.GetProfile()` — verify return type

4. **Error Handling in Steps:** If a step encounters an error, show it in the view and offer retry (enter) or back navigation (esc). Don't crash the TUI.

5. **Cleanup on Exit:** Each view that opens a database connection or file should close it when the user quits. Handle `tea.Quit` by closing resources.
