package timber

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRepoAddListRemove(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	runtime := testRuntimeForHome(home, home)

	remotePath := filepath.Join(t.TempDir(), "remote.git")
	runGitCommand(t, t.TempDir(), "init", "--bare", remotePath)
	seedBareRemote(t, remotePath)

	addResult := runTimberCommandWithRuntime(t, runtime, "repo", "add", "--name", "demo", remotePath)
	require.NoError(t, addResult.err, addResult.stderr)
	assert.Contains(t, addResult.stderr, "added repository demo")

	barePath := filepath.Join(runtime.DataHome, "timber", "repos", "demo.git")
	fetch := strings.TrimSpace(runGitCommand(t, barePath, "config", "--get", "remote.origin.fetch"))
	assert.Equal(t, "+refs/heads/*:refs/remotes/origin/*", fetch)
	originHead := strings.TrimSpace(runGitCommand(t, barePath, "symbolic-ref", "--short", "refs/remotes/origin/HEAD"))
	assert.Equal(t, "origin/main", originHead)

	listResult := runTimberCommandWithRuntime(t, runtime, "repo", "list")
	require.NoError(t, listResult.err, listResult.stderr)
	assert.Contains(t, listResult.stdout, "Name")
	assert.Contains(t, listResult.stdout, "Path")
	assert.Contains(t, listResult.stdout, "Origin")
	assert.Contains(t, listResult.stdout, "demo")
	assert.Contains(t, listResult.stdout, runtime.displayHomePath(barePath))
	assert.Contains(t, listResult.stdout, remotePath)
	assert.NotContains(t, listResult.stdout, home)

	removeResult := runTimberCommandWithRuntime(t, runtime, "repo", "remove", "demo")
	require.NoError(t, removeResult.err, removeResult.stderr)

	listAfter := runTimberCommandWithRuntime(t, runtime, "repo", "list")
	require.NoError(t, listAfter.err)
	assert.Contains(t, listAfter.stdout, "Name")
	assert.Contains(t, listAfter.stdout, "Path")
	assert.Contains(t, listAfter.stdout, "Origin")
	assert.NotContains(t, listAfter.stdout, "demo")
}

func TestRepoListShowsEmptyOriginWhenRemoteIsMissing(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	runtime := testRuntimeForHome(home, home)

	barePath := filepath.Join(runtime.DataHome, "timber", "repos", "local.git")
	require.NoError(t, os.MkdirAll(filepath.Dir(barePath), 0o755))
	runGitCommand(t, t.TempDir(), "init", "--bare", barePath)

	result := runTimberCommandWithRuntime(t, runtime, "repo", "list")
	require.NoError(t, result.err, result.stderr)
	assert.Contains(t, result.stdout, "Origin")
	assert.Contains(t, result.stdout, "local")
	assert.Contains(t, result.stdout, runtime.displayHomePath(barePath))
}

func TestRepoListQuietOutputsOnlySortedNames(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	runtime := testRuntimeForHome(home, home)

	repositoryNames := []string{"zeta", "alpha"}
	for _, repositoryName := range repositoryNames {
		require.NoError(t, os.MkdirAll(runtime.bareRepoPath(repositoryName), 0o755))
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
			result := runTimberCommandWithRuntime(t, runtime, "repo", "list", testCase.flag)

			require.NoError(t, result.err, result.stderr)
			assert.Equal(t, "alpha\nzeta\n", result.stdout)
		})
	}
}

func TestRepoAddMapsGitHubRelativePath(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "https://github.com/nnutter/timber", mustResolveRemoteURL(t, "nnutter/timber"))
	assert.Equal(t, "https://example.com/r.git", mustResolveRemoteURL(t, "https://example.com/r.git"))
	assert.Equal(t, "git@github.com:nnutter/timber.git", mustResolveRemoteURL(t, "git@github.com:nnutter/timber.git"))
}

