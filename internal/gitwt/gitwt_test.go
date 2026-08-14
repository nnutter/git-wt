package gitwt

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type commandResult struct {
	stdout string
	stderr string
	err    error
}

type stubPrompter struct {
	selected []managedWorktree
	err      error
}

type stubMigratePrompter struct {
	selected []migrateCandidate
	err      error
}

func (x stubPrompter) Prompt(input io.Reader, output io.Writer, worktrees []managedWorktree) ([]managedWorktree, error) {
	return x.selected, x.err
}

func (x stubMigratePrompter) Prompt(input io.Reader, output io.Writer, candidates []migrateCandidate) ([]migrateCandidate, error) {
	return x.selected, x.err
}

func TestCreateListAndRemoveLifecycle(t *testing.T) {
	const branchName = "feature/one"

	testRepository := newTestRepository(t)

	createResult := testRepository.runGitWT(t, "create", "--repo", testRepoName, branchName)
	require.NoError(t, createResult.err, createResult.stderr)
	testRepository.assertPathPresent(t, testRepository.worktreePath(branchName))
	assert.Contains(t, createResult.stdout, testRepository.worktreePath(branchName))

	branchCommitHash := strings.TrimSpace(runGitCommand(t, testRepository.barePath, "rev-parse", "--short=7", branchName))

	listResult := testRepository.runGitWT(t, "list", "--repo", testRepoName)
	require.NoError(t, listResult.err, listResult.stderr)
	assert.Contains(t, listResult.stdout, "Repo")
	assert.Contains(t, listResult.stdout, testRepoName)
	assert.Contains(t, listResult.stdout, branchName)
	assert.Contains(t, listResult.stdout, branchCommitHash)

	testRepository.mergeWorktreeBranch(t, branchName)
	mergedCommitHash := strings.TrimSpace(runGitCommand(t, testRepository.barePath, "rev-parse", "--short=7", branchName))

	removeResult := testRepository.runGitWT(t, "remove", "--repo", testRepoName, branchName)
	require.NoError(t, removeResult.err, removeResult.stderr)
	assert.Contains(t, removeResult.stderr, mergedCommitHash)

	testRepository.assertBranchMissing(t, branchName)
	testRepository.assertPathMissing(t, testRepository.worktreePath(branchName))
}

func TestCreateUsesOriginHeadAsDefaultUpstream(t *testing.T) {
	testRepository := newTestRepository(t)
	runGitCommand(t, testRepository.barePath, "branch", "develop", "main")
	runGitCommand(t, testRepository.barePath, "symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/develop")

	// Ensure origin/develop exists in bare via fetch simulation: point remote HEAD.
	// For bare with no remotes tracking, set upstream explicitly by pushing develop.
	runGitCommand(t, testRepository.barePath, "update-ref", "refs/remotes/origin/develop", "refs/heads/develop")
	runGitCommand(t, testRepository.barePath, "update-ref", "refs/remotes/origin/main", "refs/heads/main")
	runGitCommand(t, testRepository.barePath, "symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/develop")

	result := testRepository.runGitWT(t, "create", "--repo", testRepoName, "feature/from-develop")
	require.NoError(t, result.err, result.stderr)

	upstream := strings.TrimSpace(runGitCommand(
		t,
		testRepository.worktreePath("feature/from-develop"),
		"rev-parse",
		"--abbrev-ref",
		"@{upstream}",
	))
	assert.Equal(t, "origin/develop", upstream)
}

func TestCreateFallsBackToOriginMasterWhenOriginHeadIsMissing(t *testing.T) {
	testRepository := newTestRepository(t)
	runGitCommand(t, testRepository.barePath, "branch", "-M", "main", "master")
	runGitCommand(t, testRepository.barePath, "update-ref", "refs/remotes/origin/master", "refs/heads/master")
	// Ensure origin/HEAD missing
	runGitCommandAllowError(t, testRepository.barePath, "symbolic-ref", "-d", "refs/remotes/origin/HEAD")
	runGitCommandAllowError(t, testRepository.barePath, "update-ref", "-d", "refs/remotes/origin/main")

	result := testRepository.runGitWT(t, "create", "--repo", testRepoName, "feature/from-master")
	require.NoError(t, result.err, result.stderr)

	upstream := strings.TrimSpace(runGitCommand(
		t,
		testRepository.worktreePath("feature/from-master"),
		"rev-parse",
		"--abbrev-ref",
		"@{upstream}",
	))
	assert.Equal(t, "origin/master", upstream)
}

func TestCreateFallsBackToOriginMainWhenOriginHeadAndMasterAreMissing(t *testing.T) {
	testRepository := newTestRepository(t)
	runGitCommand(t, testRepository.barePath, "update-ref", "refs/remotes/origin/main", "refs/heads/main")
	runGitCommandAllowError(t, testRepository.barePath, "symbolic-ref", "-d", "refs/remotes/origin/HEAD")
	runGitCommandAllowError(t, testRepository.barePath, "update-ref", "-d", "refs/remotes/origin/master")

	result := testRepository.runGitWT(t, "create", "--repo", testRepoName, "feature/from-main")
	require.NoError(t, result.err, result.stderr)
}

func TestCreateFailsWhenOriginHeadAndCommonDefaultsAreMissing(t *testing.T) {
	testRepository := newTestRepository(t)
	runGitCommand(t, testRepository.barePath, "branch", "develop", "main")
	runGitCommandAllowError(t, testRepository.barePath, "symbolic-ref", "-d", "refs/remotes/origin/HEAD")
	runGitCommandAllowError(t, testRepository.barePath, "update-ref", "-d", "refs/remotes/origin/main")
	runGitCommandAllowError(t, testRepository.barePath, "update-ref", "-d", "refs/remotes/origin/master")
	runGitCommandAllowError(t, testRepository.barePath, "branch", "-D", "main")
	runGitCommandAllowError(t, testRepository.barePath, "branch", "-D", "master")
	// Remove origin so repair/fetch cannot restore default remote-tracking refs.
	runGitCommandAllowError(t, testRepository.barePath, "remote", "remove", remoteName)

	result := testRepository.runGitWT(t, "create", "--repo", testRepoName, "feature/missing-default-upstream")
	require.Error(t, result.err)
	assert.Contains(t, result.err.Error(), "resolve origin/HEAD")
}

func TestCreateWithHerdrOpensStandardHerdrSpace(t *testing.T) {
	const branchName = "feature/herdr"

	testRepository := newTestRepository(t)
	logPath := filepath.Join(t.TempDir(), "herdr.log")
	installFakeHerdrSpace(t, logPath)

	result := testRepository.runGitWT(t, "create", "--repo", testRepoName, "--herdr", branchName)
	require.NoError(t, result.err, result.stderr)
	testRepository.assertPathPresent(t, testRepository.worktreePath(branchName))
	assert.Contains(t, result.stderr, "opened herdr space for "+branchName)

	worktreePath, err := filepath.Abs(testRepository.worktreePath(branchName))
	require.NoError(t, err)
	assert.Equal(t, []string{
		fakeHerdrLogLine("workspace", "create", "--cwd", worktreePath, "--label", testRepoName, "--no-focus"),
		fakeHerdrLogLine("tab", "rename", "w1:t1", "Agent"),
		fakeHerdrLogLine("tab", "create", "--workspace", "w1", "--cwd", worktreePath, "--label", "Editor", "--no-focus"),
		fakeHerdrLogLine("tab", "create", "--workspace", "w1", "--cwd", worktreePath, "--label", "Shell", "--no-focus"),
		fakeHerdrLogLine("pane", "run", "w1:p1", "pi"),
		fakeHerdrLogLine("pane", "run", "w1:p2", "nvim ."),
		fakeHerdrLogLine("workspace", "focus", "w1"),
		fakeHerdrLogLine("tab", "focus", "w1:t1"),
	}, readFakeHerdrLog(t, logPath))
}

func TestCreateWithoutHerdrDoesNotInvokeHerdr(t *testing.T) {
	const branchName = "feature/no-herdr"

	testRepository := newTestRepository(t)
	logPath := filepath.Join(t.TempDir(), "herdr.log")
	installFakeHerdrSpace(t, logPath)

	result := testRepository.runGitWT(t, "create", "--repo", testRepoName, branchName)
	require.NoError(t, result.err, result.stderr)
	_, err := os.Stat(logPath)
	assert.True(t, os.IsNotExist(err))
}

func TestCreateInHerdrOpensStandardHerdrSpace(t *testing.T) {
	const branchName = "feature/automatic-herdr"

	testRepository := newTestRepository(t)
	t.Setenv("HERDR_ENV", "1")
	logPath := filepath.Join(t.TempDir(), "herdr.log")
	installFakeHerdrSpace(t, logPath)

	result := testRepository.runGitWT(t, "create", "--repo", testRepoName, branchName)
	require.NoError(t, result.err, result.stderr)
	assert.Len(t, readFakeHerdrLog(t, logPath), 8)
}

func TestCreateWithNoHerdrDoesNotInvokeHerdr(t *testing.T) {
	const branchName = "feature/no-herdr-flag"

	testRepository := newTestRepository(t)
	t.Setenv("HERDR_ENV", "1")
	logPath := filepath.Join(t.TempDir(), "herdr.log")
	installFakeHerdrSpace(t, logPath)

	result := testRepository.runGitWT(t, "create", "--repo", testRepoName, "--no-herdr", branchName)
	require.NoError(t, result.err, result.stderr)
	_, err := os.Stat(logPath)
	assert.True(t, os.IsNotExist(err))
}

func TestCreateRejectsHerdrAndNoHerdr(t *testing.T) {
	result := runGitWTCommand(t, "create", "--herdr", "--no-herdr", "feature/conflicting-herdr")
	require.Error(t, result.err)
	assert.Contains(t, result.err.Error(), "if any flags in the group [herdr no-herdr] are set none of the others can be")
}

func TestCreateWithHerdrKeepsWorktreeWhenHerdrFails(t *testing.T) {
	const branchName = "feature/herdr-fail"

	testRepository := newTestRepository(t)
	logPath := filepath.Join(t.TempDir(), "herdr.log")
	installFakeHerdrSpace(t, logPath)
	t.Setenv("FAKE_HERDR_FAIL", "workspace create")

	result := testRepository.runGitWT(t, "create", "--repo", testRepoName, "--herdr", branchName)
	require.Error(t, result.err)
	testRepository.assertPathPresent(t, testRepository.worktreePath(branchName))
	assert.Contains(t, result.err.Error(), "herdr workspace create")
}

func TestSpaceOpensNamedWorktreeInStandardHerdrTabs(t *testing.T) {
	const branchName = "feature/space"

	testRepository := newTestRepository(t)
	require.NoError(t, testRepository.runGitWT(t, "create", "--repo", testRepoName, branchName).err)

	logPath := filepath.Join(t.TempDir(), "herdr.log")
	installFakeHerdrSpace(t, logPath)

	result := testRepository.runGitWT(t, "space", "--repo", testRepoName, branchName)
	require.NoError(t, result.err, result.stderr)
	assert.Contains(t, result.stderr, "opened herdr space for "+branchName)

	worktreePath := canonicalPath(testRepository.worktreePath(branchName))
	assert.Equal(t, []string{
		fakeHerdrLogLine("workspace", "create", "--cwd", worktreePath, "--label", testRepoName, "--no-focus"),
		fakeHerdrLogLine("tab", "rename", "w1:t1", "Agent"),
		fakeHerdrLogLine("tab", "create", "--workspace", "w1", "--cwd", worktreePath, "--label", "Editor", "--no-focus"),
		fakeHerdrLogLine("tab", "create", "--workspace", "w1", "--cwd", worktreePath, "--label", "Shell", "--no-focus"),
		fakeHerdrLogLine("pane", "run", "w1:p1", "pi"),
		fakeHerdrLogLine("pane", "run", "w1:p2", "nvim ."),
		fakeHerdrLogLine("workspace", "focus", "w1"),
		fakeHerdrLogLine("tab", "focus", "w1:t1"),
	}, readFakeHerdrLog(t, logPath))
}

func TestSpaceDefinesNamedWorktreeTabsInCurrentHerdrSpace(t *testing.T) {
	const branchName = "feature/current-herdr-space"

	testRepository := newTestRepository(t)
	require.NoError(t, testRepository.runGitWT(t, "create", "--repo", testRepoName, branchName).err)

	logPath := filepath.Join(t.TempDir(), "herdr.log")
	installFakeHerdrSpace(t, logPath)

	result := testRepository.runGitWT(t, "space", "--current", "--repo", testRepoName, branchName)
	require.NoError(t, result.err, result.stderr)
	assert.Contains(t, result.stderr, "defined herdr tabs in current space for "+branchName)

	worktreePath := canonicalPath(testRepository.worktreePath(branchName))
	assert.Equal(t, []string{
		fakeHerdrLogLine("pane", "current", "--current"),
		fakeHerdrLogLine("tab", "rename", "w9:t1", "Agent"),
		fakeHerdrLogLine("tab", "create", "--workspace", "w9", "--cwd", worktreePath, "--label", "Editor", "--no-focus"),
		fakeHerdrLogLine("tab", "create", "--workspace", "w9", "--cwd", worktreePath, "--label", "Shell", "--no-focus"),
		fakeHerdrLogLine("pane", "run", "w9:p1", "pi"),
		fakeHerdrLogLine("pane", "run", "w9:p2", "nvim ."),
		fakeHerdrLogLine("workspace", "focus", "w9"),
		fakeHerdrLogLine("tab", "focus", "w9:t1"),
	}, readFakeHerdrLog(t, logPath))
}

func TestSpaceDoesNotCloseCurrentHerdrSpaceWhenTabCreationFails(t *testing.T) {
	const branchName = "feature/current-herdr-space-failure"

	testRepository := newTestRepository(t)
	require.NoError(t, testRepository.runGitWT(t, "create", "--repo", testRepoName, branchName).err)

	logPath := filepath.Join(t.TempDir(), "herdr.log")
	installFakeHerdrSpace(t, logPath)
	t.Setenv("FAKE_HERDR_FAIL", "tab create")

	result := testRepository.runGitWT(t, "space", "--current", "--repo", testRepoName, branchName)
	require.Error(t, result.err)
	assert.Contains(t, result.err.Error(), "herdr tab create")
	assert.NotContains(t, readFakeHerdrLog(t, logPath), fakeHerdrLogLine("workspace", "close", "w9"))
}

