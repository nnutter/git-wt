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

func TestListSucceedsWhenUpstreamRefIsMissing(t *testing.T) {
	t.Parallel()
	const branchName = "feature/no-upstream-ref"

	testRepository := newTestRepository(t)
	require.NoError(t, testRepository.runTimber(t, "create", at(testRepoName, branchName)).err)
	runGitCommand(t, testRepository.barePath, "update-ref", "-d", "refs/remotes/origin/main")

	result := testRepository.runTimber(t, "list", at(testRepoName, ""))
	require.NoError(t, result.err, result.stderr)
	assert.Contains(t, result.stdout, branchName)
}

func TestListSupportsLocalUpstream(t *testing.T) {
	t.Parallel()
	const branchName = "feature/local-upstream"

	testRepository := newTestRepository(t)
	require.NoError(t, testRepository.runTimber(t, "create", at(testRepoName, branchName)).err)
	runGitCommand(t, testRepository.barePath, "branch", "--set-upstream-to", "main", branchName)

	result := testRepository.runTimber(t, "list", at(testRepoName, ""))
	require.NoError(t, result.err, result.stderr)
	assert.Contains(t, result.stdout, branchName)
}

func TestListSupportsCustomRemoteUpstream(t *testing.T) {
	t.Parallel()
	const branchName = "feature/custom-remote"

	testRepository := newTestRepository(t)
	require.NoError(t, testRepository.runTimber(t, "create", at(testRepoName, branchName)).err)

	// Add a second remote-like ref namespace via config.
	runGitCommand(t, testRepository.barePath, "remote", "add", "upstream", testRepository.remotePath)
	runGitCommand(t, testRepository.barePath, "fetch", "upstream")
	runGitCommand(t, testRepository.barePath, "branch", "--set-upstream-to", "upstream/main", branchName)

	result := testRepository.runTimber(t, "list", at(testRepoName, ""))
	require.NoError(t, result.err, result.stderr)
}

func TestListSucceedsWhenBranchHasNoUpstream(t *testing.T) {
	t.Parallel()
	const branchName = "feature/no-upstream"

	testRepository := newTestRepository(t)
	require.NoError(t, testRepository.runTimber(t, "create", at(testRepoName, branchName)).err)
	runGitCommand(t, testRepository.barePath, "branch", "--unset-upstream", branchName)

	result := testRepository.runTimber(t, "list", at(testRepoName, ""))
	require.NoError(t, result.err, result.stderr)
	assert.Contains(t, result.stdout, branchName)
}

func TestListAutoDetectsRepoFromManagedWorktree(t *testing.T) {
	t.Parallel()
	const branchName = "feature/auto-list"

	testRepository := newTestRepository(t)
	require.NoError(t, testRepository.runTimber(t, "create", at(testRepoName, branchName)).err)

	result := testRepository.runTimberFrom(t, testRepository.worktreePath(branchName), "list")
	require.NoError(t, result.err, result.stderr)
	assert.Contains(t, result.stdout, "Name")
	assert.Contains(t, result.stdout, "Repo")
	assert.Less(t, strings.Index(result.stdout, "Name"), strings.Index(result.stdout, "Repo"))
	assert.Contains(t, result.stdout, testRepoName)
	assert.Contains(t, result.stdout, branchName)
}

func TestListReportsDirtyWorktree(t *testing.T) {
	t.Parallel()
	const branchName = "feature/dirty-list"

	testRepository := newTestRepository(t)
	require.NoError(t, testRepository.runTimber(t, "create", at(testRepoName, branchName)).err)
	testRepository.writeFileInWorktree(t, branchName, "dirty.txt", "dirty\n")

	result := testRepository.runTimber(t, "list", at(testRepoName, ""))
	require.NoError(t, result.err, result.stderr)
	assert.Contains(t, result.stdout, branchName)
	assert.Contains(t, result.stdout, "true")
}

func TestListOutsideManagedWorktreeListsAllRepos(t *testing.T) {
	t.Parallel()
	primary := newTestRepository(t)
	secondaryName := "other"
	secondaryBare := registerAdditionalRepo(t, primary, secondaryName)

	require.NoError(t, primary.runTimber(t, "create", at(testRepoName, "feature/primary")).err)
	require.NoError(t, primary.runTimber(t, "create", at(secondaryName, "feature/secondary")).err)

	result := primary.runTimber(t, "list")
	require.NoError(t, result.err, result.stderr)
	assert.Contains(t, result.stdout, testRepoName)
	assert.Contains(t, result.stdout, "feature/primary")
	assert.Contains(t, result.stdout, secondaryName)
	assert.Contains(t, result.stdout, "feature/secondary")
	assert.DirExists(t, secondaryBare)
}

func TestListInsideManagedWorktreeListsAllRepos(t *testing.T) {
	t.Parallel()
	primary := newTestRepository(t)
	secondaryName := "other"
	registerAdditionalRepo(t, primary, secondaryName)

	require.NoError(t, primary.runTimber(t, "create", at(testRepoName, "feature/primary")).err)
	require.NoError(t, primary.runTimber(t, "create", at(secondaryName, "feature/secondary")).err)

	result := primary.runTimberFrom(t, primary.worktreePath("feature/primary"), "list")
	require.NoError(t, result.err, result.stderr)
	assert.Contains(t, result.stdout, "feature/primary")
	assert.Contains(t, result.stdout, "feature/secondary")
	assert.Contains(t, result.stdout, secondaryName)
}
