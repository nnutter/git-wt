package timber

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSetupSpaceOpensNamedWorktreeInNewHerdrWorkspace(t *testing.T) {
	t.Parallel()
	const branchName = "feature/space"

	testRepository := newTestRepository(t)
	require.NoError(t, testRepository.runTimber(t, "create", at(testRepoName, branchName)).err)

	logPath := filepath.Join(t.TempDir(), "herdr.log")
	testRepository.runtime.HerdrExecutable = installFakeHerdrSpace(t, logPath)

	result := testRepository.runTimber(t, "herdr", "space", "--new", at(testRepoName, branchName))
	require.NoError(t, result.err, result.stderr)
	assert.Contains(t, result.stderr, "opened herdr space for "+branchName)

	worktreePath := canonicalPath(testRepository.worktreePath(branchName))
	assert.Equal(t, []string{
		fakeHerdrLogLine("workspace", "create", "--cwd", worktreePath, "--label", testRepoName, "--no-focus"),
		fakeHerdrLogLine("tab", "rename", "w1:t1", "Agent"),
		fakeHerdrLogLine("pane", "rename", "w1:p1", branchName),
		fakeHerdrLogLine("tab", "create", "--workspace", "w1", "--cwd", worktreePath, "--label", "Shell", "--no-focus"),
		fakeHerdrLogLine("pane", "run", "w1:p1", "pi"),
		fakeHerdrLogLine("workspace", "focus", "w1"),
		fakeHerdrLogLine("tab", "focus", "w1:t1"),
	}, readFakeHerdrLog(t, logPath))
}

func TestSetupSpaceDefinesNamedWorktreeTabsInCurrentHerdrSpace(t *testing.T) {
	t.Parallel()
	const branchName = "feature/current-herdr-space"

	testRepository := newTestRepository(t)
	require.NoError(t, testRepository.runTimber(t, "create", at(testRepoName, branchName)).err)

	logPath := filepath.Join(t.TempDir(), "herdr.log")
	testRepository.runtime.HerdrExecutable = installFakeHerdrSpace(t, logPath)

	result := testRepository.runTimber(t, "herdr", "space", at(testRepoName, branchName))
	require.NoError(t, result.err, result.stderr)
	assert.Contains(t, result.stderr, "defined herdr tabs in current space for "+branchName)

	worktreePath := canonicalPath(testRepository.worktreePath(branchName))
	assert.Equal(t, []string{
		fakeHerdrLogLine("pane", "current", "--current"),
		fakeHerdrLogLine("tab", "rename", "w9:t1", "Agent"),
		fakeHerdrLogLine("pane", "rename", "w9:p1", branchName),
		fakeHerdrLogLine("tab", "create", "--workspace", "w9", "--cwd", worktreePath, "--label", "Shell", "--no-focus"),
		fakeHerdrLogLine("pane", "run", "w9:p1", "pi"),
		fakeHerdrLogLine("workspace", "focus", "w9"),
		fakeHerdrLogLine("tab", "focus", "w9:t1"),
	}, readFakeHerdrLog(t, logPath))
}

func TestSetupSpaceDoesNotCloseCurrentHerdrSpaceWhenTabCreationFails(t *testing.T) {
	t.Parallel()
	const branchName = "feature/current-herdr-space-failure"

	testRepository := newTestRepository(t)
	require.NoError(t, testRepository.runTimber(t, "create", at(testRepoName, branchName)).err)

	logPath := filepath.Join(t.TempDir(), "herdr.log")
	testRepository.runtime.HerdrExecutable = installFakeHerdrSpace(t, logPath)
	testRepository.runtime = withTestEnvironment(testRepository.runtime, "FAKE_HERDR_FAIL=tab create")

	result := testRepository.runTimber(t, "herdr", "space", at(testRepoName, branchName))
	require.Error(t, result.err)
	assert.Contains(t, result.err.Error(), "herdr tab create")
	assert.NotContains(t, readFakeHerdrLog(t, logPath), fakeHerdrLogLine("workspace", "close", "w9"))
}

func TestSetupSpaceUsesCurrentWorktreeFromSubdirectory(t *testing.T) {
	t.Parallel()
	const branchName = "feature/current-space"

	testRepository := newTestRepository(t)
	require.NoError(t, testRepository.runTimber(t, "create", at(testRepoName, branchName)).err)

	subdirectory := filepath.Join(testRepository.worktreePath(branchName), "nested")
	require.NoError(t, os.MkdirAll(subdirectory, 0o755))
	logPath := filepath.Join(t.TempDir(), "herdr.log")
	testRepository.runtime.HerdrExecutable = installFakeHerdrSpace(t, logPath)

	result := testRepository.runTimberFrom(t, subdirectory, "herdr", "space")
	require.NoError(t, result.err, result.stderr)
	assert.Contains(t, readFakeHerdrLog(t, logPath), fakeHerdrLogLine(
		"tab", "create", "--workspace", "w9", "--cwd", canonicalPath(testRepository.worktreePath(branchName)),
		"--label", "Shell", "--no-focus",
	))
}