func TestRepoRenameMovesManagedWorktreesAndPreservesUnmanagedWorktrees(t *testing.T) {
	t.Parallel()
	const (
		branchName  = "feature/rename/nested"
		newRepoName = "renamed"
	)

	testRepository := newTestRepository(t)
	require.NoError(t, testRepository.runTimber(t, "create", at(testRepoName, branchName)).err)
	testRepository.writeFileInWorktree(t, branchName, "dirty.txt", "dirty\n")

	unmanagedPath := filepath.Join(t.TempDir(), "unmanaged")
	runGitCommand(t, testRepository.barePath, "branch", "unmanaged", "main")
	runGitCommand(t, testRepository.barePath, "worktree", "add", unmanagedPath, "unmanaged")
	detachedPath := filepath.Join(t.TempDir(), "detached")
	runGitCommand(t, testRepository.barePath, "worktree", "add", "--detach", detachedPath, "main")

	result := testRepository.runTimber(t, "repo", "rename", testRepoName, newRepoName)
	require.NoError(t, result.err, result.stderr)
	assert.Contains(t, result.stderr, "renamed repository repo to renamed")

	newBarePath := testRepository.runtime.bareRepoPath(newRepoName)
	newWorktreePath := testRepository.runtime.managedWorktreePath(newRepoName, branchName)
	assert.NoDirExists(t, testRepository.barePath)
	assert.DirExists(t, newBarePath)
	assert.NoDirExists(t, testRepository.worktreePath(branchName))
	assert.DirExists(t, newWorktreePath)
	assert.FileExists(t, filepath.Join(newWorktreePath, "dirty.txt"))
	assert.DirExists(t, unmanagedPath)
	assert.DirExists(t, detachedPath)

	managedRepository, err := openRepository(testRepository.runtime, newWorktreePath)
	require.NoError(t, err)
	managedCommonDir, err := managedRepository.commonGitDir()
	require.NoError(t, err)
	managedCommonDirMatches, err := samePath(managedCommonDir, newBarePath)
	require.NoError(t, err)
	assert.True(t, managedCommonDirMatches)

	unmanagedRepository, err := openRepository(testRepository.runtime, unmanagedPath)
	require.NoError(t, err)
	unmanagedCommonDir, err := unmanagedRepository.commonGitDir()
	require.NoError(t, err)
	unmanagedCommonDirMatches, err := samePath(unmanagedCommonDir, newBarePath)
	require.NoError(t, err)
	assert.True(t, unmanagedCommonDirMatches)

	detachedRepository, err := openRepository(testRepository.runtime, detachedPath)
	require.NoError(t, err)
	detachedCommonDir, err := detachedRepository.commonGitDir()
	require.NoError(t, err)
	detachedCommonDirMatches, err := samePath(detachedCommonDir, newBarePath)
	require.NoError(t, err)
	assert.True(t, detachedCommonDirMatches)

	listResult := runTimberCommandWithRuntime(t, testRepository.runtime, "list", at(newRepoName, ""))
	require.NoError(t, listResult.err, listResult.stderr)
	assert.Contains(t, listResult.stdout, branchName)
}

func TestRepoRenameWithoutWorktrees(t *testing.T) {
	t.Parallel()
	testRepository := newTestRepository(t)

	result := testRepository.runTimber(t, "repo", "rename", testRepoName, "renamed")
	require.NoError(t, result.err, result.stderr)
	assert.NoDirExists(t, testRepository.barePath)
	assert.DirExists(t, testRepository.runtime.bareRepoPath("renamed"))

	oldResult := testRepository.runTimber(t, "list", at(testRepoName, ""))
	require.Error(t, oldResult.err)
	assert.Contains(t, oldResult.err.Error(), `unknown repository "repo"`)
}

