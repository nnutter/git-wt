package timber

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// importFixture builds a bare remote plus a plain clone outside the Timber
// layout, ready for import. Removals go through a fake trash CLI that records
// every path it receives.
type importFixture struct {
	runtime    Runtime
	clonePath  string
	remotePath string
	trashLog   string
}

// installFakeTrash writes a trash CLI that logs and removes the paths it is
// given, emulating a successful move to the system trash.
func installFakeTrash(t *testing.T) (scriptPath string, logPath string) {
	t.Helper()

	binDir := t.TempDir()
	logPath = filepath.Join(t.TempDir(), "trash.log")
	scriptPath = filepath.Join(binDir, "trash")
	script := fmt.Sprintf(`#!/bin/sh
for arg in "$@"; do
  printf '%%s\n' "$arg" >> %q
  rm -rf "$arg"
done
`, logPath)
	require.NoError(t, os.WriteFile(scriptPath, []byte(script), 0o755))
	return scriptPath, logPath
}

func newImportFixture(t *testing.T) importFixture {
	t.Helper()

	home := t.TempDir()
	runtime := testRuntimeForHome(home, home)
	trashExecutable, trashLog := installFakeTrash(t)
	runtime.TrashExecutable = trashExecutable

	base := t.TempDir()
	remotePath := filepath.Join(base, "remote.git")
	runGitCommand(t, base, "init", "--bare", remotePath)
	seedBareRemote(t, remotePath)

	clonePath := filepath.Join(base, "project")
	runGitCommand(t, base, "clone", remotePath, clonePath)
	configureGitUser(t, clonePath)

	return importFixture{runtime: runtime, clonePath: clonePath, remotePath: remotePath, trashLog: trashLog}
}

func (x importFixture) trashedPaths(t *testing.T) []string {
	t.Helper()
	contents, err := os.ReadFile(x.trashLog)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	require.NoError(t, err)
	return strings.Split(strings.TrimSpace(string(contents)), "\n")
}

func (x importFixture) importRun(t *testing.T, args ...string) commandResult {
	t.Helper()
	fullArgs := append([]string{"repo", "import", x.clonePath}, args...)
	return runTimberFromWithRuntime(t, x.runtime, x.runtime.HomeDirectory, fullArgs...)
}

func (x importFixture) barePath(repoName string) string {
	return filepath.Join(x.runtime.HomeDirectory, ".local", "share", "timber", "repos", repoName+".git")
}

func (x importFixture) managedWorktreePath(repoName string, name string) string {
	return x.runtime.managedWorktreePath(repoName, name)
}

func (x importFixture) assertImportedBareSetup(t *testing.T, repoName string) {
	t.Helper()

	barePath := x.barePath(repoName)
	_, err := os.Stat(barePath)
	require.NoError(t, err)

	// Import must install the same origin tracking setup as repo add.
	fetch := strings.TrimSpace(runGitCommand(t, barePath, "config", "--get", "remote.origin.fetch"))
	assert.Equal(t, "+refs/heads/*:refs/remotes/origin/*", fetch)
	originURL := strings.TrimSpace(runGitCommand(t, barePath, "remote", "get-url", "origin"))
	assert.Equal(t, x.remotePath, originURL)
	originHead := strings.TrimSpace(runGitCommand(t, barePath, "symbolic-ref", "--short", "refs/remotes/origin/HEAD"))
	assert.Equal(t, "origin/main", originHead)
}

func (x importFixture) assertWorktreeBranchAt(t *testing.T, path string, branchName string) {
	t.Helper()

	runGitCommand(t, path, "status")
	branch := strings.TrimSpace(runGitCommand(t, path, "rev-parse", "--abbrev-ref", "HEAD"))
	assert.Equal(t, branchName, branch)
}

