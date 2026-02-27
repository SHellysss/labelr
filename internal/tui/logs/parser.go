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
