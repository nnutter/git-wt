package timber

import (
	"bytes"
	"cmp"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"github.com/spf13/cobra"
)

const (
	createPathFileEnvVarName = "TIMBER_CREATE_PATH_FILE"
	reposDirName             = "timber/repos"
	worktreesDirName         = "worktrees"
	worktreeRootEnvVarName   = "TIMBER_WORKTREE_ROOT"
)

// Runtime contains the process state and operating-system settings used by a
// timber command. A Runtime is captured once at the CLI boundary and should
// be treated as immutable for the lifetime of a command.
type Runtime struct {
	CurrentDirectory string

	HomeDirectory      string
	DataHome           string
	ConfigHome         string
	WorktreeRoot       string
	TemporaryDirectory string

	HerdrEnvironment bool

	CreatePathFile string
	SwitchPathFile string
	RenamePathFile string

	// Environment is passed to child processes such as git and herdr.
	Environment []string

	// HerdrExecutable overrides the executable used for Herdr commands. It is
	// primarily useful for tests; an empty value uses PATH lookup.
	HerdrExecutable string

	// TrashExecutable overrides the executable used to move removed paths to
	// the system trash. It is primarily useful for tests; an empty value uses
	// PATH lookup of "trash".
	TrashExecutable string
}

// RuntimeFromProcess captures the process state needed by a timber command.
func RuntimeFromProcess() (Runtime, error) {
	currentDirectory, err := os.Getwd()
	if err != nil {
		return Runtime{}, fmt.Errorf("get current directory: %w", err)
	}

	homeDirectory, err := os.UserHomeDir()
	if err != nil {
		return Runtime{}, fmt.Errorf("resolve home directory: %w", err)
	}

	return Runtime{
		CurrentDirectory:   currentDirectory,
		HomeDirectory:      homeDirectory,
		DataHome:           cmp.Or(os.Getenv("XDG_DATA_HOME"), filepath.Join(homeDirectory, ".local", "share")),
		ConfigHome:         cmp.Or(os.Getenv("XDG_CONFIG_HOME"), filepath.Join(homeDirectory, ".config")),
		WorktreeRoot:       cmp.Or(os.Getenv(worktreeRootEnvVarName), filepath.Join(homeDirectory, worktreesDirName)),
		TemporaryDirectory: os.TempDir(),
		HerdrEnvironment:   os.Getenv("HERDR_ENV") == "1",
		CreatePathFile:     os.Getenv(createPathFileEnvVarName),
		SwitchPathFile:     os.Getenv(switchPathFileEnvVarName),
		RenamePathFile:     os.Getenv(repoRenamePathFileEnvVarName),
		Environment:        slices.Clone(os.Environ()),
	}, nil
}

func (x Runtime) command(ctx context.Context, name string, args ...string) *exec.Cmd {
	if name == "herdr" && x.HerdrExecutable != "" {
		name = x.HerdrExecutable
	}

	command := exec.CommandContext(ctx, name, args...)
	if x.Environment != nil {
		command.Env = slices.Clone(x.Environment)
	}
	return command
}

func (x Runtime) xdgDataHome() string {
	return x.DataHome
}

func (x Runtime) xdgConfigHome() string {
	return x.ConfigHome
}

func (x Runtime) reposDirectory() string {
	return filepath.Join(x.xdgDataHome(), reposDirName)
}

func (x Runtime) worktreeRoot() string {
	return x.WorktreeRoot
}

func (x Runtime) bareRepoPath(repoName string) string {
	return filepath.Join(x.reposDirectory(), repoName+bareRepoSuffix)
}

// managedWorktreePath returns
// <worktree-root>/<repo-name>/<worktree-name>/<repo-name>.
func (x Runtime) managedWorktreePath(repoName string, worktreeName string) string {
	return filepath.Join(x.worktreeRoot(), repoName, worktreeName, repoName)
}

func (x Runtime) temporaryPath(pattern string) (string, error) {
	return os.MkdirTemp(x.TemporaryDirectory, pattern)
}

func (x Runtime) absolutePath(path string) (string, error) {
	if filepath.IsAbs(path) {
		return filepath.Clean(path), nil
	}
	if x.CurrentDirectory == "" {
		return "", fmt.Errorf("current directory is not set")
	}
	return filepath.Abs(filepath.Join(x.CurrentDirectory, path))
}