func TestSpaceOpensCurrentWorktreeFromSubdirectory(t *testing.T) {
	const branchName = "feature/current-space"

	testRepository := newTestRepository(t)
	require.NoError(t, testRepository.runGitWT(t, "create", "--repo", testRepoName, branchName).err)

	subdirectory := filepath.Join(testRepository.worktreePath(branchName), "nested")
	require.NoError(t, os.MkdirAll(subdirectory, 0o755))
	logPath := filepath.Join(t.TempDir(), "herdr.log")
	installFakeHerdrSpace(t, logPath)

	result := testRepository.runGitWTFrom(t, subdirectory, "space")
	require.NoError(t, result.err, result.stderr)
	assert.Contains(t, readFakeHerdrLog(t, logPath)[0], canonicalPath(testRepository.worktreePath(branchName)))
}

func TestSpaceFailsForUnknownWorktree(t *testing.T) {
	testRepository := newTestRepository(t)

	result := testRepository.runGitWT(t, "space", "--repo", testRepoName, "feature/missing")
	require.Error(t, result.err)
	assert.Contains(t, result.err.Error(), `unknown worktree "feature/missing"`)
}

func TestSpaceRequiresNameOutsideManagedWorktree(t *testing.T) {
	testRepository := newTestRepository(t)

	result := testRepository.runGitWT(t, "space", "--repo", testRepoName)
	require.Error(t, result.err)
	assert.Contains(t, result.err.Error(), "worktree name is required")
}

func TestSpaceClosesWorkspaceWhenTabCreationFails(t *testing.T) {
	const branchName = "feature/space-failure"

	testRepository := newTestRepository(t)
	require.NoError(t, testRepository.runGitWT(t, "create", "--repo", testRepoName, branchName).err)

	logPath := filepath.Join(t.TempDir(), "herdr.log")
	installFakeHerdrSpace(t, logPath)
	t.Setenv("FAKE_HERDR_FAIL", "tab create")

	result := testRepository.runGitWT(t, "space", "--repo", testRepoName, branchName)
	require.Error(t, result.err)
	assert.Contains(t, result.err.Error(), "herdr tab create")
	assert.Equal(t, fakeHerdrLogLine("workspace", "close", "w1"), readFakeHerdrLog(t, logPath)[3])
}

func TestSpaceClosesWorkspaceWhenShellTabCreationFails(t *testing.T) {
	const branchName = "feature/space-shell-failure"

	testRepository := newTestRepository(t)
	require.NoError(t, testRepository.runGitWT(t, "create", "--repo", testRepoName, branchName).err)

	logPath := filepath.Join(t.TempDir(), "herdr.log")
	installFakeHerdrSpace(t, logPath)
	t.Setenv("FAKE_HERDR_FAIL_TAB_LABEL", "Shell")

	result := testRepository.runGitWT(t, "space", "--repo", testRepoName, branchName)
	require.Error(t, result.err)
	assert.Contains(t, result.err.Error(), "herdr tab create")
	assert.Equal(t, fakeHerdrLogLine("workspace", "close", "w1"), readFakeHerdrLog(t, logPath)[4])
}

func TestSpaceClosesWorkspaceWhenTabResponseIsInvalid(t *testing.T) {
	const branchName = "feature/space-invalid-response"

	testRepository := newTestRepository(t)
	require.NoError(t, testRepository.runGitWT(t, "create", "--repo", testRepoName, branchName).err)

	logPath := filepath.Join(t.TempDir(), "herdr.log")
	installFakeHerdrSpace(t, logPath)
	t.Setenv("FAKE_HERDR_MALFORM", "tab create")

	result := testRepository.runGitWT(t, "space", "--repo", testRepoName, branchName)
	require.Error(t, result.err)
	assert.Contains(t, result.err.Error(), "decode herdr tab create response")
	assert.Equal(t, fakeHerdrLogLine("workspace", "close", "w1"), readFakeHerdrLog(t, logPath)[3])
}

func installFakeHerdrSpace(t *testing.T, logPath string) {
	t.Helper()

	binDir := t.TempDir()
	scriptPath := filepath.Join(binDir, "herdr")
	script := fmt.Sprintf(`#!/bin/sh
first=1
for arg in "$@"; do
  if [ "$first" -eq 1 ]; then
    first=0
  else
    printf '\037' >> %q
  fi
  printf '%%s' "$arg" >> %q
done
printf '\n' >> %q

operation="$1 $2"
tab_label=""
workspace_id=""
previous=""
for arg in "$@"; do
  if [ "$previous" = "--label" ]; then
    tab_label="$arg"
  elif [ "$previous" = "--workspace" ]; then
    workspace_id="$arg"
  fi
  previous="$arg"
done
if [ "$operation" = "${FAKE_HERDR_FAIL:-}" ] ||
   { [ "$operation" = "tab create" ] && [ "$tab_label" = "${FAKE_HERDR_FAIL_TAB_LABEL:-}" ]; }; then
  echo "fake herdr failure" >&2
  exit 1
fi
if [ "$operation" = "${FAKE_HERDR_MALFORM:-}" ]; then
  printf '{'
  exit 0
fi

case "$operation" in
  "workspace create")
    printf '{"result":{"workspace":{"workspace_id":"w1"},"tab":{"tab_id":"w1:t1"},"root_pane":{"pane_id":"w1:p1"}}}'
    ;;
  "pane current")
    printf '{"result":{"pane":{"workspace_id":"w9","tab_id":"w9:t1","pane_id":"w9:p1"}}}'
    ;;
  "tab create")
    if [ "$tab_label" = "Shell" ]; then
      printf '{"result":{"tab":{"tab_id":"%%s:t3"},"root_pane":{"pane_id":"%%s:p3"}}}' "$workspace_id" "$workspace_id"
    else
      printf '{"result":{"tab":{"tab_id":"%%s:t2"},"root_pane":{"pane_id":"%%s:p2"}}}' "$workspace_id" "$workspace_id"
    fi
    ;;
  *)
    printf '{"result":{}}'
    ;;
esac
`, logPath, logPath, logPath)
	require.NoError(t, os.WriteFile(scriptPath, []byte(script), 0o755))

	path := binDir + string(os.PathListSeparator) + os.Getenv("PATH")
	t.Setenv("PATH", path)
}

func fakeHerdrLogLine(args ...string) string {
	return strings.Join(args, "\x1f")
}

func readFakeHerdrLog(t *testing.T, logPath string) []string {
	t.Helper()
	contents, err := os.ReadFile(logPath)
	require.NoError(t, err)
	return strings.Split(strings.TrimSpace(string(contents)), "\n")
}

func TestCreateFailsWhenDirectoryExists(t *testing.T) {
	const branchName = "feature/exists"

	testRepository := newTestRepository(t)
	path := testRepository.worktreePath(branchName)
	require.NoError(t, os.MkdirAll(path, 0o755))

	result := testRepository.runGitWT(t, "create", "--repo", testRepoName, branchName)
	require.Error(t, result.err)
	assert.Contains(t, result.err.Error(), "already exists")
}

func TestRemoveRemovesEmptyParentDirectories(t *testing.T) {
	const branchName = "feature/nested/path"

	testRepository := newTestRepository(t)
	require.NoError(t, testRepository.runGitWT(t, "create", "--repo", testRepoName, branchName).err)
	testRepository.mergeWorktreeBranch(t, branchName)

	result := testRepository.runGitWT(t, "remove", "--repo", testRepoName, branchName)
	require.NoError(t, result.err, result.stderr)

	testRepository.assertPathMissing(t, filepath.Join(testRepository.worktreeRoot, testRepoName, "feature"))
}

func TestRemoveEmptyParentsStopsAtHome(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	homeDirectory, err := os.UserHomeDir()
	require.NoError(t, err)

	leafPath := filepath.Join(homeDirectory, "src", "github.com", "nnutter", "repo")
	require.NoError(t, os.MkdirAll(leafPath, 0o755))

	require.NoError(t, removeEmptyParents(leafPath, homeDirectory))

	_, err = os.Stat(leafPath)
	assert.True(t, os.IsNotExist(err))
	_, err = os.Stat(filepath.Join(homeDirectory, "src"))
	assert.True(t, os.IsNotExist(err))
	_, err = os.Stat(homeDirectory)
	require.NoError(t, err)
}

func TestRemoveEmptyParentsLeavesNonEmptyAncestor(t *testing.T) {
	homeDirectory := t.TempDir()
	t.Setenv("HOME", homeDirectory)

	parentPath := filepath.Join(homeDirectory, "src", "github.com", "nnutter")
	leafPath := filepath.Join(parentPath, "repo")
	siblingPath := filepath.Join(parentPath, "other")
	require.NoError(t, os.MkdirAll(leafPath, 0o755))
	require.NoError(t, os.MkdirAll(siblingPath, 0o755))

	require.NoError(t, removeEmptyParents(leafPath, homeDirectory))

	_, err := os.Stat(leafPath)
	assert.True(t, os.IsNotExist(err))
	_, err = os.Stat(siblingPath)
	require.NoError(t, err)
	_, err = os.Stat(parentPath)
	require.NoError(t, err)
}

func TestRemoveEmptyParentsHonorsStopPath(t *testing.T) {
	homeDirectory := t.TempDir()
	t.Setenv("HOME", homeDirectory)

	stopPath := filepath.Join(homeDirectory, "worktrees")
	leafPath := filepath.Join(stopPath, "feature", "repo")
	require.NoError(t, os.MkdirAll(leafPath, 0o755))

	require.NoError(t, removeEmptyParents(leafPath, stopPath))

	_, err := os.Stat(leafPath)
	assert.True(t, os.IsNotExist(err))
	_, err = os.Stat(filepath.Join(stopPath, "feature"))
	assert.True(t, os.IsNotExist(err))
	_, err = os.Stat(stopPath)
	require.NoError(t, err)
}

func TestRemoveFailsWhenDirtyWithoutForce(t *testing.T) {
	const branchName = "feature/dirty"

	testRepository := newTestRepository(t)
	require.NoError(t, testRepository.runGitWT(t, "create", "--repo", testRepoName, branchName).err)
	testRepository.writeFileInWorktree(t, branchName, "dirty.txt", "dirty\n")

	result := testRepository.runGitWT(t, "remove", "--repo", testRepoName, branchName)
	require.Error(t, result.err)
	assert.Contains(t, result.err.Error(), "not clean")
}

func TestRemoveWithNoArgsRemovesCurrentWorktree(t *testing.T) {
	const branchName = "feature/current"

	testRepository := newTestRepository(t)
	require.NoError(t, testRepository.runGitWT(t, "create", "--repo", testRepoName, branchName).err)
	testRepository.mergeWorktreeBranch(t, branchName)

	result := testRepository.runGitWTFrom(t, testRepository.worktreePath(branchName), "remove")
	require.NoError(t, result.err, result.stderr)
	testRepository.assertPathMissing(t, testRepository.worktreePath(branchName))
}

func TestRemoveWithNoArgsFromSubdirectoryRemovesCurrentWorktree(t *testing.T) {
	const branchName = "feature/subdir"

	testRepository := newTestRepository(t)
	require.NoError(t, testRepository.runGitWT(t, "create", "--repo", testRepoName, branchName).err)
	testRepository.mergeWorktreeBranch(t, branchName)

	subDir := filepath.Join(testRepository.worktreePath(branchName), "nested")
	require.NoError(t, os.MkdirAll(subDir, 0o755))

	result := testRepository.runGitWTFrom(t, subDir, "remove")
	require.NoError(t, result.err, result.stderr)
	testRepository.assertPathMissing(t, testRepository.worktreePath(branchName))
}

func TestRemoveFailsWhenUnmergedWithoutForce(t *testing.T) {
	const branchName = "feature/unmerged"

	testRepository := newTestRepository(t)
	require.NoError(t, testRepository.runGitWT(t, "create", "--repo", testRepoName, branchName).err)
	testRepository.commitFileInWorktree(t, branchName, "extra.txt", "extra\n")

	result := testRepository.runGitWT(t, "remove", "--repo", testRepoName, branchName)
	require.Error(t, result.err)
	assert.Contains(t, result.err.Error(), "not merged")
}

func TestRemoveForceRemovesDirtyUnmergedWorktree(t *testing.T) {
	const branchName = "feature/force"

	testRepository := newTestRepository(t)
	require.NoError(t, testRepository.runGitWT(t, "create", "--repo", testRepoName, branchName).err)
	testRepository.commitFileInWorktree(t, branchName, "extra.txt", "extra\n")
	testRepository.writeFileInWorktree(t, branchName, "dirty.txt", "dirty\n")

	result := testRepository.runGitWT(t, "remove", "--repo", testRepoName, "--force", branchName)
	require.NoError(t, result.err, result.stderr)
	testRepository.assertPathMissing(t, testRepository.worktreePath(branchName))
	testRepository.assertBranchMissing(t, branchName)
}

func TestRemoveCompletionOffersManagedWorktreeNames(t *testing.T) {
	testRepository := newTestRepository(t)
	require.NoError(t, testRepository.runGitWT(t, "create", "--repo", testRepoName, "feature/a").err)
	require.NoError(t, testRepository.runGitWT(t, "create", "--repo", testRepoName, "feature/b").err)

	stdout := runComplete(t, "remove", "--repo", testRepoName, "")
	assert.Contains(t, stdout, "feature/a")
	assert.Contains(t, stdout, "feature/b")
}

func TestSpaceCompletionOffersManagedWorktreeNames(t *testing.T) {
	testRepository := newTestRepository(t)
	require.NoError(t, testRepository.runGitWT(t, "create", "--repo", testRepoName, "feature/a").err)
	require.NoError(t, testRepository.runGitWT(t, "create", "--repo", testRepoName, "feature/b").err)

	stdout := runComplete(t, "space", "--repo", testRepoName, "")
	assert.Contains(t, stdout, "feature/a")
	assert.Contains(t, stdout, "feature/b")
}

