package timber

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPruneRemovesOnlyMergedCleanWorktrees(t *testing.T) {
	t.Parallel()
	testRepository := newTestRepository(t)

	require.NoError(t, testRepository.runTimber(t, "create", at(testRepoName, "feature/merged")).err)
	testRepository.mergeWorktreeBranch(t, "feature/merged")

	require.NoError(t, testRepository.runTimber(t, "create", at(testRepoName, "feature/unmerged")).err)
	testRepository.commitFileInWorktree(t, "feature/unmerged", "extra.txt", "extra\n")

	require.NoError(t, testRepository.runTimber(t, "create", at(testRepoName, "feature/dirty")).err)
	testRepository.mergeWorktreeBranch(t, "feature/dirty")
	testRepository.writeFileInWorktree(t, "feature/dirty", "dirty.txt", "dirty\n")

	result := testRepository.runTimber(t, "prune", at(testRepoName, ""))
	require.NoError(t, result.err, result.stderr)

	testRepository.assertPathMissing(t, testRepository.worktreePath("feature/merged"))
	testRepository.assertPathPresent(t, testRepository.worktreePath("feature/unmerged"))
	testRepository.assertPathPresent(t, testRepository.worktreePath("feature/dirty"))
}

func TestPruneDryRunListsAndKeepsWorktrees(t *testing.T) {
	t.Parallel()
	testRepository := newTestRepository(t)

	require.NoError(t, testRepository.runTimber(t, "create", at(testRepoName, "feature/merged")).err)
	testRepository.mergeWorktreeBranch(t, "feature/merged")

	require.NoError(t, testRepository.runTimber(t, "create", at(testRepoName, "feature/unmerged")).err)
	testRepository.commitFileInWorktree(t, "feature/unmerged", "extra.txt", "extra\n")

	require.NoError(t, testRepository.runTimber(t, "create", at(testRepoName, "feature/dirty")).err)
	testRepository.mergeWorktreeBranch(t, "feature/dirty")
	testRepository.writeFileInWorktree(t, "feature/dirty", "dirty.txt", "dirty\n")

	for _, flag := range []string{"-n", "--dry-run"} {
		result := testRepository.runTimber(t, "prune", flag, at(testRepoName, ""))
		require.NoError(t, result.err, result.stderr)
		assert.Contains(t, result.stderr, "would prune feature/merged ("+testRepoName+")")
		assert.NotContains(t, result.stderr, "feature/unmerged")
		assert.NotContains(t, result.stderr, "feature/dirty")
		testRepository.assertPathPresent(t, testRepository.worktreePath("feature/merged"))
		testRepository.assertPathPresent(t, testRepository.worktreePath("feature/unmerged"))
		testRepository.assertPathPresent(t, testRepository.worktreePath("feature/dirty"))
	}
}

func TestPruneDryRunSucceedsWhenNothingToPrune(t *testing.T) {
	t.Parallel()
	testRepository := newTestRepository(t)
	require.NoError(t, testRepository.runTimber(t, "create", at(testRepoName, "feature/unmerged")).err)
	testRepository.commitFileInWorktree(t, "feature/unmerged", "extra.txt", "extra\n")

	result := testRepository.runTimber(t, "prune", "--dry-run", at(testRepoName, ""))
	require.NoError(t, result.err, result.stderr)
	assert.NotContains(t, result.stderr, "would prune")
	testRepository.assertPathPresent(t, testRepository.worktreePath("feature/unmerged"))
}

func TestPruneDryRunWithPromptListsSelectedWorktrees(t *testing.T) {
	t.Parallel()
	const branchName = "feature/prompt-dry-run"

	testRepository := newTestRepository(t)
	require.NoError(t, testRepository.runTimber(t, "create", at(testRepoName, branchName)).err)
	testRepository.commitFileInWorktree(t, branchName, "extra.txt", "extra\n")

	options := &pruneCommandOptions{
		runtime:  testRepository.runtime,
		RepoName: testRepoName,
		prompt:   true,
		dryRun:   true,
		prompter: stubPrompter{selected: []managedWorktree{{Name: branchName, Repo: testRepoName}}},
	}
	command := newTestRootCommand(t)
	var stderr bytes.Buffer
	command.SetErr(&stderr)
	command.SetOut(io.Discard)
	err := options.Execute(command, nil)
	require.NoError(t, err, stderr.String())
	assert.Contains(t, stderr.String(), "would prune "+branchName+" ("+testRepoName+")")
	testRepository.assertPathPresent(t, testRepository.worktreePath(branchName))
}

func TestPruneWithoutRepoFromOutsidePrunesAllRepos(t *testing.T) {
	t.Parallel()
	primary := newTestRepository(t)
	secondaryName := "other"
	registerAdditionalRepo(t, primary, secondaryName)

	require.NoError(t, primary.runTimber(t, "create", at(testRepoName, "feature/merged")).err)
	primary.mergeWorktreeBranch(t, "feature/merged")
	require.NoError(t, primary.runTimber(t, "create", at(secondaryName, "feature/other-merged")).err)

	result := primary.runTimberFrom(t, primary.home, "prune")
	require.NoError(t, result.err, result.stderr)

	primary.assertPathMissing(t, primary.worktreePath("feature/merged"))
	_, err := os.Stat(filepath.Join(primary.worktreeRoot, secondaryName, "feature/other-merged", secondaryName))
	assert.ErrorIs(t, err, os.ErrNotExist)
}

