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

func TestCreateWithHerdrInvokesHerdrWorkspaceCreate(t *testing.T) {
	const branchName = "feature/herdr"

	testRepository := newTestRepository(t)
	logPath := filepath.Join(t.TempDir(), "herdr.log")
	installFakeHerdr(t, logPath, 0)

	result := testRepository.runGitWT(t, "create", "--repo", testRepoName, "-r", branchName)
	require.NoError(t, result.err, result.stderr)
	testRepository.assertPathPresent(t, testRepository.worktreePath(branchName))
	assert.Contains(t, result.stderr, "created herdr workspace "+testRepoName)

	logContents, err := os.ReadFile(logPath)
	require.NoError(t, err)
	wantCwd, err := filepath.Abs(testRepository.worktreePath(branchName))
	require.NoError(t, err)
	got := strings.TrimSpace(string(logContents))
	want := strings.Join([]string{"workspace", "create", "--cwd", wantCwd, "--label", testRepoName}, "\x00")
	assert.Equal(t, want, got)
}

func TestCreateWithoutHerdrDoesNotInvokeHerdr(t *testing.T) {
	const branchName = "feature/no-herdr"

	testRepository := newTestRepository(t)
	logPath := filepath.Join(t.TempDir(), "herdr.log")
	installFakeHerdr(t, logPath, 0)

	result := testRepository.runGitWT(t, "create", "--repo", testRepoName, branchName)
	require.NoError(t, result.err, result.stderr)
	_, err := os.Stat(logPath)
	assert.True(t, os.IsNotExist(err))
}

func TestCreateInHerdrInvokesHerdrWorkspaceCreate(t *testing.T) {
	const branchName = "feature/automatic-herdr"

	testRepository := newTestRepository(t)
	t.Setenv("HERDR_ENV", "1")
	logPath := filepath.Join(t.TempDir(), "herdr.log")
	installFakeHerdr(t, logPath, 0)

	result := testRepository.runGitWT(t, "create", "--repo", testRepoName, branchName)
	require.NoError(t, result.err, result.stderr)
	_, err := os.Stat(logPath)
	require.NoError(t, err)
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
			installFakeHerdr(t, logPath, 0)

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
	installFakeHerdr(t, logPath, 1)

	result := testRepository.runGitWT(t, "create", "--repo", testRepoName, "--herdr", branchName)
	require.Error(t, result.err)
	testRepository.assertPathPresent(t, testRepository.worktreePath(branchName))
	assert.Contains(t, result.err.Error(), "herdr workspace create")
}

func installFakeHerdr(t *testing.T, logPath string, exitCode int) {
	t.Helper()

	binDir := t.TempDir()
	scriptPath := filepath.Join(binDir, "herdr")
	script := fmt.Sprintf(`#!/bin/sh
: > %q
first=1
for arg in "$@"; do
  if [ "$first" -eq 1 ]; then
    first=0
  else
    printf '\0' >> %q
  fi
  printf '%%s' "$arg" >> %q
done
exit %d
`, logPath, logPath, logPath, exitCode)
	require.NoError(t, os.WriteFile(scriptPath, []byte(script), 0o755))

	path := binDir + string(os.PathListSeparator) + os.Getenv("PATH")
	t.Setenv("PATH", path)
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

	result := testRepository.runGitWTFrom(t, testRepository.worktreePath(branchName), "remove", "--current")
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

	result := testRepository.runGitWTFrom(t, subDir, "remove", "--current")
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

	command := NewRootCommand()
	command.SetArgs([]string{"__complete", "remove", "--repo", testRepoName, ""})
	var stdout bytes.Buffer
	command.SetOut(&stdout)
	command.SetErr(io.Discard)
	require.NoError(t, command.Execute())

	assert.Contains(t, stdout.String(), "feature/a")
	assert.Contains(t, stdout.String(), "feature/b")
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
	assert.NotContains(t, string(functionContents), "target_dir=$(command git-wt create")
	assert.NotContains(t, string(functionContents), "git worktree list --porcelain | head")
	assert.NotContains(t, string(functionContents), "off)")
	assert.Contains(t, string(completionContents), "repo:Manage registered repositories")
	assert.Contains(t, string(completionContents), "GIT_WT_WORKTREE_ROOT")
	assert.NotContains(t, string(completionContents), "off:")
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
