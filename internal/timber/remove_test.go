package timber

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRemoveRemovesEmptyParentDirectories(t *testing.T) {
	t.Parallel()
	const branchName = "feature/nested/path"

	testRepository := newTestRepository(t)
	require.NoError(t, testRepository.runTimber(t, "create", at(testRepoName, branchName)).err)
	testRepository.mergeWorktreeBranch(t, branchName)

	result := testRepository.runTimber(t, "remove", at(testRepoName, branchName))
	require.NoError(t, result.err, result.stderr)

	testRepository.assertPathMissing(t, filepath.Join(testRepository.worktreeRoot, testRepoName, "feature"))
}

func TestRemoveEmptyParentsStopsAtHome(t *testing.T) {
	t.Parallel()
	homeDirectory := t.TempDir()

	leafPath := filepath.Join(homeDirectory, "src", "github.com", "nnutter", "repo")
	require.NoError(t, os.MkdirAll(leafPath, 0o755))

	require.NoError(t, removeEmptyParents(leafPath, homeDirectory))

	_, err := os.Stat(leafPath)
	require.ErrorIs(t, err, os.ErrNotExist)
	_, err = os.Stat(filepath.Join(homeDirectory, "src"))
	require.ErrorIs(t, err, os.ErrNotExist)
	_, err = os.Stat(homeDirectory)
	require.NoError(t, err)
}

func TestRemoveEmptyParentsLeavesNonEmptyAncestor(t *testing.T) {
	t.Parallel()
	homeDirectory := t.TempDir()

	parentPath := filepath.Join(homeDirectory, "src", "github.com", "nnutter")
	leafPath := filepath.Join(parentPath, "repo")
	siblingPath := filepath.Join(parentPath, "other")
	require.NoError(t, os.MkdirAll(leafPath, 0o755))
	require.NoError(t, os.MkdirAll(siblingPath, 0o755))

	require.NoError(t, removeEmptyParents(leafPath, homeDirectory))

	_, err := os.Stat(leafPath)
	require.ErrorIs(t, err, os.ErrNotExist)
	_, err = os.Stat(siblingPath)
	require.NoError(t, err)
	_, err = os.Stat(parentPath)
	require.NoError(t, err)
}

func TestRemoveEmptyParentsHonorsStopPath(t *testing.T) {
	t.Parallel()
	homeDirectory := t.TempDir()

	stopPath := filepath.Join(homeDirectory, "worktrees")
	leafPath := filepath.Join(stopPath, "feature", "repo")
	require.NoError(t, os.MkdirAll(leafPath, 0o755))

	require.NoError(t, removeEmptyParents(leafPath, stopPath))

	_, err := os.Stat(leafPath)
	require.ErrorIs(t, err, os.ErrNotExist)
	_, err = os.Stat(filepath.Join(stopPath, "feature"))
	require.ErrorIs(t, err, os.ErrNotExist)
	_, err = os.Stat(stopPath)
	require.NoError(t, err)
}

func TestRemoveFailsWhenDirtyWithoutForce(t *testing.T) {
	t.Parallel()
	const branchName = "feature/dirty"

	testRepository := newTestRepository(t)
	require.NoError(t, testRepository.runTimber(t, "create", at(testRepoName, branchName)).err)
	testRepository.writeFileInWorktree(t, branchName, "dirty.txt", "dirty\n")

	result := testRepository.runTimber(t, "remove", at(testRepoName, branchName))
	require.Error(t, result.err)
	assert.Contains(t, result.err.Error(), "not clean")
}

func TestRemoveWithNoArgsRemovesCurrentWorktree(t *testing.T) {
	t.Parallel()
	const branchName = "feature/current"

	testRepository := newTestRepository(t)
	require.NoError(t, testRepository.runTimber(t, "create", at(testRepoName, branchName)).err)
	testRepository.mergeWorktreeBranch(t, branchName)

	result := testRepository.runTimberFrom(t, testRepository.worktreePath(branchName), "remove")
	require.NoError(t, result.err, result.stderr)
	testRepository.assertPathMissing(t, testRepository.worktreePath(branchName))
}

func TestRemoveWithNoArgsFromSubdirectoryRemovesCurrentWorktree(t *testing.T) {
	t.Parallel()
	const branchName = "feature/subdir"

	testRepository := newTestRepository(t)
	require.NoError(t, testRepository.runTimber(t, "create", at(testRepoName, branchName)).err)
	testRepository.mergeWorktreeBranch(t, branchName)

	subDir := filepath.Join(testRepository.worktreePath(branchName), "nested")
	require.NoError(t, os.MkdirAll(subDir, 0o755))

	result := testRepository.runTimberFrom(t, subDir, "remove")
	require.NoError(t, result.err, result.stderr)
	testRepository.assertPathMissing(t, testRepository.worktreePath(branchName))
}