func TestSetupSpaceFailsForUnknownWorktree(t *testing.T) {
	t.Parallel()
	testRepository := newTestRepository(t)

	result := testRepository.runTimber(t, "herdr", "space", at(testRepoName, "feature/missing"))
	require.Error(t, result.err)
	assert.Contains(t, result.err.Error(), `unknown worktree "feature/missing"`)
}

func TestSetupSpaceRequiresNameOutsideManagedWorktree(t *testing.T) {
	t.Parallel()
	testRepository := newTestRepository(t)

	result := testRepository.runTimber(t, "herdr", "space", at(testRepoName, ""))
	require.Error(t, result.err)
	assert.Contains(t, result.err.Error(), "worktree name is required")
}

func TestSetupSpaceClosesNewWorkspaceWhenTabCreationFails(t *testing.T) {
	t.Parallel()
	const branchName = "feature/space-failure"

	testRepository := newTestRepository(t)
	require.NoError(t, testRepository.runTimber(t, "create", at(testRepoName, branchName)).err)

	logPath := filepath.Join(t.TempDir(), "herdr.log")
	testRepository.runtime.HerdrExecutable = installFakeHerdrSpace(t, logPath)
	testRepository.runtime = withTestEnvironment(testRepository.runtime, "FAKE_HERDR_FAIL=tab create")

	result := testRepository.runTimber(t, "herdr", "space", "--new", at(testRepoName, branchName))
	require.Error(t, result.err)
	assert.Contains(t, result.err.Error(), "herdr tab create")
	assert.Equal(t, fakeHerdrLogLine("workspace", "close", "w1"), readFakeHerdrLog(t, logPath)[4])
}

func TestSetupSpaceClosesNewWorkspaceWhenShellTabCreationFails(t *testing.T) {
	t.Parallel()
	const branchName = "feature/space-shell-failure"

	testRepository := newTestRepository(t)
	require.NoError(t, testRepository.runTimber(t, "create", at(testRepoName, branchName)).err)

	logPath := filepath.Join(t.TempDir(), "herdr.log")
	testRepository.runtime.HerdrExecutable = installFakeHerdrSpace(t, logPath)
	testRepository.runtime = withTestEnvironment(testRepository.runtime, "FAKE_HERDR_FAIL_TAB_LABEL=Shell")

	result := testRepository.runTimber(t, "herdr", "space", "-n", at(testRepoName, branchName))
	require.Error(t, result.err)
	assert.Contains(t, result.err.Error(), "herdr tab create")
	assert.Equal(t, fakeHerdrLogLine("workspace", "close", "w1"), readFakeHerdrLog(t, logPath)[4])
}

func TestSetupSpaceClosesNewWorkspaceWhenTabResponseIsInvalid(t *testing.T) {
	t.Parallel()
	const branchName = "feature/space-invalid-response"

	testRepository := newTestRepository(t)
	require.NoError(t, testRepository.runTimber(t, "create", at(testRepoName, branchName)).err)

	logPath := filepath.Join(t.TempDir(), "herdr.log")
	testRepository.runtime.HerdrExecutable = installFakeHerdrSpace(t, logPath)
	testRepository.runtime = withTestEnvironment(testRepository.runtime, "FAKE_HERDR_MALFORM=tab create")

	result := testRepository.runTimber(t, "herdr", "space", "--new", at(testRepoName, branchName))
	require.Error(t, result.err)
	assert.Contains(t, result.err.Error(), "decode herdr tab create response")
	assert.Equal(t, fakeHerdrLogLine("workspace", "close", "w1"), readFakeHerdrLog(t, logPath)[4])
}

func TestSetupSpaceCompletionOffersManagedWorktreeNames(t *testing.T) {
	t.Parallel()
	testRepository := newTestRepository(t)
	require.NoError(t, testRepository.runTimber(t, "create", at(testRepoName, "feature/a")).err)
	require.NoError(t, testRepository.runTimber(t, "create", at(testRepoName, "feature/b")).err)

	stdout := runCompleteWithRuntime(t, testRepository.runtime, "herdr", "space", "")
	assert.Contains(t, stdout, "feature/a")
	assert.Contains(t, stdout, "feature/b")
}
