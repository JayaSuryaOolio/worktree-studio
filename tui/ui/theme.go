package ui

import "github.com/charmbracelet/lipgloss"

// Colors are the browser's Graphite-dark tokens (web/src/styles/tokens.css)
// with 256/16-color fallbacks. This is the only file that names a color;
// tui/DESIGN.md has the rules, chiefly: accent marks where keys go, nothing else.
var (
	colText      = lipgloss.CompleteColor{TrueColor: "#dfe1e4", ANSI256: "253", ANSI: "15"}
	colText2     = lipgloss.CompleteColor{TrueColor: "#9aa1a8", ANSI256: "247", ANSI: "7"}
	colText3     = lipgloss.CompleteColor{TrueColor: "#6c747c", ANSI256: "243", ANSI: "8"}
	colRule      = lipgloss.CompleteColor{TrueColor: "#3a3f45", ANSI256: "238", ANSI: "8"}
	colSurface2  = lipgloss.CompleteColor{TrueColor: "#202429", ANSI256: "235", ANSI: "0"}
	colAccent    = lipgloss.CompleteColor{TrueColor: "#e0913c", ANSI256: "172", ANSI: "3"}
	colAttention = lipgloss.CompleteColor{TrueColor: "#f0b429", ANSI256: "214", ANSI: "11"}
	colDanger    = lipgloss.CompleteColor{TrueColor: "#d1685f", ANSI256: "167", ANSI: "1"}
)

var (
	textStyle     = lipgloss.NewStyle().Foreground(colText)
	mutedStyle    = lipgloss.NewStyle().Foreground(colText2)
	dimStyle      = lipgloss.NewStyle().Foreground(colText3)
	ruleStyle     = lipgloss.NewStyle().Foreground(colRule)
	titleStyle    = lipgloss.NewStyle().Foreground(colText).Bold(true)
	selectedStyle = lipgloss.NewStyle().Foreground(colText).Background(colSurface2).Bold(true)
	accentStyle   = lipgloss.NewStyle().Foreground(colAccent).Bold(true)
	chipStyle     = lipgloss.NewStyle().Foreground(colAccent).Background(colSurface2).Bold(true).Padding(0, 1)
	attnStyle     = lipgloss.NewStyle().Foreground(colAttention)
	errStyle      = lipgloss.NewStyle().Foreground(colDanger)
	boxStyle      = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(colRule).Padding(1, 2)
	menuStyle     = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(colRule).Padding(0, 1)
)
