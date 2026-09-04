package timber

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGroupListTableRowsAddsRuleAfterEverySecondWorktree(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name               string
		worktrees          []managedWorktree
		groupedNames       []string
		horizontalRuleRows int
	}{
		{
			name: "odd number of worktrees",
			worktrees: []managedWorktree{
				{Name: "one", Clean: true},
				{Name: "two", Clean: true},
				{Name: "three", Clean: true},
				{Name: "four", Clean: true},
				{Name: "five", Clean: true},
			},
			groupedNames:       []string{"one\ntwo", "three\nfour", "five"},
			horizontalRuleRows: 3,
		},
		{
			name: "even number of worktrees",
			worktrees: []managedWorktree{
				{Name: "one", Clean: true},
				{Name: "two", Clean: true},
				{Name: "three", Clean: true},
				{Name: "four", Clean: true},
			},
			groupedNames:       []string{"one\ntwo", "three\nfour"},
			horizontalRuleRows: 2,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			rows := groupListTableRows(testCase.worktrees, newListStatusFormatter(testCase.worktrees))
			require.Len(t, rows, len(testCase.groupedNames))
			for index, names := range testCase.groupedNames {
				assert.Equal(t, names, rows[index][0])
			}

			tableView := newOutputTable("Name", "Repo", "Status", "Commit", "Dirty").BorderRow(true)
			tableView.Rows(rows...)
			tableOutput := dottedListRowRules(tableView.String())
			assert.Equal(t, testCase.horizontalRuleRows, strings.Count(tableOutput, "├"))
			assert.Equal(t, 1, strings.Count(tableOutput, "├─"))
			assert.Equal(t, testCase.horizontalRuleRows-1, strings.Count(tableOutput, "├┈"))
		})
	}
}

func TestListStatusFormatterAlignsAndColorsIndicators(t *testing.T) {
	t.Parallel()
	worktrees := []managedWorktree{
		{ListStatus: listStatus{Upstream: "origin/dev", Ahead: 18, Behind: 62}},
		{ListStatus: listStatus{Upstream: "origin/dev", Ahead: 1, Behind: 391}},
		{ListStatus: listStatus{Upstream: "origin/dev", Behind: 161}},
		{ListStatus: listStatus{Upstream: "origin/dev"}},
		{},
	}

	formatter := newListStatusFormatter(worktrees)

	assert.Equal(
		t,
		aheadStatusStyle.Render("↑18")+" "+behindStatusStyle.Render("↓ 62")+" [origin/dev]",
		formatter.format(worktrees[0].ListStatus),
	)
	assert.Equal(
		t,
		aheadStatusStyle.Render("↑ 1")+" "+behindStatusStyle.Render("↓391")+" [origin/dev]",
		formatter.format(worktrees[1].ListStatus),
	)
	assert.Equal(
		t,
		"    "+behindStatusStyle.Render("↓161")+" [origin/dev]",
		formatter.format(worktrees[2].ListStatus),
	)
	assert.Equal(t, "         [origin/dev]", formatter.format(worktrees[3].ListStatus))
	assert.Empty(t, formatter.format(worktrees[4].ListStatus))
}

func TestFormatDirtyStatusColorsTrueYellow(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "false", formatDirtyStatus(true))
	assert.Equal(t, warningStyle.Render("true"), formatDirtyStatus(false))
}