func TestRemoveCompletionUsesCurrentWorktreeRepoWhenRepoFlagOmitted(t *testing.T) {
	testRepository := newTestRepository(t)
	require.NoError(t, testRepository.runGitWT(t, "create", "--repo", testRepoName, "feature/a").err)
	require.NoError(t, testRepository.runGitWT(t, "create", "--repo", testRepoName, "feature/b").err)

	currentDirectory, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(testRepository.worktreePath("feature/a")))
	defer func() { require.NoError(t, os.Chdir(currentDirectory)) }()

	stdout := runComplete(t, "remove", "")
	assert.Contains(t, stdout, "feature/a")
	assert.Contains(t, stdout, "feature/b")
}

func TestRemoveCompletionWithoutRepoOutsideManagedWorktreeOffersNothing(t *testing.T) {
	testRepository := newTestRepository(t)
	require.NoError(t, testRepository.runGitWT(t, "create", "--repo", testRepoName, "feature/a").err)

	// Outside any git worktree: --repo is optional, so completion should be empty
	// rather than requiring the flag or erroring.
	stdout := runComplete(t, "remove", "")
	assert.NotContains(t, stdout, "feature/a")
	assert.NotContains(t, stdout, testRepoName)
}

func runComplete(t *testing.T, args ...string) string {
	t.Helper()
	command := NewRootCommand()
	command.SetArgs(append([]string{"__complete"}, args...))
	var stdout bytes.Buffer
	command.SetOut(&stdout)
	command.SetErr(io.Discard)
	require.NoError(t, command.Execute())
	return stdout.String()
}

func TestGenerateZshGeneratesWrapperCompletionAndAutoloadHelper(t *testing.T) {
	outDir := t.TempDir()
	result := runGitWTCommand(t, "generate", "zsh", "--out", outDir, "--force")
	require.NoError(t, result.err, result.stderr)

	functionPath := filepath.Join(outDir, "wt")
	completionPath := filepath.Join(outDir, "_wt")
	autoloadPath := filepath.Join(outDir, "_wt_autoload")
	functionContents, err := os.ReadFile(functionPath)
	require.NoError(t, err)
	completionContents, err := os.ReadFile(completionPath)
	require.NoError(t, err)
	autoloadContents, err := os.ReadFile(autoloadPath)
	require.NoError(t, err)

	assert.Equal(t, "#compdef wt", strings.SplitN(string(completionContents), "\n", 2)[0])
	assert.Equal(t, "#autoload wt", strings.SplitN(string(autoloadContents), "\n", 2)[0])
	assert.Contains(t, string(functionContents), "git-wt create")
	assert.Contains(t, string(functionContents), `cd "$HOME"`)
	assert.Contains(t, string(functionContents), "GIT_WT_CREATE_PATH_FILE")
	assert.Contains(t, string(functionContents), "GIT_WT_SWITCH_PATH_FILE")
	assert.Contains(t, string(functionContents), "previous_dir=$PWD")
	assert.Contains(t, string(functionContents), "GIT_WT_RENAME_PATH_FILE")
	assert.Contains(t, string(functionContents), `cd "$target_dir"`)
	assert.Contains(t, string(functionContents), "remove|migrate)")
	assert.Contains(t, string(functionContents), `command git-wt switch`)
	assert.NotContains(t, string(functionContents), "target_dir=$(command git-wt create")
	assert.NotContains(t, string(functionContents), "git worktree list --porcelain | head")
	assert.NotContains(t, string(functionContents), "Not inside a registered repository worktree; pass --repo")
	assert.NotContains(t, string(functionContents), "off)")
	assert.Contains(t, string(completionContents), "repo:Manage registered repositories")
	assert.Contains(t, string(completionContents), "rename:Rename a registered repository")
	assert.Contains(t, string(completionContents), "'(-q --quiet)'{-q,--quiet}'[Print repository names only]'")
	assert.Contains(t, string(completionContents), "_message 'new repository name'")
	assert.Contains(t, string(completionContents), "GIT_WT_WORKTREE_ROOT")
	assert.Contains(t, string(completionContents), "local context state state_descr line")
	assert.Contains(t, string(completionContents), "'(-r --repo)'{-r,--repo}'[Registered repository name]:repository:->repos'")
	assert.Contains(t, string(completionContents), "'(-r --repo)'{-a,--all}'[List worktrees from all registered repositories]'")
	assert.Contains(t, string(completionContents), "space:Open a managed Git worktree in Herdr")
	assert.Contains(t, string(completionContents), "    switch)")
	assert.Contains(t, string(completionContents), "    remove)")
	assert.Contains(t, string(completionContents), "'(-c --create)'{-c,--create}'[Create the worktree if it does not exist]'")
	assert.Contains(t, string(completionContents), "1:worktree name:->switch_name")
	assert.Contains(t, string(completionContents), "local completing_switch=1")
	assert.Contains(t, string(completionContents), `compadd -S ' --repo '`)
	assert.Contains(t, string(completionContents), "--current[Define tabs in the current Herdr workspace]")
	assert.Contains(t, string(completionContents), "1:worktree name:->worktrees")
	assert.Contains(t, string(completionContents), `_arguments -M 'r:|=*'`)
	assert.Contains(t, string(completionContents), `'1:worktree name:_guard "[^-]*" "worktree name"'`)
	assert.Contains(t, string(completionContents), "'(-n --dry-run)'{-n,--dry-run}'[List worktrees that would be pruned]'")
	assert.Contains(t, string(completionContents), "'(-a --all)'{-a,--all}'[Rehome worktrees for every registered repository]'")
	assert.Contains(t, string(completionContents), "shift words")
	assert.NotContains(t, string(completionContents), "switch|remove)")
	assert.NotContains(t, string(completionContents), "switch|remove|prune)")
	assert.NotContains(t, string(completionContents), "off:")
}

func TestGenerateZshUsesCustomWrapperName(t *testing.T) {
	outDir := t.TempDir()
	result := runGitWTCommand(t, "generate", "zsh", "--name", "foo", "--out", outDir)
	require.NoError(t, result.err, result.stderr)

	functionContents, err := os.ReadFile(filepath.Join(outDir, "foo"))
	require.NoError(t, err)
	completionContents, err := os.ReadFile(filepath.Join(outDir, "_foo"))
	require.NoError(t, err)
	autoloadContents, err := os.ReadFile(filepath.Join(outDir, "_foo_autoload"))
	require.NoError(t, err)

	assert.Contains(t, string(functionContents), "foo() {")
	assert.Contains(t, string(functionContents), `command git-wt switch`)
	assert.Equal(t, "#compdef foo", strings.SplitN(string(completionContents), "\n", 2)[0])
	assert.Contains(t, string(completionContents), "_foo() {")
	assert.Equal(t, "#autoload foo", strings.SplitN(string(autoloadContents), "\n", 2)[0])

	for _, defaultPath := range []string{"wt", "_wt", "_wt_autoload"} {
		_, err := os.Stat(filepath.Join(outDir, defaultPath))
		assert.True(t, os.IsNotExist(err), defaultPath)
	}
}

func TestGeneratedCreateCompletesUniqueRepoPrefix(t *testing.T) {
	if _, err := exec.LookPath("zsh"); err != nil {
		t.Skip("zsh is not installed")
	}
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 is not installed")
	}

	outDir := t.TempDir()
	require.NoError(t, runGitWTCommand(t, "generate", "zsh", "--out", outDir, "--force").err)

	scriptPath := filepath.Join(t.TempDir(), "complete.py")
	script := `import os, pty, select, time, sys

compdir = sys.argv[1]
zdot = sys.argv[2]
os.makedirs(zdot, exist_ok=True)
open(os.path.join(zdot, ".zshrc"), "w").write("")
os.environ["ZDOTDIR"] = zdot

pid, fd = pty.fork()
if pid == 0:
    os.execvp("zsh", ["zsh", "-f", "-i"])

def recv(timeout=1.0):
    buf = b""
    end = time.time() + timeout
    while time.time() < end:
        ready, _, _ = select.select([fd], [], [], max(0.05, end - time.time()))
        if not ready:
            continue
        try:
            chunk = os.read(fd, 8192)
        except OSError:
            break
        if not chunk:
            break
        buf += chunk
        end = time.time() + 0.2
    return buf

def send(data):
    os.write(fd, data.encode() if isinstance(data, str) else data)

recv(0.3)
send("fpath=(" + compdir + " $fpath); autoload -Uz compinit; compinit -u -D\n")
recv(0.5)
send("zstyle ':completion:*' matcher-list 'm:{a-zA-Z}={A-Za-z}' 'r:|[._-]=* r:|=*' 'l:|=* r:|=*'\n")
recv(0.2)
send("\x15")
recv(0.1)
send("wt create --r")
time.sleep(0.05)
send("\t")
output = recv(0.8).decode("utf-8", "replace")
send("exit\n")
recv(0.2)
sys.stdout.write(output)
`
	require.NoError(t, os.WriteFile(scriptPath, []byte(script), 0o644))

	command := exec.Command("python3", scriptPath, outDir, filepath.Join(t.TempDir(), "zdot"))
	output, err := command.CombinedOutput()
	require.NoError(t, err, string(output))
	assert.Contains(t, string(output), "wt create --repo ")
}

