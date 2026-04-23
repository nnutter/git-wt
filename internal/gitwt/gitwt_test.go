package gitwt

import (
	"bytes"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
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

func (x stubPrompter) Prompt(input io.Reader, output io.Writer, worktrees []managedWorktree) ([]managedWorktree, error) {
	return x.selected, x.err
}

func TestCreateListAndRemoveLifecycle(t *testing.T) {
	const branchName = "feature/one"

	testRepository := newTestRepository(t)

	createResult := testRepository.runGitWT(t, "create", branchName)
	if createResult.err != nil {
		t.Fatalf("create failed: %v\n%s", createResult.err, createResult.stderr)
	}

	listResult := testRepository.runGitWT(t, "list")
	if listResult.err != nil {
		t.Fatalf("list failed: %v\n%s", listResult.err, listResult.stderr)
	}
	if !strings.Contains(listResult.stdout, branchName) {
		t.Fatalf("list output missing worktree name: %s", listResult.stdout)
	}

	testRepository.mergeWorktreeBranch(t, branchName)

	removeResult := testRepository.runGitWT(t, "remove", branchName)
	if removeResult.err != nil {
		t.Fatalf("remove failed: %v\n%s", removeResult.err, removeResult.stderr)
	}

	testRepository.assertBranchMissing(t, branchName)
	testRepository.assertPathMissing(t, testRepository.worktreePath(branchName))
}

func TestCreateFailsWhenBranchExists(t *testing.T) {
	const branchName = "feature/existing"

	testRepository := newTestRepository(t)
	runGitCommand(t, testRepository.mainPath, "branch", branchName, "main")

	result := testRepository.runGitWT(t, "create", branchName)
	if result.err == nil {
		t.Fatal("expected create to fail when branch exists")
	}
}

func TestCreateFailsWhenDirectoryExists(t *testing.T) {
	const branchName = "feature/existing"

	testRepository := newTestRepository(t)
	worktreePath := testRepository.worktreePath(branchName)
	if err := os.MkdirAll(worktreePath, 0o755); err != nil {
		t.Fatalf("create worktree directory: %v", err)
	}

	result := testRepository.runGitWT(t, "create", branchName)
	if result.err == nil {
		t.Fatal("expected create to fail when directory exists")
	}
}

func TestRemoveFailsWhenDirtyWithoutForce(t *testing.T) {
	const branchName = "feature/dirty"
	const dirtyFileName = "dirty.txt"
	const dirtyFileContents = "dirty"

	testRepository := newTestRepository(t)
	testRepository.runGitWT(t, "create", branchName)
	writeFile(t, filepath.Join(testRepository.worktreePath(branchName), dirtyFileName), dirtyFileContents)

	result := testRepository.runGitWT(t, "remove", branchName)
	if result.err == nil {
		t.Fatal("expected remove to fail for dirty worktree")
	}
}

func TestRemoveFailsWhenUnmergedWithoutForce(t *testing.T) {
	const branchName = "feature/unmerged"
	const workFileName = "work.txt"
	const workFileContents = "change"

	testRepository := newTestRepository(t)
	testRepository.runGitWT(t, "create", branchName)
	testRepository.commitFileInWorktree(t, branchName, workFileName, workFileContents)

	result := testRepository.runGitWT(t, "remove", branchName)
	if result.err == nil {
		t.Fatal("expected remove to fail for unmerged branch")
	}
}

func TestRemoveForceRemovesDirtyUnmergedWorktree(t *testing.T) {
	const branchName = "feature/force"
	const workFileName = "work.txt"
	const workFileContents = "change"
	const dirtyFileName = "dirty.txt"
	const dirtyFileContents = "dirty"

	testRepository := newTestRepository(t)
	testRepository.runGitWT(t, "create", branchName)
	testRepository.commitFileInWorktree(t, branchName, workFileName, workFileContents)
	writeFile(t, filepath.Join(testRepository.worktreePath(branchName), dirtyFileName), dirtyFileContents)

	result := testRepository.runGitWT(t, "remove", "--force", branchName)
	if result.err != nil {
		t.Fatalf("force remove failed: %v\n%s", result.err, result.stderr)
	}

	testRepository.assertBranchMissing(t, branchName)
	testRepository.assertPathMissing(t, testRepository.worktreePath(branchName))
}

func TestPruneRemovesOnlyMergedCleanWorktrees(t *testing.T) {
	const mergedBranchName = "feature/merged"
	const unmergedBranchName = "feature/unmerged"
	const workFileName = "work.txt"
	const workFileContents = "change"

	testRepository := newTestRepository(t)
	testRepository.runGitWT(t, "create", mergedBranchName)
	testRepository.runGitWT(t, "create", unmergedBranchName)
	testRepository.mergeWorktreeBranch(t, mergedBranchName)
	testRepository.commitFileInWorktree(t, unmergedBranchName, workFileName, workFileContents)

	result := testRepository.runGitWT(t, "prune")
	if result.err != nil {
		t.Fatalf("prune failed: %v\n%s", result.err, result.stderr)
	}

	testRepository.assertBranchMissing(t, mergedBranchName)
	testRepository.assertPathMissing(t, testRepository.worktreePath(mergedBranchName))
	testRepository.assertBranchPresent(t, unmergedBranchName)
}

func TestPrunePromptCanForceRemoveSelectedWorktrees(t *testing.T) {
	const branchName = "feature/prompt"
	const workFileName = "work.txt"
	const workFileContents = "change"

	testRepository := newTestRepository(t)
	createResult := testRepository.runGitWT(t, "create", branchName)
	if createResult.err != nil {
		t.Fatalf("create failed: %v", createResult.err)
	}
	testRepository.commitFileInWorktree(t, branchName, workFileName, workFileContents)

	command := &cobra.Command{}
	command.SetIn(bytes.NewBuffer(nil))
	var stderr bytes.Buffer
	command.SetErr(&stderr)
	currentDirectory, err := os.Getwd()
	if err != nil {
		t.Fatalf("get current directory: %v", err)
	}
	if err := os.Chdir(testRepository.mainPath); err != nil {
		t.Fatalf("change directory: %v", err)
	}
	defer func() {
		if err := os.Chdir(currentDirectory); err != nil {
			t.Fatalf("restore directory: %v", err)
		}
	}()

	options := &pruneCommandOptions{
		prompt:   true,
		prompter: stubPrompter{selected: []managedWorktree{{Name: branchName}}},
	}

	if err := options.Execute(command, nil); err != nil {
		t.Fatalf("prompt prune failed: %v\n%s", err, stderr.String())
	}

	testRepository.assertBranchMissing(t, branchName)
	testRepository.assertPathMissing(t, testRepository.worktreePath(branchName))
}

type testRepository struct {
	rootPath   string
	mainPath   string
	remotePath string
}

func newTestRepository(t *testing.T) testRepository {
	t.Helper()

	rootPath := t.TempDir()
	remotePath := filepath.Join(rootPath, "remote.git")
	mainPath := filepath.Join(rootPath, "repo")

	runGitCommand(t, rootPath, "init", "--bare", remotePath)
	runGitCommand(t, rootPath, "init", "--initial-branch=main", mainPath)
	runGitCommand(t, mainPath, "config", "user.name", "Test User")
	runGitCommand(t, mainPath, "config", "user.email", "test@example.com")
	runGitCommand(t, mainPath, "remote", "add", remoteName, remotePath)
	writeFile(t, filepath.Join(mainPath, "README.md"), "initial\n")
	runGitCommand(t, mainPath, "add", "README.md")
	runGitCommand(t, mainPath, "commit", "-m", "initial")
	runGitCommand(t, mainPath, "push", "-u", remoteName, "main")
	runGitCommand(t, mainPath, "remote", "set-head", remoteName, "main")

	return testRepository{
		rootPath:   rootPath,
		mainPath:   mainPath,
		remotePath: remotePath,
	}
}

func (x testRepository) worktreePath(branchName string) string {
	return managedWorktreePath(x.mainPath, branchName)
}

func (x testRepository) runGitWT(t *testing.T, args ...string) commandResult {
	t.Helper()

	command := NewRootCommand()
	command.SetArgs(args)
	command.SetIn(bytes.NewBuffer(nil))

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	command.SetOut(&stdout)
	command.SetErr(&stderr)

	currentDirectory, err := os.Getwd()
	if err != nil {
		t.Fatalf("get current directory: %v", err)
	}
	if err := os.Chdir(x.mainPath); err != nil {
		t.Fatalf("change directory: %v", err)
	}
	defer func() {
		if err := os.Chdir(currentDirectory); err != nil {
			t.Fatalf("restore directory: %v", err)
		}
	}()

	err = command.Execute()
	return commandResult{stdout: stdout.String(), stderr: stderr.String(), err: err}
}

func (x testRepository) commitFileInWorktree(t *testing.T, branchName string, fileName string, contents string) {
	t.Helper()
	worktreePath := x.worktreePath(branchName)
	writeFile(t, filepath.Join(worktreePath, fileName), contents)
	runGitCommand(t, worktreePath, "add", fileName)
	runGitCommand(t, worktreePath, "commit", "-m", "change")
}

func (x testRepository) mergeWorktreeBranch(t *testing.T, branchName string) {
	t.Helper()
	runGitCommand(t, x.mainPath, "merge", "--ff-only", branchName)
	runGitCommand(t, x.mainPath, "push", remoteName, "main")
	runGitCommand(t, x.mainPath, "fetch", remoteName)
}

func (x testRepository) assertBranchMissing(t *testing.T, branchName string) {
	t.Helper()
	command := exec.Command("git", "show-ref", "--verify", "--quiet", "refs/heads/"+branchName)
	command.Dir = x.mainPath
	err := command.Run()
	if exitError, ok := err.(*exec.ExitError); ok && exitError.ExitCode() == 1 {
		return
	}
	if err == nil {
		t.Fatalf("expected branch %s to be missing", branchName)
	}
	t.Fatalf("unexpected error checking branch %s: %v", branchName, err)
}

func (x testRepository) assertBranchPresent(t *testing.T, branchName string) {
	t.Helper()
	runGitCommand(t, x.mainPath, "show-ref", "--verify", "refs/heads/"+branchName)
}

func (x testRepository) assertPathMissing(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expected path %s to be missing", path)
	}
}

func writeFile(t *testing.T, path string, contents string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
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
