package tui

import "charm.land/lipgloss/v2"

// Catppuccin Mocha, matching the house style. Colors are image/color values in Lip Gloss v2.
var (
	green  = lipgloss.Color("#a6e3a1")
	red    = lipgloss.Color("#f38ba8")
	yellow = lipgloss.Color("#f9e2af")
	blue   = lipgloss.Color("#89b4fa")
	mauve  = lipgloss.Color("#cba6f7")
	subtle = lipgloss.Color("#6c7086")

	titleStyle   = lipgloss.NewStyle().Bold(true).Foreground(mauve)
	repoStyle    = lipgloss.NewStyle().Bold(true).Foreground(blue)
	selStyle     = lipgloss.NewStyle().Foreground(green)
	keepStyle    = lipgloss.NewStyle().Foreground(subtle)
	cursorStyle  = lipgloss.NewStyle().Bold(true).Foreground(yellow)
	warnStyle    = lipgloss.NewStyle().Foreground(red)
	helpStyle    = lipgloss.NewStyle().Foreground(subtle)
	passStyle    = lipgloss.NewStyle().Foreground(green)
	failStyle    = lipgloss.NewStyle().Foreground(red)
	emphStyle    = lipgloss.NewStyle().Bold(true)
	sizeStyle    = lipgloss.NewStyle().Foreground(yellow)
	optOutStyle  = lipgloss.NewStyle().Foreground(subtle).Italic(true)
	statusStyles = map[string]lipgloss.Style{
		"merged":  lipgloss.NewStyle().Foreground(green),
		"empty":   lipgloss.NewStyle().Foreground(blue),
		"missing": lipgloss.NewStyle().Foreground(yellow),
		"gone":    lipgloss.NewStyle().Foreground(mauve),
	}
)
