package gitwt

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/spf13/cobra"
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

	createResult := testRepository.runGitWT(t, "create", branchName)
	if createResult.err != nil {
		t.Fatalf("create failed: %v\n%s", createResult.err, createResult.stderr)
	}
	testRepository.assertPathPresent(t, filepath.Join(testRepository.rootPath, branchName))
	branchCommitHash := strings.TrimSpace(runGitCommand(t, testRepository.mainPath, "rev-parse", "--short=7", branchName))

	listResult := testRepository.runGitWT(t, "list")
	if listResult.err != nil {
		t.Fatalf("list failed: %v\n%s", listResult.err, listResult.stderr)
	}
	if !strings.Contains(listResult.stdout, branchName) {
		t.Fatalf("list output missing worktree name: %s", listResult.stdout)
	}
	if !strings.Contains(listResult.stdout, "main") {
		t.Fatalf("list output missing main worktree: %s", listResult.stdout)
	}
	if strings.Contains(listResult.stdout, "Path") {
		t.Fatalf("list output contains removed Path column: %s", listResult.stdout)
	}
	if !strings.Contains(listResult.stdout, branchCommitHash) {
		t.Fatalf("list output missing commit hash %s: %s", branchCommitHash, listResult.stdout)
	}

	testRepository.mergeWorktreeBranch(t, branchName)
	mergedCommitHash := strings.TrimSpace(runGitCommand(t, testRepository.mainPath, "rev-parse", "--short=7", branchName))

	removeResult := testRepository.runGitWT(t, "remove", branchName)
	if removeResult.err != nil {
		t.Fatalf("remove failed: %v\n%s", removeResult.err, removeResult.stderr)
	}
	if !strings.Contains(removeResult.stderr, mergedCommitHash) {
		t.Fatalf("remove output missing commit hash %s: %s", mergedCommitHash, removeResult.stderr)
	}

	testRepository.assertBranchMissing(t, branchName)
	testRepository.assertPathMissing(t, testRepository.worktreePath(branchName))
}

func TestCreateSucceedsWithWorktreeConfig(t *testing.T) {
	testRepository := newTestRepository(t)
	runGitCommand(t, testRepository.mainPath, "config", "extensions.worktreeConfig", "true")

	result := testRepository.runGitWT(t, "create", "feature/worktree-config")
	if result.err != nil {
		t.Fatalf("create failed: %v\n%s", result.err, result.stderr)
	}
}

func TestListSucceedsWithWorktreeConfig(t *testing.T) {
	const branchName = "feature/worktree-config"

	testRepository := newTestRepository(t)
	runGitCommand(t, testRepository.mainPath, "config", "extensions.worktreeConfig", "true")
	testRepository.runGitWT(t, "create", branchName)

	result := testRepository.runGitWT(t, "list")
	if result.err != nil {
		t.Fatalf("list failed: %v\n%s", result.err, result.stderr)
	}
}

func TestListNamesMainWorktreeMainRegardlessOfBranch(t *testing.T) {
	testRepository := newTestRepository(t)
	runGitCommand(t, testRepository.mainPath, "checkout", "-b", "dev")

	repository, err := openRepository(testRepository.mainPath)
	require.NoError(t, err)
	worktrees, _, err := managedWorktreesFromRepository(repository)
	require.NoError(t, err)

	mainWorktree, err := managedWorktreeForPath(worktrees, testRepository.mainPath)
	require.NoError(t, err)
	assert.Equal(t, "main", mainWorktree.Name)
	assert.Equal(t, branchReference("dev"), mainWorktree.BranchReference)

	result := testRepository.runGitWT(t, "list")
	require.NoError(t, result.err)
	assert.Contains(t, result.stdout, "main (dev)")
}

func TestCreateUsesOriginHeadAsDefaultUpstream(t *testing.T) {
	const defaultBranch = "default"
	const branchName = "feature/origin-head"
	const fileName = "default.txt"
	const fileContents = "default branch\n"

	testRepository := newTestRepository(t)
	runGitCommand(t, testRepository.mainPath, "checkout", "-b", defaultBranch, remoteName+"/main")
	testRepository.writeFile(t, filepath.Join(testRepository.mainPath, fileName), fileContents)
	runGitCommand(t, testRepository.mainPath, "add", fileName)
	runGitCommand(t, testRepository.mainPath, "commit", "-m", "default branch")
	runGitCommand(t, testRepository.mainPath, "push", "-u", remoteName, defaultBranch)
	runGitCommand(t, testRepository.mainPath, "checkout", "main")
	runGitCommand(t, testRepository.mainPath, "remote", "set-head", remoteName, defaultBranch)

	result := testRepository.runGitWT(t, "create", branchName)
	if result.err != nil {
		t.Fatalf("create failed: %v\n%s", result.err, result.stderr)
	}

	createdCommit := strings.TrimSpace(runGitCommand(t, testRepository.mainPath, "rev-parse", branchName))
	upstreamCommit := strings.TrimSpace(runGitCommand(t, testRepository.mainPath, "rev-parse", remoteName+"/"+defaultBranch))
	if createdCommit != upstreamCommit {
		t.Fatalf("created branch commit = %s, want %s", createdCommit, upstreamCommit)
	}

	upstream := strings.TrimSpace(runGitCommand(t, testRepository.mainPath, "rev-parse", "--abbrev-ref", branchName+"@{upstream}"))
	if upstream != remoteName+"/"+defaultBranch {
		t.Fatalf("created branch upstream = %q, want %q", upstream, remoteName+"/"+defaultBranch)
	}
}

