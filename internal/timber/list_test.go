package timber

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestListStatusFormatterAlignsAndColorsIndicators(t *testing.T) {
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

func TestFormatDirtyStatusColorsFalseYellow(t *testing.T) {
	assert.Equal(t, warningStyle.Render("false"), formatDirtyStatus(true))
	assert.Equal(t, "true", formatDirtyStatus(false))
}