func (x Runtime) resolveCreateWorktreeName(repoName string, name string) (string, error) {
	if name != "" {
		return name, nil
	}
	return x.unusedWorktreeName(repoName)
}

func (x Runtime) reportCreatedWorktreePath(command *cobra.Command, worktreePath string) error {
	if pathFile := x.CreatePathFile; pathFile != "" {
		if err := x.writePathFile(pathFile, worktreePath); err != nil {
			return fmt.Errorf("write created worktree path file: %w", err)
		}
		return nil
	}

	_, err := fmt.Fprintln(command.OutOrStdout(), worktreePath)
	return err
}

// displayHomePath replaces a leading home directory with "~" for display.
func (x Runtime) displayHomePath(path string) string {
	home := x.HomeDirectory
	if home == "" {
		return path
	}

	cleanPath := filepath.Clean(path)
	cleanHome := filepath.Clean(home)
	if cleanPath == cleanHome {
		return "~"
	}
	prefix := cleanHome + string(filepath.Separator)
	if relative, ok := strings.CutPrefix(cleanPath, prefix); ok {
		return "~" + string(filepath.Separator) + relative
	}
	return path
}

func (x Runtime) openHerdrSpace(ctx context.Context, worktree managedWorktree) (returnErr error) {
	space, returnErr := x.createHerdrSpace(ctx, worktree)
	if space.workspaceID == "" {
		return returnErr
	}

	defer func() {
		if returnErr != nil {
			returnErr = errors.Join(returnErr, space.close(context.WithoutCancel(ctx)))
		}
	}()

	if returnErr != nil {
		return returnErr
	}
	if returnErr = space.configure(ctx); returnErr != nil {
		return returnErr
	}
	return space.focus(ctx)
}

func (x Runtime) defineCurrentHerdrSpace(ctx context.Context, worktree managedWorktree) error {
	space, err := x.currentHerdrSpace(ctx, worktree)
	if err != nil {
		return err
	}
	if err := space.configure(ctx); err != nil {
		return err
	}
	return space.focus(ctx)
}

func (x Runtime) createHerdrSpace(ctx context.Context, worktree managedWorktree) (herdrSpace, error) {
	absolutePath, err := x.herdrWorktreePath(worktree)
	if err != nil {
		return herdrSpace{}, err
	}

	output, err := x.runHerdr(
		ctx,
		"workspace", "create", "--cwd", absolutePath,
		"--label", worktree.Repo, "--no-focus",
	)
	if err != nil {
		return herdrSpace{}, err
	}

	return parseHerdrSpace(x, output, absolutePath, worktree.Name)
}

func (x Runtime) currentHerdrSpace(ctx context.Context, worktree managedWorktree) (herdrSpace, error) {
	absolutePath, err := x.herdrWorktreePath(worktree)
	if err != nil {
		return herdrSpace{}, err
	}

	output, err := x.runHerdr(ctx, "pane", "current", "--current")
	if err != nil {
		return herdrSpace{}, err
	}
	return parseCurrentHerdrSpace(x, output, absolutePath, worktree.Name)
}

func (x Runtime) herdrWorktreePath(worktree managedWorktree) (string, error) {
	absolutePath, err := x.absolutePath(worktree.Path)
	if err != nil {
		return "", fmt.Errorf("resolve worktree path for herdr: %w", err)
	}
	return absolutePath, nil
}

func (x Runtime) runHerdr(ctx context.Context, args ...string) ([]byte, error) {
	command := x.command(ctx, "herdr", args...)
	output, err := command.CombinedOutput()
	if err == nil {
		return output, nil
	}

	operation := strings.Join(args[:min(2, len(args))], " ")
	message := strings.TrimSpace(string(output))
	if message == "" {
		return nil, fmt.Errorf("herdr %s: %w", operation, err)
	}
	return nil, fmt.Errorf("herdr %s: %w: %s", operation, err, message)
}

func (x Runtime) herdrPluginInstallDirectory() string {
	return filepath.Join(x.xdgConfigHome(), "herdr", "plugins", herdrPluginDirectoryName)
}

