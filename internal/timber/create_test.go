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

func TestCreateFetchesOriginBeforeCreatingWorktree(t *testing.T) {
	t.Parallel()
	const branchName = "feature/fresh"

	testRepository := newTestRepository(t)
	updaterPath := filepath.Join(t.TempDir(), "updater")
	runGitCommand(t, filepath.Dir(updaterPath), "clone", testRepository.remotePath, updaterPath)
	configureGitUser(t, updaterPath)
	require.NoError(t, os.WriteFile(filepath.Join(updaterPath, "fresh.txt"), []byte("fresh\n"), 0o644))
	runGitCommand(t, updaterPath, "add", "fresh.txt")
	runGitCommand(t, updaterPath, "commit", "-m", "advance remote")
	runGitCommand(t, updaterPath, "push", remoteName, "main")
	remoteCommit := strings.TrimSpace(runGitCommand(t, updaterPath, "rev-parse", "HEAD"))

	result := testRepository.runTimber(t, "create", at(testRepoName, branchName))
	require.NoError(t, result.err, result.stderr)
	worktreeCommit := strings.TrimSpace(runGitCommand(t, testRepository.worktreePath(branchName), "rev-parse", "HEAD"))
	assert.Equal(t, remoteCommit, worktreeCommit)
}

func TestCreateUsesOriginHeadAsDefaultUpstream(t *testing.T) {
	t.Parallel()
	testRepository := newTestRepository(t)
	runGitCommand(t, testRepository.barePath, "branch", "develop", "main")
	runGitCommand(t, testRepository.barePath, "push", remoteName, "develop")
	runGitCommand(t, testRepository.barePath, "symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/develop")

	result := testRepository.runTimber(t, "create", at(testRepoName, "feature/from-develop"))
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
	t.Parallel()
	testRepository := newTestRepository(t)
	runGitCommand(t, testRepository.barePath, "branch", "-M", "main", "master")
	runGitCommand(t, testRepository.barePath, "push", remoteName, "master")
	runGitCommand(t, testRepository.remotePath, "symbolic-ref", "HEAD", "refs/heads/missing")
	// Ensure origin/HEAD missing
	runGitCommandAllowError(t, testRepository.barePath, "symbolic-ref", "-d", "refs/remotes/origin/HEAD")
	runGitCommandAllowError(t, testRepository.barePath, "update-ref", "-d", "refs/remotes/origin/main")

	result := testRepository.runTimber(t, "create", at(testRepoName, "feature/from-master"))
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
	t.Parallel()
	testRepository := newTestRepository(t)
	runGitCommand(t, testRepository.remotePath, "symbolic-ref", "HEAD", "refs/heads/missing")
	runGitCommand(t, testRepository.barePath, "update-ref", "refs/remotes/origin/main", "refs/heads/main")
	runGitCommandAllowError(t, testRepository.barePath, "symbolic-ref", "-d", "refs/remotes/origin/HEAD")
	runGitCommandAllowError(t, testRepository.barePath, "update-ref", "-d", "refs/remotes/origin/master")

	result := testRepository.runTimber(t, "create", at(testRepoName, "feature/from-main"))
	require.NoError(t, result.err, result.stderr)
}

func TestCreateFailsWhenOriginHeadAndCommonDefaultsAreMissing(t *testing.T) {
	t.Parallel()
	testRepository := newTestRepository(t)
	runGitCommand(t, testRepository.barePath, "branch", "develop", "main")
	runGitCommand(t, testRepository.barePath, "push", remoteName, "develop")
	runGitCommand(t, testRepository.remotePath, "symbolic-ref", "HEAD", "refs/heads/missing")
	runGitCommand(t, testRepository.remotePath, "update-ref", "-d", "refs/heads/main")
	runGitCommandAllowError(t, testRepository.barePath, "symbolic-ref", "-d", "refs/remotes/origin/HEAD")
	runGitCommandAllowError(t, testRepository.barePath, "update-ref", "-d", "refs/remotes/origin/main")
	runGitCommandAllowError(t, testRepository.barePath, "update-ref", "-d", "refs/remotes/origin/master")
	runGitCommandAllowError(t, testRepository.barePath, "branch", "-D", "main")
	runGitCommandAllowError(t, testRepository.barePath, "branch", "-D", "master")

	result := testRepository.runTimber(t, "create", at(testRepoName, "feature/missing-default-upstream"))
	require.Error(t, result.err)
	assert.Contains(t, result.err.Error(), "resolve origin/HEAD")
}