func TestCreateFailsWhenOriginHeadIsMissing(t *testing.T) {
	testRepository := newTestRepository(t)
	runGitCommand(t, testRepository.mainPath, "remote", "set-head", "--delete", remoteName)

	result := testRepository.runGitWT(t, "create", "feature/missing-origin-head")
	if result.err == nil {
		t.Fatal("create succeeded without origin/HEAD")
	}
	if !strings.Contains(result.err.Error(), "resolve origin/HEAD") {
		t.Fatalf("create error = %q, want origin/HEAD resolution error", result.err)
	}

	var exitError *exec.ExitError
	if !errors.As(result.err, &exitError) {
		t.Fatalf("create error = %q, want Git command error", result.err)
	}
}

func TestCreateWithHerdrInvokesHerdrWorkspaceCreate(t *testing.T) {
	const branchName = "feature/herdr"

	testRepository := newTestRepository(t)
	logPath := filepath.Join(t.TempDir(), "herdr.log")
	installFakeHerdr(t, logPath, 0)

	result := testRepository.runGitWT(t, "create", "-r", branchName)
	if result.err != nil {
		t.Fatalf("create -r failed: %v\n%s", result.err, result.stderr)
	}
	testRepository.assertPathPresent(t, testRepository.worktreePath(branchName))
	if !strings.Contains(result.stderr, "created herdr workspace "+branchName) {
		t.Fatalf("expected herdr status message, got stderr:\n%s", result.stderr)
	}

	logContents, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read herdr log: %v", err)
	}
	wantCwd, err := filepath.Abs(testRepository.worktreePath(branchName))
	if err != nil {
		t.Fatalf("abs worktree path: %v", err)
	}
	got := strings.TrimSpace(string(logContents))
	want := strings.Join([]string{"workspace", "create", "--cwd", wantCwd, "--label", branchName}, "\x00")
	if got != want {
		t.Fatalf("herdr args\n got: %q\nwant: %q", got, want)
	}
}

func TestCreateWithoutHerdrDoesNotInvokeHerdr(t *testing.T) {
	const branchName = "feature/no-herdr"

	testRepository := newTestRepository(t)
	logPath := filepath.Join(t.TempDir(), "herdr.log")
	installFakeHerdr(t, logPath, 0)

	result := testRepository.runGitWT(t, "create", branchName)
	if result.err != nil {
		t.Fatalf("create failed: %v\n%s", result.err, result.stderr)
	}
	if _, err := os.Stat(logPath); !os.IsNotExist(err) {
		t.Fatalf("expected herdr not to run, log err=%v", err)
	}
}

func TestCreateInHerdrInvokesHerdrWorkspaceCreate(t *testing.T) {
	const branchName = "feature/automatic-herdr"

	testRepository := newTestRepository(t)
	t.Setenv("HERDR_ENV", "1")
	logPath := filepath.Join(t.TempDir(), "herdr.log")
	installFakeHerdr(t, logPath, 0)

	result := testRepository.runGitWT(t, "create", branchName)
	if result.err != nil {
		t.Fatalf("create in Herdr failed: %v\n%s", result.err, result.stderr)
	}
	if _, err := os.Stat(logPath); err != nil {
		t.Fatalf("expected Herdr to run: %v", err)
	}
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

			result := testRepository.runGitWT(t, "create", testCase.flag, branchName)
			if result.err != nil {
				t.Fatalf("create %s failed: %v\n%s", testCase.flag, result.err, result.stderr)
			}
			if _, err := os.Stat(logPath); !os.IsNotExist(err) {
				t.Fatalf("expected Herdr not to run, log err=%v", err)
			}
		})
	}
}

func TestCreateRejectsHerdrAndNoHerdr(t *testing.T) {
	result := runGitWTCommand(t, "create", "-r", "-R", "feature/conflicting-herdr")
	if result.err == nil {
		t.Fatal("expected create with conflicting Herdr flags to fail")
	}
	if !strings.Contains(result.err.Error(), "if any flags in the group [herdr no-herdr] are set none of the others can be") {
		t.Fatalf("expected mutually exclusive flag error, got: %v", result.err)
	}
}