func (x Runtime) herdrConfigFilePath() string {
	return filepath.Join(x.xdgConfigHome(), "herdr", "config.toml")
}

func (x Runtime) reportHerdrPluginInstall(command *cobra.Command, destination string) error {
	status := command.ErrOrStderr()
	if _, err := fmt.Fprintf(status, "%s\n", statusStyle.Render("installed herdr plugin to "+destination)); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(status, "%s\n", statusStyle.Render("linked herdr plugin "+herdrPluginID)); err != nil {
		return err
	}

	_, err := fmt.Fprintf(
		command.OutOrStdout(),
		"Add this keybinding to %s:\n\n%s",
		x.herdrConfigFilePath(),
		herdrKeybindingTOML,
	)
	return err
}

func (x Runtime) removeEmptySourceParents(path string) error {
	return removeEmptyParents(path, x.HomeDirectory)
}

func (x Runtime) applyMigrationCandidate(repository *Repository, candidate migrateCandidate) (retErr error) {
	currentPath := filepath.Clean(candidate.CurrentPath)
	targetPath := filepath.Clean(candidate.TargetPath)

	stagingDirectory, err := x.temporaryPath("timber-migrate-")
	if err != nil {
		return fmt.Errorf("create migration staging directory: %w", err)
	}
	defer func() {
		if err := os.RemoveAll(stagingDirectory); retErr == nil {
			retErr = err
		}
	}()

	if err := copyDirectoryContents(currentPath, stagingDirectory, ".git"); err != nil {
		return fmt.Errorf("stage worktree %q: %w", currentPath, err)
	}

	// Remove the old worktree path so git worktree add can create targetPath.
	if err := os.RemoveAll(currentPath); err != nil {
		return fmt.Errorf("remove old worktree %q: %w", currentPath, err)
	}
	if err := x.removeEmptySourceParents(currentPath); err != nil {
		return err
	}
	if currentPath != targetPath {
		if _, err := os.Stat(targetPath); err == nil {
			return fmt.Errorf("worktree directory %q already exists", targetPath)
		}
	}

	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		return fmt.Errorf("create worktree parent directory %q: %w", filepath.Dir(targetPath), err)
	}

	if _, err := repository.git("worktree", "add", targetPath, candidate.BranchName); err != nil {
		return err
	}

	// Restore local modifications over the clean checkout.
	if err := copyDirectoryContents(stagingDirectory, targetPath, ".git"); err != nil {
		return fmt.Errorf("restore worktree contents to %q: %w", targetPath, err)
	}

	if err := ensureBranchUpstream(repository, candidate.BranchName); err != nil {
		return err
	}
	return nil
}

func (x Runtime) moveLinkedWorktree(repository *Repository, candidate migrateCandidate) error {
	currentPath := filepath.Clean(candidate.CurrentPath)
	targetPath := filepath.Clean(candidate.TargetPath)
	if currentPath == targetPath {
		return nil
	}

	sourcePath := currentPath
	if pathIsWithin(currentPath, targetPath) {
		stagingPath, err := unusedTempPathIn(x.worktreeRoot(), "timber-migrate-")
		if err != nil {
			return err
		}
		if _, err := repository.git("worktree", "move", currentPath, stagingPath); err != nil {
			return err
		}
		sourcePath = stagingPath
	}

	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		return fmt.Errorf("create worktree parent directory %q: %w", filepath.Dir(targetPath), err)
	}
	if _, err := os.Stat(targetPath); err == nil {
		return fmt.Errorf("worktree directory %q already exists", targetPath)
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect worktree directory %q: %w", targetPath, err)
	}

	if _, err := repository.git("worktree", "move", sourcePath, targetPath); err != nil {
		return err
	}
	return x.removeEmptySourceParents(currentPath)
}

