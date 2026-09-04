package timber

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type stubMigratePrompter struct {
	selected []migrateCandidate
	err      error
}

func (x stubMigratePrompter) Prompt(input io.Reader, output io.Writer, candidates []migrateCandidate) ([]migrateCandidate, error) {
	return x.selected, x.err
}

func TestMigrateRegistersBareAndRehomesWorktrees(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	worktreeRootPath := filepath.Join(home, "worktrees")
	runtime := testRuntimeForHome(home, home)

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

	runtime.CurrentDirectory = clonePath
	result := runTimberFromWithRuntime(t, runtime, clonePath, "migrate", "--name", "project")
	require.NoError(t, result.err, result.stderr)

	barePath := filepath.Join(home, ".local", "share", "timber", "repos", "project.git")
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

	listResult := runTimberCommandWithRuntime(t, runtime, "list", at("project", ""))
	require.NoError(t, listResult.err, listResult.stderr)
	assert.Contains(t, listResult.stdout, "main")
	assert.Contains(t, listResult.stdout, "feature/login")

	// Creating another worktree should resolve origin/HEAD without repair hacks.
	createResult := runTimberCommandWithRuntime(t, runtime, "create", at("project", "feature/after-migrate"))
	require.NoError(t, createResult.err, createResult.stderr)
}

func TestMigrateOmitsSoleDefaultBranchWorktree(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	worktreeRootPath := filepath.Join(home, "worktrees")
	runtime := testRuntimeForHome(home, home)

	base := t.TempDir()
	remotePath := filepath.Join(base, "remote.git")
	runGitCommand(t, base, "init", "--bare", remotePath)
	seedBareRemote(t, remotePath)

	clonePath := filepath.Join(base, "project")
	runGitCommand(t, base, "clone", remotePath, clonePath)
	configureGitUser(t, clonePath)

	runtime.CurrentDirectory = clonePath
	result := runTimberFromWithRuntime(t, runtime, clonePath, "migrate", "--name", "project")
	require.NoError(t, result.err, result.stderr)
	assert.Contains(t, result.stderr, "omitted default-branch worktree")

	barePath := filepath.Join(home, ".local", "share", "timber", "repos", "project.git")
	_, err := os.Stat(barePath)
	require.NoError(t, err)

	// No managed worktree should be created for the default branch alone.
	_, err = os.Stat(filepath.Join(worktreeRootPath, "project", "main", "project"))
	require.ErrorIs(t, err, os.ErrNotExist)

	listResult := runTimberCommandWithRuntime(t, runtime, "list", at("project", ""))
	require.NoError(t, listResult.err, listResult.stderr)
	assert.NotContains(t, listResult.stdout, "main")

	// Source checkout is removed after bare registration.
	_, err = os.Stat(clonePath)
	assert.ErrorIs(t, err, os.ErrNotExist)
}

func TestMigrateOmitsSoleDefaultRemovesEmptySourceParents(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	runtime := testRuntimeForHome(home, home)

	remoteParent := t.TempDir()
	remotePath := filepath.Join(remoteParent, "remote.git")
	runGitCommand(t, remoteParent, "init", "--bare", remotePath)
	seedBareRemote(t, remotePath)

	clonePath := filepath.Join(home, "src", "github.com", "nnutter", "project")
	require.NoError(t, os.MkdirAll(filepath.Dir(clonePath), 0o755))
	runGitCommand(t, home, "clone", remotePath, clonePath)
	configureGitUser(t, clonePath)

	runtime.CurrentDirectory = clonePath
	result := runTimberFromWithRuntime(t, runtime, clonePath, "migrate", "--name", "project")
	require.NoError(t, result.err, result.stderr)

	_, err := os.Stat(clonePath)
	require.ErrorIs(t, err, os.ErrNotExist)
	_, err = os.Stat(filepath.Join(home, "src"))
	require.ErrorIs(t, err, os.ErrNotExist)
	_, err = os.Stat(home)
	require.NoError(t, err)
}

