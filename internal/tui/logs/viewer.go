package logs

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/Pankaj3112/labelr/internal/tui"
)

const tailPollInterval = 500 * time.Millisecond

const maxRetryDelay = 5 * time.Second

type tailMsg struct {
	lines []string
	file  *os.File // non-nil only on initial open
}

type tailErrMsg struct {
	err error
}

type retryOpenMsg struct{}

type Viewer struct {
	filePath   string
	viewport   viewport.Model
	entries    []LogEntry
	file       *os.File
	autoScroll bool
	filter     string // "" = all, "ERROR", "WARN"
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
		// Set the file handle if this is the initial open
		if msg.file != nil {
			if v.file != nil {
				v.file.Close()
			}
			v.file = msg.file
		}
		for _, line := range msg.lines {
			v.entries = append(v.entries, ParseLine(line))
		}
		v.rebuildContent()
		if v.autoScroll {
			v.viewport.GotoBottom()
		}
		return v, v.pollTail()

	case tailErrMsg:
		// File might have been rotated; retry after a delay to avoid tight loops
		if v.file != nil {
			v.file.Close()
			v.file = nil
		}
		return v, tea.Tick(maxRetryDelay, func(t time.Time) tea.Msg {
			return retryOpenMsg{}
		})

	case retryOpenMsg:
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

		// Return the file handle via message so it's set on the main goroutine
		return tailMsg{lines: lines, file: f}
	}
}

func (v *Viewer) pollTail() tea.Cmd {
	// Capture the file handle on the main goroutine to avoid races
	f := v.file
	return tea.Tick(tailPollInterval, func(t time.Time) tea.Msg {
		if f == nil {
			return tailErrMsg{err: fmt.Errorf("file closed")}
		}

		var lines []string
		reader := bufio.NewReader(f)
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
