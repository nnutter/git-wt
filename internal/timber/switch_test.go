package timber

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSwitchCompletionOffersWorktreeNamesAcrossRepos(t *testing.T) {
	t.Parallel()
	primary := newTestRepository(t)
	secondaryName := "other"
	registerAdditionalRepo(t, primary, secondaryName)
	require.NoError(t, primary.runTimber(t, "create", at(testRepoName, "feature/current")).err)
	require.NoError(t, primary.runTimber(t, "create", at(secondaryName, "feature/other")).err)

	runtime := primary.runtime
	runtime.CurrentDirectory = primary.worktreePath("feature/current")
	scoped := runCompleteWithRuntime(t, runtime, "switch", "")
	assert.Contains(t, scoped, "feature/current")
	assert.Contains(t, scoped, "feature/other")
	assert.NotContains(t, scoped, "feature/current@")
	assert.NotContains(t, scoped, "feature/other@")
}

func TestSwitchResolvesPathWithRepoFlag(t *testing.T) {
	t.Parallel()
	const branchName = "feature/switch-repo"

	testRepository := newTestRepository(t)
	require.NoError(t, testRepository.runTimber(t, "create", at(testRepoName, branchName)).err)

	result := testRepository.runTimber(t, "switch", at(testRepoName, branchName))
	require.NoError(t, result.err, result.stderr)
	assert.Equal(t, testRepository.worktreePath(branchName), strings.TrimSpace(result.stdout))
}

func TestSwitchResolvesRepoFromCurrentWorktree(t *testing.T) {
	t.Parallel()
	testRepository := newTestRepository(t)
	require.NoError(t, testRepository.runTimber(t, "create", at(testRepoName, "feature/from")).err)
	require.NoError(t, testRepository.runTimber(t, "create", at(testRepoName, "feature/to")).err)

	result := testRepository.runTimberFrom(t, testRepository.worktreePath("feature/from"), "switch", "feature/to")
	require.NoError(t, result.err, result.stderr)
	assert.Equal(t, testRepository.worktreePath("feature/to"), strings.TrimSpace(result.stdout))
}

func TestSwitchInfersUniqueRepoOutsideWorktree(t *testing.T) {
	t.Parallel()
	primary := newTestRepository(t)
	registerAdditionalRepo(t, primary, "other")
	require.NoError(t, primary.runTimber(t, "create", at(testRepoName, "feature/login")).err)

	result := primary.runTimberFrom(t, primary.home, "switch", "feature/login")
	require.NoError(t, result.err, result.stderr)
	assert.Equal(t, primary.worktreePath("feature/login"), strings.TrimSpace(result.stdout))
}

func TestSwitchRequiresRepoWhenWorktreeNameIsAmbiguous(t *testing.T) {
	t.Parallel()
	primary := newTestRepository(t)
	secondaryName := "other"
	registerAdditionalRepo(t, primary, secondaryName)
	require.NoError(t, primary.runTimber(t, "create", at(testRepoName, "feature/login")).err)
	require.NoError(t, primary.runTimber(t, "create", at(secondaryName, "feature/login")).err)

	result := primary.runTimberFrom(t, primary.home, "switch", "feature/login")
	require.Error(t, result.err)
	assert.Contains(t, result.err.Error(), "qualify as <worktree>@<repo>")
	assert.Contains(t, result.err.Error(), testRepoName)
	assert.Contains(t, result.err.Error(), secondaryName)
}

func TestSwitchReportsMissingWorktreeOutsideWorktree(t *testing.T) {
	t.Parallel()
	primary := newTestRepository(t)
	registerAdditionalRepo(t, primary, "other")

	result := primary.runTimberFrom(t, primary.home, "switch", "missing")
	require.Error(t, result.err)
	assert.Contains(t, result.err.Error(), "not found")
}