func TestMigrateOmitsSoleDefaultKeepsNonEmptySourceParent(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	runtime := testRuntimeForHome(home, home)

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

	runtime.CurrentDirectory = clonePath
	result := runTimberFromWithRuntime(t, runtime, clonePath, "migrate", "--name", "project")
	require.NoError(t, result.err, result.stderr)

	_, err := os.Stat(clonePath)
	require.ErrorIs(t, err, os.ErrNotExist)
	_, err = os.Stat(siblingPath)
	require.NoError(t, err)
	_, err = os.Stat(parentPath)
	require.NoError(t, err)
}

func TestMigrateRemovesEmptyParentsOfRehomedWorktrees(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	worktreeRootPath := filepath.Join(home, "worktrees")
	runtime := testRuntimeForHome(home, home)

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

	runtime.CurrentDirectory = clonePath
	result := runTimberFromWithRuntime(t, runtime, clonePath, "migrate", "--name", "project")
	require.NoError(t, result.err, result.stderr)

	_, err := os.Stat(clonePath)
	require.ErrorIs(t, err, os.ErrNotExist)
	_, err = os.Stat(featurePath)
	require.ErrorIs(t, err, os.ErrNotExist)
	_, err = os.Stat(filepath.Join(home, "src"))
	require.ErrorIs(t, err, os.ErrNotExist)
	_, err = os.Stat(home)
	require.NoError(t, err)
	_, err = os.Stat(filepath.Join(worktreeRootPath, "project", "main", "project"))
	require.NoError(t, err)
	_, err = os.Stat(filepath.Join(worktreeRootPath, "project", "feature/login", "project"))
	require.NoError(t, err)
}

func TestMigrateKeepsSoleNonDefaultBranchWorktree(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	worktreeRootPath := filepath.Join(home, "worktrees")
	runtime := testRuntimeForHome(home, home)

	base := t.TempDir()
	remotePath := filepath.Join(base, "remote.git")
	runGitCommand(t, base, "init", "--bare", remotePath)
	seedBareRemote(t, remotePath)

	clonePath := filepath.Join(base, "project")
	runGitCommand(t, base, "clone", remotePath, clonePath)
	configureGitUser(t, clonePath)
	runGitCommand(t, clonePath, "checkout", "-b", "feature/only")

	runtime.CurrentDirectory = clonePath
	result := runTimberFromWithRuntime(t, runtime, clonePath, "migrate", "--name", "project")
	require.NoError(t, result.err, result.stderr)
	assert.NotContains(t, result.stderr, "omitted default-branch worktree")

	_, err := os.Stat(filepath.Join(worktreeRootPath, "project", "feature/only", "project"))
	require.NoError(t, err)
}

func TestMigratePromptCanSkipSelectedWorktrees(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	worktreeRootPath := filepath.Join(home, "worktrees")

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
		runtime: testRuntimeForHome(home, clonePath),
		name:    "project",
		prompt:  true,
		prompter: stubMigratePrompter{selected: []migrateCandidate{{
			Action:      "migrate",
			Name:        "main",
			BranchName:  "main",
			CurrentPath: clonePath,
			TargetPath:  filepath.Join(worktreeRootPath, "project", "main", "project"),
		}}},
	}

	command := newTestRootCommandWithRuntime(t, options.runtime)
	var stderr bytes.Buffer
	command.SetErr(&stderr)
	command.SetOut(io.Discard)

	require.NoError(t, options.Execute(command, nil), stderr.String())

	_, err := os.Stat(filepath.Join(worktreeRootPath, "project", "main", "project"))
	require.NoError(t, err)
	// Skipped feature worktree remains at original path (or was left alone).
	_, err = os.Stat(featurePath)
	require.NoError(t, err)
}

