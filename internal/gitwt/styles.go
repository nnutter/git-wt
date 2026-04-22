package gitwt

import "charm.land/lipgloss/v2"

var (
	statusStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
	warningStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("11"))
	errorStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Bold(true)
)
