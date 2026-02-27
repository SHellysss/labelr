package sync

import (
	"context"
	"fmt"
	"time"

	"github.com/charmbracelet/bubbles/key"
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
	msgs    []struct{ ID, ThreadID string }
	skipped int
	err     error
}

type queueDoneMsg struct {
	count int
	err   error
}

type SyncView struct {
	phase    phase
	lastStr  string
	duration time.Duration
	client   *gmail.Client
	store    *db.Store
	spinner  spinner.Model
	msgs     []struct{ ID, ThreadID string }
	skipped  int // already processed count
	cursor   int // 0 = Yes, 1 = No
	queued   int
	err      error
	width    int
	height   int
}

func New(lastStr string, duration time.Duration, client *gmail.Client, store *db.Store) *SyncView {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(tui.ColorGreen)

	return &SyncView{
		phase:    phaseFetching,
		lastStr:  lastStr,
		duration: duration,
		client:   client,
		store:    store,
		spinner:  s,
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

	case fetchDoneMsg:
		if msg.err != nil {
			v.err = msg.err
			v.phase = phaseDone
			return v, nil
		}
		v.msgs = msg.msgs
		v.skipped = msg.skipped
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
		return fmt.Sprintf("  %s Fetching emails from the last %s...",
			v.spinner.View(),
			v.lastStr,
		)

	case phaseConfirm:
		yes := "    Yes"
		no := "    No"
		if v.cursor == 0 {
			yes = "  " + tui.SuccessStyle.Render("● ") + "Yes"
		} else {
			no = "  " + tui.SuccessStyle.Render("● ") + "No"
		}
		summary := fmt.Sprintf("  Found %s new emails.",
			lipgloss.NewStyle().Bold(true).Render(fmt.Sprintf("%d", len(v.msgs))))
		if v.skipped > 0 {
			summary += tui.DimStyle.Render(fmt.Sprintf(" (%d already processed)", v.skipped))
		}
		return fmt.Sprintf("%s\n\n  Queue these for labeling?\n\n%s\n%s",
			summary, yes, no,
		)

	case phaseQueuing:
		return fmt.Sprintf("  %s Queuing %d emails for labeling...",
			v.spinner.View(),
			len(v.msgs),
		)

	case phaseDone:
		if v.err != nil {
			return "  " + tui.ErrorStyle.Render("✗ "+v.err.Error())
		}
		if len(v.msgs) == 0 {
			if v.skipped > 0 {
				return "  " + tui.DimStyle.Render(fmt.Sprintf("No new emails. All %d were already processed.", v.skipped))
			}
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
		afterEpoch := time.Now().Add(-v.duration).Unix()
		allMsgs, err := v.client.ListMessagesSince(ctx, afterEpoch)
		if err != nil {
			return fetchDoneMsg{err: err}
		}

		// Filter out messages already in the DB
		var newMsgs []struct{ ID, ThreadID string }
		skipped := 0
		for _, m := range allMsgs {
			if v.store.MessageExists(m.ID) {
				skipped++
			} else {
				newMsgs = append(newMsgs, m)
			}
		}

		return fetchDoneMsg{msgs: newMsgs, skipped: skipped, err: nil}
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