func (x Runtime) writePathFile(pathFile string, value string) (err error) {
	temporaryDirectory, err := x.absolutePath(x.TemporaryDirectory)
	if err != nil {
		return fmt.Errorf("resolve temporary directory: %w", err)
	}
	pathFile, err = x.absolutePath(pathFile)
	if err != nil {
		return fmt.Errorf("resolve path file: %w", err)
	}

	relativePath, err := filepath.Rel(temporaryDirectory, pathFile)
	if err != nil {
		return fmt.Errorf("relate path file to temporary directory: %w", err)
	}
	if relativePath == ".." || strings.HasPrefix(relativePath, ".."+string(filepath.Separator)) {
		return fmt.Errorf("path file %q is outside temporary directory", pathFile)
	}

	root, err := os.OpenRoot(temporaryDirectory)
	if err != nil {
		return fmt.Errorf("open temporary directory: %w", err)
	}
	defer func() {
		if closeErr := root.Close(); err == nil {
			err = closeErr
		}
	}()

	return root.WriteFile(relativePath, []byte(value+"\n"), 0o600)
}

func (x Runtime) reportPruneDryRun(command *cobra.Command, worktrees []managedWorktree) error {
	for _, worktree := range worktrees {
		message := fmt.Sprintf(
			"would prune %s (%s) at %s",
			worktree.Name,
			worktree.Repo,
			x.displayHomePath(worktree.Path),
		)
		if _, err := fmt.Fprintf(command.ErrOrStderr(), "%s\n", statusStyle.Render(message)); err != nil {
			return err
		}
	}
	return nil
}

func (x Runtime) parseQualifiedName(raw string) (qualifiedName, error) {
	name, repo, found := strings.CutLast(raw, "@")
	if !found {
		return qualifiedName{Name: raw}, nil
	}
	if repo == "" {
		return qualifiedName{}, fmt.Errorf("missing repository name after @")
	}
	if _, err := x.registeredRepoByName(repo); err != nil {
		return qualifiedName{}, err
	}
	return qualifiedName{Name: name, Repo: repo}, nil
}

func (x Runtime) parseRepoOnlyArg(raw string) (string, error) {
	qualified, err := x.parseQualifiedName(raw)
	if err != nil {
		return "", err
	}
	if qualified.Name != "" || qualified.Repo == "" {
		return "", fmt.Errorf("expected @<repo>, got %q", raw)
	}
	return qualified.Repo, nil
}

func (x Runtime) inferUniqueRepoForWorktree(worktreeName string) (string, error) {
	repos, err := x.listRegisteredRepos()
	if err != nil {
		return "", err
	}

	var matches []string
	for _, repo := range repos {
		worktreePath := x.managedWorktreePath(repo.Name, worktreeName)
		_, err := os.Stat(worktreePath)
		if err == nil {
			matches = append(matches, repo.Name)
			continue
		}
		if !errors.Is(err, os.ErrNotExist) {
			return "", fmt.Errorf("inspect worktree directory %q: %w", worktreePath, err)
		}
	}

	switch len(matches) {
	case 1:
		return matches[0], nil
	case 0:
		return "", fmt.Errorf("worktree %s not found", worktreeName)
	default:
		slices.Sort(matches)
		return "", fmt.Errorf(
			"worktree %q exists in multiple repositories; qualify as <worktree>@<repo> (%s)",
			worktreeName,
			strings.Join(matches, ", "),
		)
	}
}

func (x Runtime) completeQualifiedWorktreeNames(_ *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) > 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	if name, repoPrefix, found := strings.CutLast(toComplete, "@"); found {
		if name == "" {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		return x.completeRepoSuffix(name, repoPrefix, true)
	}

	return x.completeWorktreeNamesAcrossRepos(toComplete)
}

func (x Runtime) completeCreateArgs(_ *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) > 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	if name, repoPrefix, found := strings.CutLast(toComplete, "@"); found {
		return x.completeRepoSuffix(name, repoPrefix, false)
	}
	return x.completeRepoQualifiers(nil, args, toComplete)
}

func (x Runtime) completeRepoQualifiers(_ *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) > 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	if toComplete != "" && !strings.HasPrefix(toComplete, "@") {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	return x.completeRepoSuffix("", strings.TrimPrefix(toComplete, "@"), false)
}

func (x Runtime) completeRepoSuffix(worktreeName string, repoPrefix string, requireWorktree bool) ([]string, cobra.ShellCompDirective) {
	repos, err := x.listRegisteredRepos()
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}

	names := make([]string, 0)
	for _, repo := range repos {
		if !strings.HasPrefix(repo.Name, repoPrefix) {
			continue
		}
		if requireWorktree {
			if _, err := os.Stat(x.managedWorktreePath(repo.Name, worktreeName)); err != nil {
				continue
			}
		}
		names = append(names, worktreeName+"@"+repo.Name)
	}
	slices.Sort(names)
	return names, cobra.ShellCompDirectiveNoFileComp
}

