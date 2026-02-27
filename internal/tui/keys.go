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