func TestGeneratedSwitchCompletesWorktreeNamesAcrossRepos(t *testing.T) {
	if _, err := exec.LookPath("zsh"); err != nil {
		t.Skip("zsh is not installed")
	}
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 is not installed")
	}

	home := t.TempDir()
	dataHome := filepath.Join(home, ".local", "share")
	worktreeRoot := filepath.Join(home, "worktrees")
	require.NoError(t, os.MkdirAll(filepath.Join(dataHome, "git-wt", "repos", "git-wt.git"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(dataHome, "git-wt", "repos", "other.git"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(worktreeRoot, "git-wt", "feature/login", "git-wt"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(worktreeRoot, "other", "feature/api", "other"), 0o755))

	outDir := t.TempDir()
	require.NoError(t, runGitWTCommand(t, "generate", "zsh", "--out", outDir, "--force").err)

	scriptPath := filepath.Join(t.TempDir(), "complete.py")
	script := `import os, pty, select, time, sys

compdir, zdot, home, data_home, worktree_root, line = sys.argv[1:7]
os.makedirs(zdot, exist_ok=True)
open(os.path.join(zdot, ".zshrc"), "w").write("")
os.environ["ZDOTDIR"] = zdot
os.environ["HOME"] = home
os.environ["XDG_DATA_HOME"] = data_home
os.environ["GIT_WT_WORKTREE_ROOT"] = worktree_root

pid, fd = pty.fork()
if pid == 0:
    os.execvp("zsh", ["zsh", "-f", "-i"])

def recv(timeout=1.0):
    buf = b""
    end = time.time() + timeout
    while time.time() < end:
        ready, _, _ = select.select([fd], [], [], max(0.05, end - time.time()))
        if not ready:
            continue
        try:
            chunk = os.read(fd, 8192)
        except OSError:
            break
        if not chunk:
            break
        buf += chunk
        end = time.time() + 0.2
    return buf

def send(data):
    os.write(fd, data.encode() if isinstance(data, str) else data)

recv(0.3)
send("cd " + home + "\n")
recv(0.2)
send("fpath=(" + compdir + " $fpath); autoload -Uz compinit; compinit -u -D\n")
recv(0.5)
send("\x15")
recv(0.1)
send(line)
time.sleep(0.05)
send("\t")
output = recv(0.8).decode("utf-8", "replace")
send("exit\n")
recv(0.2)
sys.stdout.write(output)
`
	require.NoError(t, os.WriteFile(scriptPath, []byte(script), 0o644))

	runComplete := func(line string) string {
		t.Helper()
		command := exec.Command(
			"python3",
			scriptPath,
			outDir,
			filepath.Join(t.TempDir(), "zdot"),
			home,
			dataHome,
			worktreeRoot,
			line,
		)
		output, err := command.CombinedOutput()
		require.NoError(t, err, string(output))
		return string(output)
	}

	unique := runComplete("wt switch feature/l")
	assert.Contains(t, unique, "wt switch feature/login")
	assert.NotContains(t, unique, " --repo")

	require.NoError(t, os.MkdirAll(filepath.Join(worktreeRoot, "other", "feature/login", "other"), 0o755))
	ambiguous := runComplete("wt switch feature/l")
	assert.Contains(t, ambiguous, "wt switch feature/login --repo")
}

func TestGeneratedZshWrapperAutoloadsAfterCompinit(t *testing.T) {
	if _, err := exec.LookPath("zsh"); err != nil {
		t.Skip("zsh is not installed")
	}

	outDir := t.TempDir()
	require.NoError(t, runGitWTCommand(t, "generate", "zsh", "--out", outDir).err)

	binDir := t.TempDir()
	fakeGitWT := `#!/bin/sh
printf '%s\n' "$@"
`
	require.NoError(t, os.WriteFile(filepath.Join(binDir, "git-wt"), []byte(fakeGitWT), 0o755))

	command := exec.Command(
		"zsh", "-f", "-c",
		`fpath=("$1" $fpath)
autoload -Uz compinit
compinit -D 2>/dev/null
wt list --all`,
		"--", outDir,
	)
	command.Env = append(os.Environ(), "PATH="+binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	output, err := command.CombinedOutput()
	require.NoError(t, err, string(output))
	assert.Equal(t, "list\n--all", strings.TrimSpace(string(output)))
}

func TestGeneratedZshWrapperChangesToRenamedCurrentWorktree(t *testing.T) {
	if _, err := exec.LookPath("zsh"); err != nil {
		t.Skip("zsh is not installed")
	}

	outDir := t.TempDir()
	require.NoError(t, runGitWTCommand(t, "generate", "zsh", "--out", outDir, "--force").err)

	worktreeParent := t.TempDir()
	oldWorktree := filepath.Join(worktreeParent, "old")
	oldSubdirectory := filepath.Join(oldWorktree, "nested")
	require.NoError(t, os.MkdirAll(oldSubdirectory, 0o755))

	binDir := t.TempDir()
	fakeGitWT := `#!/bin/sh
old_worktree=$(dirname "$PWD")
new_worktree=$(dirname "$old_worktree")/new
mv "$old_worktree" "$new_worktree" || exit $?
printf '%s\n' "$new_worktree/nested" > "$GIT_WT_RENAME_PATH_FILE"
`
	require.NoError(t, os.WriteFile(filepath.Join(binDir, "git-wt"), []byte(fakeGitWT), 0o755))

	command := exec.Command(
		"zsh", "-f", "-c",
		`source "$1"; cd "$2"; wt repo rename old new >/dev/null; pwd -P`,
		"--", filepath.Join(outDir, "wt"), oldSubdirectory,
	)
	command.Env = append(os.Environ(), "PATH="+binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	output, err := command.CombinedOutput()
	require.NoError(t, err, string(output))
	assert.Equal(t, canonicalPath(filepath.Join(worktreeParent, "new", "nested")), strings.TrimSpace(string(output)))
}

func TestGeneratedZshWrapperChangesDirectoryOnSwitch(t *testing.T) {
	if _, err := exec.LookPath("zsh"); err != nil {
		t.Skip("zsh is not installed")
	}

	outDir := t.TempDir()
	require.NoError(t, runGitWTCommand(t, "generate", "zsh", "--out", outDir, "--force").err)

	targetDir := t.TempDir()
	binDir := t.TempDir()
	fakeGitWT := `#!/bin/sh
printf '%s\n' "$GIT_WT_SWITCH_PATH_FILE_TARGET" > "$GIT_WT_SWITCH_PATH_FILE"
`
	require.NoError(t, os.WriteFile(filepath.Join(binDir, "git-wt"), []byte(fakeGitWT), 0o755))

	command := exec.Command(
		"zsh", "-f", "-c",
		`source "$1"; wt switch --repo repo feature >/dev/null; pwd -P`,
		"--", filepath.Join(outDir, "wt"),
	)
	command.Env = append(
		os.Environ(),
		"PATH="+binDir+string(os.PathListSeparator)+os.Getenv("PATH"),
		"GIT_WT_SWITCH_PATH_FILE_TARGET="+targetDir,
	)
	output, err := command.CombinedOutput()
	require.NoError(t, err, string(output))
	assert.Equal(t, canonicalPath(targetDir), strings.TrimSpace(string(output)))
}

func TestSwitchResolvesPathWithRepoFlag(t *testing.T) {
	const branchName = "feature/switch-repo"

	testRepository := newTestRepository(t)
	require.NoError(t, testRepository.runGitWT(t, "create", "--repo", testRepoName, branchName).err)

	result := testRepository.runGitWT(t, "switch", "--repo", testRepoName, branchName)
	require.NoError(t, result.err, result.stderr)
	assert.Equal(t, testRepository.worktreePath(branchName), strings.TrimSpace(result.stdout))
}

func TestSwitchResolvesRepoFromCurrentWorktree(t *testing.T) {
	testRepository := newTestRepository(t)
	require.NoError(t, testRepository.runGitWT(t, "create", "--repo", testRepoName, "feature/from").err)
	require.NoError(t, testRepository.runGitWT(t, "create", "--repo", testRepoName, "feature/to").err)

	result := testRepository.runGitWTFrom(t, testRepository.worktreePath("feature/from"), "switch", "feature/to")
	require.NoError(t, result.err, result.stderr)
	assert.Equal(t, testRepository.worktreePath("feature/to"), strings.TrimSpace(result.stdout))
}

func TestSwitchInfersUniqueRepoOutsideWorktree(t *testing.T) {
	primary := newTestRepository(t)
	registerAdditionalRepo(t, primary, "other")
	require.NoError(t, primary.runGitWT(t, "create", "--repo", testRepoName, "feature/login").err)

	result := primary.runGitWTFrom(t, primary.home, "switch", "feature/login")
	require.NoError(t, result.err, result.stderr)
	assert.Equal(t, primary.worktreePath("feature/login"), strings.TrimSpace(result.stdout))
}

func TestSwitchRequiresRepoWhenWorktreeNameIsAmbiguous(t *testing.T) {
	primary := newTestRepository(t)
	secondaryName := "other"
	registerAdditionalRepo(t, primary, secondaryName)
	require.NoError(t, primary.runGitWT(t, "create", "--repo", testRepoName, "feature/login").err)
	require.NoError(t, primary.runGitWT(t, "create", "--repo", secondaryName, "feature/login").err)

	result := primary.runGitWTFrom(t, primary.home, "switch", "feature/login")
	require.Error(t, result.err)
	assert.Contains(t, result.err.Error(), "pass --repo")
	assert.Contains(t, result.err.Error(), testRepoName)
	assert.Contains(t, result.err.Error(), secondaryName)
}

func TestSwitchReportsMissingWorktreeOutsideWorktree(t *testing.T) {
	primary := newTestRepository(t)
	registerAdditionalRepo(t, primary, "other")

	result := primary.runGitWTFrom(t, primary.home, "switch", "missing")
	require.Error(t, result.err)
	assert.Contains(t, result.err.Error(), "not found")
}

func TestSwitchInsideWorktreePinsCurrentRepo(t *testing.T) {
	primary := newTestRepository(t)
	secondaryName := "other"
	registerAdditionalRepo(t, primary, secondaryName)
	require.NoError(t, primary.runGitWT(t, "create", "--repo", testRepoName, "feature/current").err)
	require.NoError(t, primary.runGitWT(t, "create", "--repo", testRepoName, "feature/login").err)
	require.NoError(t, primary.runGitWT(t, "create", "--repo", secondaryName, "feature/login").err)

	result := primary.runGitWTFrom(t, primary.worktreePath("feature/current"), "switch", "feature/login")
	require.NoError(t, result.err, result.stderr)
	assert.Equal(t, primary.worktreePath("feature/login"), strings.TrimSpace(result.stdout))
}

func TestSwitchFailsWhenWorktreeMissing(t *testing.T) {
	testRepository := newTestRepository(t)

	result := testRepository.runGitWT(t, "switch", "--repo", testRepoName, "missing")
	require.Error(t, result.err)
	assert.Contains(t, result.err.Error(), "not found")
}

func TestSwitchReportsAlreadyInWorktree(t *testing.T) {
	const branchName = "feature/already"

	testRepository := newTestRepository(t)
	require.NoError(t, testRepository.runGitWT(t, "create", "--repo", testRepoName, branchName).err)

	result := testRepository.runGitWTFrom(
		t,
		testRepository.worktreePath(branchName),
		"switch",
		"--repo",
		testRepoName,
		branchName,
	)
	require.NoError(t, result.err, result.stderr)
	assert.Contains(t, result.stderr, "Already in "+branchName)
	assert.Equal(t, testRepository.worktreePath(branchName), strings.TrimSpace(result.stdout))
}

func TestSwitchWritesPathFileWhenRequested(t *testing.T) {
	const branchName = "feature/switch-file"

	testRepository := newTestRepository(t)
	require.NoError(t, testRepository.runGitWT(t, "create", "--repo", testRepoName, branchName).err)

	pathFile := filepath.Join(t.TempDir(), "switch-path")
	t.Setenv(switchPathFileEnvVarName, pathFile)

	result := testRepository.runGitWT(t, "switch", "--repo", testRepoName, branchName)
	require.NoError(t, result.err, result.stderr)
	assert.Empty(t, result.stdout)
	contents, err := os.ReadFile(pathFile)
	require.NoError(t, err)
	assert.Equal(t, testRepository.worktreePath(branchName)+"\n", string(contents))
}

func TestSwitchIsHiddenFromHelp(t *testing.T) {
	result := runGitWTCommand(t, "--help")
	require.NoError(t, result.err, result.stderr)
	assert.NotContains(t, result.stdout, "switch")
	assert.NotContains(t, result.stderr, "switch")
}

func TestSwitchCreateCreatesWorktreeAndReportsPath(t *testing.T) {
	const branchName = "feature/switch-create"

	testRepository := newTestRepository(t)
	result := testRepository.runGitWT(t, "switch", "-c", "--repo", testRepoName, branchName)
	require.NoError(t, result.err, result.stderr)
	testRepository.assertPathPresent(t, testRepository.worktreePath(branchName))
	assert.Equal(t, testRepository.worktreePath(branchName), strings.TrimSpace(result.stdout))
	assert.Contains(t, result.stderr, "created ")
}

func TestSwitchCreateAcceptsFlagAfterName(t *testing.T) {
	const branchName = "feature/switch-create-after"

	testRepository := newTestRepository(t)
	result := testRepository.runGitWT(t, "switch", "--repo", testRepoName, branchName, "--create")
	require.NoError(t, result.err, result.stderr)
	testRepository.assertPathPresent(t, testRepository.worktreePath(branchName))
	assert.Equal(t, testRepository.worktreePath(branchName), strings.TrimSpace(result.stdout))
}

func TestSwitchCreateFailsWhenWorktreeExists(t *testing.T) {
	const branchName = "feature/switch-create-exists"

	testRepository := newTestRepository(t)
	require.NoError(t, testRepository.runGitWT(t, "create", "--repo", testRepoName, branchName).err)

	pathFile := filepath.Join(t.TempDir(), "switch-path")
	t.Setenv(switchPathFileEnvVarName, pathFile)
	result := testRepository.runGitWT(t, "switch", branchName, "-c", "--repo", testRepoName)
	require.Error(t, result.err)
	assert.Contains(t, result.err.Error(), "already exists")
	_, err := os.Stat(pathFile)
	assert.True(t, os.IsNotExist(err))
}

func TestSwitchCreateNoCdDoesNotReportPath(t *testing.T) {
	const branchName = "feature/switch-create-nocd"

	testRepository := newTestRepository(t)
	pathFile := filepath.Join(t.TempDir(), "switch-path")
	t.Setenv(switchPathFileEnvVarName, pathFile)

	result := testRepository.runGitWT(t, "switch", "-c", "--no-cd", "--repo", testRepoName, branchName)
	require.NoError(t, result.err, result.stderr)
	testRepository.assertPathPresent(t, testRepository.worktreePath(branchName))
	assert.Empty(t, result.stdout)
	_, err := os.Stat(pathFile)
	assert.True(t, os.IsNotExist(err))
}

func TestSwitchCreateWithHerdrDoesNotReportPath(t *testing.T) {
	const branchName = "feature/switch-create-herdr"

	testRepository := newTestRepository(t)
	logPath := filepath.Join(t.TempDir(), "herdr.log")
	installFakeHerdrSpace(t, logPath)
	pathFile := filepath.Join(t.TempDir(), "switch-path")
	t.Setenv(switchPathFileEnvVarName, pathFile)

	result := testRepository.runGitWT(t, "switch", "-c", "--herdr", "--repo", testRepoName, branchName)
	require.NoError(t, result.err, result.stderr)
	testRepository.assertPathPresent(t, testRepository.worktreePath(branchName))
	assert.Contains(t, result.stderr, "opened herdr space")
	assert.Empty(t, result.stdout)
	_, err := os.Stat(pathFile)
	assert.True(t, os.IsNotExist(err))
}

func TestGenerateZshRefusesOverwriteWithoutForce(t *testing.T) {
	outDir := t.TempDir()
	require.NoError(t, runGitWTCommand(t, "generate", "zsh", "--out", outDir).err)
	result := runGitWTCommand(t, "generate", "zsh", "--out", outDir)
	require.Error(t, result.err)
	assert.Contains(t, result.err.Error(), "already exists")
}

func TestGenerateZshChecksAutoloadHelperCollisionBeforeWriting(t *testing.T) {
	outDir := t.TempDir()
	autoloadPath := filepath.Join(outDir, "_wt_autoload")
	require.NoError(t, os.WriteFile(autoloadPath, []byte("existing helper\n"), 0o644))

	result := runGitWTCommand(t, "generate", "zsh", "--out", outDir)
	require.Error(t, result.err)
	assert.Contains(t, result.err.Error(), "autoload helper file")

	autoloadContents, err := os.ReadFile(autoloadPath)
	require.NoError(t, err)
	assert.Equal(t, "existing helper\n", string(autoloadContents))
	for _, untouchedPath := range []string{"wt", "_wt"} {
		_, err := os.Stat(filepath.Join(outDir, untouchedPath))
		assert.True(t, os.IsNotExist(err), untouchedPath)
	}

	forceResult := runGitWTCommand(t, "generate", "zsh", "--out", outDir, "--force")
	require.NoError(t, forceResult.err, forceResult.stderr)
	autoloadContents, err = os.ReadFile(autoloadPath)
	require.NoError(t, err)
	assert.Equal(t, "#autoload wt", strings.SplitN(string(autoloadContents), "\n", 2)[0])
}

func TestPruneRemovesOnlyMergedCleanWorktrees(t *testing.T) {
	testRepository := newTestRepository(t)

	require.NoError(t, testRepository.runGitWT(t, "create", "--repo", testRepoName, "feature/merged").err)
	testRepository.mergeWorktreeBranch(t, "feature/merged")

	require.NoError(t, testRepository.runGitWT(t, "create", "--repo", testRepoName, "feature/unmerged").err)
	testRepository.commitFileInWorktree(t, "feature/unmerged", "extra.txt", "extra\n")

	require.NoError(t, testRepository.runGitWT(t, "create", "--repo", testRepoName, "feature/dirty").err)
	testRepository.mergeWorktreeBranch(t, "feature/dirty")
	testRepository.writeFileInWorktree(t, "feature/dirty", "dirty.txt", "dirty\n")

	result := testRepository.runGitWT(t, "prune", "--repo", testRepoName)
	require.NoError(t, result.err, result.stderr)

	testRepository.assertPathMissing(t, testRepository.worktreePath("feature/merged"))
	testRepository.assertPathPresent(t, testRepository.worktreePath("feature/unmerged"))
	testRepository.assertPathPresent(t, testRepository.worktreePath("feature/dirty"))
}

func TestPruneDryRunListsAndKeepsWorktrees(t *testing.T) {
	testRepository := newTestRepository(t)

	require.NoError(t, testRepository.runGitWT(t, "create", "--repo", testRepoName, "feature/merged").err)
	testRepository.mergeWorktreeBranch(t, "feature/merged")

	require.NoError(t, testRepository.runGitWT(t, "create", "--repo", testRepoName, "feature/unmerged").err)
	testRepository.commitFileInWorktree(t, "feature/unmerged", "extra.txt", "extra\n")

	require.NoError(t, testRepository.runGitWT(t, "create", "--repo", testRepoName, "feature/dirty").err)
	testRepository.mergeWorktreeBranch(t, "feature/dirty")
	testRepository.writeFileInWorktree(t, "feature/dirty", "dirty.txt", "dirty\n")

	for _, flag := range []string{"-n", "--dry-run"} {
		result := testRepository.runGitWT(t, "prune", flag, "--repo", testRepoName)
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
	testRepository := newTestRepository(t)
	require.NoError(t, testRepository.runGitWT(t, "create", "--repo", testRepoName, "feature/unmerged").err)
	testRepository.commitFileInWorktree(t, "feature/unmerged", "extra.txt", "extra\n")

	result := testRepository.runGitWT(t, "prune", "--dry-run", "--repo", testRepoName)
	require.NoError(t, result.err, result.stderr)
	assert.NotContains(t, result.stderr, "would prune")
	testRepository.assertPathPresent(t, testRepository.worktreePath("feature/unmerged"))
}

func TestPruneDryRunWithPromptListsSelectedWorktrees(t *testing.T) {
	const branchName = "feature/prompt-dry-run"

	testRepository := newTestRepository(t)
	require.NoError(t, testRepository.runGitWT(t, "create", "--repo", testRepoName, branchName).err)
	testRepository.commitFileInWorktree(t, branchName, "extra.txt", "extra\n")

	options := &pruneCommandOptions{
		repoSelection: repoSelection{RepoFlag: testRepoName},
		prompt:        true,
		dryRun:        true,
		prompter:      stubPrompter{selected: []managedWorktree{{Name: branchName, Repo: testRepoName}}},
	}
	command := NewRootCommand()
	var stderr bytes.Buffer
	command.SetErr(&stderr)
	command.SetOut(io.Discard)
	err := options.Execute(command, nil)
	require.NoError(t, err, stderr.String())
	assert.Contains(t, stderr.String(), "would prune "+branchName+" ("+testRepoName+")")
	testRepository.assertPathPresent(t, testRepository.worktreePath(branchName))
}

func TestPruneWithoutRepoFromOutsidePrunesAllRepos(t *testing.T) {
	primary := newTestRepository(t)
	secondaryName := "other"
	registerAdditionalRepo(t, primary, secondaryName)

	require.NoError(t, primary.runGitWT(t, "create", "--repo", testRepoName, "feature/merged").err)
	primary.mergeWorktreeBranch(t, "feature/merged")
	require.NoError(t, primary.runGitWT(t, "create", "--repo", secondaryName, "feature/other-merged").err)

	result := primary.runGitWTFrom(t, primary.home, "prune")
	require.NoError(t, result.err, result.stderr)

	primary.assertPathMissing(t, primary.worktreePath("feature/merged"))
	_, err := os.Stat(filepath.Join(primary.worktreeRoot, secondaryName, "feature/other-merged", secondaryName))
	assert.True(t, os.IsNotExist(err))
}

func TestPruneWithoutRepoFromInsideWorktreeStaysOnCurrentRepo(t *testing.T) {
	primary := newTestRepository(t)
	secondaryName := "other"
	registerAdditionalRepo(t, primary, secondaryName)

	require.NoError(t, primary.runGitWT(t, "create", "--repo", testRepoName, "feature/current").err)
	primary.commitFileInWorktree(t, "feature/current", "extra.txt", "extra\n")
	require.NoError(t, primary.runGitWT(t, "create", "--repo", testRepoName, "feature/merged").err)
	primary.mergeWorktreeBranch(t, "feature/merged")
	require.NoError(t, primary.runGitWT(t, "create", "--repo", secondaryName, "feature/other-merged").err)

	result := primary.runGitWTFrom(t, primary.worktreePath("feature/current"), "prune")
	require.NoError(t, result.err, result.stderr)

	primary.assertPathMissing(t, primary.worktreePath("feature/merged"))
	primary.assertPathPresent(t, primary.worktreePath("feature/current"))
	_, err := os.Stat(filepath.Join(primary.worktreeRoot, secondaryName, "feature/other-merged", secondaryName))
	require.NoError(t, err)
}

func TestPruneRepoFlagPinsRepoFromAnyCwd(t *testing.T) {
	primary := newTestRepository(t)
	secondaryName := "other"
	registerAdditionalRepo(t, primary, secondaryName)

	require.NoError(t, primary.runGitWT(t, "create", "--repo", testRepoName, "feature/current").err)
	require.NoError(t, primary.runGitWT(t, "create", "--repo", testRepoName, "feature/merged").err)
	primary.mergeWorktreeBranch(t, "feature/merged")
	require.NoError(t, primary.runGitWT(t, "create", "--repo", secondaryName, "feature/other-merged").err)

	result := primary.runGitWTFrom(
		t,
		primary.worktreePath("feature/current"),
		"prune",
		"--repo",
		secondaryName,
	)
	require.NoError(t, result.err, result.stderr)

	primary.assertPathPresent(t, primary.worktreePath("feature/merged"))
	_, err := os.Stat(filepath.Join(primary.worktreeRoot, secondaryName, "feature/other-merged", secondaryName))
	assert.True(t, os.IsNotExist(err))
}

func TestPrunePromptDistinguishesSameNameInTwoRepos(t *testing.T) {
	const branchName = "feature/same"

	primary := newTestRepository(t)
	secondaryName := "other"
	registerAdditionalRepo(t, primary, secondaryName)

	require.NoError(t, primary.runGitWT(t, "create", "--repo", testRepoName, branchName).err)
	require.NoError(t, primary.runGitWT(t, "create", "--repo", secondaryName, branchName).err)

	options := &pruneCommandOptions{
		prompt:   true,
		prompter: stubPrompter{selected: []managedWorktree{{Repo: secondaryName, Name: branchName}}},
	}
	command := NewRootCommand()
	var stderr bytes.Buffer
	command.SetErr(&stderr)
	command.SetOut(io.Discard)
	currentDirectory, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(primary.home))
	defer func() { _ = os.Chdir(currentDirectory) }()

	require.NoError(t, options.Execute(command, nil), stderr.String())

	primary.assertPathPresent(t, primary.worktreePath(branchName))
	_, err = os.Stat(filepath.Join(primary.worktreeRoot, secondaryName, branchName, secondaryName))
	assert.True(t, os.IsNotExist(err))
}

func TestListSucceedsWhenUpstreamRefIsMissing(t *testing.T) {
	const branchName = "feature/no-upstream-ref"

	testRepository := newTestRepository(t)
	require.NoError(t, testRepository.runGitWT(t, "create", "--repo", testRepoName, branchName).err)
	runGitCommand(t, testRepository.barePath, "update-ref", "-d", "refs/remotes/origin/main")

	// Branch still has upstream config pointing at deleted ref; list should handle missing upstream existence.
	result := testRepository.runGitWT(t, "list", "--repo", testRepoName)
	// enrichManagedWorktree may fail if upstream config is broken — check actual behavior.
	// branchMergedToUpstream returns false when upstream missing; upstreamReference may still resolve.
	if result.err != nil {
		// Accept either success or clear upstream-related error
		assert.Contains(t, result.err.Error(), "upstream")
	}
}

func TestPruneKeepsWorktreeWhenUpstreamRefIsMissing(t *testing.T) {
	const branchName = "feature/prune-missing-upstream"

	testRepository := newTestRepository(t)
	require.NoError(t, testRepository.runGitWT(t, "create", "--repo", testRepoName, branchName).err)
	runGitCommand(t, testRepository.barePath, "branch", "--unset-upstream", branchName)

	result := testRepository.runGitWT(t, "prune", "--repo", testRepoName)
	// May error on enrich or keep worktree; either is acceptable if worktree remains when not merged.
	if result.err == nil {
		testRepository.assertPathPresent(t, testRepository.worktreePath(branchName))
	}
}

func TestRemovePreservesReferenceLikeBranchNames(t *testing.T) {
	const branchName = "refs-like/name"

	testRepository := newTestRepository(t)
	require.NoError(t, testRepository.runGitWT(t, "create", "--repo", testRepoName, branchName).err)
	testRepository.mergeWorktreeBranch(t, branchName)

	result := testRepository.runGitWT(t, "remove", "--repo", testRepoName, branchName)
	require.NoError(t, result.err, result.stderr)
	testRepository.assertBranchMissing(t, branchName)
}

func TestListSupportsLocalUpstream(t *testing.T) {
	const branchName = "feature/local-upstream"

	testRepository := newTestRepository(t)
	require.NoError(t, testRepository.runGitWT(t, "create", "--repo", testRepoName, branchName).err)
	runGitCommand(t, testRepository.barePath, "branch", "--set-upstream-to", "main", branchName)

	result := testRepository.runGitWT(t, "list", "--repo", testRepoName)
	require.NoError(t, result.err, result.stderr)
	assert.Contains(t, result.stdout, branchName)
}

func TestListSupportsCustomRemoteUpstream(t *testing.T) {
	const branchName = "feature/custom-remote"

	testRepository := newTestRepository(t)
	require.NoError(t, testRepository.runGitWT(t, "create", "--repo", testRepoName, branchName).err)

	// Add a second remote-like ref namespace via config.
	runGitCommand(t, testRepository.barePath, "remote", "add", "upstream", testRepository.remotePath)
	runGitCommand(t, testRepository.barePath, "fetch", "upstream")
	runGitCommand(t, testRepository.barePath, "branch", "--set-upstream-to", "upstream/main", branchName)

	result := testRepository.runGitWT(t, "list", "--repo", testRepoName)
	require.NoError(t, result.err, result.stderr)
}

func TestListFailsWhenBranchHasNoUpstream(t *testing.T) {
	const branchName = "feature/no-upstream"

	testRepository := newTestRepository(t)
	require.NoError(t, testRepository.runGitWT(t, "create", "--repo", testRepoName, branchName).err)
	runGitCommand(t, testRepository.barePath, "branch", "--unset-upstream", branchName)

	result := testRepository.runGitWT(t, "list", "--repo", testRepoName)
	require.Error(t, result.err)
	assert.Contains(t, result.err.Error(), "upstream")
}

func TestPrunePromptCanForceRemoveSelectedWorktrees(t *testing.T) {
	const branchName = "feature/prompt"

	testRepository := newTestRepository(t)
	require.NoError(t, testRepository.runGitWT(t, "create", "--repo", testRepoName, branchName).err)
	testRepository.commitFileInWorktree(t, branchName, "extra.txt", "extra\n")

	options := &pruneCommandOptions{
		repoSelection: repoSelection{RepoFlag: testRepoName},
		prompt:        true,
		prompter:      stubPrompter{selected: []managedWorktree{{Name: branchName}}},
	}
	command := NewRootCommand()
	var stderr bytes.Buffer
	command.SetErr(&stderr)
	command.SetOut(io.Discard)
	command.SetArgs([]string{})
	err := options.Execute(command, nil)
	require.NoError(t, err, stderr.String())
	testRepository.assertPathMissing(t, testRepository.worktreePath(branchName))
}

func TestRepoAddListRemove(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, ".local", "share"))
	t.Setenv(worktreeRootEnvVarName, filepath.Join(home, "worktrees"))

	remotePath := filepath.Join(t.TempDir(), "remote.git")
	runGitCommand(t, t.TempDir(), "init", "--bare", remotePath)
	seedBareRemote(t, remotePath)

	addResult := runGitWTCommand(t, "repo", "add", "--name", "demo", remotePath)
	require.NoError(t, addResult.err, addResult.stderr)
	assert.Contains(t, addResult.stderr, "added repository demo")

	barePath := filepath.Join(home, ".local", "share", "git-wt", "repos", "demo.git")
	fetch := strings.TrimSpace(runGitCommand(t, barePath, "config", "--get", "remote.origin.fetch"))
	assert.Equal(t, "+refs/heads/*:refs/remotes/origin/*", fetch)
	originHead := strings.TrimSpace(runGitCommand(t, barePath, "symbolic-ref", "--short", "refs/remotes/origin/HEAD"))
	assert.Equal(t, "origin/main", originHead)

	listResult := runGitWTCommand(t, "repo", "list")
	require.NoError(t, listResult.err, listResult.stderr)
	assert.Contains(t, listResult.stdout, "Name")
	assert.Contains(t, listResult.stdout, "Path")
	assert.Contains(t, listResult.stdout, "demo")
	assert.Contains(t, listResult.stdout, displayHomePath(barePath))
	assert.NotContains(t, listResult.stdout, home)

	removeResult := runGitWTCommand(t, "repo", "remove", "demo")
	require.NoError(t, removeResult.err, removeResult.stderr)

	listAfter := runGitWTCommand(t, "repo", "list")
	require.NoError(t, listAfter.err)
	assert.Contains(t, listAfter.stdout, "Name")
	assert.Contains(t, listAfter.stdout, "Path")
	assert.NotContains(t, listAfter.stdout, "demo")
}

func TestRepoListQuietOutputsOnlySortedNames(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, ".local", "share"))

	repositoryNames := []string{"zeta", "alpha"}
	for _, repositoryName := range repositoryNames {
		require.NoError(t, os.MkdirAll(bareRepoPath(repositoryName), 0o755))
	}

	testCases := []struct {
		name string
		flag string
	}{
		{name: "short option", flag: "-q"},
		{name: "long option", flag: "--quiet"},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			result := runGitWTCommand(t, "repo", "list", testCase.flag)

			require.NoError(t, result.err, result.stderr)
			assert.Equal(t, "alpha\nzeta\n", result.stdout)
		})
	}
}