func TestRepoRenameReportsMovedCurrentDirectory(t *testing.T) {
	t.Parallel()
	const branchName = "feature/current-rename"

	testRepository := newTestRepository(t)
	require.NoError(t, testRepository.runTimber(t, "create", at(testRepoName, branchName)).err)
	subdirectory := filepath.Join(testRepository.worktreePath(branchName), "nested")
	require.NoError(t, os.MkdirAll(subdirectory, 0o755))
	pathFile := filepath.Join(t.TempDir(), "renamed-path")
	runtime := testRepository.runtime
	runtime.CurrentDirectory = subdirectory
	runtime.RenamePathFile = pathFile

	result := runTimberCommandWithRuntime(t, runtime, "repo", "rename", testRepoName, "renamed")
	require.NoError(t, result.err, result.stderr)

	contents, err := os.ReadFile(pathFile)
	require.NoError(t, err)
	assert.Equal(t, testRepository.runtime.managedWorktreePath("renamed", branchName)+string(filepath.Separator)+"nested\n", string(contents))
}

func TestRepoRenameRejectsInvalidAndConflictingNames(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name      string
		newName   string
		prepare   func(*testing.T, testRepository)
		wantError string
	}{
		{
			name:      "same name",
			newName:   testRepoName,
			prepare:   func(*testing.T, testRepository) {},
			wantError: "already named",
		},
		{
			name:      "invalid name",
			newName:   "invalid/name",
			prepare:   func(*testing.T, testRepository) {},
			wantError: "must not contain path separators",
		},
		{
			name:    "repository collision",
			newName: "existing",
			prepare: func(t *testing.T, testRepository testRepository) {
				require.NoError(t, os.MkdirAll(testRepository.runtime.bareRepoPath("existing"), 0o755))
			},
			wantError: "already exists",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			testRepository := newTestRepository(t)
			testCase.prepare(t, testRepository)

			result := testRepository.runTimber(t, "repo", "rename", testRepoName, testCase.newName)
			require.Error(t, result.err)
			assert.Contains(t, result.err.Error(), testCase.wantError)
			assert.DirExists(t, testRepository.barePath)
		})
	}
}

func TestRepoRenameRejectsUnknownRepository(t *testing.T) {
	t.Parallel()
	newTestRepository(t)

	result := runTimberCommand(t, "repo", "rename", "missing", "renamed")
	require.Error(t, result.err)
	assert.Contains(t, result.err.Error(), `unknown repository "missing"`)
}

func TestRepoRenameRejectsPrunableWorktree(t *testing.T) {
	t.Parallel()
	testRepository := newTestRepository(t)
	prunablePath := filepath.Join(t.TempDir(), "prunable")
	runGitCommand(t, testRepository.barePath, "branch", "prunable", "main")
	runGitCommand(t, testRepository.barePath, "worktree", "add", prunablePath, "prunable")
	require.NoError(t, os.RemoveAll(prunablePath))

	result := testRepository.runTimber(t, "repo", "rename", testRepoName, "renamed")
	require.Error(t, result.err)
	assert.Contains(t, result.err.Error(), "prunable")
	assert.DirExists(t, testRepository.barePath)
	assert.NoDirExists(t, testRepository.runtime.bareRepoPath("renamed"))
}

func TestRepoRenameRejectsWorktreeDestinationCollision(t *testing.T) {
	t.Parallel()
	const branchName = "feature/collision"

	testRepository := newTestRepository(t)
	require.NoError(t, testRepository.runTimber(t, "create", at(testRepoName, branchName)).err)
	require.NoError(t, os.MkdirAll(testRepository.runtime.managedWorktreePath("renamed", branchName), 0o755))

	result := testRepository.runTimber(t, "repo", "rename", testRepoName, "renamed")
	require.Error(t, result.err)
	assert.Contains(t, result.err.Error(), "worktree directory")
	assert.DirExists(t, testRepository.barePath)
	assert.DirExists(t, testRepository.worktreePath(branchName))
}