func (x Runtime) completeWorktreeNamesAcrossRepos(toComplete string) ([]string, cobra.ShellCompDirective) {
	repos, err := x.listRegisteredRepos()
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}

	reposForName := make(map[string][]string)
	var names []string
	for _, repo := range repos {
		for _, name := range x.managedWorktreeNamesOnDisk(repo.Name, toComplete) {
			if _, exists := reposForName[name]; !exists {
				names = append(names, name)
			}
			reposForName[name] = append(reposForName[name], repo.Name)
		}
	}

	completions := make([]string, 0, len(names))
	for _, name := range names {
		reposWithName := reposForName[name]
		if len(reposWithName) == 1 {
			completions = append(completions, name)
			continue
		}
		for _, repoName := range reposWithName {
			completions = append(completions, name+"@"+repoName)
		}
	}
	slices.Sort(completions)
	return completions, cobra.ShellCompDirectiveNoFileComp
}

func (x Runtime) listRegisteredRepos() ([]registeredRepo, error) {
	directory := x.reposDirectory()
	entries, err := os.ReadDir(directory)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return make([]registeredRepo, 0), nil
		}
		return nil, fmt.Errorf("read repos directory %q: %w", directory, err)
	}

	repos := make([]registeredRepo, 0)
	for _, entry := range entries {
		name := entry.Name()
		repoName, found := strings.CutSuffix(name, bareRepoSuffix)
		if !found || repoName == "" {
			continue
		}

		fullPath := filepath.Join(directory, name)
		info, err := os.Stat(fullPath)
		if err != nil {
			return nil, fmt.Errorf("stat registered repo %q: %w", name, err)
		}
		if !info.IsDir() {
			continue
		}

		repos = append(repos, registeredRepo{
			Name:     repoName,
			BarePath: fullPath,
		})
	}

	slices.SortFunc(repos, func(left, right registeredRepo) int {
		return strings.Compare(left.Name, right.Name)
	})
	return repos, nil
}

func (x Runtime) registeredRepoByName(name string) (registeredRepo, error) {
	repos, err := x.listRegisteredRepos()
	if err != nil {
		return registeredRepo{}, err
	}
	index := slices.IndexFunc(repos, func(repo registeredRepo) bool {
		return repo.Name == name
	})
	if index >= 0 {
		return repos[index], nil
	}
	return registeredRepo{}, fmt.Errorf("unknown repository %q", name)
}

func (x Runtime) openRegisteredRepository(name string) (*Repository, registeredRepo, error) {
	repo, err := x.registeredRepoByName(name)
	if err != nil {
		return nil, registeredRepo{}, err
	}
	repository, err := openBareRepository(x, repo.BarePath)
	if err != nil {
		return nil, registeredRepo{}, err
	}
	return repository, repo, nil
}

// managedWorktreeNamesOnDisk lists worktree names under the managed root for repoName
// (layout: <root>/<repo-name>/<worktree-name>/<repo-name>), filtered by toComplete prefix.
func (x Runtime) managedWorktreeNamesOnDisk(repoName string, toComplete string) []string {
	repoRoot := filepath.Join(x.worktreeRoot(), repoName)
	var names []string
	_ = filepath.WalkDir(repoRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if !entry.IsDir() {
			return nil
		}
		if entry.Name() != repoName {
			return nil
		}
		if _, err := os.Stat(filepath.Join(path, ".git")); err != nil {
			return nil
		}
		parent := filepath.Dir(path)
		name, err := filepath.Rel(repoRoot, parent)
		if err != nil || name == "." || strings.HasPrefix(name, "..") {
			return nil
		}
		if strings.HasPrefix(name, toComplete) {
			names = append(names, name)
		}
		return filepath.SkipDir
	})
	slices.Sort(names)
	return names
}

func (x Runtime) completeRegisteredRepoNames(_ *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) > 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	return x.completeRegisteredRepoFlagValues(nil, nil, toComplete)
}