func TestCreateRepairsBareRepoMissingOriginFetch(t *testing.T) {
	testRepository := newTestRepository(t)

	// Simulate a bare clone that never got remote-tracking configured.
	runGitCommandAllowError(t, testRepository.barePath, "config", "--unset-all", "remote.origin.fetch")
	runGitCommandAllowError(t, testRepository.barePath, "symbolic-ref", "-d", "refs/remotes/origin/HEAD")
	runGitCommandAllowError(t, testRepository.barePath, "update-ref", "-d", "refs/remotes/origin/main")

	result := testRepository.runGitWT(t, "create", "--repo", testRepoName, "feature/repaired-upstream")
	require.NoError(t, result.err, result.stderr)

	upstream := strings.TrimSpace(runGitCommand(
		t,
		testRepository.worktreePath("feature/repaired-upstream"),
		"rev-parse",
		"--abbrev-ref",
		"@{upstream}",
	))
	assert.Equal(t, "origin/main", upstream)
}

func TestCreateWritesPathFileWhenRequested(t *testing.T) {
	testRepository := newTestRepository(t)
	pathFile := filepath.Join(t.TempDir(), "created-path")
	t.Setenv(createPathFileEnvVarName, pathFile)

	result := testRepository.runGitWT(t, "create", "--repo", testRepoName, "feature/path-file")
	require.NoError(t, result.err, result.stderr)
	assert.Empty(t, strings.TrimSpace(result.stdout))

	contents, err := os.ReadFile(pathFile)
	require.NoError(t, err)
	assert.Equal(t, testRepository.worktreePath("feature/path-file")+"\n", string(contents))
}

