package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var (
	// Base Colors
	HeaderColor  = lipgloss.Color("63")  // Indigo
	SuccessColor = lipgloss.Color("41")  // Spring Green
	ErrorColor   = lipgloss.Color("196") // Red
	WarningColor = lipgloss.Color("214") // Orange
	InfoColor    = lipgloss.Color("39")  // Deep Sky Blue
	MutedColor   = lipgloss.Color("240") // Dark Gray

	// Core Styles
	HeaderStyle = lipgloss.NewStyle().
			Foreground(HeaderColor).
			Bold(true).
			Padding(0, 1).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(HeaderColor)

	SubheaderStyle = lipgloss.NewStyle().
			Foreground(HeaderColor).
			Bold(true).
			MarginTop(1)

	SuccessStyle = lipgloss.NewStyle().Foreground(SuccessColor).Bold(true)
	ErrorStyle   = lipgloss.NewStyle().Foreground(ErrorColor).Bold(true)
	WarningStyle = lipgloss.NewStyle().Foreground(WarningColor).Bold(true)
	InfoStyle    = lipgloss.NewStyle().Foreground(InfoColor)
	MutedStyle   = lipgloss.NewStyle().Foreground(MutedColor)

	// List/Item Styles
	BulletStyle = lipgloss.NewStyle().Foreground(HeaderColor).SetString("• ")
	ItemStyle   = lipgloss.NewStyle().PaddingLeft(2)

	// Box Styles
	SummaryBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(SuccessColor).
			Padding(0, 1).
			MarginTop(1).
			MarginBottom(1)
)

// PrintHeader prints a styled top-level header for a command.
func PrintHeader(title string) {
	fmt.Println(HeaderStyle.Render(title))
}

// PrintSubheader prints a styled section title.
func PrintSubheader(title string) {
	fmt.Println(SubheaderStyle.Render(title))
}

// PrintSuccess prints a success message.
func PrintSuccess(msg string) {
	fmt.Printf("%s %s\n", SuccessStyle.Render("✔"), msg)
}

// PrintError prints an error message.
func PrintError(msg string) {
	fmt.Printf("%s %s\n", ErrorStyle.Render("✖"), msg)
}

// PrintWarning prints a warning message.
func PrintWarning(msg string) {
	fmt.Printf("%s %s\n", WarningStyle.Render("⚠"), msg)
}

// PrintInfo prints an informational message.
func PrintInfo(msg string) {
	fmt.Printf("%s %s\n", InfoStyle.Render("ℹ"), msg)
}

// PrintBullet prints a bulleted item.
func PrintBullet(msg string) {
	fmt.Println(ItemStyle.Render(BulletStyle.String() + msg))
}

// PrintMuted prints muted helper text.
func PrintMuted(msg string) {
	fmt.Println(MutedStyle.Render(msg))
}

// PrintEmptyLine prints an empty line.
func PrintEmptyLine() {
	fmt.Println()
}

// FormatSummary formats a summary string into a styled box.
func FormatSummary(lines ...string) string {
	content := strings.Join(lines, "\n")
	return SummaryBoxStyle.Render(content)
}
