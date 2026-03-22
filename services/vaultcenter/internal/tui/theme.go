package tui

import "github.com/charmbracelet/lipgloss"

// One Dark color palette
var (
	colorBg        = lipgloss.Color("#282c34")
	colorFg        = lipgloss.Color("#abb2bf")
	colorRed       = lipgloss.Color("#e06c75")
	colorGreen     = lipgloss.Color("#98c379")
	colorYellow    = lipgloss.Color("#e5c07b")
	colorBlue      = lipgloss.Color("#61afef")
	colorCyan      = lipgloss.Color("#56b6c2")
	colorMagenta   = lipgloss.Color("#c678dd")
	colorDimFg     = lipgloss.Color("#5c6370")
	colorHighlight = lipgloss.Color("#3e4452")
)

var (
	styleTitle = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorCyan).
			MarginBottom(1)

	styleHeader = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorBlue)

	styleActive = lipgloss.NewStyle().
			Foreground(colorBg).
			Background(colorGreen).
			Bold(true).
			Padding(0, 1)

	styleInactive = lipgloss.NewStyle().
			Foreground(colorFg).
			Background(colorHighlight).
			Padding(0, 1)

	styleStatusBar = lipgloss.NewStyle().
			Foreground(colorDimFg).
			MarginTop(1)

	styleError = lipgloss.NewStyle().
			Foreground(colorRed).
			Bold(true)

	styleSuccess = lipgloss.NewStyle().
			Foreground(colorGreen)

	styleLabel = lipgloss.NewStyle().
			Foreground(colorMagenta).
			Bold(true).
			Width(16)

	styleValue = lipgloss.NewStyle().
			Foreground(colorFg)

	styleReveal = lipgloss.NewStyle().
			Foreground(colorYellow).
			Bold(true)

	styleDim = lipgloss.NewStyle().
			Foreground(colorDimFg)
)

func truncate(s string, maxLen int) string {
	if maxLen < 3 || len(s) <= maxLen {
		return s
	}
	return s[:maxLen-2] + ".."
}