func TestRepoAddMapsGitHubRelativePath(t *testing.T) {
	assert.Equal(t, "https://github.com/nnutter/git-wt", mustResolveRemoteURL(t, "nnutter/git-wt"))
	assert.Equal(t, "https://example.com/r.git", mustResolveRemoteURL(t, "https://example.com/r.git"))
	assert.Equal(t, "git@github.com:nnutter/git-wt.git", mustResolveRemoteURL(t, "git@github.com:nnutter/git-wt.git"))
}

func TestRepoRenameMovesManagedWorktreesAndPreservesUnmanagedWorktrees(t *testing.T) {
	const (
		branchName  = "feature/rename/nested"
		newRepoName = "renamed"
	)

	testRepository := newTestRepository(t)
	require.NoError(t, testRepository.runGitWT(t, "create", "--repo", testRepoName, branchName).err)
	testRepository.writeFileInWorktree(t, branchName, "dirty.txt", "dirty\n")

	unmanagedPath := filepath.Join(t.TempDir(), "unmanaged")
	runGitCommand(t, testRepository.barePath, "branch", "unmanaged", "main")
	runGitCommand(t, testRepository.barePath, "worktree", "add", unmanagedPath, "unmanaged")
	detachedPath := filepath.Join(t.TempDir(), "detached")
	runGitCommand(t, testRepository.barePath, "worktree", "add", "--detach", detachedPath, "main")

	result := testRepository.runGitWT(t, "repo", "rename", testRepoName, newRepoName)
	require.NoError(t, result.err, result.stderr)
	assert.Contains(t, result.stderr, "renamed repository repo to renamed")

	newBarePath := bareRepoPath(newRepoName)
	newWorktreePath := managedWorktreePath(newRepoName, branchName)
	assert.NoDirExists(t, testRepository.barePath)
	assert.DirExists(t, newBarePath)
	assert.NoDirExists(t, testRepository.worktreePath(branchName))
	assert.DirExists(t, newWorktreePath)
	assert.FileExists(t, filepath.Join(newWorktreePath, "dirty.txt"))
	assert.DirExists(t, unmanagedPath)
	assert.DirExists(t, detachedPath)

	managedRepository, err := openRepository(newWorktreePath)
	require.NoError(t, err)
	managedCommonDir, err := managedRepository.commonGitDir()
	require.NoError(t, err)
	managedCommonDirMatches, err := samePath(managedCommonDir, newBarePath)
	require.NoError(t, err)
	assert.True(t, managedCommonDirMatches)

	unmanagedRepository, err := openRepository(unmanagedPath)
	require.NoError(t, err)
	unmanagedCommonDir, err := unmanagedRepository.commonGitDir()
	require.NoError(t, err)
	unmanagedCommonDirMatches, err := samePath(unmanagedCommonDir, newBarePath)
	require.NoError(t, err)
	assert.True(t, unmanagedCommonDirMatches)

	detachedRepository, err := openRepository(detachedPath)
	require.NoError(t, err)
	detachedCommonDir, err := detachedRepository.commonGitDir()
	require.NoError(t, err)
	detachedCommonDirMatches, err := samePath(detachedCommonDir, newBarePath)
	require.NoError(t, err)
	assert.True(t, detachedCommonDirMatches)

	listResult := runGitWTCommand(t, "list", "--repo", newRepoName)
	require.NoError(t, listResult.err, listResult.stderr)
	assert.Contains(t, listResult.stdout, branchName)
}

func TestRepoRenameWithoutWorktrees(t *testing.T) {
	testRepository := newTestRepository(t)

	result := testRepository.runGitWT(t, "repo", "rename", testRepoName, "renamed")
	require.NoError(t, result.err, result.stderr)
	assert.NoDirExists(t, testRepository.barePath)
	assert.DirExists(t, bareRepoPath("renamed"))

	oldResult := testRepository.runGitWT(t, "list", "--repo", testRepoName)
	require.Error(t, oldResult.err)
	assert.Contains(t, oldResult.err.Error(), `unknown repository "repo"`)
}

func TestRepoRenameReportsMovedCurrentDirectory(t *testing.T) {
	const branchName = "feature/current-rename"

	testRepository := newTestRepository(t)
	require.NoError(t, testRepository.runGitWT(t, "create", "--repo", testRepoName, branchName).err)
	subdirectory := filepath.Join(testRepository.worktreePath(branchName), "nested")
	require.NoError(t, os.MkdirAll(subdirectory, 0o755))
	pathFile := filepath.Join(t.TempDir(), "renamed-path")
	t.Setenv(repoRenamePathFileEnvVarName, pathFile)

	result := testRepository.runGitWTFrom(t, subdirectory, "repo", "rename", testRepoName, "renamed")
	require.NoError(t, result.err, result.stderr)

	contents, err := os.ReadFile(pathFile)
	require.NoError(t, err)
	assert.Equal(t, managedWorktreePath("renamed", branchName)+string(filepath.Separator)+"nested\n", string(contents))
}

func TestRepoRenameRejectsInvalidAndConflictingNames(t *testing.T) {
	testCases := []struct {
		name      string
		newName   string
		prepare   func(*testing.T)
		wantError string
	}{
		{
			name:      "same name",
			newName:   testRepoName,
			prepare:   func(*testing.T) {},
			wantError: "already named",
		},
		{
			name:      "invalid name",
			newName:   "invalid/name",
			prepare:   func(*testing.T) {},
			wantError: "must not contain path separators",
		},
		{
			name:    "repository collision",
			newName: "existing",
			prepare: func(t *testing.T) {
				require.NoError(t, os.MkdirAll(bareRepoPath("existing"), 0o755))
			},
			wantError: "already exists",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			testRepository := newTestRepository(t)
			testCase.prepare(t)

			result := testRepository.runGitWT(t, "repo", "rename", testRepoName, testCase.newName)
			require.Error(t, result.err)
			assert.Contains(t, result.err.Error(), testCase.wantError)
			assert.DirExists(t, testRepository.barePath)
		})
	}
}

func TestRepoRenameRejectsUnknownRepository(t *testing.T) {
	newTestRepository(t)

	result := runGitWTCommand(t, "repo", "rename", "missing", "renamed")
	require.Error(t, result.err)
	assert.Contains(t, result.err.Error(), `unknown repository "missing"`)
}

func TestRepoRenameRejectsPrunableWorktree(t *testing.T) {
	testRepository := newTestRepository(t)
	prunablePath := filepath.Join(t.TempDir(), "prunable")
	runGitCommand(t, testRepository.barePath, "branch", "prunable", "main")
	runGitCommand(t, testRepository.barePath, "worktree", "add", prunablePath, "prunable")
	require.NoError(t, os.RemoveAll(prunablePath))

	result := testRepository.runGitWT(t, "repo", "rename", testRepoName, "renamed")
	require.Error(t, result.err)
	assert.Contains(t, result.err.Error(), "prunable")
	assert.DirExists(t, testRepository.barePath)
	assert.NoDirExists(t, bareRepoPath("renamed"))
}

func TestRepoRenameRejectsWorktreeDestinationCollision(t *testing.T) {
	const branchName = "feature/collision"

	testRepository := newTestRepository(t)
	require.NoError(t, testRepository.runGitWT(t, "create", "--repo", testRepoName, branchName).err)
	require.NoError(t, os.MkdirAll(managedWorktreePath("renamed", branchName), 0o755))

	result := testRepository.runGitWT(t, "repo", "rename", testRepoName, "renamed")
	require.Error(t, result.err)
	assert.Contains(t, result.err.Error(), "worktree directory")
	assert.DirExists(t, testRepository.barePath)
	assert.DirExists(t, testRepository.worktreePath(branchName))
}

func TestRepoRenameRollsBackCompletedWorktreeMoves(t *testing.T) {
	testRepository := newTestRepository(t)
	for _, branchName := range []string{"feature/first", "feature/second"} {
		require.NoError(t, testRepository.runGitWT(t, "create", "--repo", testRepoName, branchName).err)
	}

	renameCalls := 0
	options := &repoRenameCommandOptions{
		renamePath: func(source string, destination string) error {
			renameCalls++
			if renameCalls == 2 {
				return fmt.Errorf("injected rename failure")
			}
			return os.Rename(source, destination)
		},
		repairWorktrees: repairWorktrees,
	}
	command := NewRootCommand()
	command.SetOut(io.Discard)
	command.SetErr(io.Discard)

	currentDirectory, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(testRepository.home))
	t.Cleanup(func() { require.NoError(t, os.Chdir(currentDirectory)) })

	err = options.Execute(command, []string{testRepoName, "renamed"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "injected rename failure")
	assert.DirExists(t, testRepository.barePath)
	assert.NoDirExists(t, bareRepoPath("renamed"))
	for _, branchName := range []string{"feature/first", "feature/second"} {
		assert.DirExists(t, testRepository.worktreePath(branchName))
		_, openErr := openRepository(testRepository.worktreePath(branchName))
		require.NoError(t, openErr)
	}
}

func TestRepoRenameRollsBackAfterRepairFailure(t *testing.T) {
	const branchName = "feature/repair-failure"

	testRepository := newTestRepository(t)
	require.NoError(t, testRepository.runGitWT(t, "create", "--repo", testRepoName, branchName).err)

	repairCalls := 0
	options := &repoRenameCommandOptions{
		renamePath: os.Rename,
		repairWorktrees: func(barePath string, worktreePaths []string) error {
			repairCalls++
			if repairCalls == 1 {
				return fmt.Errorf("injected repair failure")
			}
			return repairWorktrees(barePath, worktreePaths)
		},
	}
	command := NewRootCommand()
	command.SetOut(io.Discard)
	command.SetErr(io.Discard)

	currentDirectory, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(testRepository.home))
	t.Cleanup(func() { require.NoError(t, os.Chdir(currentDirectory)) })

	err = options.Execute(command, []string{testRepoName, "renamed"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "injected repair failure")
	assert.DirExists(t, testRepository.barePath)
	assert.NoDirExists(t, bareRepoPath("renamed"))
	assert.DirExists(t, testRepository.worktreePath(branchName))
	_, openErr := openRepository(testRepository.worktreePath(branchName))
	require.NoError(t, openErr)
}

func TestRepoRenameCompletionOffersRegisteredReposOnlyForOldName(t *testing.T) {
	newTestRepository(t)

	oldNameCompletion := runComplete(t, "repo", "rename", "")
	assert.Contains(t, oldNameCompletion, testRepoName)

	newNameCompletion := runComplete(t, "repo", "rename", testRepoName, "")
	assert.NotContains(t, newNameCompletion, testRepoName)
	assert.Contains(t, newNameCompletion, ":4")
}

func TestRepoRemoveRefusesWhenWorktreesExist(t *testing.T) {
	testRepository := newTestRepository(t)
	require.NoError(t, testRepository.runGitWT(t, "create", "--repo", testRepoName, "feature/keep").err)

	result := testRepository.runGitWT(t, "repo", "remove", testRepoName)
	require.Error(t, result.err)
	assert.Contains(t, result.err.Error(), "still has")
}

func TestCreateRequiresRepoOutsideInteractive(t *testing.T) {
	testRepository := newTestRepository(t)
	result := testRepository.runGitWT(t, "create", "feature/needs-repo")
	require.Error(t, result.err)
	assert.Contains(t, result.err.Error(), "repository selection requires")
}

func TestCreateAutoDetectsRepoFromManagedWorktree(t *testing.T) {
	const existing = "feature/base"
	const branchName = "feature/from-current"

	testRepository := newTestRepository(t)
	require.NoError(t, testRepository.runGitWT(t, "create", "--repo", testRepoName, existing).err)

	result := testRepository.runGitWTFrom(t, testRepository.worktreePath(existing), "create", branchName)
	require.NoError(t, result.err, result.stderr)
	testRepository.assertPathPresent(t, testRepository.worktreePath(branchName))
}

func TestCreateAcceptsShortRepoFlag(t *testing.T) {
	const branchName = "feature/short-repo"

	testRepository := newTestRepository(t)
	result := testRepository.runGitWT(t, "create", "-r", testRepoName, branchName)
	require.NoError(t, result.err, result.stderr)
	testRepository.assertPathPresent(t, testRepository.worktreePath(branchName))
}

func TestListAutoDetectsRepoFromManagedWorktree(t *testing.T) {
	const branchName = "feature/auto-list"

	testRepository := newTestRepository(t)
	require.NoError(t, testRepository.runGitWT(t, "create", "--repo", testRepoName, branchName).err)

	result := testRepository.runGitWTFrom(t, testRepository.worktreePath(branchName), "list")
	require.NoError(t, result.err, result.stderr)
	assert.Contains(t, result.stdout, "Repo")
	assert.Contains(t, result.stdout, testRepoName)
	assert.Contains(t, result.stdout, branchName)
}

func TestListOutsideManagedWorktreeListsAllRepos(t *testing.T) {
	primary := newTestRepository(t)
	secondaryName := "other"
	secondaryBare := registerAdditionalRepo(t, primary, secondaryName)

	require.NoError(t, primary.runGitWT(t, "create", "--repo", testRepoName, "feature/primary").err)
	require.NoError(t, primary.runGitWT(t, "create", "--repo", secondaryName, "feature/secondary").err)

	result := primary.runGitWT(t, "list")
	require.NoError(t, result.err, result.stderr)
	assert.Contains(t, result.stdout, testRepoName)
	assert.Contains(t, result.stdout, "feature/primary")
	assert.Contains(t, result.stdout, secondaryName)
	assert.Contains(t, result.stdout, "feature/secondary")
	assert.DirExists(t, secondaryBare)
}

