package timber

import (
	"fmt"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
)

type listStatusFormatter struct {
	aheadCountWidth  int
	behindCountWidth int
}

func newListStatusFormatter(worktrees []managedWorktree) listStatusFormatter {
	var formatter listStatusFormatter
	for _, worktree := range worktrees {
		status := worktree.ListStatus
		if status.Ahead > 0 {
			formatter.aheadCountWidth = max(formatter.aheadCountWidth, len(strconv.Itoa(status.Ahead)))
		}
		if status.Behind > 0 {
			formatter.behindCountWidth = max(formatter.behindCountWidth, len(strconv.Itoa(status.Behind)))
		}
	}
	return formatter
}

func (x listStatusFormatter) format(status listStatus) string {
	parts := make([]string, 0, 3)
	if x.aheadCountWidth > 0 {
		indicator := formatListStatusIndicator("↑", status.Ahead, x.aheadCountWidth)
		if status.Ahead > 0 {
			indicator = aheadStatusStyle.Render(indicator)
		}
		parts = append(parts, indicator)
	}
	if x.behindCountWidth > 0 {
		indicator := formatListStatusIndicator("↓", status.Behind, x.behindCountWidth)
		if status.Behind > 0 {
			indicator = behindStatusStyle.Render(indicator)
		}
		parts = append(parts, indicator)
	}
	if status.Upstream != "" {
		parts = append(parts, "["+status.Upstream+"]")
	}
	return strings.TrimRight(strings.Join(parts, " "), " ")
}

func formatListStatusIndicator(arrow string, count int, countWidth int) string {
	if count == 0 {
		return strings.Repeat(" ", lipgloss.Width(arrow)+countWidth)
	}
	return arrow + fmt.Sprintf("%*d", countWidth, count)
}
