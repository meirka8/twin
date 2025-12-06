package main

import "github.com/charmbracelet/lipgloss"

var (
	// Styles
	docStyle             = lipgloss.NewStyle().Margin(1, 2)
	activeStyle          = lipgloss.NewStyle().Border(lipgloss.NormalBorder(), true).BorderForeground(lipgloss.Color("63"))
	inactiveStyle        = lipgloss.NewStyle().Border(lipgloss.NormalBorder(), true).BorderForeground(lipgloss.Color("240"))
	cursorStyle          = lipgloss.NewStyle().Background(lipgloss.Color("63")).Foreground(lipgloss.Color("255"))
	selectionStyle       = lipgloss.NewStyle().Background(lipgloss.Color("220")).Foreground(lipgloss.Color("0"))
	dirStyle             = lipgloss.NewStyle().Foreground(lipgloss.Color("33"))
	fileStyle            = lipgloss.NewStyle().Foreground(lipgloss.Color("255"))
	statusBar            = lipgloss.NewStyle().Background(lipgloss.Color("235")).Foreground(lipgloss.Color("250")).Padding(0, 1)
	statusBarActive      = lipgloss.NewStyle().Background(lipgloss.Color("63")).Foreground(lipgloss.Color("255")).Padding(0, 1)
	inputPromptStyle     = lipgloss.NewStyle().Background(lipgloss.Color("235")).Foreground(lipgloss.Color("255")).Padding(0, 1)
	confirmPromptStyle   = lipgloss.NewStyle().Background(lipgloss.Color("166")).Foreground(lipgloss.Color("255")).Padding(0, 1)
	overwritePromptStyle = lipgloss.NewStyle().Background(lipgloss.Color("202")).Foreground(lipgloss.Color("0")).Padding(0, 1)
	previewStyle         = lipgloss.NewStyle().Border(lipgloss.DoubleBorder(), true).BorderForeground(lipgloss.Color("205")).Padding(1, 2)

	// Hint Styles
	modifierStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("240")).Padding(0, 1)
	modifierActiveStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("255")).Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("63")).Padding(0, 1)
	hintKeyStyle        = lipgloss.NewStyle().Foreground(lipgloss.Color("63")).Bold(true)
	hintDescStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("250"))
	hintCardStyle       = lipgloss.NewStyle().Border(lipgloss.NormalBorder(), false, false, false, true).BorderForeground(lipgloss.Color("240")).Padding(0, 1).MarginRight(1)
)