func (x Runtime) completeRegisteredRepoFlagValues(_ *cobra.Command, _ []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	repos, err := x.listRegisteredRepos()
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}
	names := make([]string, 0, len(repos))
	for _, repo := range repos {
		if strings.HasPrefix(repo.Name, toComplete) {
			names = append(names, repo.Name)
		}
	}
	return names, cobra.ShellCompDirectiveNoFileComp
}

func (x Runtime) buildImportPlan(sourcePath string, requestedName string) (importPlan, error) {
	sourceRepository, err := openRepository(x, sourcePath)
	if err != nil {
		bare, bareErr := pathIsBareRepository(x, sourcePath)
		if bareErr == nil && bare {
			display := x.displayHomePath(sourcePath)
			return importPlan{}, fmt.Errorf("%s is a bare repository; use 'timber repo add %s' instead", display, display)
		}
		return importPlan{}, fmt.Errorf("open repository at %s: %w", x.displayHomePath(sourcePath), err)
	}

	commonDir, err := sourceRepository.commonGitDir()
	if err != nil {
		return importPlan{}, err
	}

	if err := rejectRegisteredSource(x, commonDir); err != nil {
		return importPlan{}, err
	}

	porcelainWorktrees, err := sourceRepository.listPorcelainWorktrees()
	if err != nil {
		return importPlan{}, err
	}
	if len(porcelainWorktrees) == 0 {
		return importPlan{}, errors.New("repository has no worktrees")
	}

	// A bare repository with linked worktrees lists the bare directory as its
	// main worktree; such a repository is already in the managed shape.
	mainEntry := porcelainWorktrees[0]
	bareMain, err := samePath(mainEntry.Path, commonDir)
	if err != nil {
		return importPlan{}, err
	}
	if bareMain {
		display := x.displayHomePath(mainEntry.Path)
		return importPlan{}, fmt.Errorf("%s is a bare repository; use 'timber repo add %s' instead", display, display)
	}
	mainPath := mainEntry.Path

	// Resolve the canonical main checkout so subdirectory or linked-worktree
	// arguments still import the whole repository.
	sourceRepository, err = openRepository(x, mainPath)
	if err != nil {
		return importPlan{}, err
	}

	repoName := requestedName
	if repoName == "" {
		repoName = defaultRepoNameForMigrate(sourceRepository, mainPath)
	}
	if err := validateRepoName(repoName); err != nil {
		return importPlan{}, err
	}

	barePath := x.bareRepoPath(repoName)
	if _, err := os.Stat(barePath); err == nil {
		return importPlan{}, fmt.Errorf("repository %q already exists at %s", repoName, barePath)
	} else if !errors.Is(err, os.ErrNotExist) {
		return importPlan{}, fmt.Errorf("inspect repository path %q: %w", barePath, err)
	}

	plan := importPlan{
		runtime:  x,
		source:   sourceRepository,
		repoName: repoName,
		barePath: barePath,
		mainPath: mainPath,
	}
	if err := plan.collectWorktrees(porcelainWorktrees); err != nil {
		return importPlan{}, err
	}
	if err := plan.validateTargets(); err != nil {
		return importPlan{}, err
	}
	if err := x.validateTrashCommand(); err != nil {
		return importPlan{}, err
	}
	return plan, nil
}

// trashExecutable returns the command used to move paths to the system trash.
func (x Runtime) trashExecutable() string {
	return cmp.Or(x.TrashExecutable, trashCommandName)
}

// validateTrashCommand fails when the trash CLI is not available so the
// import can stop before mutating anything.
func (x Runtime) validateTrashCommand() error {
	executable := x.trashExecutable()
	if _, err := exec.LookPath(executable); err != nil {
		return fmt.Errorf(
			"trash command %q not found; install a trash CLI (e.g. trash-cli) so old worktrees are removed to the system trash instead of deleted",
			executable,
		)
	}
	return nil
}

// trashPaths moves paths to the system trash with the trash CLI.
func (x Runtime) trashPaths(paths ...string) error {
	trashablePaths := make([]string, 0, len(paths))
	for _, path := range paths {
		if path != "" {
			trashablePaths = append(trashablePaths, path)
		}
	}
	if len(trashablePaths) == 0 {
		return nil
	}

	command := x.command(context.Background(), x.trashExecutable(), trashablePaths...)
	var stderr bytes.Buffer
	command.Stderr = &stderr
	if err := command.Run(); err != nil {
		return fmt.Errorf(
			"trash %s: %w: %s",
			strings.Join(trashablePaths, " "),
			err,
			strings.TrimSpace(stderr.String()),
		)
	}
	return nil
}

