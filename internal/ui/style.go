package ui

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"
)

var (
	green  = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	red    = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	yellow = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
	dim    = lipgloss.NewStyle().Faint(true)
	bold   = lipgloss.NewStyle().Bold(true)
)

// Success prints a green checkmark followed by the message.
func Success(msg string) {
	fmt.Printf("  %s %s\n", green.Render("✓"), msg)
}

// Error prints a red X followed by the message.
func Error(msg string) {
	fmt.Printf("  %s %s\n", red.Render("✗"), msg)
}

// Info prints an arrow followed by the message.
func Info(msg string) {
	fmt.Printf("  %s %s\n", dim.Render("→"), msg)
}

// Dim prints dimmed text.
func Dim(msg string) {
	fmt.Printf("  %s\n", dim.Render(msg))
}

// Bold prints bold text.
func Bold(msg string) {
	fmt.Printf("  %s\n", bold.Render(msg))
}

// BoldStr returns bold-rendered text without printing.
func BoldStr(msg string) string {
	return bold.Render(msg)
}

// Header prints a bold header with a blank line before it.
func Header(msg string) {
	fmt.Println()
	fmt.Printf("  %s\n", bold.Render(msg))
}

// KeyValue prints a key-value pair with the key bold and right-padded.
func KeyValue(key, value string) {
	fmt.Printf("  %-12s %s\n", bold.Render(key), value)
}

// StatusDot returns a colored dot: green if running, red if stopped.
func StatusDot(running bool) string {
	if running {
		return green.Render("●")
	}
	return red.Render("●")
}

// Green renders text in green.
func Green(msg string) string {
	return green.Render(msg)
}

// Red renders text in red.
func Red(msg string) string {
	return red.Render(msg)
}

// Yellow renders text in yellow.
func Yellow(msg string) string {
	return yellow.Render(msg)
}