func TestRemoveFailsWhenUnmergedWithoutForce(t *testing.T) {
	t.Parallel()
	const branchName = "feature/unmerged"

	testRepository := newTestRepository(t)
	require.NoError(t, testRepository.runTimber(t, "create", at(testRepoName, branchName)).err)
	testRepository.commitFileInWorktree(t, branchName, "extra.txt", "extra\n")

	result := testRepository.runTimber(t, "remove", at(testRepoName, branchName))
	require.Error(t, result.err)
	assert.Contains(t, result.err.Error(), "not merged")
}

func TestRemoveForceRemovesDirtyUnmergedWorktree(t *testing.T) {
	t.Parallel()
	const branchName = "feature/force"

	testRepository := newTestRepository(t)
	require.NoError(t, testRepository.runTimber(t, "create", at(testRepoName, branchName)).err)
	testRepository.commitFileInWorktree(t, branchName, "extra.txt", "extra\n")
	testRepository.writeFileInWorktree(t, branchName, "dirty.txt", "dirty\n")

	result := testRepository.runTimber(t, "remove", "--force", at(testRepoName, branchName))
	require.NoError(t, result.err, result.stderr)
	testRepository.assertPathMissing(t, testRepository.worktreePath(branchName))
	testRepository.assertBranchMissing(t, branchName)
}

func TestRemoveCompletionOffersManagedWorktreeNames(t *testing.T) {
	t.Parallel()
	testRepository := newTestRepository(t)
	require.NoError(t, testRepository.runTimber(t, "create", at(testRepoName, "feature/a")).err)
	require.NoError(t, testRepository.runTimber(t, "create", at(testRepoName, "feature/b")).err)

	stdout := runCompleteWithRuntime(t, testRepository.runtime, "remove", "")
	assert.Contains(t, stdout, "feature/a")
	assert.Contains(t, stdout, "feature/b")
}

func TestRemoveCompletionUsesCurrentWorktreeRepoWhenRepoFlagOmitted(t *testing.T) {
	t.Parallel()
	testRepository := newTestRepository(t)
	require.NoError(t, testRepository.runTimber(t, "create", at(testRepoName, "feature/a")).err)
	require.NoError(t, testRepository.runTimber(t, "create", at(testRepoName, "feature/b")).err)

	runtime := testRepository.runtime
	runtime.CurrentDirectory = testRepository.worktreePath("feature/a")
	stdout := runCompleteWithRuntime(t, runtime, "remove", "")
	assert.Contains(t, stdout, "feature/a")
	assert.Contains(t, stdout, "feature/b")
}

func TestRemoveCompletionOutsideManagedWorktreeOffersUniqueNames(t *testing.T) {
	t.Parallel()
	testRepository := newTestRepository(t)
	require.NoError(t, testRepository.runTimber(t, "create", at(testRepoName, "feature/a")).err)

	stdout := runCompleteWithRuntime(t, testRepository.runtime, "remove", "")
	assert.Contains(t, stdout, "feature/a")
	assert.NotContains(t, stdout, "feature/a@")
}

func TestRemovePreservesReferenceLikeBranchNames(t *testing.T) {
	t.Parallel()
	const branchName = "refs-like/name"

	testRepository := newTestRepository(t)
	require.NoError(t, testRepository.runTimber(t, "create", at(testRepoName, branchName)).err)
	testRepository.mergeWorktreeBranch(t, branchName)

	result := testRepository.runTimber(t, "remove", at(testRepoName, branchName))
	require.NoError(t, result.err, result.stderr)
	testRepository.assertBranchMissing(t, branchName)
}

func TestRemoveInfersUniqueRepoOutsideWorktree(t *testing.T) {
	t.Parallel()
	primary := newTestRepository(t)
	registerAdditionalRepo(t, primary, "other")
	require.NoError(t, primary.runTimber(t, "create", at(testRepoName, "feature/login")).err)
	primary.mergeWorktreeBranch(t, "feature/login")

	result := primary.runTimberFrom(t, primary.home, "remove", "feature/login")
	require.NoError(t, result.err, result.stderr)
	primary.assertPathMissing(t, primary.worktreePath("feature/login"))
}

func TestRemoveAutoDetectsRepoFromManagedWorktree(t *testing.T) {
	t.Parallel()
	const branchName = "feature/auto-remove"

	testRepository := newTestRepository(t)
	require.NoError(t, testRepository.runTimber(t, "create", at(testRepoName, branchName)).err)
	testRepository.mergeWorktreeBranch(t, branchName)

	result := testRepository.runTimberFrom(t, testRepository.worktreePath(branchName), "remove", branchName)
	require.NoError(t, result.err, result.stderr)
	testRepository.assertPathMissing(t, testRepository.worktreePath(branchName))
}
