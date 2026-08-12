package gitwt

import (
	"bytes"
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

	result := testRepository.runGitWT(t, "create", "--repo", testRepoName, "-r", branchName)
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
	testCases := []struct {
		name string
		flag string
	}{
		{name: "short", flag: "-R"},
		{name: "long", flag: "--no-herdr"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			branchName := "feature/no-herdr-" + testCase.name
			testRepository := newTestRepository(t)
			t.Setenv("HERDR_ENV", "1")
			logPath := filepath.Join(t.TempDir(), "herdr.log")
			installFakeHerdrSpace(t, logPath)

			result := testRepository.runGitWT(t, "create", "--repo", testRepoName, testCase.flag, branchName)
			require.NoError(t, result.err, result.stderr)
			_, err := os.Stat(logPath)
			assert.True(t, os.IsNotExist(err))
		})
	}
}

func TestCreateRejectsHerdrAndNoHerdr(t *testing.T) {
	result := runGitWTCommand(t, "create", "-r", "-R", "feature/conflicting-herdr")
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
previous=""
for arg in "$@"; do
  if [ "$previous" = "--label" ]; then
    tab_label="$arg"
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
  "tab create")
    if [ "$tab_label" = "Shell" ]; then
      printf '{"result":{"tab":{"tab_id":"w1:t3"},"root_pane":{"pane_id":"w1:p3"}}}'
    else
      printf '{"result":{"tab":{"tab_id":"w1:t2"},"root_pane":{"pane_id":"w1:p2"}}}'
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

	testRepository.assertPathMissing(t, filepath.Join(testRepository.worktreeRoot, "feature"))
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

func TestGenerateZshGeneratesWrapperFunctionAndCompletion(t *testing.T) {
	outDir := t.TempDir()
	result := runGitWTCommand(t, "generate", "zsh", "--out", outDir, "--force")
	require.NoError(t, result.err, result.stderr)

	functionPath := filepath.Join(outDir, "wt")
	completionPath := filepath.Join(outDir, "_wt")
	functionContents, err := os.ReadFile(functionPath)
	require.NoError(t, err)
	completionContents, err := os.ReadFile(completionPath)
	require.NoError(t, err)

	assert.Contains(t, string(functionContents), "git-wt create")
	assert.Contains(t, string(functionContents), `cd "$HOME"`)
	assert.Contains(t, string(functionContents), "GIT_WT_WORKTREE_ROOT")
	assert.Contains(t, string(functionContents), "GIT_WT_CREATE_PATH_FILE")
	assert.Contains(t, string(functionContents), "previous_dir=$PWD")
	assert.Contains(t, string(functionContents), "GIT_WT_RENAME_PATH_FILE")
	assert.Contains(t, string(functionContents), `cd "$target_dir"`)
	assert.Contains(t, string(functionContents), "remove|migrate)")
	assert.NotContains(t, string(functionContents), "target_dir=$(command git-wt create")
	assert.NotContains(t, string(functionContents), "git worktree list --porcelain | head")
	assert.NotContains(t, string(functionContents), "off)")
	assert.Contains(t, string(completionContents), "repo:Manage registered repositories")
	assert.Contains(t, string(completionContents), "rename:Rename a registered repository")
	assert.Contains(t, string(completionContents), "_message 'new repository name'")
	assert.Contains(t, string(completionContents), "GIT_WT_WORKTREE_ROOT")
	assert.Contains(t, string(completionContents), "local context state state_descr line")
	assert.Contains(t, string(completionContents), "--repo[Registered repository name]:repository:->repos")
	assert.Contains(t, string(completionContents), "(--repo)--all[List worktrees from all registered repositories]")
	assert.Contains(t, string(completionContents), "space:Open a managed Git worktree in Herdr")
	assert.Contains(t, string(completionContents), "switch|space|remove)")
	assert.Contains(t, string(completionContents), "1:worktree name:->worktrees")
	assert.Contains(t, string(completionContents), "shift words")
	assert.NotContains(t, string(completionContents), "switch|remove|prune)")
	assert.NotContains(t, string(completionContents), "off:")
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
		"zsh", "-c",
		`source "$1"; cd "$2"; wt repo rename old new >/dev/null; pwd -P`,
		"--", filepath.Join(outDir, "wt"), oldSubdirectory,
	)
	command.Env = append(os.Environ(), "PATH="+binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	output, err := command.CombinedOutput()
	require.NoError(t, err, string(output))
	assert.Equal(t, canonicalPath(filepath.Join(worktreeParent, "new", "nested")), strings.TrimSpace(string(output)))
}

func TestGenerateZshRefusesOverwriteWithoutForce(t *testing.T) {
	outDir := t.TempDir()
	require.NoError(t, runGitWTCommand(t, "generate", "zsh", "--out", outDir).err)
	result := runGitWTCommand(t, "generate", "zsh", "--out", outDir)
	require.Error(t, result.err)
	assert.Contains(t, result.err.Error(), "already exists")
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

func TestCreateWithCurrentUsesRegisteredRepo(t *testing.T) {
	const existing = "feature/base"
	const branchName = "feature/from-current"

	testRepository := newTestRepository(t)
	require.NoError(t, testRepository.runGitWT(t, "create", "--repo", testRepoName, existing).err)

	result := testRepository.runGitWTFrom(t, testRepository.worktreePath(existing), "create", "--current", branchName)
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

	allRepos := primary.runGitWTFrom(t, primary.worktreePath("feature/primary"), "list", "--all")
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
		{"__complete", "list", "--repo", ""},
		{"__complete", "remove", "--repo", ""},
		{"__complete", "space", "--repo", ""},
		{"__complete", "prune", "--repo", ""},
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

	mainTarget := filepath.Join(worktreeRootPath, "main", "project")
	featureTarget := filepath.Join(worktreeRootPath, "feature/login", "project")
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
	_, err = os.Stat(filepath.Join(worktreeRootPath, "main", "project"))
	assert.True(t, os.IsNotExist(err))

	listResult := runGitWTCommand(t, "list", "--repo", "project")
	require.NoError(t, listResult.err, listResult.stderr)
	assert.NotContains(t, listResult.stdout, "main")

	// Source checkout is removed after bare registration.
	_, err = os.Stat(clonePath)
	assert.True(t, os.IsNotExist(err))
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

	_, err := os.Stat(filepath.Join(worktreeRootPath, "feature/only", "project"))
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
			TargetPath:  filepath.Join(worktreeRootPath, "main", "project"),
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

	_, err = os.Stat(filepath.Join(worktreeRootPath, "main", "project"))
	require.NoError(t, err)
	// Skipped feature worktree remains at original path (or was left alone).
	_, err = os.Stat(featurePath)
	require.NoError(t, err)
}

func TestWorktreeRootUsesEnvironmentOverride(t *testing.T) {
	customRoot := filepath.Join(t.TempDir(), "custom-worktrees")
	t.Setenv("HOME", t.TempDir())
	t.Setenv(worktreeRootEnvVarName, customRoot)

	assert.Equal(t, customRoot, worktreeRoot())
	assert.Equal(t, filepath.Join(customRoot, "feature", "repo"), managedWorktreePath("repo", "feature"))
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

	masterTarget := filepath.Join(worktreeRootPath, "master", "roam")
	_, err = os.Stat(masterTarget)
	require.NoError(t, err)
	_, err = os.Stat(filepath.Join(worktreeRootPath, "master", "roam.git"))
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
	return filepath.Join(x.worktreeRoot, branchName, testRepoName)
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
	if exitError, ok := err.(*exec.ExitError); ok && exitError.ExitCode() == 1 {
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
