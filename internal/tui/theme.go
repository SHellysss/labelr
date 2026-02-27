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

	ProgressBarColor   = ColorGreen
	ProgressBarBgColor = lipgloss.Color("237")
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