func TestCreateWithHerdrOpensStandardHerdrSpace(t *testing.T) {
	t.Parallel()
	const branchName = "feature/herdr"

	testRepository := newTestRepository(t)
	logPath := filepath.Join(t.TempDir(), "herdr.log")
	testRepository.runtime.HerdrExecutable = installFakeHerdrSpace(t, logPath)

	result := testRepository.runTimber(t, "create", "--herdr", at(testRepoName, branchName))
	require.NoError(t, result.err, result.stderr)
	testRepository.assertPathPresent(t, testRepository.worktreePath(branchName))
	assert.Contains(t, result.stderr, "opened herdr space for "+branchName)

	worktreePath, err := filepath.Abs(testRepository.worktreePath(branchName))
	require.NoError(t, err)
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

func TestCreateWithoutHerdrDoesNotInvokeHerdr(t *testing.T) {
	t.Parallel()
	const branchName = "feature/no-herdr"

	testRepository := newTestRepository(t)
	logPath := filepath.Join(t.TempDir(), "herdr.log")
	testRepository.runtime.HerdrExecutable = installFakeHerdrSpace(t, logPath)

	result := testRepository.runTimber(t, "create", at(testRepoName, branchName))
	require.NoError(t, result.err, result.stderr)
	_, err := os.Stat(logPath)
	assert.ErrorIs(t, err, os.ErrNotExist)
}

func TestCreateInHerdrOpensStandardHerdrSpace(t *testing.T) {
	t.Parallel()
	const branchName = "feature/automatic-herdr"

	testRepository := newTestRepository(t)
	testRepository.runtime.HerdrEnvironment = true
	logPath := filepath.Join(t.TempDir(), "herdr.log")
	testRepository.runtime.HerdrExecutable = installFakeHerdrSpace(t, logPath)

	result := testRepository.runTimber(t, "create", at(testRepoName, branchName))
	require.NoError(t, result.err, result.stderr)
	assert.Len(t, readFakeHerdrLog(t, logPath), 7)
}

func TestCreateWithNoHerdrDoesNotInvokeHerdr(t *testing.T) {
	t.Parallel()
	const branchName = "feature/no-herdr-flag"

	testRepository := newTestRepository(t)
	testRepository.runtime.HerdrEnvironment = true
	logPath := filepath.Join(t.TempDir(), "herdr.log")
	testRepository.runtime.HerdrExecutable = installFakeHerdrSpace(t, logPath)

	result := testRepository.runTimber(t, "create", "--no-herdr", at(testRepoName, branchName))
	require.NoError(t, result.err, result.stderr)
	_, err := os.Stat(logPath)
	assert.ErrorIs(t, err, os.ErrNotExist)
}

func TestCreateRejectsHerdrAndNoHerdr(t *testing.T) {
	t.Parallel()
	result := runTimberCommand(t, "create", "--herdr", "--no-herdr", "feature/conflicting-herdr")
	require.Error(t, result.err)
	assert.Contains(t, result.err.Error(), "if any flags in the group [herdr no-herdr] are set none of the others can be")
}

func TestTUICreateCreatesSelectedWorktree(t *testing.T) {
	t.Parallel()
	const branchName = "feature/ui-create"

	testRepository := newTestRepository(t)
	prompter := &stubCreateWizardPrompter{
		selection: createWizardSelection{repoName: testRepoName, worktreeName: branchName},
	}
	options := &tuiCreateCommandOptions{runtime: testRepository.runtime}
	result := runTUICreate(t, options, prompter)

	require.NoError(t, result.err, result.stderr)
	testRepository.assertPathPresent(t, testRepository.worktreePath(branchName))
	assert.Contains(t, result.stdout, testRepository.worktreePath(branchName))
	require.Len(t, prompter.repos, 1)
	assert.Equal(t, testRepoName, prompter.repos[0].Name)
}

func TestTUICreateCancelDoesNotCreateWorktree(t *testing.T) {
	t.Parallel()
	const branchName = "feature/ui-cancel"

	testRepository := newTestRepository(t)
	options := &tuiCreateCommandOptions{runtime: testRepository.runtime}
	result := runTUICreate(t, options, &stubCreateWizardPrompter{
		selection: createWizardSelection{cancelled: true},
	})

	require.NoError(t, result.err, result.stderr)
	testRepository.assertPathMissing(t, testRepository.worktreePath(branchName))
	assert.Empty(t, result.stdout)
}

func TestTUICreateFailsWhenNoRepositoriesAreRegistered(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	options := &tuiCreateCommandOptions{runtime: testRuntimeForHome(home, home)}

	result := runTUICreate(t, options, &stubCreateWizardPrompter{})
	require.Error(t, result.err)
	assert.Contains(t, result.err.Error(), "no registered repositories")
}

func TestTUICreateRequiresInteractiveTerminal(t *testing.T) {
	t.Parallel()
	prompter := bubbleteaCreateWizardPrompter{interactive: func() bool { return false }}
	_, err := prompter.Prompt(bytes.NewBuffer(nil), io.Discard, []registeredRepo{{Name: testRepoName}}, nil, true)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "interactive terminal")
}