func TestRepoRenameRollsBackCompletedWorktreeMoves(t *testing.T) {
	t.Parallel()
	testRepository := newTestRepository(t)
	for _, branchName := range []string{"feature/first", "feature/second"} {
		require.NoError(t, testRepository.runTimber(t, "create", at(testRepoName, branchName)).err)
	}

	renameCalls := 0
	options := &repoRenameCommandOptions{
		runtime: testRepository.runtime,
		renamePath: func(source string, destination string) error {
			renameCalls++
			if renameCalls == 2 {
				return fmt.Errorf("injected rename failure")
			}
			return os.Rename(source, destination)
		},
		repairWorktrees: func(barePath string, worktreePaths []string) error {
			return repairWorktrees(testRepository.runtime, barePath, worktreePaths)
		},
	}
	command := newTestRootCommandWithRuntime(t, testRepository.runtime)
	command.SetOut(io.Discard)
	command.SetErr(io.Discard)

	err := options.Execute(command, []string{testRepoName, "renamed"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "injected rename failure")
	assert.DirExists(t, testRepository.barePath)
	assert.NoDirExists(t, testRepository.runtime.bareRepoPath("renamed"))
	for _, branchName := range []string{"feature/first", "feature/second"} {
		assert.DirExists(t, testRepository.worktreePath(branchName))
		_, openErr := openRepository(testRepository.runtime, testRepository.worktreePath(branchName))
		require.NoError(t, openErr)
	}
}

func TestRepoRenameRollsBackAfterRepairFailure(t *testing.T) {
	t.Parallel()
	const branchName = "feature/repair-failure"

	testRepository := newTestRepository(t)
	require.NoError(t, testRepository.runTimber(t, "create", at(testRepoName, branchName)).err)

	repairCalls := 0
	options := &repoRenameCommandOptions{
		runtime:    testRepository.runtime,
		renamePath: os.Rename,
		repairWorktrees: func(barePath string, worktreePaths []string) error {
			repairCalls++
			if repairCalls == 1 {
				return fmt.Errorf("injected repair failure")
			}
			return repairWorktrees(testRepository.runtime, barePath, worktreePaths)
		},
	}
	command := newTestRootCommandWithRuntime(t, testRepository.runtime)
	command.SetOut(io.Discard)
	command.SetErr(io.Discard)

	err := options.Execute(command, []string{testRepoName, "renamed"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "injected repair failure")
	assert.DirExists(t, testRepository.barePath)
	assert.NoDirExists(t, testRepository.runtime.bareRepoPath("renamed"))
	assert.DirExists(t, testRepository.worktreePath(branchName))
	_, openErr := openRepository(testRepository.runtime, testRepository.worktreePath(branchName))
	require.NoError(t, openErr)
}

func TestRepoRenameCompletionOffersRegisteredReposOnlyForOldName(t *testing.T) {
	t.Parallel()
	testRepository := newTestRepository(t)

	oldNameCompletion := runCompleteWithRuntime(t, testRepository.runtime, "repo", "rename", "")
	assert.Contains(t, oldNameCompletion, testRepoName)

	newNameCompletion := runCompleteWithRuntime(t, testRepository.runtime, "repo", "rename", testRepoName, "")
	assert.NotContains(t, newNameCompletion, testRepoName)
	assert.Contains(t, newNameCompletion, ":4")
}

func TestRepoRemoveRefusesWhenWorktreesExist(t *testing.T) {
	t.Parallel()
	testRepository := newTestRepository(t)
	require.NoError(t, testRepository.runTimber(t, "create", at(testRepoName, "feature/keep")).err)

	result := testRepository.runTimber(t, "repo", "remove", testRepoName)
	require.Error(t, result.err)
	assert.Contains(t, result.err.Error(), "still has")
}

func TestRepoQualifierCompletionOffersRegisteredRepos(t *testing.T) {
	t.Parallel()
	testRepository := newTestRepository(t)
	registerAdditionalRepo(t, testRepository, "other")

	for _, args := range [][]string{
		{"create", "@"},
		{"create", ""},
		{"list", "@"},
		{"prune", "@"},
	} {
		stdout := runCompleteWithRuntime(t, testRepository.runtime, args...)
		assert.Contains(t, stdout, at(testRepoName, ""), "args=%v", args)
		assert.Contains(t, stdout, "@other", "args=%v", args)
	}
}
