package timber

import (
	"charm.land/lipgloss/v2"
	"charm.land/lipgloss/v2/table"
)

var (
	statusStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
	warningStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("11"))
	// errorStyle        = lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Bold(true)
	aheadStatusStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
	behindStatusStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("12"))
)

func newOutputTable(headers ...string) *table.Table {
	return table.New().
		Headers(headers...).
		Border(lipgloss.NormalBorder()).
		BorderHeader(true).
		StyleFunc(func(row int, column int) lipgloss.Style {
			if row == table.HeaderRow {
				return lipgloss.NewStyle().Bold(true).PaddingLeft(1).PaddingRight(1)
			}
			return lipgloss.NewStyle().PaddingLeft(1).PaddingRight(1)
		})
}