func TestMigrateRehomesRegisteredWorktrees(t *testing.T) {
	t.Parallel()
	const branchName = "feature/login"

	testRepository := newTestRepository(t)
	legacyPath := addLegacyWorktree(t, testRepository.barePath, testRepository.worktreeRoot, testRepoName, branchName)
	require.NoError(t, os.WriteFile(filepath.Join(legacyPath, "dirty.txt"), []byte("dirty\n"), 0o644))

	result := testRepository.runTimberFrom(t, legacyPath, "migrate")
	require.NoError(t, result.err, result.stderr)
	assert.Contains(t, result.stderr, testRepository.worktreePath(branchName))

	testRepository.assertPathPresent(t, testRepository.worktreePath(branchName))
	testRepository.assertPathMissing(t, legacyPath)
	assert.FileExists(t, filepath.Join(testRepository.worktreePath(branchName), "dirty.txt"))

	listResult := testRepository.runTimber(t, "list", at(testRepoName, ""))
	require.NoError(t, listResult.err, listResult.stderr)
	assert.Contains(t, listResult.stdout, branchName)
}

func TestMigrateSkipsRegisteredWorktreesAlreadyAtManagedPath(t *testing.T) {
	t.Parallel()
	const branchName = "feature/login"

	testRepository := newTestRepository(t)
	require.NoError(t, testRepository.runTimber(t, "create", at(testRepoName, branchName)).err)

	result := testRepository.runTimberFrom(t, testRepository.worktreePath(branchName), "migrate")
	require.NoError(t, result.err, result.stderr)
	assert.Contains(t, result.stderr, "no worktrees to rehome")
	testRepository.assertPathPresent(t, testRepository.worktreePath(branchName))
}

func TestMigrateRehomesAllRegisteredWorktreesFromOutsideGit(t *testing.T) {
	t.Parallel()
	primary := newTestRepository(t)
	secondaryName := "other"
	secondaryBarePath := registerAdditionalRepo(t, primary, secondaryName)

	legacyPrimary := addLegacyWorktree(t, primary.barePath, primary.worktreeRoot, testRepoName, "feature/one")
	legacySecondary := addLegacyWorktree(t, secondaryBarePath, primary.worktreeRoot, secondaryName, "feature/two")

	result := primary.runTimberFrom(t, primary.home, "migrate", "--all")
	require.NoError(t, result.err, result.stderr)

	primary.assertPathPresent(t, primary.worktreePath("feature/one"))
	primary.assertPathMissing(t, legacyPrimary)
	_, err := os.Stat(filepath.Join(primary.worktreeRoot, secondaryName, "feature/two", secondaryName))
	require.NoError(t, err)
	_, err = os.Stat(legacySecondary)
	assert.ErrorIs(t, err, os.ErrNotExist)
}

func TestMigrateRehomesRegisteredWorktreeNamedLikeRepo(t *testing.T) {
	t.Parallel()
	testRepository := newTestRepository(t)
	legacyPath := addLegacyWorktree(t, testRepository.barePath, testRepository.worktreeRoot, testRepoName, testRepoName)

	result := testRepository.runTimberFrom(t, legacyPath, "migrate")
	require.NoError(t, result.err, result.stderr)

	newPath := testRepository.worktreePath(testRepoName)
	testRepository.assertPathPresent(t, newPath)
	assert.NoFileExists(t, filepath.Join(legacyPath, ".git"))

	listResult := testRepository.runTimber(t, "list", at(testRepoName, ""))
	require.NoError(t, listResult.err, listResult.stderr)
	assert.Contains(t, listResult.stdout, testRepoName)
}

func TestMigrateStripsGitSuffixFromNameFlag(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	worktreeRootPath := filepath.Join(home, "worktrees")
	runtime := testRuntimeForHome(home, home)

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

	runtime.CurrentDirectory = clonePath
	result := runTimberFromWithRuntime(t, runtime, clonePath, "migrate", "--name", "roam.git")
	require.NoError(t, result.err, result.stderr)

	barePath := filepath.Join(home, ".local", "share", "timber", "repos", "roam.git")
	_, err := os.Stat(barePath)
	require.NoError(t, err)

	masterTarget := filepath.Join(worktreeRootPath, "roam", "master", "roam")
	_, err = os.Stat(masterTarget)
	require.NoError(t, err)
	_, err = os.Stat(filepath.Join(worktreeRootPath, "roam", "master", "roam.git"))
	assert.ErrorIs(t, err, os.ErrNotExist)
}