func TestTUICreateListsEveryRepositoryFromAManagedWorktree(t *testing.T) {
	t.Parallel()
	const currentBranch = "feature/current"
	const createdBranch = "topic/from-other"
	const secondaryName = "other"

	testRepository := newTestRepository(t)
	registerAdditionalRepo(t, testRepository, secondaryName)
	require.NoError(t, testRepository.runTimber(t, "create", at(testRepoName, currentBranch)).err)

	prompter := &stubCreateWizardPrompter{
		selection: createWizardSelection{repoName: secondaryName, worktreeName: createdBranch},
	}
	options := new(tuiCreateCommandOptions)
	options.runtime = testRepository.runtime
	options.runtime.CurrentDirectory = testRepository.worktreePath(currentBranch)
	result := runTUICreate(t, options, prompter)

	require.NoError(t, result.err, result.stderr)
	secondaryPath := filepath.Join(testRepository.worktreeRoot, secondaryName, createdBranch, secondaryName)
	_, statErr := os.Stat(secondaryPath)
	require.NoError(t, statErr)
	testRepository.assertPathMissing(t, testRepository.worktreePath(createdBranch))
	repoNames := make([]string, 0, len(prompter.repos))
	for _, repo := range prompter.repos {
		repoNames = append(repoNames, repo.Name)
	}
	assert.Equal(t, []string{secondaryName, testRepoName}, repoNames)
}

func TestTUICreateWithHerdrOpensStandardHerdrSpace(t *testing.T) {
	t.Parallel()
	const branchName = "feature/ui-herdr"

	testRepository := newTestRepository(t)
	logPath := filepath.Join(t.TempDir(), "herdr.log")
	testRepository.runtime.HerdrExecutable = installFakeHerdrSpace(t, logPath)

	options := new(tuiCreateCommandOptions)
	options.runtime = testRepository.runtime
	options.herdr = true
	result := runTUICreate(t, options, &stubCreateWizardPrompter{
		selection: createWizardSelection{repoName: testRepoName, worktreeName: branchName},
	})

	require.NoError(t, result.err, result.stderr)
	testRepository.assertPathPresent(t, testRepository.worktreePath(branchName))
	assert.Contains(t, result.stderr, "opened herdr space for "+branchName)
	assert.Len(t, readFakeHerdrLog(t, logPath), 7)
}

func TestTUICreateOpensSelectedWorktreeInHerdrSpace(t *testing.T) {
	t.Parallel()
	const branchName = "feature/ui-open"

	testRepository := newTestRepository(t)
	require.NoError(t, testRepository.runTimber(t, "create", at(testRepoName, branchName)).err)
	logPath := filepath.Join(t.TempDir(), "herdr.log")
	testRepository.runtime.HerdrExecutable = installFakeHerdrSpace(t, logPath)

	prompter := &stubCreateWizardPrompter{
		selection: createWizardSelection{
			action:       wizardActionOpen,
			repoName:     testRepoName,
			worktreeName: branchName,
		},
	}
	options := &tuiCreateCommandOptions{runtime: testRepository.runtime}
	result := runTUICreate(t, options, prompter)

	require.NoError(t, result.err, result.stderr)
	assert.Contains(t, result.stderr, "opened herdr space for "+branchName)
	assert.Len(t, readFakeHerdrLog(t, logPath), 7)
	require.Len(t, prompter.worktrees, 1)
	assert.Equal(t, testRepoName, prompter.worktrees[0].Repo)
	assert.Equal(t, branchName, prompter.worktrees[0].Name)
}

func TestTUICreateWithNoHerdrDoesNotInvokeHerdr(t *testing.T) {
	t.Parallel()
	const branchName = "feature/ui-no-herdr"

	testRepository := newTestRepository(t)
	testRepository.runtime.HerdrEnvironment = true
	logPath := filepath.Join(t.TempDir(), "herdr.log")
	testRepository.runtime.HerdrExecutable = installFakeHerdrSpace(t, logPath)

	options := new(tuiCreateCommandOptions)
	options.runtime = testRepository.runtime
	options.noHerdr = true
	result := runTUICreate(t, options, &stubCreateWizardPrompter{
		selection: createWizardSelection{repoName: testRepoName, worktreeName: branchName},
	})

	require.NoError(t, result.err, result.stderr)
	_, err := os.Stat(logPath)
	assert.ErrorIs(t, err, os.ErrNotExist)
}