func TestRepoImportRegistersBareAndMovesMainWorktree(t *testing.T) {
	t.Parallel()
	fixture := newImportFixture(t)

	result := fixture.importRun(t, "--name", "project")
	require.NoError(t, result.err, result.stderr)

	fixture.assertImportedBareSetup(t, "project")

	// The main worktree becomes a managed worktree under timber.
	mainTarget := fixture.managedWorktreePath("project", "main")
	fixture.assertWorktreeBranchAt(t, mainTarget, "main")

	// The source checkout is gone.
	_, err := os.Stat(fixture.clonePath)
	assert.ErrorIs(t, err, os.ErrNotExist)

	listResult := runTimberFromWithRuntime(t, fixture.runtime, fixture.runtime.HomeDirectory, "list", at("project", ""))
	require.NoError(t, listResult.err, listResult.stderr)
	assert.Contains(t, listResult.stdout, "main")
}

func TestRepoImportSummaryShowsEachMove(t *testing.T) {
	t.Parallel()
	fixture := newImportFixture(t)
	featurePath := filepath.Join(filepath.Dir(fixture.clonePath), "feature-worktree")
	runGitCommand(t, fixture.clonePath, "branch", "feature/login")
	runGitCommand(t, fixture.clonePath, "worktree", "add", featurePath, "feature/login")

	result := fixture.importRun(t, "--name", "project")
	require.NoError(t, result.err, result.stderr)

	assert.Contains(t, result.stderr, "registered repository project at "+fixture.runtime.displayHomePath(fixture.barePath("project")))
	assert.Contains(t, result.stderr, "imported 2 worktrees:")
	assert.Contains(t, result.stderr, "main: "+fixture.runtime.displayHomePath(fixture.clonePath)+
		" -> "+fixture.runtime.displayHomePath(fixture.managedWorktreePath("project", "main")))
	assert.Contains(t, result.stderr, "feature/login: "+fixture.runtime.displayHomePath(featurePath)+
		" -> "+fixture.runtime.displayHomePath(fixture.managedWorktreePath("project", "feature/login")))
	assert.Contains(t, result.stderr, "moved to the system trash")
}

func TestRepoImportTrashesOldWorktrees(t *testing.T) {
	t.Parallel()
	fixture := newImportFixture(t)
	featurePath := filepath.Join(filepath.Dir(fixture.clonePath), "feature-worktree")
	runGitCommand(t, fixture.clonePath, "branch", "feature/login")
	runGitCommand(t, fixture.clonePath, "worktree", "add", featurePath, "feature/login")

	result := fixture.importRun(t, "--name", "project")
	require.NoError(t, result.err, result.stderr)

	// Old worktrees go through the trash CLI instead of being deleted.
	trashed := fixture.trashedPaths(t)
	assert.Contains(t, trashed, fixture.clonePath)
	assert.Contains(t, trashed, featurePath)
}

func TestRepoImportMovesLinkedWorktrees(t *testing.T) {
	t.Parallel()
	fixture := newImportFixture(t)
	featurePath := filepath.Join(filepath.Dir(fixture.clonePath), "feature-worktree")
	runGitCommand(t, fixture.clonePath, "branch", "feature/login")
	runGitCommand(t, fixture.clonePath, "worktree", "add", featurePath, "feature/login")

	result := fixture.importRun(t, "--name", "project")
	require.NoError(t, result.err, result.stderr)

	mainTarget := fixture.managedWorktreePath("project", "main")
	featureTarget := fixture.managedWorktreePath("project", "feature/login")
	fixture.assertWorktreeBranchAt(t, mainTarget, "main")
	fixture.assertWorktreeBranchAt(t, featureTarget, "feature/login")

	_, err := os.Stat(featurePath)
	assert.ErrorIs(t, err, os.ErrNotExist)

	listResult := runTimberFromWithRuntime(t, fixture.runtime, fixture.runtime.HomeDirectory, "list", at("project", ""))
	require.NoError(t, listResult.err, listResult.stderr)
	assert.Contains(t, listResult.stdout, "main")
	assert.Contains(t, listResult.stdout, "feature/login")
}