func TestCreateWithHerdrKeepsWorktreeWhenHerdrFails(t *testing.T) {
	const branchName = "feature/herdr-fail"

	testRepository := newTestRepository(t)
	logPath := filepath.Join(t.TempDir(), "herdr.log")
	installFakeHerdr(t, logPath, 1)

	result := testRepository.runGitWT(t, "create", "--herdr", branchName)
	if result.err == nil {
		t.Fatal("expected create --herdr to fail when herdr fails")
	}
	testRepository.assertPathPresent(t, testRepository.worktreePath(branchName))
	if !strings.Contains(result.err.Error(), "herdr workspace create") {
		t.Fatalf("expected herdr error, got: %v", result.err)
	}
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

	if err := os.WriteFile(scriptPath, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake herdr: %v", err)
	}

	path := binDir + string(os.PathListSeparator) + os.Getenv("PATH")
	t.Setenv("PATH", path)
}

func TestCreateFailsWhenBranchExists(t *testing.T) {
	const branchName = "feature/existing"
	const workFileName = "work.txt"
	workFileContents := uuid.NewString()

	testRepository := newTestRepository(t)
	t.Chdir(testRepository.mainPath)
	assertCurrentBranch(t, "main")

	t.Log(runGitCommand(t, testRepository.mainPath, "checkout", "-b", branchName, remoteName+"/main"))
	assertCurrentBranch(t, branchName)
	testRepository.writeFile(t, workFileName, workFileContents)
	t.Log(runGitCommand(t, testRepository.mainPath, "add", workFileName))
	t.Log(runGitCommand(t, testRepository.mainPath, "commit", "-m", "Added "+workFileName, workFileName))

	t.Log(runGitCommand(t, testRepository.mainPath, "checkout", "main"))
	assertCurrentBranch(t, "main")
	testRepository.assertPathMissing(t, workFileName)

	result := testRepository.runGitWT(t, "create", branchName)
	t.Log(result.stderr)
	t.Log(result.stdout)
	if result.err != nil {
		t.Log(result.err)
		t.Fatal("expected create to succeed even when branch exists")
	}

	t.Chdir(testRepository.worktreePath(branchName))
	assertCurrentBranch(t, branchName)
	testRepository.assertPathPresent(t, workFileName)
	if workFileContents != testRepository.readFile(t, workFileName) {
		t.Fatal("expected workFile contents to match")
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
	dirtyFilePath := filepath.Join(testRepository.worktreePath(branchName), dirtyFileName)
	testRepository.writeFile(t, dirtyFilePath, dirtyFileContents)

	result := testRepository.runGitWT(t, "remove", branchName)
	if result.err == nil {
		t.Fatal("expected remove to fail for dirty worktree")
	}
}

func TestRemoveWithNoArgsRemovesCurrentWorktree(t *testing.T) {
	const branchName = "feature/current"

	testRepository := newTestRepository(t)
	testRepository.runGitWT(t, "create", branchName)
	testRepository.mergeWorktreeBranch(t, branchName)
	mergedCommitHash := strings.TrimSpace(runGitCommand(t, testRepository.mainPath, "rev-parse", "--short=7", branchName))

	result := testRepository.runGitWTFrom(t, testRepository.worktreePath(branchName), "remove")
	if result.err != nil {
		t.Fatalf("remove failed: %v\n%s", result.err, result.stderr)
	}
	if !strings.Contains(result.stderr, mergedCommitHash) {
		t.Fatalf("remove output missing commit hash %s: %s", mergedCommitHash, result.stderr)
	}

	testRepository.assertBranchMissing(t, branchName)
	testRepository.assertPathMissing(t, testRepository.worktreePath(branchName))
}

func TestRemoveWithNoArgsFromSubdirectoryRemovesCurrentWorktree(t *testing.T) {
	const branchName = "feature/subdir"
	const subDirectoryName = "nested"

	testRepository := newTestRepository(t)
	testRepository.runGitWT(t, "create", branchName)
	testRepository.mergeWorktreeBranch(t, branchName)

	worktreePath := testRepository.worktreePath(branchName)
	subDirectoryPath := filepath.Join(worktreePath, subDirectoryName)
	if err := os.MkdirAll(subDirectoryPath, 0o755); err != nil {
		t.Fatalf("create subdirectory: %v", err)
	}

	result := testRepository.runGitWTFrom(t, subDirectoryPath, "remove")
	if result.err != nil {
		t.Fatalf("remove failed: %v\n%s", result.err, result.stderr)
	}

	testRepository.assertBranchMissing(t, branchName)
	testRepository.assertPathMissing(t, worktreePath)
}

func TestRemoveWithNoArgsFailsFromMain(t *testing.T) {
	testRepository := newTestRepository(t)

	result := testRepository.runGitWT(t, "remove")
	if result.err == nil {
		t.Fatal("expected remove to fail from main worktree")
	}
	if !strings.Contains(result.err.Error(), "cannot remove main worktree") {
		t.Fatalf("expected main worktree error, got: %v", result.err)
	}
}

func TestRemoveFailsForMainWorktreeByName(t *testing.T) {
	testRepository := newTestRepository(t)
	runGitCommand(t, testRepository.mainPath, "checkout", "-b", "dev")

	result := testRepository.runGitWT(t, "remove", "main")
	if result.err == nil {
		t.Fatal("expected remove to fail for main worktree")
	}
	if !strings.Contains(result.err.Error(), "cannot remove main worktree") {
		t.Fatalf("expected main worktree error, got: %v", result.err)
	}
	testRepository.assertPathPresent(t, testRepository.mainPath)
	testRepository.assertBranchPresent(t, "dev")
}

func TestRemoveWithNoArgsFailsWhenDirtyWithoutForce(t *testing.T) {
	const branchName = "feature/dirty-current"
	const dirtyFileName = "dirty.txt"
	const dirtyFileContents = "dirty"

	testRepository := newTestRepository(t)
	testRepository.runGitWT(t, "create", branchName)
	worktreePath := testRepository.worktreePath(branchName)
	testRepository.writeFile(t, filepath.Join(worktreePath, dirtyFileName), dirtyFileContents)

	result := testRepository.runGitWTFrom(t, worktreePath, "remove")
	if result.err == nil {
		t.Fatal("expected remove to fail for dirty worktree")
	}
	testRepository.assertPathPresent(t, worktreePath)
	testRepository.assertBranchPresent(t, branchName)
}

func TestRemoveWithNoArgsForceRemovesDirtyUnmergedWorktree(t *testing.T) {
	const branchName = "feature/force-current"
	const workFileName = "work.txt"
	const workFileContents = "change"
	const dirtyFileName = "dirty.txt"
	const dirtyFileContents = "dirty"

	testRepository := newTestRepository(t)
	testRepository.runGitWT(t, "create", branchName)
	worktreePath := testRepository.worktreePath(branchName)
	t.Chdir(worktreePath)
	testRepository.commitFileInWorktree(t, workFileName, workFileContents)
	testRepository.writeFile(t, dirtyFileName, dirtyFileContents)
	t.Chdir(testRepository.mainPath)

	result := testRepository.runGitWTFrom(t, worktreePath, "remove", "--force")
	if result.err != nil {
		t.Fatalf("force remove failed: %v\n%s", result.err, result.stderr)
	}

	testRepository.assertBranchMissing(t, branchName)
	testRepository.assertPathMissing(t, worktreePath)
}

func TestRemoveFailsWhenUnmergedWithoutForce(t *testing.T) {
	const branchName = "feature/unmerged"
	const workFileName = "work.txt"
	const workFileContents = "change"

	testRepository := newTestRepository(t)
	testRepository.runGitWT(t, "create", branchName)
	t.Chdir(testRepository.worktreePath(branchName))
	testRepository.commitFileInWorktree(t, workFileName, workFileContents)

	t.Chdir(testRepository.mainPath)
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
	t.Chdir(testRepository.worktreePath(branchName))
	testRepository.commitFileInWorktree(t, workFileName, workFileContents)
	testRepository.writeFile(t, dirtyFileName, dirtyFileContents)

	t.Chdir(testRepository.mainPath)
	result := testRepository.runGitWT(t, "remove", "--force", branchName)
	if result.err != nil {
		t.Fatalf("force remove failed: %v\n%s", result.err, result.stderr)
	}

	testRepository.assertBranchMissing(t, branchName)
	testRepository.assertPathMissing(t, testRepository.worktreePath(branchName))
}

func TestRemoveCompletionOffersManagedWorktreeNames(t *testing.T) {
	const firstBranchName = "feature/alpha"
	const secondBranchName = "feature/beta"

	testRepository := newTestRepository(t)
	testRepository.runGitWT(t, "create", firstBranchName)
	testRepository.runGitWT(t, "create", secondBranchName)

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

	command := NewRemoveCommand()
	completions, directive := command.ValidArgsFunction(command, nil, "feature/")
	if directive != cobra.ShellCompDirectiveNoFileComp {
		t.Fatalf("expected no-file completion directive, got %v", directive)
	}
	if !slices.Contains(completions, firstBranchName) {
		t.Fatalf("missing completion for %q: %v", firstBranchName, completions)
	}
	if !slices.Contains(completions, secondBranchName) {
		t.Fatalf("missing completion for %q: %v", secondBranchName, completions)
	}
	if slices.Contains(completions, "main") {
		t.Fatalf("unexpected completion for main worktree: %v", completions)
	}
	filteredCompletions, _ := command.ValidArgsFunction(command, nil, "feature/al")
	if len(filteredCompletions) != 1 || filteredCompletions[0] != firstBranchName {
		t.Fatalf("expected filtered completion for %q, got %v", firstBranchName, filteredCompletions)
	}
}

func TestGenerateZshGeneratesWrapperFunctionAndCompletion(t *testing.T) {
	outDir := t.TempDir()
	const functionName = "wt"

	result := runGitWTCommand(t, "generate", "zsh", "--name", functionName, "--out", outDir)
	if result.err != nil {
		t.Fatalf("generate zsh failed: %v\n%s", result.err, result.stderr)
	}

	functionPath := filepath.Join(outDir, functionName)
	completionPath := filepath.Join(outDir, "_"+functionName)

	functionContent, err := os.ReadFile(functionPath)
	if err != nil {
		t.Fatalf("read function file: %v", err)
	}
	completionContent, err := os.ReadFile(completionPath)
	if err != nil {
		t.Fatalf("read completion file: %v", err)
	}

	functionText := string(functionContent)
	for _, want := range []string{
		functionName + "() {",
		"case \"$1\" in",
		"create)",
		"--no-cd)",
		"-r|--herdr)",
		"-R|--no-herdr)",
		"local no_cd=0 herdr=0 no_herdr=0",
		"${HERDR_ENV:-0} == 1 && ! no_herdr",
		"command git-wt create \"${forward[@]}\"",
		"switch)",
		"remove)",
		"command git-wt \"$@\"",
		"cd \"$main_dir\"",
		"cd \"$target_dir\"",
		"$root_dir/$name",
		"$root_dir/$arg",
		"local root_dir=${main_dir:h}",
		"git worktree list --porcelain",
		"Usage: " + functionName + " switch <worktree>",
	} {
		if !strings.Contains(functionText, want) {
			t.Fatalf("function missing %q:\n%s", want, functionText)
		}
	}
	if strings.Contains(functionText, "Usage: "+functionName+" <worktree>") {
		t.Fatalf("function still uses bare worktree usage:\n%s", functionText)
	}
	if strings.Contains(functionText, "${name//\\//.}") || strings.Contains(functionText, "${arg//\\//.}") {
		t.Fatalf("function still normalizes slashes in paths:\n%s", functionText)
	}
	if !strings.Contains(functionText, "-r|--herdr)\n                herdr=1\n                forward+=(\"$arg\")") {
		t.Fatalf("function does not make --herdr imply --no-cd:\n%s", functionText)
	}
	if !strings.Contains(functionText, "-R|--no-herdr)\n                no_herdr=1\n                forward+=(\"$arg\")") {
		t.Fatalf("function does not suppress automatic Herdr behavior:\n%s", functionText)
	}

	completionText := string(completionContent)
	for _, want := range []string{
		"#compdef " + functionName,
		"switch:Switch to a worktree",
		"remove:Remove a managed Git worktree",
		"create:Create a managed Git worktree",
		"case $words[2] in",
		"create)",
		"--no-cd[Create without changing directories]",
		"'(-r --herdr)'{-r,--herdr}'[Also create a Herdr workspace for the new worktree]'",
		"'(-R --no-herdr)'{-R,--no-herdr}'[Do not create a Herdr workspace]'",
		"'(-u --upstream)'{-u,--upstream}'[Upstream branch]:upstream branch:'",
		"switch|remove)",
		"worktrees=(main)",
		"worktree_path=${line#worktree }",
		"branch=${line#branch refs/heads/}",
		"$worktree_path\" == \"$root_dir/$branch",
	} {
		if !strings.Contains(completionText, want) {
			t.Fatalf("completion missing %q:\n%s", want, completionText)
		}
	}
}

func TestGenerateZshRefusesOverwriteWithoutForce(t *testing.T) {
	outDir := t.TempDir()
	const functionName = "wt"

	first := runGitWTCommand(t, "generate", "zsh", "--name", functionName, "--out", outDir)
	if first.err != nil {
		t.Fatalf("first generate zsh failed: %v\n%s", first.err, first.stderr)
	}

	second := runGitWTCommand(t, "generate", "zsh", "--name", functionName, "--out", outDir)
	if second.err == nil {
		t.Fatal("expected second generate zsh without --force to fail")
	}

	forced := runGitWTCommand(t, "generate", "zsh", "--name", functionName, "--out", outDir, "--force")
	if forced.err != nil {
		t.Fatalf("generate zsh --force failed: %v\n%s", forced.err, forced.stderr)
	}
}

func TestPruneRemovesOnlyMergedCleanWorktrees(t *testing.T) {
	const mergedBranchName = "feature/merged"
	const unmergedBranchName = "feature/unmerged"
	const workFileName = "work.txt"
	const workFileContents = "change"

	testRepository := newTestRepository(t)
	t.Chdir(testRepository.mainPath)
	testRepository.commitFileInWorktree(t, workFileName, workFileContents)
	testRepository.runGitWT(t, "create", mergedBranchName)
	testRepository.runGitWT(t, "create", unmergedBranchName)
	t.Chdir(testRepository.worktreePath(unmergedBranchName))
	testRepository.commitFileInWorktree(t, workFileName, workFileContents)

	t.Chdir(testRepository.mainPath)
	result := testRepository.runGitWT(t, "prune")
	if result.err != nil {
		t.Fatalf("prune failed: %v\n%s", result.err, result.stderr)
	}

	testRepository.assertBranchMissing(t, mergedBranchName)
	testRepository.assertPathMissing(t, testRepository.worktreePath(mergedBranchName))
	testRepository.assertBranchPresent(t, unmergedBranchName)
	testRepository.assertPathPresent(t, testRepository.worktreePath(unmergedBranchName))
}

func TestListSucceedsWhenUpstreamRefIsMissing(t *testing.T) {
	const branchName = "feature/missing-upstream"

	testRepository := newTestRepository(t)
	createResult := testRepository.runGitWT(t, "create", branchName)
	if createResult.err != nil {
		t.Fatalf("create failed: %v\n%s", createResult.err, createResult.stderr)
	}

	runGitCommand(t, testRepository.mainPath, "update-ref", "-d", "refs/remotes/origin/main")

	listResult := testRepository.runGitWT(t, "list")
	if listResult.err != nil {
		t.Fatalf("list failed: %v\n%s", listResult.err, listResult.stderr)
	}
	if !strings.Contains(listResult.stdout, branchName) {
		t.Fatalf("list output missing worktree name: %s", listResult.stdout)
	}
}

func TestPruneKeepsWorktreeWhenUpstreamRefIsMissing(t *testing.T) {
	const branchName = "feature/missing-upstream"

	testRepository := newTestRepository(t)
	createResult := testRepository.runGitWT(t, "create", branchName)
	if createResult.err != nil {
		t.Fatalf("create failed: %v\n%s", createResult.err, createResult.stderr)
	}

	runGitCommand(t, testRepository.mainPath, "update-ref", "-d", "refs/remotes/origin/main")

	pruneResult := testRepository.runGitWT(t, "prune")
	if pruneResult.err != nil {
		t.Fatalf("prune failed: %v\n%s", pruneResult.err, pruneResult.stderr)
	}

	testRepository.assertBranchPresent(t, branchName)
	testRepository.assertPathPresent(t, testRepository.worktreePath(branchName))
}

func TestRemovePreservesReferenceLikeBranchNames(t *testing.T) {
	const ordinaryBranchName = "topic"
	const referenceLikeBranchName = "refs/remotes/topic"

	testRepository := newTestRepository(t)
	testRepository.createLocalBranch(t, ordinaryBranchName)
	testRepository.createLocalBranch(t, referenceLikeBranchName)

	createResult := testRepository.runGitWT(t, "create", referenceLikeBranchName)
	if createResult.err != nil {
		t.Fatalf("create failed: %v\n%s", createResult.err, createResult.stderr)
	}

	listResult := testRepository.runGitWT(t, "list")
	if !strings.Contains(listResult.stdout, referenceLikeBranchName) {
		t.Fatalf("list output missing branch %q: %s", referenceLikeBranchName, listResult.stdout)
	}

	removeResult := testRepository.runGitWT(t, "remove", referenceLikeBranchName)
	if removeResult.err != nil {
		t.Fatalf("remove failed: %v\n%s", removeResult.err, removeResult.stderr)
	}

	testRepository.assertBranchMissing(t, referenceLikeBranchName)
	testRepository.assertBranchPresent(t, ordinaryBranchName)
}

func TestListSupportsLocalUpstream(t *testing.T) {
	const branchName = "feature/local-upstream"

	testRepository := newTestRepository(t)
	testRepository.runGitWT(t, "create", branchName)
	runGitCommand(t, testRepository.mainPath, "config", "branch."+branchName+".remote", ".")
	runGitCommand(t, testRepository.mainPath, "config", "branch."+branchName+".merge", "refs/heads/main")

	result := testRepository.runGitWT(t, "list")
	if result.err != nil {
		t.Fatalf("list failed: %v\n%s", result.err, result.stderr)
	}
}

func TestListSupportsCustomRemoteUpstream(t *testing.T) {
	const branchName = "feature/custom-remote"
	const customRemote = "upstream"

	testRepository := newTestRepository(t)
	testRepository.runGitWT(t, "create", branchName)
	runGitCommand(t, testRepository.mainPath, "remote", "add", customRemote, testRepository.remotePath)
	runGitCommand(t, testRepository.mainPath, "fetch", customRemote)
	runGitCommand(t, testRepository.mainPath, "config", "branch."+branchName+".remote", customRemote)
	runGitCommand(t, testRepository.mainPath, "config", "branch."+branchName+".merge", "refs/heads/main")

	result := testRepository.runGitWT(t, "list")
	if result.err != nil {
		t.Fatalf("list failed: %v\n%s", result.err, result.stderr)
	}
}

func TestListFailsWhenTrackingConfigurationDoesNotMapToFetchRefspec(t *testing.T) {
	const branchName = "feature/unmapped-upstream"

	testCases := []struct {
		name   string
		remote string
		setup  func(testRepository)
	}{
		{
			name:   "missing remote",
			remote: "missing",
		},
		{
			name:   "unmapped fetch refspec",
			remote: "upstream",
			setup: func(testRepository testRepository) {
				runGitCommand(t, testRepository.mainPath, "remote", "add", "upstream", testRepository.remotePath)
				runGitCommand(t, testRepository.mainPath, "config", "remote.upstream.fetch", "+refs/changes/*:refs/remotes/upstream/changes/*")
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			testRepository := newTestRepository(t)
			if testCase.setup != nil {
				testCase.setup(testRepository)
			}

			createResult := testRepository.runGitWT(t, "create", branchName)
			if createResult.err != nil {
				t.Fatalf("create failed: %v\n%s", createResult.err, createResult.stderr)
			}
			runGitCommand(t, testRepository.mainPath, "config", "branch."+branchName+".remote", testCase.remote)
			runGitCommand(t, testRepository.mainPath, "config", "branch."+branchName+".merge", "refs/heads/main")

			listResult := testRepository.runGitWT(t, "list")
			if listResult.err == nil {
				t.Fatal("list succeeded with an unmapped upstream")
			}
			if !strings.Contains(listResult.err.Error(), "does not map to a known fetch refspec") {
				t.Fatalf("list error = %q, want unmapped upstream error", listResult.err)
			}
		})
	}
}

func TestListFailsWhenBranchHasNoUpstream(t *testing.T) {
	const branchName = "feature/no-upstream"

	testRepository := newTestRepository(t)
	testRepository.createLocalBranch(t, branchName)
	testRepository.runGitWT(t, "migrate")

	result := testRepository.runGitWT(t, "list")
	if result.err == nil {
		t.Fatal("list succeeded for a branch without an upstream")
	}
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
	t.Chdir(testRepository.worktreePath(branchName))
	testRepository.commitFileInWorktree(t, workFileName, workFileContents)

	t.Chdir(testRepository.mainPath)
	testRepository.runGitWT(t, "prune")
	testRepository.assertBranchPresent(t, branchName)
	testRepository.assertPathPresent(t, testRepository.worktreePath(branchName))

	command := &cobra.Command{}
	command.SetIn(bytes.NewBuffer(nil))
	var stderr bytes.Buffer
	command.SetErr(&stderr)
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

func TestMigrateRenamesExistingUnmanagedWorktrees(t *testing.T) {
	const branchOne = "feature/alpha"
	const branchTwo = "feature/beta"

	testRepository := newTestRepository(t)
	legacyPathOne := filepath.Join(testRepository.rootPath, "legacy-alpha")
	legacyPathTwo := filepath.Join(testRepository.rootPath, "legacy-beta")

	testRepository.createLocalBranch(t, branchOne)
	testRepository.createLocalBranch(t, branchTwo)
	runGitCommand(t, testRepository.mainPath, "worktree", "add", legacyPathOne, branchOne)
	runGitCommand(t, testRepository.mainPath, "worktree", "add", legacyPathTwo, branchTwo)

	result := testRepository.runGitWT(t, "migrate")
	if result.err != nil {
		t.Fatalf("migrate failed: %v\n%s", result.err, result.stderr)
	}

	testRepository.assertPathMissing(t, legacyPathOne)
	testRepository.assertPathMissing(t, legacyPathTwo)
	testRepository.assertPathPresent(t, testRepository.worktreePath(branchOne))
	testRepository.assertPathPresent(t, testRepository.worktreePath(branchTwo))
	assertCurrentBranchAtPath(t, testRepository.worktreePath(branchOne), branchOne)
	assertCurrentBranchAtPath(t, testRepository.worktreePath(branchTwo), branchTwo)
	testRepository.assertPathPresent(t, testRepository.mainPath)
	assertCurrentBranchAtPath(t, testRepository.mainPath, "main")
}

func TestMigrateCreatesWorktreesForExistingBranches(t *testing.T) {
	const branchOne = "feature/alpha"
	const branchTwo = "feature/beta"

	testRepository := newTestRepository(t)
	testRepository.createLocalBranch(t, branchOne)
	testRepository.createLocalBranch(t, branchTwo)

	result := testRepository.runGitWT(t, "migrate")
	if result.err != nil {
		t.Fatalf("migrate failed: %v\n%s", result.err, result.stderr)
	}

	testRepository.assertPathPresent(t, testRepository.worktreePath(branchOne))
	testRepository.assertPathPresent(t, testRepository.worktreePath(branchTwo))
	assertCurrentBranchAtPath(t, testRepository.worktreePath(branchOne), branchOne)
	assertCurrentBranchAtPath(t, testRepository.worktreePath(branchTwo), branchTwo)
	testRepository.assertPathPresent(t, testRepository.mainPath)
	assertCurrentBranchAtPath(t, testRepository.mainPath, "main")
}

func TestMigratePromptCanSkipSelectedWorktrees(t *testing.T) {
	const selectedBranch = "feature/selected"
	const skippedBranch = "feature/skipped"

	testRepository := newTestRepository(t)
	selectedLegacyPath := filepath.Join(testRepository.rootPath, "legacy-selected")
	skippedLegacyPath := filepath.Join(testRepository.rootPath, "legacy-skipped")

	testRepository.createLocalBranch(t, selectedBranch)
	testRepository.createLocalBranch(t, skippedBranch)
	runGitCommand(t, testRepository.mainPath, "worktree", "add", selectedLegacyPath, selectedBranch)
	runGitCommand(t, testRepository.mainPath, "worktree", "add", skippedLegacyPath, skippedBranch)

	command := &cobra.Command{}
	command.SetIn(bytes.NewBuffer(nil))
	var stderr bytes.Buffer
	command.SetErr(&stderr)
	t.Chdir(testRepository.mainPath)

	options := &migrateCommandOptions{
		prompt: true,
		prompter: stubMigratePrompter{selected: []migrateCandidate{{
			Name:        selectedBranch,
			CurrentPath: selectedLegacyPath,
			TargetPath:  testRepository.worktreePath(selectedBranch),
		}}},
	}

	if err := options.Execute(command, nil); err != nil {
		t.Fatalf("prompt migrate failed: %v\n%s", err, stderr.String())
	}

	testRepository.assertPathMissing(t, selectedLegacyPath)
	testRepository.assertPathPresent(t, testRepository.worktreePath(selectedBranch))
	testRepository.assertPathPresent(t, skippedLegacyPath)
	testRepository.assertPathMissing(t, testRepository.worktreePath(skippedBranch))
	testRepository.assertPathPresent(t, testRepository.mainPath)
	assertCurrentBranchAtPath(t, testRepository.mainPath, "main")
}

type testRepository struct {
	rootPath   string
	mainPath   string
	remotePath string
}

func newTestRepository(t *testing.T) testRepository {
	t.Helper()
	t.Setenv("HERDR_ENV", "")

	rootPath := t.TempDir()
	remotePath := filepath.Join(rootPath, "remote.git")
	mainPath := filepath.Join(rootPath, "main")

	runGitCommand(t, rootPath, "init", "--bare", remotePath)
	runGitCommand(t, rootPath, "init", "--initial-branch=main", mainPath)
	runGitCommand(t, mainPath, "config", "user.name", "Test User")
	runGitCommand(t, mainPath, "config", "user.email", "test@example.com")
	runGitCommand(t, mainPath, "remote", "add", remoteName, remotePath)

	filePath := filepath.Join(mainPath, "README.md")
	if err := os.WriteFile(filePath, []byte("initial\n"), 0o644); err != nil {
		t.Fatalf("write %s: %v", filePath, err)
	}

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

func (x testRepository) createLocalBranch(t *testing.T, branchName string) {
	t.Helper()
	runGitCommand(t, x.mainPath, "branch", branchName, "main")
	if _, err := os.Stat(x.worktreePath(branchName)); err == nil {
		t.Fatalf("expected worktree path %s to be unused", x.worktreePath(branchName))
	}
}

func (x testRepository) runGitWT(t *testing.T, args ...string) commandResult {
	t.Helper()
	return x.runGitWTFrom(t, x.mainPath, args...)
}

func (x testRepository) runGitWTFrom(t *testing.T, directory string, args ...string) commandResult {
	t.Helper()

	currentDirectory, err := os.Getwd()
	if err != nil {
		t.Fatalf("get current directory: %v", err)
	}
	if err := os.Chdir(directory); err != nil {
		t.Fatalf("change directory: %v", err)
	}
	defer func() {
		if err := os.Chdir(currentDirectory); err != nil {
			t.Fatalf("restore directory: %v", err)
		}
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

func (x testRepository) commitFileInWorktree(t *testing.T, fileName string, contents string) {
	t.Helper()
	x.writeFile(t, fileName, contents)
	runGitCommand(t, "", "add", fileName)
	runGitCommand(t, "", "commit", "-m", "change")
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

func assertCurrentBranchAtPath(t *testing.T, path string, branchName string) {
	t.Helper()
	currentBranch := strings.TrimSpace(runGitCommand(t, path, "branch", "--show-current"))
	if currentBranch != branchName {
		t.Fatalf("expected current branch at %s to be %s, not %s", path, branchName, currentBranch)
	}
}

func assertCurrentBranch(t *testing.T, branchName string) {
	t.Helper()
	currentBranch := strings.TrimSpace(runGitCommand(t, "", "branch", "--show-current"))
	if branchName != currentBranch {
		t.Fatalf("expected current branch to be %s, not %v", branchName, currentBranch)
	}
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

func (x testRepository) readFile(t *testing.T, path string) string {
	t.Helper()
	bs, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	return string(bs)
}

func (x testRepository) writeFile(t *testing.T, path string, contents string) {
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
