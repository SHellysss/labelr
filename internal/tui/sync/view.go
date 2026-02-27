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