func TestRepoImportPreservesUncommittedChanges(t *testing.T) {
	t.Parallel()
	fixture := newImportFixture(t)
	featurePath := filepath.Join(filepath.Dir(fixture.clonePath), "feature-worktree")
	runGitCommand(t, fixture.clonePath, "branch", "feature/login")
	runGitCommand(t, fixture.clonePath, "worktree", "add", featurePath, "feature/login")

	require.NoError(t, os.WriteFile(filepath.Join(fixture.clonePath, "README.md"), []byte("uncommitted edit\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(featurePath, "README.md"), []byte("feature edit\n"), 0o644))

	result := fixture.importRun(t, "--name", "project")
	require.NoError(t, result.err, result.stderr)

	mainContents, err := os.ReadFile(filepath.Join(fixture.managedWorktreePath("project", "main"), "README.md"))
	require.NoError(t, err)
	assert.Equal(t, "uncommitted edit\n", string(mainContents))

	featureContents, err := os.ReadFile(filepath.Join(fixture.managedWorktreePath("project", "feature/login"), "README.md"))
	require.NoError(t, err)
	assert.Equal(t, "feature edit\n", string(featureContents))
}

func TestRepoImportPreservesUntrackedFiles(t *testing.T) {
	t.Parallel()
	fixture := newImportFixture(t)

	nestedDirectory := filepath.Join(fixture.clonePath, "notes", "deep")
	require.NoError(t, os.MkdirAll(nestedDirectory, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(nestedDirectory, "todo.txt"), []byte("keep me\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(fixture.clonePath, "scratch.txt"), []byte("scratch\n"), 0o644))

	result := fixture.importRun(t, "--name", "project")
	require.NoError(t, result.err, result.stderr)

	mainTarget := fixture.managedWorktreePath("project", "main")
	contents, err := os.ReadFile(filepath.Join(mainTarget, "notes", "deep", "todo.txt"))
	require.NoError(t, err)
	assert.Equal(t, "keep me\n", string(contents))
	contents, err = os.ReadFile(filepath.Join(mainTarget, "scratch.txt"))
	require.NoError(t, err)
	assert.Equal(t, "scratch\n", string(contents))
}

func TestRepoImportPreservesGitignoredFiles(t *testing.T) {
	t.Parallel()
	fixture := newImportFixture(t)

	require.NoError(t, os.WriteFile(filepath.Join(fixture.clonePath, ".gitignore"), []byte("build/\n"), 0o644))
	buildDirectory := filepath.Join(fixture.clonePath, "build")
	require.NoError(t, os.MkdirAll(buildDirectory, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(buildDirectory, "artifact.bin"), []byte("binary\n"), 0o644))

	result := fixture.importRun(t, "--name", "project")
	require.NoError(t, result.err, result.stderr)

	contents, err := os.ReadFile(filepath.Join(fixture.managedWorktreePath("project", "main"), "build", "artifact.bin"))
	require.NoError(t, err)
	assert.Equal(t, "binary\n", string(contents))
}

func TestRepoImportRecreatesDetachedWorktree(t *testing.T) {
	t.Parallel()
	fixture := newImportFixture(t)
	detachedPath := filepath.Join(filepath.Dir(fixture.clonePath), "detached-worktree")
	runGitCommand(t, fixture.clonePath, "worktree", "add", "--detach", detachedPath, "HEAD")

	detachedHash := strings.TrimSpace(runGitCommand(t, detachedPath, "rev-parse", "HEAD"))

	result := fixture.importRun(t, "--name", "project")
	require.NoError(t, result.err, result.stderr)

	detachedTarget := fixture.runtime.managedWorktreePath("project", shortCommitHash(detachedHash))
	fixture.assertWorktreeBranchAt(t, detachedTarget, "HEAD")
	_, err := os.Stat(detachedTarget)
	require.NoError(t, err)

	assert.Contains(t, result.stderr, "imported 2 worktrees:")

	_, err = os.Stat(detachedPath)
	assert.ErrorIs(t, err, os.ErrNotExist)
}

func TestRepoImportSkipsPrunableWorktrees(t *testing.T) {
	t.Parallel()
	fixture := newImportFixture(t)
	featurePath := filepath.Join(filepath.Dir(fixture.clonePath), "feature-worktree")
	runGitCommand(t, fixture.clonePath, "branch", "feature/login")
	runGitCommand(t, fixture.clonePath, "worktree", "add", featurePath, "feature/login")
	require.NoError(t, os.RemoveAll(featurePath))

	result := fixture.importRun(t, "--name", "project")
	require.NoError(t, result.err, result.stderr)

	// The missing worktree is reported, not silently dropped.
	assert.Contains(t, result.stderr, "skipped worktree")
	assert.Contains(t, result.stderr, featurePath)
	assert.Contains(t, result.stderr, "imported 1 worktrees:")

	// Its branch still survives in the new bare repository.
	runGitCommand(t, fixture.barePath("project"), "show-ref", "--verify", "refs/heads/feature/login")

	mainTarget := fixture.managedWorktreePath("project", "main")
	fixture.assertWorktreeBranchAt(t, mainTarget, "main")
}

func TestRepoImportSkipsWorktreeWithNoCommits(t *testing.T) {
	t.Parallel()
	fixture := newImportFixture(t)

	// An unborn repository has no commits to move; import registers it and
	// reports the skipped worktree instead of failing.
	unbornPath := filepath.Join(filepath.Dir(fixture.clonePath), "unborn")
	require.NoError(t, os.MkdirAll(unbornPath, 0o755))
	runGitCommand(t, unbornPath, "init")

	runtime := fixture.runtime
	runtime.CurrentDirectory = runtime.HomeDirectory
	result := runTimberFromWithRuntime(t, runtime, runtime.HomeDirectory, "repo", "import", unbornPath)
	require.NoError(t, result.err, result.stderr)

	assert.Contains(t, result.stderr, "skipped worktree")
	assert.Contains(t, result.stderr, unbornPath)
	assert.Contains(t, result.stderr, "no commits yet")
	assert.Contains(t, result.stderr, "imported 0 worktrees:")

	// The source repository is left in place.
	_, err := os.Stat(unbornPath)
	assert.NoError(t, err)
	_, err = os.Stat(filepath.Join(runtime.WorktreeRoot, "unborn"))
	assert.ErrorIs(t, err, os.ErrNotExist)
}

func TestRepoImportPreservesLocalOnlyBranches(t *testing.T) {
	t.Parallel()
	fixture := newImportFixture(t)
	runGitCommand(t, fixture.clonePath, "branch", "local-only")

	result := fixture.importRun(t, "--name", "project")
	require.NoError(t, result.err, result.stderr)

	runGitCommand(t, fixture.barePath("project"), "show-ref", "--verify", "refs/heads/local-only")
}

func TestRepoImportDerivesNameFromRemote(t *testing.T) {
	t.Parallel()
	fixture := newImportFixture(t)

	result := fixture.importRun(t)
	require.NoError(t, result.err, result.stderr)

	assert.Contains(t, result.stderr, "registered repository remote at "+fixture.runtime.displayHomePath(fixture.barePath("remote")))
	_, err := os.Stat(fixture.barePath("remote"))
	require.NoError(t, err)
}

func TestRepoImportFailsBeforeMutatingWhenTargetExists(t *testing.T) {
	t.Parallel()
	fixture := newImportFixture(t)

	targetPath := fixture.managedWorktreePath("project", "main")
	require.NoError(t, os.MkdirAll(targetPath, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(targetPath, "README.md"), []byte("occupied\n"), 0o644))

	result := fixture.importRun(t, "--name", "project")
	require.Error(t, result.err)
	assert.Contains(t, result.err.Error(), "already exists")

	// Nothing was touched: source remains, no bare repository was created.
	_, err := os.Stat(fixture.clonePath)
	require.NoError(t, err)
	_, err = os.Stat(fixture.barePath("project"))
	assert.ErrorIs(t, err, os.ErrNotExist)
}

func TestRepoImportFailsWhenRepoNameAlreadyExists(t *testing.T) {
	t.Parallel()
	fixture := newImportFixture(t)

	reposDirectory := filepath.Join(fixture.runtime.HomeDirectory, ".local", "share", "timber", "repos")
	require.NoError(t, os.MkdirAll(reposDirectory, 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(reposDirectory, "project.git"), 0o755))

	result := fixture.importRun(t, "--name", "project")
	require.Error(t, result.err)
	assert.Contains(t, result.err.Error(), "already exists")

	_, err := os.Stat(fixture.clonePath)
	require.NoError(t, err)
}

func TestRepoImportFailsWhenAlreadyRegistered(t *testing.T) {
	t.Parallel()
	testRepository := newTestRepository(t)
	require.NoError(t, testRepository.runTimber(t, "create", at(testRepoName, "main")).err)

	result := testRepository.runTimberFrom(
		t,
		testRepository.worktreePath("main"),
		"repo", "import", testRepository.worktreePath("main"),
	)
	require.Error(t, result.err)
	assert.Contains(t, result.err.Error(), "already registered")
}

func TestRepoImportFailsOutsideRepository(t *testing.T) {
	t.Parallel()
	fixture := newImportFixture(t)
	emptyPath := filepath.Join(filepath.Dir(fixture.clonePath), "not-a-repo")
	require.NoError(t, os.MkdirAll(emptyPath, 0o755))

	runtime := fixture.runtime
	runtime.CurrentDirectory = runtime.HomeDirectory
	result := runTimberFromWithRuntime(t, runtime, runtime.HomeDirectory, "repo", "import", emptyPath)
	require.Error(t, result.err)
}

func TestRepoImportSuggestsRepoAddForBareRepository(t *testing.T) {
	t.Parallel()
	fixture := newImportFixture(t)

	runtime := fixture.runtime
	runtime.CurrentDirectory = runtime.HomeDirectory
	result := runTimberFromWithRuntime(t, runtime, runtime.HomeDirectory, "repo", "import", fixture.remotePath)
	require.Error(t, result.err)
	assert.Contains(t, result.err.Error(), "bare repository")
	assert.Contains(t, result.err.Error(), "timber repo add")
}

func TestRepoImportSuggestsRepoAddForBareRepositoryWithWorktrees(t *testing.T) {
	t.Parallel()
	fixture := newImportFixture(t)

	// A bare repository with a linked worktree is already in the managed
	// shape; importing it must point at repo add instead.
	base := filepath.Dir(fixture.clonePath)
	barePath := filepath.Join(base, "already-bare.git")
	runGitCommand(t, base, "clone", "--bare", fixture.remotePath, barePath)
	linkedPath := filepath.Join(base, "linked")
	runGitCommand(t, barePath, "worktree", "add", linkedPath, "main")

	runtime := fixture.runtime
	runtime.CurrentDirectory = runtime.HomeDirectory
	result := runTimberFromWithRuntime(t, runtime, runtime.HomeDirectory, "repo", "import", linkedPath)
	require.Error(t, result.err)
	assert.Contains(t, result.err.Error(), "bare repository")
	assert.Contains(t, result.err.Error(), "timber repo add")

	// The source worktree is untouched.
	_, err := os.Stat(linkedPath)
	require.NoError(t, err)
}

func TestRepoImportFailsBeforeMutatingWhenTrashCommandMissing(t *testing.T) {
	t.Parallel()
	fixture := newImportFixture(t)

	runtime := fixture.runtime
	runtime.TrashExecutable = "timber-missing-trash"
	result := runTimberFromWithRuntime(t, runtime, runtime.HomeDirectory, "repo", "import", fixture.clonePath, "--name", "project")
	require.Error(t, result.err)
	assert.Contains(t, result.err.Error(), "trash command")

	// Nothing was touched: source remains, no bare repository was created.
	_, err := os.Stat(fixture.clonePath)
	require.NoError(t, err)
	_, err = os.Stat(fixture.barePath("project"))
	assert.ErrorIs(t, err, os.ErrNotExist)
}

func TestRepoImportFailsOnMissingPath(t *testing.T) {
	t.Parallel()
	fixture := newImportFixture(t)

	runtime := fixture.runtime
	runtime.CurrentDirectory = runtime.HomeDirectory
	missingPath := filepath.Join(filepath.Dir(fixture.clonePath), "missing")
	result := runTimberFromWithRuntime(t, runtime, runtime.HomeDirectory, "repo", "import", missingPath)
	require.Error(t, result.err)
}