func TestTUICreateRejectsHerdrAndNoHerdr(t *testing.T) {
	t.Parallel()
	result := runTimberCommand(t, "tui", "--herdr", "--no-herdr")
	require.Error(t, result.err)
	assert.Contains(t, result.err.Error(), "if any flags in the group [herdr no-herdr] are set none of the others can be")
}

func TestCreateWithHerdrKeepsWorktreeWhenHerdrFails(t *testing.T) {
	t.Parallel()
	const branchName = "feature/herdr-fail"

	testRepository := newTestRepository(t)
	logPath := filepath.Join(t.TempDir(), "herdr.log")
	testRepository.runtime.HerdrExecutable = installFakeHerdrSpace(t, logPath)
	testRepository.runtime = withTestEnvironment(testRepository.runtime, "FAKE_HERDR_FAIL=workspace create")

	result := testRepository.runTimber(t, "create", "--herdr", at(testRepoName, branchName))
	require.Error(t, result.err)
	testRepository.assertPathPresent(t, testRepository.worktreePath(branchName))
	assert.Contains(t, result.err.Error(), "herdr workspace create")
}

func TestCreateFailsWhenDirectoryExists(t *testing.T) {
	t.Parallel()
	const branchName = "feature/exists"

	testRepository := newTestRepository(t)
	path := testRepository.worktreePath(branchName)
	require.NoError(t, os.MkdirAll(path, 0o755))

	result := testRepository.runTimber(t, "create", at(testRepoName, branchName))
	require.Error(t, result.err)
	assert.Contains(t, result.err.Error(), "already exists")
}

func TestCreateRepairsBareRepoMissingOriginFetch(t *testing.T) {
	t.Parallel()
	testRepository := newTestRepository(t)

	// Simulate a bare clone that never got remote-tracking configured.
	runGitCommandAllowError(t, testRepository.barePath, "config", "--unset-all", "remote.origin.fetch")
	runGitCommandAllowError(t, testRepository.barePath, "symbolic-ref", "-d", "refs/remotes/origin/HEAD")
	runGitCommandAllowError(t, testRepository.barePath, "update-ref", "-d", "refs/remotes/origin/main")

	result := testRepository.runTimber(t, "create", at(testRepoName, "feature/repaired-upstream"))
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
	t.Parallel()
	testRepository := newTestRepository(t)
	pathFile := filepath.Join(t.TempDir(), "created-path")
	runtime := testRepository.runtime
	runtime.CreatePathFile = pathFile

	result := runTimberCommandWithRuntime(t, runtime, "create", at(testRepoName, "feature/path-file"))
	require.NoError(t, result.err, result.stderr)
	assert.Empty(t, strings.TrimSpace(result.stdout))

	contents, err := os.ReadFile(pathFile)
	require.NoError(t, err)
	assert.Equal(t, testRepository.worktreePath("feature/path-file")+"\n", string(contents))
}

func TestCreateRequiresRepoOutsideInteractive(t *testing.T) {
	t.Parallel()
	testRepository := newTestRepository(t)
	result := testRepository.runTimber(t, "create", "feature/needs-repo")
	require.Error(t, result.err)
	assert.Contains(t, result.err.Error(), "repository selection requires")
}

func TestCreateAutoDetectsRepoFromManagedWorktree(t *testing.T) {
	t.Parallel()
	const existing = "feature/base"
	const branchName = "feature/from-current"

	testRepository := newTestRepository(t)
	require.NoError(t, testRepository.runTimber(t, "create", at(testRepoName, existing)).err)

	result := testRepository.runTimberFrom(t, testRepository.worktreePath(existing), "create", branchName)
	require.NoError(t, result.err, result.stderr)
	testRepository.assertPathPresent(t, testRepository.worktreePath(branchName))
}

func TestCreateAcceptsRepoQualifier(t *testing.T) {
	t.Parallel()
	const branchName = "feature/qualified"

	testRepository := newTestRepository(t)
	result := testRepository.runTimber(t, "create", at(testRepoName, branchName))
	require.NoError(t, result.err, result.stderr)
	testRepository.assertPathPresent(t, testRepository.worktreePath(branchName))
}

func TestCreateRejectsAtInWorktreeName(t *testing.T) {
	t.Parallel()
	testRepository := newTestRepository(t)
	result := testRepository.runTimber(t, "create", "foo@bar@"+testRepoName)
	require.Error(t, result.err)
	assert.Contains(t, result.err.Error(), "must not contain @")
}

func TestCreateRejectsUnknownRepoQualifier(t *testing.T) {
	t.Parallel()
	testRepository := newTestRepository(t)
	result := testRepository.runTimber(t, "create", "feature/login@unknown")
	require.Error(t, result.err)
	assert.Contains(t, result.err.Error(), "unknown repository")
}