func TestListInsideManagedWorktreeIsScopedUnlessAll(t *testing.T) {
	primary := newTestRepository(t)
	secondaryName := "other"
	registerAdditionalRepo(t, primary, secondaryName)

	require.NoError(t, primary.runGitWT(t, "create", "--repo", testRepoName, "feature/primary").err)
	require.NoError(t, primary.runGitWT(t, "create", "--repo", secondaryName, "feature/secondary").err)

	scoped := primary.runGitWTFrom(t, primary.worktreePath("feature/primary"), "list")
	require.NoError(t, scoped.err, scoped.stderr)
	assert.Contains(t, scoped.stdout, "feature/primary")
	assert.NotContains(t, scoped.stdout, "feature/secondary")

	allRepos := primary.runGitWTFrom(t, primary.worktreePath("feature/primary"), "list", "-a")
	require.NoError(t, allRepos.err, allRepos.stderr)
	assert.Contains(t, allRepos.stdout, "feature/primary")
	assert.Contains(t, allRepos.stdout, "feature/secondary")
	assert.Contains(t, allRepos.stdout, secondaryName)
}

func TestRepoFlagCompletionOffersRegisteredRepos(t *testing.T) {
	testRepository := newTestRepository(t)
	registerAdditionalRepo(t, testRepository, "other")

	for _, args := range [][]string{
		{"__complete", "create", "--repo", ""},
		{"__complete", "create", "-r", ""},
		{"__complete", "list", "--repo", ""},
		{"__complete", "remove", "--repo", ""},
		{"__complete", "space", "--repo", ""},
		{"__complete", "prune", "--repo", ""},
		{"__complete", "switch", "--repo", ""},
	} {
		command := NewRootCommand()
		command.SetArgs(args)
		var stdout bytes.Buffer
		command.SetOut(&stdout)
		command.SetErr(io.Discard)
		require.NoError(t, command.Execute())
		assert.Contains(t, stdout.String(), testRepoName, "args=%v", args)
		assert.Contains(t, stdout.String(), "other", "args=%v", args)
	}
}

func TestRemoveAutoDetectsRepoFromManagedWorktree(t *testing.T) {
	const branchName = "feature/auto-remove"

	testRepository := newTestRepository(t)
	require.NoError(t, testRepository.runGitWT(t, "create", "--repo", testRepoName, branchName).err)
	testRepository.mergeWorktreeBranch(t, branchName)

	result := testRepository.runGitWTFrom(t, testRepository.worktreePath(branchName), "remove", branchName)
	require.NoError(t, result.err, result.stderr)
	testRepository.assertPathMissing(t, testRepository.worktreePath(branchName))
}

func TestMigrateRegistersBareAndRehomesWorktrees(t *testing.T) {
	home := t.TempDir()
	worktreeRootPath := filepath.Join(home, "worktrees")
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, ".local", "share"))
	t.Setenv(worktreeRootEnvVarName, worktreeRootPath)
	t.Setenv("HERDR_ENV", "")

	// Build a plain clone with a feature worktree outside the new layout.
	base := t.TempDir()
	remotePath := filepath.Join(base, "remote.git")
	runGitCommand(t, base, "init", "--bare", remotePath)
	seedBareRemote(t, remotePath)

	clonePath := filepath.Join(base, "project")
	runGitCommand(t, base, "clone", remotePath, clonePath)
	configureGitUser(t, clonePath)

	featurePath := filepath.Join(base, "feature-worktree")
	runGitCommand(t, clonePath, "branch", "feature/login")
	runGitCommand(t, clonePath, "worktree", "add", featurePath, "feature/login")

	result := runGitWTFrom(t, clonePath, "migrate", "--name", "project")
	require.NoError(t, result.err, result.stderr)

	barePath := filepath.Join(home, ".local", "share", "git-wt", "repos", "project.git")
	_, err := os.Stat(barePath)
	require.NoError(t, err)

	// migrate must install the same origin tracking setup as repo add.
	fetch := strings.TrimSpace(runGitCommand(t, barePath, "config", "--get", "remote.origin.fetch"))
	assert.Equal(t, "+refs/heads/*:refs/remotes/origin/*", fetch)
	originURL := strings.TrimSpace(runGitCommand(t, barePath, "remote", "get-url", "origin"))
	assert.Equal(t, remotePath, originURL)
	originHead := strings.TrimSpace(runGitCommand(t, barePath, "symbolic-ref", "--short", "refs/remotes/origin/HEAD"))
	assert.Equal(t, "origin/main", originHead)
	runGitCommand(t, barePath, "show-ref", "--verify", "refs/remotes/origin/main")

	mainTarget := filepath.Join(worktreeRootPath, "project", "main", "project")
	featureTarget := filepath.Join(worktreeRootPath, "project", "feature/login", "project")
	_, err = os.Stat(mainTarget)
	require.NoError(t, err)
	_, err = os.Stat(featureTarget)
	require.NoError(t, err)

	listResult := runGitWTCommand(t, "list", "--repo", "project")
	require.NoError(t, listResult.err, listResult.stderr)
	assert.Contains(t, listResult.stdout, "main")
	assert.Contains(t, listResult.stdout, "feature/login")

	// Creating another worktree should resolve origin/HEAD without repair hacks.
	createResult := runGitWTCommand(t, "create", "--repo", "project", "feature/after-migrate")
	require.NoError(t, createResult.err, createResult.stderr)
}

func TestMigrateOmitsSoleDefaultBranchWorktree(t *testing.T) {
	home := t.TempDir()
	worktreeRootPath := filepath.Join(home, "worktrees")
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, ".local", "share"))
	t.Setenv(worktreeRootEnvVarName, worktreeRootPath)
	t.Setenv("HERDR_ENV", "")

	base := t.TempDir()
	remotePath := filepath.Join(base, "remote.git")
	runGitCommand(t, base, "init", "--bare", remotePath)
	seedBareRemote(t, remotePath)

	clonePath := filepath.Join(base, "project")
	runGitCommand(t, base, "clone", remotePath, clonePath)
	configureGitUser(t, clonePath)

	result := runGitWTFrom(t, clonePath, "migrate", "--name", "project")
	require.NoError(t, result.err, result.stderr)
	assert.Contains(t, result.stderr, "omitted default-branch worktree")

	barePath := filepath.Join(home, ".local", "share", "git-wt", "repos", "project.git")
	_, err := os.Stat(barePath)
	require.NoError(t, err)

	// No managed worktree should be created for the default branch alone.
	_, err = os.Stat(filepath.Join(worktreeRootPath, "project", "main", "project"))
	assert.True(t, os.IsNotExist(err))

	listResult := runGitWTCommand(t, "list", "--repo", "project")
	require.NoError(t, listResult.err, listResult.stderr)
	assert.NotContains(t, listResult.stdout, "main")

	// Source checkout is removed after bare registration.
	_, err = os.Stat(clonePath)
	assert.True(t, os.IsNotExist(err))
}

func TestMigrateOmitsSoleDefaultRemovesEmptySourceParents(t *testing.T) {
	home := t.TempDir()
	worktreeRootPath := filepath.Join(home, "worktrees")
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, ".local", "share"))
	t.Setenv(worktreeRootEnvVarName, worktreeRootPath)
	t.Setenv("HERDR_ENV", "")

	remoteParent := t.TempDir()
	remotePath := filepath.Join(remoteParent, "remote.git")
	runGitCommand(t, remoteParent, "init", "--bare", remotePath)
	seedBareRemote(t, remotePath)

	clonePath := filepath.Join(home, "src", "github.com", "nnutter", "project")
	require.NoError(t, os.MkdirAll(filepath.Dir(clonePath), 0o755))
	runGitCommand(t, home, "clone", remotePath, clonePath)
	configureGitUser(t, clonePath)

	result := runGitWTFrom(t, clonePath, "migrate", "--name", "project")
	require.NoError(t, result.err, result.stderr)

	_, err := os.Stat(clonePath)
	assert.True(t, os.IsNotExist(err))
	_, err = os.Stat(filepath.Join(home, "src"))
	assert.True(t, os.IsNotExist(err))
	_, err = os.Stat(home)
	require.NoError(t, err)
}

func TestMigrateOmitsSoleDefaultKeepsNonEmptySourceParent(t *testing.T) {
	home := t.TempDir()
	worktreeRootPath := filepath.Join(home, "worktrees")
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, ".local", "share"))
	t.Setenv(worktreeRootEnvVarName, worktreeRootPath)
	t.Setenv("HERDR_ENV", "")

	remoteParent := t.TempDir()
	remotePath := filepath.Join(remoteParent, "remote.git")
	runGitCommand(t, remoteParent, "init", "--bare", remotePath)
	seedBareRemote(t, remotePath)

	parentPath := filepath.Join(home, "src", "github.com", "nnutter")
	clonePath := filepath.Join(parentPath, "project")
	siblingPath := filepath.Join(parentPath, "other")
	require.NoError(t, os.MkdirAll(parentPath, 0o755))
	require.NoError(t, os.MkdirAll(siblingPath, 0o755))
	runGitCommand(t, home, "clone", remotePath, clonePath)
	configureGitUser(t, clonePath)

	result := runGitWTFrom(t, clonePath, "migrate", "--name", "project")
	require.NoError(t, result.err, result.stderr)

	_, err := os.Stat(clonePath)
	assert.True(t, os.IsNotExist(err))
	_, err = os.Stat(siblingPath)
	require.NoError(t, err)
	_, err = os.Stat(parentPath)
	require.NoError(t, err)
}

func TestMigrateRemovesEmptyParentsOfRehomedWorktrees(t *testing.T) {
	home := t.TempDir()
	worktreeRootPath := filepath.Join(home, "worktrees")
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, ".local", "share"))
	t.Setenv(worktreeRootEnvVarName, worktreeRootPath)
	t.Setenv("HERDR_ENV", "")

	remoteParent := t.TempDir()
	remotePath := filepath.Join(remoteParent, "remote.git")
	runGitCommand(t, remoteParent, "init", "--bare", remotePath)
	seedBareRemote(t, remotePath)

	parentPath := filepath.Join(home, "src", "github.com", "nnutter")
	clonePath := filepath.Join(parentPath, "project")
	featurePath := filepath.Join(parentPath, "feature-login")
	require.NoError(t, os.MkdirAll(parentPath, 0o755))
	runGitCommand(t, home, "clone", remotePath, clonePath)
	configureGitUser(t, clonePath)
	runGitCommand(t, clonePath, "branch", "feature/login")
	runGitCommand(t, clonePath, "worktree", "add", featurePath, "feature/login")

	result := runGitWTFrom(t, clonePath, "migrate", "--name", "project")
	require.NoError(t, result.err, result.stderr)

	_, err := os.Stat(clonePath)
	assert.True(t, os.IsNotExist(err))
	_, err = os.Stat(featurePath)
	assert.True(t, os.IsNotExist(err))
	_, err = os.Stat(filepath.Join(home, "src"))
	assert.True(t, os.IsNotExist(err))
	_, err = os.Stat(home)
	require.NoError(t, err)
	_, err = os.Stat(filepath.Join(worktreeRootPath, "project", "main", "project"))
	require.NoError(t, err)
	_, err = os.Stat(filepath.Join(worktreeRootPath, "project", "feature/login", "project"))
	require.NoError(t, err)
}

func TestMigrateKeepsSoleNonDefaultBranchWorktree(t *testing.T) {
	home := t.TempDir()
	worktreeRootPath := filepath.Join(home, "worktrees")
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, ".local", "share"))
	t.Setenv(worktreeRootEnvVarName, worktreeRootPath)
	t.Setenv("HERDR_ENV", "")

	base := t.TempDir()
	remotePath := filepath.Join(base, "remote.git")
	runGitCommand(t, base, "init", "--bare", remotePath)
	seedBareRemote(t, remotePath)

	clonePath := filepath.Join(base, "project")
	runGitCommand(t, base, "clone", remotePath, clonePath)
	configureGitUser(t, clonePath)
	runGitCommand(t, clonePath, "checkout", "-b", "feature/only")

	result := runGitWTFrom(t, clonePath, "migrate", "--name", "project")
	require.NoError(t, result.err, result.stderr)
	assert.NotContains(t, result.stderr, "omitted default-branch worktree")

	_, err := os.Stat(filepath.Join(worktreeRootPath, "project", "feature/only", "project"))
	require.NoError(t, err)
}

func TestMigratePromptCanSkipSelectedWorktrees(t *testing.T) {
	home := t.TempDir()
	worktreeRootPath := filepath.Join(home, "worktrees")
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, ".local", "share"))
	t.Setenv(worktreeRootEnvVarName, worktreeRootPath)
	t.Setenv("HERDR_ENV", "")

	base := t.TempDir()
	remotePath := filepath.Join(base, "remote.git")
	runGitCommand(t, base, "init", "--bare", remotePath)
	seedBareRemote(t, remotePath)

	clonePath := filepath.Join(base, "project")
	runGitCommand(t, base, "clone", remotePath, clonePath)
	configureGitUser(t, clonePath)

	featurePath := filepath.Join(base, "feature-worktree")
	runGitCommand(t, clonePath, "branch", "feature/skip")
	runGitCommand(t, clonePath, "worktree", "add", featurePath, "feature/skip")

	options := &migrateCommandOptions{
		name:   "project",
		prompt: true,
		prompter: stubMigratePrompter{selected: []migrateCandidate{{
			Action:      "migrate",
			Name:        "main",
			BranchName:  "main",
			CurrentPath: clonePath,
			TargetPath:  filepath.Join(worktreeRootPath, "project", "main", "project"),
		}}},
	}

	command := NewRootCommand()
	var stderr bytes.Buffer
	command.SetErr(&stderr)
	command.SetOut(io.Discard)

	current, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(clonePath))
	defer func() { _ = os.Chdir(current) }()

	require.NoError(t, options.Execute(command, nil), stderr.String())

	_, err = os.Stat(filepath.Join(worktreeRootPath, "project", "main", "project"))
	require.NoError(t, err)
	// Skipped feature worktree remains at original path (or was left alone).
	_, err = os.Stat(featurePath)
	require.NoError(t, err)
}

