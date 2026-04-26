package gitwt

import (
	"os"

	"charm.land/lipgloss/v2"
)

var (
	errorStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("1")).Bold(true)
	warnStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("3")).Bold(true)
	infoStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("6"))
)

func printError(format string, args ...any) {
	os.Stderr.WriteString(errorStyle.Render("error: "+sprintf(format, args...)) + "\n")
}

func printWarn(format string, args ...any) {
	os.Stderr.WriteString(warnStyle.Render("warn: "+sprintf(format, args...)) + "\n")
}

func printInfo(format string, args ...any) {
	os.Stderr.WriteString(infoStyle.Render(sprintf(format, args...)) + "\n")
}

func sprintf(format string, args ...any) string {
	return lipgloss.Sprintf(format, args...)
}