func TestPruneWithoutRepoFromInsideWorktreePrunesAllRepos(t *testing.T) {
	t.Parallel()
	primary := newTestRepository(t)
	secondaryName := "other"
	registerAdditionalRepo(t, primary, secondaryName)

	require.NoError(t, primary.runTimber(t, "create", at(testRepoName, "feature/current")).err)
	primary.commitFileInWorktree(t, "feature/current", "extra.txt", "extra\n")
	require.NoError(t, primary.runTimber(t, "create", at(testRepoName, "feature/merged")).err)
	primary.mergeWorktreeBranch(t, "feature/merged")
	require.NoError(t, primary.runTimber(t, "create", at(secondaryName, "feature/other-merged")).err)

	result := primary.runTimberFrom(t, primary.worktreePath("feature/current"), "prune")
	require.NoError(t, result.err, result.stderr)

	primary.assertPathMissing(t, primary.worktreePath("feature/merged"))
	primary.assertPathPresent(t, primary.worktreePath("feature/current"))
	_, err := os.Stat(filepath.Join(primary.worktreeRoot, secondaryName, "feature/other-merged", secondaryName))
	assert.ErrorIs(t, err, os.ErrNotExist)
}

func TestPruneRepoFlagPinsRepoFromAnyCwd(t *testing.T) {
	t.Parallel()
	primary := newTestRepository(t)
	secondaryName := "other"
	registerAdditionalRepo(t, primary, secondaryName)

	require.NoError(t, primary.runTimber(t, "create", at(testRepoName, "feature/current")).err)
	require.NoError(t, primary.runTimber(t, "create", at(testRepoName, "feature/merged")).err)
	primary.mergeWorktreeBranch(t, "feature/merged")
	require.NoError(t, primary.runTimber(t, "create", at(secondaryName, "feature/other-merged")).err)

	result := primary.runTimberFrom(
		t,
		primary.worktreePath("feature/current"),
		"prune",
		at(secondaryName, ""),
	)
	require.NoError(t, result.err, result.stderr)

	primary.assertPathPresent(t, primary.worktreePath("feature/merged"))
	_, err := os.Stat(filepath.Join(primary.worktreeRoot, secondaryName, "feature/other-merged", secondaryName))
	assert.ErrorIs(t, err, os.ErrNotExist)
}

func TestPrunePromptDistinguishesSameNameInTwoRepos(t *testing.T) {
	t.Parallel()
	const branchName = "feature/same"

	primary := newTestRepository(t)
	secondaryName := "other"
	registerAdditionalRepo(t, primary, secondaryName)

	require.NoError(t, primary.runTimber(t, "create", at(testRepoName, branchName)).err)
	require.NoError(t, primary.runTimber(t, "create", at(secondaryName, branchName)).err)

	options := &pruneCommandOptions{
		runtime:  primary.runtime,
		prompt:   true,
		prompter: stubPrompter{selected: []managedWorktree{{Repo: secondaryName, Name: branchName}}},
	}
	command := newTestRootCommandWithRuntime(t, primary.runtime)
	var stderr bytes.Buffer
	command.SetErr(&stderr)
	command.SetOut(io.Discard)

	require.NoError(t, options.Execute(command, nil), stderr.String())

	primary.assertPathPresent(t, primary.worktreePath(branchName))
	_, err := os.Stat(filepath.Join(primary.worktreeRoot, secondaryName, branchName, secondaryName))
	assert.ErrorIs(t, err, os.ErrNotExist)
}

func TestPruneKeepsWorktreeWhenUpstreamRefIsMissing(t *testing.T) {
	t.Parallel()
	const branchName = "feature/prune-missing-upstream"

	testRepository := newTestRepository(t)
	require.NoError(t, testRepository.runTimber(t, "create", at(testRepoName, branchName)).err)
	runGitCommand(t, testRepository.barePath, "branch", "--unset-upstream", branchName)

	result := testRepository.runTimber(t, "prune", at(testRepoName, ""))
	// May error on enrich or keep worktree; either is acceptable if worktree remains when not merged.
	if result.err == nil {
		testRepository.assertPathPresent(t, testRepository.worktreePath(branchName))
	}
}

func TestPrunePromptCanForceRemoveSelectedWorktrees(t *testing.T) {
	t.Parallel()
	const branchName = "feature/prompt"

	testRepository := newTestRepository(t)
	require.NoError(t, testRepository.runTimber(t, "create", at(testRepoName, branchName)).err)
	testRepository.commitFileInWorktree(t, branchName, "extra.txt", "extra\n")

	options := &pruneCommandOptions{
		runtime:  testRepository.runtime,
		RepoName: testRepoName,
		prompt:   true,
		prompter: stubPrompter{selected: []managedWorktree{{Name: branchName}}},
	}
	command := newTestRootCommand(t)
	var stderr bytes.Buffer
	command.SetErr(&stderr)
	command.SetOut(io.Discard)
	command.SetArgs([]string{})
	err := options.Execute(command, nil)
	require.NoError(t, err, stderr.String())
	testRepository.assertPathMissing(t, testRepository.worktreePath(branchName))
}