func (x Runtime) completeRepoRenameArguments(
	command *cobra.Command,
	args []string,
	toComplete string,
) ([]string, cobra.ShellCompDirective) {
	if len(args) == 0 {
		return x.completeRegisteredRepoNames(command, args, toComplete)
	}
	return nil, cobra.ShellCompDirectiveNoFileComp
}

func (x Runtime) buildRepositoryRenamePlan(oldName string, requestedNewName string) (repositoryRenamePlan, error) {
	sourceRepo, err := x.registeredRepoByName(oldName)
	if err != nil {
		return repositoryRenamePlan{}, err
	}

	newName := normalizeRepoName(requestedNewName)
	if err := validateRepoName(newName); err != nil {
		return repositoryRenamePlan{}, err
	}
	if sourceRepo.Name == newName {
		return repositoryRenamePlan{}, fmt.Errorf("repository is already named %q", newName)
	}

	plan := repositoryRenamePlan{
		runtime:    x,
		sourceRepo: sourceRepo,
		destinationRepo: registeredRepo{
			Name:     newName,
			BarePath: x.bareRepoPath(newName),
		},
	}
	if err := plan.validateDestinationRepo(); err != nil {
		return repositoryRenamePlan{}, err
	}
	if err := plan.collectWorktrees(); err != nil {
		return repositoryRenamePlan{}, err
	}
	if err := plan.validateWorktreeDestinations(); err != nil {
		return repositoryRenamePlan{}, err
	}
	if err := plan.findCurrentTargetDirectory(); err != nil {
		return repositoryRenamePlan{}, err
	}
	return plan, nil
}

func (x Runtime) reportRenamedCurrentPath(path string) error {
	pathFile := x.RenamePathFile
	if pathFile == "" || path == "" {
		return nil
	}
	if err := x.writePathFile(pathFile, path); err != nil {
		return fmt.Errorf("write renamed worktree path file: %w", err)
	}
	return nil
}

func (x Runtime) reportAlreadyInWorktree(command *cobra.Command, name string, worktreePath string) error {
	currentDirectory := x.CurrentDirectory
	same, err := samePath(currentDirectory, worktreePath)
	if err != nil || !same {
		return err
	}
	_, err = fmt.Fprintf(command.ErrOrStderr(), "Already in %s\n", name)
	return err
}

func (x Runtime) reportSwitchWorktreePath(command *cobra.Command, worktreePath string) error {
	if pathFile := x.SwitchPathFile; pathFile != "" {
		if err := x.writePathFile(pathFile, worktreePath); err != nil {
			return fmt.Errorf("write switch worktree path file: %w", err)
		}
		return nil
	}

	_, err := fmt.Fprintln(command.OutOrStdout(), worktreePath)
	return err
}

func (x Runtime) listRegisteredReposForWizard() ([]registeredRepo, error) {
	repos, err := x.listRegisteredRepos()
	if err != nil {
		return nil, err
	}
	if len(repos) == 0 {
		return nil, errors.New("no registered repositories; run timber repo add first")
	}
	return repos, nil
}

func (x Runtime) listWorktreesForWizard(repos []registeredRepo) ([]managedWorktree, error) {
	return x.collectWorktrees(repos, func(_ *Repository, worktree managedWorktree) (managedWorktree, error) {
		return worktree, nil
	})
}

func (x Runtime) enrichManagedWorktree(repository *Repository, worktree managedWorktree) (managedWorktree, error) {
	worktreeRepository, err := openRepository(x, worktree.Path)
	if err != nil {
		return managedWorktree{}, err
	}

	clean, err := worktreeRepository.isClean()
	if err != nil {
		return managedWorktree{}, err
	}

	status, err := worktreeRepository.status()
	if err != nil {
		return managedWorktree{}, err
	}

	worktree.Status = status
	worktree.Clean = clean

	upstreamRef, err := repository.upstreamReference(worktree.Name)
	if err != nil {
		return managedWorktree{}, err
	}

	merged, err := repository.branchMergedToUpstream(worktree.BranchReference, upstreamRef)
	if err != nil {
		return managedWorktree{}, err
	}

	worktree.UpstreamRef = upstreamRef
	worktree.Merged = merged

	return worktree, nil
}