func TestSwitchRequiresRepoWhenNameIsAmbiguousInsideWorktree(t *testing.T) {
	t.Parallel()
	primary := newTestRepository(t)
	secondaryName := "other"
	registerAdditionalRepo(t, primary, secondaryName)
	require.NoError(t, primary.runTimber(t, "create", at(testRepoName, "feature/current")).err)
	require.NoError(t, primary.runTimber(t, "create", at(testRepoName, "feature/login")).err)
	require.NoError(t, primary.runTimber(t, "create", at(secondaryName, "feature/login")).err)

	result := primary.runTimberFrom(t, primary.worktreePath("feature/current"), "switch", "feature/login")
	require.Error(t, result.err)
	assert.Contains(t, result.err.Error(), "qualify as <worktree>@<repo>")
	assert.Contains(t, result.err.Error(), testRepoName)
	assert.Contains(t, result.err.Error(), secondaryName)
}

func TestSwitchInfersUniqueRepoInsideWorktree(t *testing.T) {
	t.Parallel()
	primary := newTestRepository(t)
	secondaryName := "other"
	registerAdditionalRepo(t, primary, secondaryName)
	require.NoError(t, primary.runTimber(t, "create", at(testRepoName, "feature/current")).err)
	require.NoError(t, primary.runTimber(t, "create", at(secondaryName, "feature/other")).err)

	result := primary.runTimberFrom(t, primary.worktreePath("feature/current"), "switch", "feature/other")
	require.NoError(t, result.err, result.stderr)
	assert.Equal(t, primary.runtime.managedWorktreePath(secondaryName, "feature/other"), strings.TrimSpace(result.stdout))
}

func TestSwitchFailsWhenWorktreeMissing(t *testing.T) {
	t.Parallel()
	testRepository := newTestRepository(t)

	result := testRepository.runTimber(t, "switch", at(testRepoName, "missing"))
	require.Error(t, result.err)
	assert.Contains(t, result.err.Error(), "not found")
}

func TestSwitchReportsAlreadyInWorktree(t *testing.T) {
	t.Parallel()
	const branchName = "feature/already"

	testRepository := newTestRepository(t)
	require.NoError(t, testRepository.runTimber(t, "create", at(testRepoName, branchName)).err)

	result := testRepository.runTimberFrom(
		t,
		testRepository.worktreePath(branchName),
		"switch",
		at(testRepoName, branchName),
	)
	require.NoError(t, result.err, result.stderr)
	assert.Contains(t, result.stderr, "Already in "+branchName)
	assert.Equal(t, testRepository.worktreePath(branchName), strings.TrimSpace(result.stdout))
}

func TestSwitchWritesPathFileWhenRequested(t *testing.T) {
	t.Parallel()
	const branchName = "feature/switch-file"

	testRepository := newTestRepository(t)
	require.NoError(t, testRepository.runTimber(t, "create", at(testRepoName, branchName)).err)

	pathFile := filepath.Join(t.TempDir(), "switch-path")
	runtime := testRepository.runtime
	runtime.SwitchPathFile = pathFile

	result := runTimberCommandWithRuntime(t, runtime, "switch", at(testRepoName, branchName))
	require.NoError(t, result.err, result.stderr)
	assert.Empty(t, result.stdout)
	contents, err := os.ReadFile(pathFile)
	require.NoError(t, err)
	assert.Equal(t, testRepository.worktreePath(branchName)+"\n", string(contents))
}

func TestSwitchIsHiddenFromHelp(t *testing.T) {
	t.Parallel()
	result := runTimberCommand(t, "--help")
	require.NoError(t, result.err, result.stderr)
	assert.NotContains(t, result.stdout, "switch")
	assert.NotContains(t, result.stderr, "switch")
}

func TestSwitchCreateCreatesWorktreeAndReportsPath(t *testing.T) {
	t.Parallel()
	const branchName = "feature/switch-create"

	testRepository := newTestRepository(t)
	result := testRepository.runTimber(t, "switch", "-c", at(testRepoName, branchName))
	require.NoError(t, result.err, result.stderr)
	testRepository.assertPathPresent(t, testRepository.worktreePath(branchName))
	assert.Equal(t, testRepository.worktreePath(branchName), strings.TrimSpace(result.stdout))
	assert.Contains(t, result.stderr, "created ")
}