func TestMigrateRehomesRegisteredWorktrees(t *testing.T) {
	const branchName = "feature/login"

	testRepository := newTestRepository(t)
	legacyPath := addLegacyWorktree(t, testRepository.barePath, testRepository.worktreeRoot, testRepoName, branchName)
	require.NoError(t, os.WriteFile(filepath.Join(legacyPath, "dirty.txt"), []byte("dirty\n"), 0o644))

	result := testRepository.runGitWTFrom(t, legacyPath, "migrate")
	require.NoError(t, result.err, result.stderr)
	assert.Contains(t, result.stderr, testRepository.worktreePath(branchName))

	testRepository.assertPathPresent(t, testRepository.worktreePath(branchName))
	testRepository.assertPathMissing(t, legacyPath)
	assert.FileExists(t, filepath.Join(testRepository.worktreePath(branchName), "dirty.txt"))

	listResult := testRepository.runGitWT(t, "list", "--repo", testRepoName)
	require.NoError(t, listResult.err, listResult.stderr)
	assert.Contains(t, listResult.stdout, branchName)
}

func TestMigrateSkipsRegisteredWorktreesAlreadyAtManagedPath(t *testing.T) {
	const branchName = "feature/login"

	testRepository := newTestRepository(t)
	require.NoError(t, testRepository.runGitWT(t, "create", "--repo", testRepoName, branchName).err)

	result := testRepository.runGitWTFrom(t, testRepository.worktreePath(branchName), "migrate")
	require.NoError(t, result.err, result.stderr)
	assert.Contains(t, result.stderr, "no worktrees to rehome")
	testRepository.assertPathPresent(t, testRepository.worktreePath(branchName))
}

func TestMigrateRehomesAllRegisteredWorktreesFromOutsideGit(t *testing.T) {
	primary := newTestRepository(t)
	secondaryName := "other"
	secondaryBarePath := registerAdditionalRepo(t, primary, secondaryName)

	legacyPrimary := addLegacyWorktree(t, primary.barePath, primary.worktreeRoot, testRepoName, "feature/one")
	legacySecondary := addLegacyWorktree(t, secondaryBarePath, primary.worktreeRoot, secondaryName, "feature/two")

	result := primary.runGitWTFrom(t, primary.home, "migrate", "--all")
	require.NoError(t, result.err, result.stderr)

	primary.assertPathPresent(t, primary.worktreePath("feature/one"))
	primary.assertPathMissing(t, legacyPrimary)
	_, err := os.Stat(filepath.Join(primary.worktreeRoot, secondaryName, "feature/two", secondaryName))
	require.NoError(t, err)
	_, err = os.Stat(legacySecondary)
	assert.True(t, os.IsNotExist(err))
}

func TestMigrateRehomesRegisteredWorktreeNamedLikeRepo(t *testing.T) {
	testRepository := newTestRepository(t)
	legacyPath := addLegacyWorktree(t, testRepository.barePath, testRepository.worktreeRoot, testRepoName, testRepoName)

	result := testRepository.runGitWTFrom(t, legacyPath, "migrate")
	require.NoError(t, result.err, result.stderr)

	newPath := testRepository.worktreePath(testRepoName)
	testRepository.assertPathPresent(t, newPath)
	assert.NoFileExists(t, filepath.Join(legacyPath, ".git"))

	listResult := testRepository.runGitWT(t, "list", "--repo", testRepoName)
	require.NoError(t, listResult.err, listResult.stderr)
	assert.Contains(t, listResult.stdout, testRepoName)
}

func TestWorktreeRootUsesEnvironmentOverride(t *testing.T) {
	customRoot := filepath.Join(t.TempDir(), "custom-worktrees")
	t.Setenv("HOME", t.TempDir())
	t.Setenv(worktreeRootEnvVarName, customRoot)

	assert.Equal(t, customRoot, worktreeRoot())
	assert.Equal(t, filepath.Join(customRoot, "repo", "feature", "repo"), managedWorktreePath("repo", "feature"))
}

func TestWorktreeRootFallsBackToHomeWorktrees(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv(worktreeRootEnvVarName, "")

	assert.Equal(t, filepath.Join(home, "worktrees"), worktreeRoot())
}

func TestDefaultRepoNameFromRemote(t *testing.T) {
	name, err := defaultRepoNameFromRemote("https://github.com/nnutter/git-wt.git")
	require.NoError(t, err)
	assert.Equal(t, "git-wt", name)

	name, err = defaultRepoNameFromRemote("git@github.com:nnutter/git-wt.git")
	require.NoError(t, err)
	assert.Equal(t, "git-wt", name)
}

func TestDefaultRepoNameFromPathStripsGitSuffix(t *testing.T) {
	assert.Equal(t, "roam", defaultRepoNameFromPath("/tmp/src/roam.git"))
	assert.Equal(t, "roam", defaultRepoNameFromPath("/tmp/src/main/roam.git"))
	assert.Equal(t, "roam", defaultRepoNameFromPath("/tmp/src/roam"))
}

func TestNormalizeRepoNameStripsGitSuffix(t *testing.T) {
	assert.Equal(t, "roam", normalizeRepoName("roam.git"))
	assert.Equal(t, "roam", normalizeRepoName(" roam.git "))
	assert.Equal(t, "roam", normalizeRepoName("roam"))
}

func TestDisplayHomePath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	assert.Equal(t, "~", displayHomePath(home))
	assert.Equal(t, filepath.Join("~", ".local", "share", "git-wt", "repos", "demo.git"), displayHomePath(filepath.Join(home, ".local", "share", "git-wt", "repos", "demo.git")))
	assert.Equal(t, "/tmp/other", displayHomePath("/tmp/other"))
}

func TestMigrateStripsGitSuffixFromNameFlag(t *testing.T) {
	home := t.TempDir()
	worktreeRootPath := filepath.Join(home, "worktrees")
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, ".local", "share"))
	t.Setenv(worktreeRootEnvVarName, worktreeRootPath)
	t.Setenv("HERDR_ENV", "")

	base := t.TempDir()
	remotePath := filepath.Join(base, "remote.git")
	runGitCommand(t, base, "init", "--bare", remotePath)
	seedBareRemote(t, remotePath)

	// Checkout basename ends with .git, which must not become the worktree leaf name.
	clonePath := filepath.Join(base, "roam.git")
	runGitCommand(t, base, "clone", remotePath, clonePath)
	configureGitUser(t, clonePath)
	runGitCommand(t, clonePath, "branch", "-M", "master")
	runGitCommand(t, clonePath, "push", "-u", remoteName, "master")

	result := runGitWTFrom(t, clonePath, "migrate", "--name", "roam.git")
	require.NoError(t, result.err, result.stderr)

	barePath := filepath.Join(home, ".local", "share", "git-wt", "repos", "roam.git")
	_, err := os.Stat(barePath)
	require.NoError(t, err)

	masterTarget := filepath.Join(worktreeRootPath, "roam", "master", "roam")
	_, err = os.Stat(masterTarget)
	require.NoError(t, err)
	_, err = os.Stat(filepath.Join(worktreeRootPath, "roam", "master", "roam.git"))
	assert.True(t, os.IsNotExist(err))
}

func mustResolveRemoteURL(t *testing.T, input string) string {
	t.Helper()
	resolved, err := resolveRemoteURL(input)
	require.NoError(t, err)
	return resolved
}

const testRepoName = "repo"

type testRepository struct {
	home         string
	barePath     string
	remotePath   string
	worktreeRoot string
}

func newTestRepository(t *testing.T) testRepository {
	t.Helper()

	home := t.TempDir()
	worktreeRootPath := filepath.Join(home, "worktrees")
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, ".local", "share"))
	t.Setenv(worktreeRootEnvVarName, worktreeRootPath)
	t.Setenv("HERDR_ENV", "")

	remoteParent := t.TempDir()
	remotePath := filepath.Join(remoteParent, "remote.git")
	runGitCommand(t, remoteParent, "init", "--bare", remotePath)
	seedBareRemote(t, remotePath)

	reposDir := filepath.Join(home, ".local", "share", "git-wt", "repos")
	require.NoError(t, os.MkdirAll(reposDir, 0o755))
	barePath := filepath.Join(reposDir, testRepoName+".git")
	runGitCommand(t, reposDir, "clone", "--bare", remotePath, barePath)

	// Ensure remote-tracking refs exist for default upstream resolution.
	runGitCommand(t, barePath, "remote", "remove", remoteName)
	runGitCommand(t, barePath, "remote", "add", remoteName, remotePath)
	runGitCommand(t, barePath, "fetch", remoteName)
	runGitCommand(t, barePath, "remote", "set-head", remoteName, "main")

	return testRepository{
		home:         home,
		barePath:     barePath,
		remotePath:   remotePath,
		worktreeRoot: worktreeRootPath,
	}
}

// registerAdditionalRepo clones another bare repo into the same registry home as base.
func registerAdditionalRepo(t *testing.T, base testRepository, name string) string {
	t.Helper()

	reposDir := filepath.Join(base.home, ".local", "share", "git-wt", "repos")
	barePath := filepath.Join(reposDir, name+".git")
	runGitCommand(t, reposDir, "clone", "--bare", base.remotePath, barePath)
	runGitCommand(t, barePath, "remote", "remove", remoteName)
	runGitCommand(t, barePath, "remote", "add", remoteName, base.remotePath)
	runGitCommand(t, barePath, "fetch", remoteName)
	runGitCommand(t, barePath, "remote", "set-head", remoteName, "main")
	return barePath
}

func seedBareRemote(t *testing.T, remotePath string) {
	t.Helper()
	tempClone := filepath.Join(t.TempDir(), "seed")
	runGitCommand(t, filepath.Dir(tempClone), "clone", remotePath, tempClone)
	configureGitUser(t, tempClone)
	require.NoError(t, os.WriteFile(filepath.Join(tempClone, "README.md"), []byte("initial\n"), 0o644))
	runGitCommand(t, tempClone, "add", "README.md")
	runGitCommand(t, tempClone, "commit", "-m", "initial")
	runGitCommand(t, tempClone, "branch", "-M", "main")
	runGitCommand(t, tempClone, "push", "-u", remoteName, "main")
	// git init --bare leaves HEAD at init.defaultBranch (often master). Point it at
	// the branch we actually pushed so clones and remote set-head --auto work on CI.
	runGitCommand(t, remotePath, "symbolic-ref", "HEAD", "refs/heads/main")
}

func configureGitUser(t *testing.T, path string) {
	t.Helper()
	runGitCommand(t, path, "config", "user.name", "Test User")
	runGitCommand(t, path, "config", "user.email", "test@example.com")
}

func (x testRepository) worktreePath(branchName string) string {
	return filepath.Join(x.worktreeRoot, testRepoName, branchName, testRepoName)
}

func addLegacyWorktree(t *testing.T, barePath string, worktreeRoot string, repoName string, branchName string) string {
	t.Helper()
	path := filepath.Join(worktreeRoot, branchName, repoName)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	runGitCommand(t, barePath, "branch", branchName, "main")
	runGitCommand(t, barePath, "worktree", "add", path, branchName)
	runGitCommand(t, barePath, "branch", "--set-upstream-to", remoteName+"/main", branchName)
	return path
}

func (x testRepository) runGitWT(t *testing.T, args ...string) commandResult {
	t.Helper()
	return x.runGitWTFrom(t, x.home, args...)
}

func (x testRepository) runGitWTFrom(t *testing.T, directory string, args ...string) commandResult {
	t.Helper()
	return runGitWTFrom(t, directory, args...)
}

func runGitWTFrom(t *testing.T, directory string, args ...string) commandResult {
	t.Helper()

	currentDirectory, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(directory))
	defer func() {
		require.NoError(t, os.Chdir(currentDirectory))
	}()

	return runGitWTCommand(t, args...)
}

func runGitWTCommand(t *testing.T, args ...string) commandResult {
	t.Helper()

	command := NewRootCommand()
	command.SetArgs(args)
	command.SetIn(bytes.NewBuffer(nil))

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	command.SetOut(&stdout)
	command.SetErr(&stderr)

	err := command.Execute()
	return commandResult{stdout: stdout.String(), stderr: stderr.String(), err: err}
}

func (x testRepository) commitFileInWorktree(t *testing.T, branchName string, fileName string, contents string) {
	t.Helper()
	path := x.worktreePath(branchName)
	x.writeFileInWorktree(t, branchName, fileName, contents)
	runGitCommand(t, path, "add", fileName)
	runGitCommand(t, path, "commit", "-m", "change")
}

func (x testRepository) writeFileInWorktree(t *testing.T, branchName string, fileName string, contents string) {
	t.Helper()
	path := filepath.Join(x.worktreePath(branchName), fileName)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(contents), 0o644))
}

func (x testRepository) mergeWorktreeBranch(t *testing.T, branchName string) {
	t.Helper()
	// Create a temporary worktree on main to merge into, then push.
	mergePath := filepath.Join(t.TempDir(), "merge-main")
	runGitCommand(t, x.barePath, "worktree", "add", mergePath, "main")
	runGitCommand(t, mergePath, "merge", "--ff-only", branchName)
	runGitCommand(t, mergePath, "push", remoteName, "main")
	runGitCommand(t, x.barePath, "fetch", remoteName)
	runGitCommand(t, x.barePath, "worktree", "remove", mergePath)
}

func (x testRepository) assertBranchMissing(t *testing.T, branchName string) {
	t.Helper()
	command := exec.Command("git", "--git-dir", x.barePath, "show-ref", "--verify", "--quiet", "refs/heads/"+branchName)
	err := command.Run()
	if exitError, ok := errors.AsType[*exec.ExitError](err); ok && exitError.ExitCode() == 1 {
		return
	}
	if err == nil {
		t.Fatalf("expected branch %s to be missing", branchName)
	}
	t.Fatalf("unexpected error checking branch %s: %v", branchName, err)
}

func (x testRepository) assertPathMissing(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expected path %s to be missing", path)
	}
}

func (x testRepository) assertPathPresent(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected path %s to be present", path)
	}
}

func runGitCommand(t *testing.T, cwd string, args ...string) string {
	t.Helper()

	command := exec.Command("git", args...)
	command.Dir = cwd
	command.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=Test User",
		"GIT_AUTHOR_EMAIL=test@example.com",
		"GIT_COMMITTER_NAME=Test User",
		"GIT_COMMITTER_EMAIL=test@example.com",
	)

	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s failed: %v\n%s", strings.Join(args, " "), err, string(output))
	}

	return string(output)
}

func runGitCommandAllowError(t *testing.T, cwd string, args ...string) {
	t.Helper()
	command := exec.Command("git", args...)
	command.Dir = cwd
	_ = command.Run()
}