func (x Runtime) managedWorktreesFromRepository(repository *Repository, repoName string) ([]managedWorktree, error) {
	porcelainWorktrees, err := repository.listPorcelainWorktrees()
	if err != nil {
		return nil, err
	}

	currentDirectory := x.CurrentDirectory

	managedWorktrees := make([]managedWorktree, 0)
	for _, porcelainWorktree := range porcelainWorktrees {
		branchName := porcelainWorktree.branchName()
		if branchName == "" {
			continue
		}

		expectedPath := x.managedWorktreePath(repoName, branchName)
		same, err := samePath(expectedPath, porcelainWorktree.Path)
		if err != nil {
			return nil, err
		}
		if !same {
			continue
		}

		managedWorktrees = append(managedWorktrees, managedWorktree{
			Repo:            repoName,
			Name:            branchName,
			Path:            porcelainWorktree.Path,
			DisplayPath:     currentRelativePath(currentDirectory, porcelainWorktree.Path),
			CommitHash:      porcelainWorktree.CommitHash,
			BranchReference: referenceName(porcelainWorktree.BranchRef),
		})
	}

	slices.SortFunc(managedWorktrees, compareManagedWorktrees)

	return managedWorktrees, nil
}

func (x Runtime) collectManagedWorktrees(repos []registeredRepo) ([]managedWorktree, error) {
	return x.collectWorktrees(repos, x.enrichManagedWorktree)
}

func (x Runtime) collectListedWorktrees(repos []registeredRepo) ([]managedWorktree, error) {
	return x.collectWorktrees(repos, x.enrichWorktreeForList)
}

func (x Runtime) collectWorktrees(repos []registeredRepo, enrich worktreeEnricher) ([]managedWorktree, error) {
	worktrees := make([]managedWorktree, 0)
	for _, repo := range repos {
		repository, err := openBareRepository(x, repo.BarePath)
		if err != nil {
			return nil, err
		}

		repoWorktrees, err := x.managedWorktreesFromRepository(repository, repo.Name)
		if err != nil {
			return nil, err
		}

		for _, worktree := range repoWorktrees {
			enrichedWorktree, err := enrich(repository, worktree)
			if err != nil {
				return nil, err
			}
			worktrees = append(worktrees, enrichedWorktree)
		}
	}

	slices.SortFunc(worktrees, compareManagedWorktrees)
	return worktrees, nil
}

func (x Runtime) enrichWorktreeForList(_ *Repository, worktree managedWorktree) (managedWorktree, error) {
	result, err := gitOutput(x, worktree.Path, "status", "--porcelain=v2", "--branch")
	if err != nil {
		return managedWorktree{}, fmt.Errorf("read worktree status: %w", err)
	}

	status, clean, err := parsePorcelainStatus(result.stdout)
	if err != nil {
		return managedWorktree{}, fmt.Errorf("parse worktree status: %w", err)
	}
	worktree.ListStatus = status
	worktree.Clean = clean
	return worktree, nil
}

func (x Runtime) selectManagedWorktree(worktrees []managedWorktree, name string) (managedWorktree, error) {
	if name != "" {
		return managedWorktreeByName(worktrees, name)
	}

	currentDirectory := x.CurrentDirectory

	currentRepository, err := openRepository(x, currentDirectory)
	if err != nil {
		return managedWorktree{}, fmt.Errorf("worktree name is required when not inside a managed worktree: %w", err)
	}

	return managedWorktreeForPath(worktrees, currentRepository.WorkTree)
}

func (x Runtime) unusedWorktreeName(repoName string) (string, error) {
	return firstUnusedWorktreeName(randomWorktreeName, func(name string) (bool, error) {
		return x.worktreeDirectoryExists(repoName, name)
	})
}

func (x Runtime) worktreeDirectoryExists(repoName string, name string) (bool, error) {
	path := x.managedWorktreePath(repoName, name)
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return false, fmt.Errorf("inspect worktree directory %q: %w", path, err)
}