func TestSwitchCreateAcceptsFlagAfterName(t *testing.T) {
	t.Parallel()
	const branchName = "feature/switch-create-after"

	testRepository := newTestRepository(t)
	result := testRepository.runTimber(t, "switch", at(testRepoName, branchName), "--create")
	require.NoError(t, result.err, result.stderr)
	testRepository.assertPathPresent(t, testRepository.worktreePath(branchName))
	assert.Equal(t, testRepository.worktreePath(branchName), strings.TrimSpace(result.stdout))
}

func TestSwitchCreateFailsWhenWorktreeExists(t *testing.T) {
	t.Parallel()
	const branchName = "feature/switch-create-exists"

	testRepository := newTestRepository(t)
	require.NoError(t, testRepository.runTimber(t, "create", at(testRepoName, branchName)).err)

	pathFile := filepath.Join(t.TempDir(), "switch-path")
	runtime := testRepository.runtime
	runtime.SwitchPathFile = pathFile
	result := runTimberCommandWithRuntime(t, runtime, "switch", at(testRepoName, branchName), "-c")
	require.Error(t, result.err)
	assert.Contains(t, result.err.Error(), "already exists")
	_, err := os.Stat(pathFile)
	assert.ErrorIs(t, err, os.ErrNotExist)
}

func TestSwitchCreateNoCdDoesNotReportPath(t *testing.T) {
	t.Parallel()
	const branchName = "feature/switch-create-nocd"

	testRepository := newTestRepository(t)
	pathFile := filepath.Join(t.TempDir(), "switch-path")
	runtime := testRepository.runtime
	runtime.SwitchPathFile = pathFile

	result := runTimberCommandWithRuntime(t, runtime, "switch", "-c", "--no-cd", at(testRepoName, branchName))
	require.NoError(t, result.err, result.stderr)
	testRepository.assertPathPresent(t, testRepository.worktreePath(branchName))
	assert.Empty(t, result.stdout)
	_, err := os.Stat(pathFile)
	assert.ErrorIs(t, err, os.ErrNotExist)
}

func TestSwitchCreateWithHerdrDoesNotReportPath(t *testing.T) {
	t.Parallel()
	const branchName = "feature/switch-create-herdr"

	testRepository := newTestRepository(t)
	logPath := filepath.Join(t.TempDir(), "herdr.log")
	testRepository.runtime.HerdrExecutable = installFakeHerdrSpace(t, logPath)
	pathFile := filepath.Join(t.TempDir(), "switch-path")
	runtime := testRepository.runtime
	runtime.SwitchPathFile = pathFile

	result := runTimberCommandWithRuntime(t, runtime, "switch", "-c", "--herdr", at(testRepoName, branchName))
	require.NoError(t, result.err, result.stderr)
	testRepository.assertPathPresent(t, testRepository.worktreePath(branchName))
	assert.Contains(t, result.stderr, "opened herdr space")
	assert.Empty(t, result.stdout)
	_, err := os.Stat(pathFile)
	assert.ErrorIs(t, err, os.ErrNotExist)
}

func TestSwitchCompletionQualifiesAmbiguousNamesFromInsideWorktree(t *testing.T) {
	t.Parallel()
	primary := newTestRepository(t)
	secondaryName := "other"
	registerAdditionalRepo(t, primary, secondaryName)
	require.NoError(t, primary.runTimber(t, "create", at(testRepoName, "feature/login")).err)
	require.NoError(t, primary.runTimber(t, "create", at(secondaryName, "feature/login")).err)

	runtime := primary.runtime
	runtime.CurrentDirectory = primary.worktreePath("feature/login")
	stdout := runCompleteWithRuntime(t, runtime, "switch", "feature/l")
	assert.Contains(t, stdout, at(testRepoName, "feature/login"))
	assert.Contains(t, stdout, at(secondaryName, "feature/login"))
}
