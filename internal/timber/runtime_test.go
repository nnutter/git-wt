package timber

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRuntimeFromProcessCapturesProcessState(t *testing.T) {
	t.Parallel()

	runtime, err := RuntimeFromProcess()
	require.NoError(t, err)

	currentDirectory, err := os.Getwd()
	require.NoError(t, err)
	homeDirectory, err := os.UserHomeDir()
	require.NoError(t, err)

	assert.Equal(t, currentDirectory, runtime.CurrentDirectory)
	assert.Equal(t, homeDirectory, runtime.HomeDirectory)
	assert.NotEmpty(t, runtime.DataHome)
	assert.NotEmpty(t, runtime.ConfigHome)
	assert.NotEmpty(t, runtime.WorktreeRoot)
	assert.Equal(t, os.TempDir(), runtime.TemporaryDirectory)
	assert.NotNil(t, runtime.Environment)
}

func TestParseQualifiedName(t *testing.T) {
	t.Parallel()
	testRepository := newTestRepository(t)
	runtime := testRepository.runtime

	qualified, err := runtime.parseQualifiedName("feature/login")
	require.NoError(t, err)
	assert.Equal(t, qualifiedName{Name: "feature/login"}, qualified)

	qualified, err = runtime.parseQualifiedName(at(testRepoName, "feature/login"))
	require.NoError(t, err)
	assert.Equal(t, qualifiedName{Name: "feature/login", Repo: testRepoName}, qualified)

	qualified, err = runtime.parseQualifiedName(at(testRepoName, ""))
	require.NoError(t, err)
	assert.Equal(t, qualifiedName{Repo: testRepoName}, qualified)

	qualified, err = runtime.parseQualifiedName("feature@nested@" + testRepoName)
	require.NoError(t, err)
	assert.Equal(t, qualifiedName{Name: "feature@nested", Repo: testRepoName}, qualified)

	_, err = runtime.parseQualifiedName("feature/login@")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing repository")

	_, err = runtime.parseQualifiedName("feature/login@unknown")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown repository")
}

func TestParseRepoOnlyArg(t *testing.T) {
	t.Parallel()
	testRepository := newTestRepository(t)
	runtime := testRepository.runtime

	repo, err := runtime.parseRepoOnlyArg(at(testRepoName, ""))
	require.NoError(t, err)
	assert.Equal(t, testRepoName, repo)

	_, err = runtime.parseRepoOnlyArg(at(testRepoName, "feature/login"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "expected @<repo>")

	_, err = runtime.parseRepoOnlyArg(testRepoName)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "expected @<repo>")
}

func TestWritePathFileRejectsPathOutsideTemporaryDirectory(t *testing.T) {
	t.Parallel()
	pathFile := filepath.Join(os.TempDir(), "..", "timber-path-file")

	runtime := testRuntime(t)
	err := runtime.writePathFile(pathFile, "worktree")

	require.Error(t, err)
	require.Contains(t, err.Error(), "outside temporary directory")
}

func TestWorktreeRootUsesEnvironmentOverride(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	customRoot := filepath.Join(t.TempDir(), "custom-worktrees")
	runtime := testRuntimeForHome(home, home)
	runtime.WorktreeRoot = customRoot

	assert.Equal(t, customRoot, runtime.worktreeRoot())
	assert.Equal(t, filepath.Join(customRoot, "repo", "feature", "repo"), runtime.managedWorktreePath("repo", "feature"))
}

func TestWorktreeRootFallsBackToHomeWorktrees(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	runtime := testRuntimeForHome(home, home)
	assert.Equal(t, filepath.Join(home, "worktrees"), runtime.worktreeRoot())
}

func TestDisplayHomePath(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	runtime := testRuntimeForHome(home, home)

	assert.Equal(t, "~", runtime.displayHomePath(home))
	assert.Equal(t, filepath.Join("~", ".local", "share", "timber", "repos", "demo.git"), runtime.displayHomePath(filepath.Join(home, ".local", "share", "timber", "repos", "demo.git")))
	assert.Equal(t, "/tmp/other", runtime.displayHomePath("/tmp/other"))
}
