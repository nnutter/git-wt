package timber

import (
	"cmp"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/samber/lo"
	"github.com/spf13/cobra"
)

type migrateCandidate struct {
	Action             string
	Repo               string
	Name               string
	CurrentPath        string
	TargetPath         string
	DisplayCurrentPath string
	DisplayTargetPath  string
	BranchName         string
}

type migratePrompter interface {
	Prompt(io.Reader, io.Writer, []migrateCandidate) ([]migrateCandidate, error)
}

type migrateCommandOptions struct {
	name     string
	prompt   bool
	all      bool
	prompter migratePrompter
}

type huhMigratePrompter struct{}

func NewMigrateCommand() *cobra.Command {
	options := &migrateCommandOptions{prompter: huhMigratePrompter{}}

	command := &cobra.Command{
		Use:   "migrate",
		Short: "Register a clone as bare, or rehome worktrees into the managed layout",
		Args:  cobra.NoArgs,
		RunE:  options.Execute,
	}

	command.Flags().StringVar(&options.name, "name", "", "Repository name (default: derived from checkout)")
	command.Flags().BoolVarP(&options.prompt, "prompt", "p", false, "Prompt before migrating worktrees")
	command.Flags().BoolVarP(&options.all, "all", "a", false, "Rehome worktrees for every registered repository")

	return command
}

func (x *migrateCommandOptions) Execute(command *cobra.Command, args []string) error {
	repos, err := listRegisteredRepos()
	if err != nil {
		return err
	}
	if x.all {
		return x.rehomeRegisteredRepositories(command, repos)
	}

	sourceRepository, err := openRepository(".")
	if err != nil {
		return err
	}
	return x.migrateFromRepository(command, sourceRepository, repos)
}

func (x *migrateCommandOptions) migrateFromRepository(
	command *cobra.Command,
	sourceRepository *Repository,
	repos []registeredRepo,
) error {
	repo, repository, err := registeredRepositoryFor(sourceRepository, repos)
	if err != nil {
		return err
	}
	if repository != nil {
		return x.rehomeRegisteredRepository(command, repo, repository)
	}
	return x.migrateClone(command, sourceRepository)
}

func registeredRepositoryFor(
	source *Repository,
	repos []registeredRepo,
) (registeredRepo, *Repository, error) {
	commonDir, err := source.commonGitDir()
	if err != nil {
		return registeredRepo{}, nil, err
	}

	for _, repo := range repos {
		same, err := samePath(repo.BarePath, commonDir)
		if err != nil {
			return registeredRepo{}, nil, err
		}
		if !same {
			continue
		}

		repository, err := openBareRepository(repo.BarePath)
		if err != nil {
			return registeredRepo{}, nil, err
		}
		return repo, repository, nil
	}

	return registeredRepo{}, nil, nil
}

func (x *migrateCommandOptions) migrateClone(command *cobra.Command, sourceRepository *Repository) error {
	mainPath, err := sourceRepository.mainWorktreePath()
	if err != nil {
		return err
	}

	repoName := normalizeRepoName(x.name)
	if repoName == "" {
		repoName = defaultRepoNameForMigrate(sourceRepository, mainPath)
	}
	if err := validateRepoName(repoName); err != nil {
		return err
	}

	targetBarePath := bareRepoPath(repoName)
	if _, err := os.Stat(targetBarePath); err == nil {
		return fmt.Errorf("repository %q already exists at %s", repoName, targetBarePath)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("inspect repository path %q: %w", targetBarePath, err)
	}

	candidates, omitSoleDefaultSource, err := migrationCandidatesFromRepository(sourceRepository, repoName)
	if err != nil {
		return err
	}

	selectedCandidates, err := x.selectMigrationCandidates(command, candidates)
	if err != nil {
		return err
	}

	if err := ensureDirectory(filepath.Dir(targetBarePath)); err != nil {
		return err
	}

	// Clone bare from the current checkout so local branches/refs are preserved.
	if _, err := gitOutput(".", "clone", "--bare", mainPath, targetBarePath); err != nil {
		return err
	}

	// clone --bare points origin at the source path and omits remote.origin.fetch.
	// Replace origin with the source's real remote (when present) and always
	// install fetch refspecs + origin/HEAD the same way repo add does.
	if err := setupMigratedBareOrigin(sourceRepository, targetBarePath); err != nil {
		return err
	}

	bareRepository, err := openBareRepository(targetBarePath)
	if err != nil {
		return err
	}

	if _, err := fmt.Fprintf(
		command.ErrOrStderr(),
		"%s\n",
		statusStyle.Render("registered repository "+repoName+" at "+targetBarePath),
	); err != nil {
		return err
	}

	// A plain clone on origin/HEAD with no linked worktrees only needs the bare
	// repo; drop the source checkout instead of creating a managed worktree.
	if omitSoleDefaultSource && len(selectedCandidates) == 0 {
		if err := os.RemoveAll(mainPath); err != nil {
			return fmt.Errorf("remove source checkout %q: %w", mainPath, err)
		}
		if err := removeEmptySourceParents(mainPath); err != nil {
			return err
		}
		_, err = fmt.Fprintf(
			command.ErrOrStderr(),
			"%s\n",
			statusStyle.Render("omitted default-branch worktree; bare repository only"),
		)
		return err
	}

	return reportAppliedCandidates(command, bareRepository, selectedCandidates, applyMigrationCandidate)
}

func (x *migrateCommandOptions) rehomeRegisteredRepository(
	command *cobra.Command,
	repo registeredRepo,
	repository *Repository,
) error {
	candidates, err := rehomeCandidatesFromRepository(repository, repo.Name)
	if err != nil {
		return err
	}
	return x.rehomeCandidates(command, candidates)
}

func (x *migrateCommandOptions) rehomeRegisteredRepositories(
	command *cobra.Command,
	repos []registeredRepo,
) error {
	candidates := make([]migrateCandidate, 0)
	for _, repo := range repos {
		repository, err := openBareRepository(repo.BarePath)
		if err != nil {
			return err
		}
		repoCandidates, err := rehomeCandidatesFromRepository(repository, repo.Name)
		if err != nil {
			return err
		}
		candidates = append(candidates, repoCandidates...)
	}
	return x.rehomeCandidates(command, candidates)
}

func (x *migrateCommandOptions) rehomeCandidates(
	command *cobra.Command,
	candidates []migrateCandidate,
) error {
	if len(candidates) == 0 {
		_, err := fmt.Fprintf(
			command.ErrOrStderr(),
			"%s\n",
			statusStyle.Render("no worktrees to rehome"),
		)
		return err
	}

	selectedCandidates, err := x.selectMigrationCandidates(command, candidates)
	if err != nil {
		return err
	}

	for _, candidate := range selectedCandidates {
		repository, err := openBareRepository(bareRepoPath(candidate.Repo))
		if err != nil {
			return err
		}
		if err := reportAppliedCandidate(command, repository, candidate, moveLinkedWorktree); err != nil {
			return err
		}
	}
	return nil
}

func (x *migrateCommandOptions) selectMigrationCandidates(
	command *cobra.Command,
	candidates []migrateCandidate,
) ([]migrateCandidate, error) {
	selectedCandidates := candidates
	if x.prompt {
		var err error
		selectedCandidates, err = x.prompter.Prompt(command.InOrStdin(), command.ErrOrStderr(), candidates)
		if err != nil {
			return nil, err
		}
	}
	if err := validateMigrationCandidates(selectedCandidates); err != nil {
		return nil, err
	}
	return selectedCandidates, nil
}

type applyCandidateFunc func(*Repository, migrateCandidate) error

func reportAppliedCandidates(
	command *cobra.Command,
	repository *Repository,
	candidates []migrateCandidate,
	apply applyCandidateFunc,
) error {
	for _, candidate := range candidates {
		if err := reportAppliedCandidate(command, repository, candidate, apply); err != nil {
			return err
		}
	}
	return nil
}

func reportAppliedCandidate(
	command *cobra.Command,
	repository *Repository,
	candidate migrateCandidate,
	apply applyCandidateFunc,
) error {
	if err := apply(repository, candidate); err != nil {
		return err
	}
	message := fmt.Sprintf("%sd %s to %s", candidate.Action, candidate.Name, candidate.TargetPath)
	_, err := fmt.Fprintf(command.ErrOrStderr(), "%s\n", statusStyle.Render(message))
	return err
}

func removeEmptySourceParents(path string) error {
	homeDirectory, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("resolve home directory: %w", err)
	}
	return removeEmptyParents(path, homeDirectory)
}

func defaultRepoNameForMigrate(source *Repository, mainPath string) string {
	if result, err := source.git("remote", "get-url", remoteName); err == nil {
		if name, err := defaultRepoNameFromRemote(result.stdout); err == nil {
			return name
		}
	}
	return defaultRepoNameFromPath(mainPath)
}

func defaultRepoNameFromPath(mainPath string) string {
	return normalizeRepoName(filepath.Base(mainPath))
}

func setupMigratedBareOrigin(source *Repository, barePath string) error {
	bare, err := openBareRepository(barePath)
	if err != nil {
		return err
	}

	originURL := ""
	if result, err := source.git("remote", "get-url", remoteName); err == nil {
		originURL = result.stdout
	}

	// Drop the clone-default origin (it points at the ephemeral source checkout).
	_, _ = bare.git("remote", "remove", remoteName)

	if originURL == "" {
		// Local-only source repositories have no origin to track.
		return nil
	}

	if _, err := bare.git("remote", "add", remoteName, originURL); err != nil {
		return err
	}
	return configureBareOriginTracking(barePath)
}

func applyMigrationCandidate(repository *Repository, candidate migrateCandidate) error {
	currentPath := filepath.Clean(candidate.CurrentPath)
	targetPath := filepath.Clean(candidate.TargetPath)

	stagingDirectory, err := os.MkdirTemp("", "timber-migrate-")
	if err != nil {
		return fmt.Errorf("create migration staging directory: %w", err)
	}
	defer os.RemoveAll(stagingDirectory)

	if err := copyDirectoryContents(currentPath, stagingDirectory, ".git"); err != nil {
		return fmt.Errorf("stage worktree %q: %w", currentPath, err)
	}

	// Remove the old worktree path so git worktree add can create targetPath.
	if err := os.RemoveAll(currentPath); err != nil {
		return fmt.Errorf("remove old worktree %q: %w", currentPath, err)
	}
	if err := removeEmptySourceParents(currentPath); err != nil {
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

func ensureBranchUpstream(repository *Repository, branchName string) error {
	_, err := repository.upstreamReference(branchName)
	if err == nil {
		return nil
	}

	upstreamBranch, resolveErr := repository.remoteHeadBranch()
	if resolveErr != nil {
		// Local-only repositories may have no origin; leave upstream unset.
		return nil
	}
	_, err = repository.git("branch", "--set-upstream-to", upstreamBranch, branchName)
	return err
}

func copyDirectoryContents(sourceDirectory string, destinationDirectory string, skipNames ...string) (err error) {
	sourceRoot, err := os.OpenRoot(sourceDirectory)
	if err != nil {
		return fmt.Errorf("open source directory %q: %w", sourceDirectory, err)
	}
	defer func() {
		if closeErr := sourceRoot.Close(); err == nil {
			err = closeErr
		}
	}()

	destinationRoot, err := os.OpenRoot(destinationDirectory)
	if err != nil {
		return fmt.Errorf("open destination directory %q: %w", destinationDirectory, err)
	}
	defer func() {
		if closeErr := destinationRoot.Close(); err == nil {
			err = closeErr
		}
	}()

	skip := make(map[string]struct{}, len(skipNames))
	for _, name := range skipNames {
		skip[name] = struct{}{}
	}

	return filepath.WalkDir(sourceDirectory, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == sourceDirectory {
			return nil
		}

		relativePath, err := filepath.Rel(sourceDirectory, path)
		if err != nil {
			return err
		}
		if _, excluded := skip[entry.Name()]; excluded && filepath.Dir(relativePath) == "." {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		destinationPath := filepath.Join(destinationDirectory, relativePath)
		if entry.IsDir() {
			return os.MkdirAll(destinationPath, 0o755)
		}

		info, err := entry.Info()
		if err != nil {
			return err
		}
		contents, err := sourceRoot.ReadFile(relativePath)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(destinationPath), 0o755); err != nil {
			return err
		}
		return destinationRoot.WriteFile(relativePath, contents, info.Mode().Perm())
	})
}

func (huhMigratePrompter) Prompt(input io.Reader, output io.Writer, candidates []migrateCandidate) ([]migrateCandidate, error) {
	selectedKeys := make([]string, 0, len(candidates))
	options := lo.Map(candidates, func(candidate migrateCandidate, _ int) huh.Option[string] {
		label := candidate.Repo + "/" + candidate.Name + " (" + candidate.DisplayCurrentPath + " -> " + candidate.DisplayTargetPath + ")"
		return huh.NewOption(label, migrateCandidateKey(candidate)).Selected(true)
	})

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewMultiSelect[string]().
				Title("Select worktrees to migrate").
				Options(options...).
				Value(&selectedKeys),
		),
	).WithInput(input).WithOutput(output)

	if err := form.Run(); err != nil {
		return nil, err
	}

	selectedCandidates := make([]migrateCandidate, 0, len(selectedKeys))
	for _, selectedKey := range selectedKeys {
		candidate, err := migrateCandidateByKey(candidates, selectedKey)
		if err != nil {
			return nil, err
		}
		selectedCandidates = append(selectedCandidates, candidate)
	}

	return selectedCandidates, nil
}

func migrationCandidatesFromRepository(repository *Repository, repoName string) ([]migrateCandidate, bool, error) {
	candidates, err := collectBranchedWorktreeCandidates(repository, repoName)
	if err != nil {
		return nil, false, err
	}

	omitSoleDefaultSource, err := shouldOmitSoleDefaultWorktree(repository, candidates)
	if err != nil {
		return nil, false, err
	}
	if omitSoleDefaultSource {
		return nil, true, nil
	}

	return candidates, false, nil
}

func rehomeCandidatesFromRepository(repository *Repository, repoName string) ([]migrateCandidate, error) {
	candidates, err := collectBranchedWorktreeCandidates(repository, repoName)
	if err != nil {
		return nil, err
	}

	rehomeCandidates := make([]migrateCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		same, err := samePath(candidate.CurrentPath, candidate.TargetPath)
		if err != nil {
			return nil, err
		}
		if same {
			continue
		}
		rehomeCandidates = append(rehomeCandidates, candidate)
	}
	return rehomeCandidates, nil
}

func collectBranchedWorktreeCandidates(repository *Repository, repoName string) ([]migrateCandidate, error) {
	porcelainWorktrees, err := repository.listPorcelainWorktrees()
	if err != nil {
		return nil, err
	}

	currentDirectory, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("get current directory: %w", err)
	}

	candidates := make([]migrateCandidate, 0, len(porcelainWorktrees))
	for _, porcelainWorktree := range porcelainWorktrees {
		branchName := porcelainWorktree.branchName()
		if branchName == "" {
			continue
		}

		targetPath := managedWorktreePath(repoName, branchName)
		candidates = append(candidates, migrateCandidate{
			Action:             "migrate",
			Repo:               repoName,
			Name:               branchName,
			BranchName:         branchName,
			CurrentPath:        porcelainWorktree.Path,
			TargetPath:         targetPath,
			DisplayCurrentPath: currentRelativePath(currentDirectory, porcelainWorktree.Path),
			DisplayTargetPath:  currentRelativePath(currentDirectory, targetPath),
		})
	}

	slices.SortFunc(candidates, func(left, right migrateCandidate) int {
		if repoOrder := cmp.Compare(left.Repo, right.Repo); repoOrder != 0 {
			return repoOrder
		}
		return cmp.Compare(left.Name, right.Name)
	})
	return candidates, nil
}

// shouldOmitSoleDefaultWorktree reports whether migrate should register only the
// bare repo: one branched worktree whose branch is origin/HEAD (or the same
// default-branch fallback create uses). Dirty checkouts are kept as worktrees so
// local modifications are not discarded.
func shouldOmitSoleDefaultWorktree(repository *Repository, candidates []migrateCandidate) (bool, error) {
	if len(candidates) != 1 {
		return false, nil
	}

	defaultUpstream, err := repository.remoteHeadBranch()
	if err != nil {
		// No resolvable default branch — keep migrating the sole worktree.
		return false, nil
	}
	defaultBranch := defaultBranchName(defaultUpstream)
	if candidates[0].BranchName != defaultBranch {
		return false, nil
	}

	sourceRepository, err := openRepository(candidates[0].CurrentPath)
	if err != nil {
		return false, err
	}
	clean, err := sourceRepository.isClean()
	if err != nil {
		return false, err
	}
	return clean, nil
}

func defaultBranchName(upstream string) string {
	upstream = strings.TrimSpace(upstream)
	if after, found := strings.CutPrefix(upstream, remoteName+"/"); found {
		return after
	}
	return shortReference(referenceName(upstream))
}

func validateMigrationCandidates(candidates []migrateCandidate) error {
	targetPaths := make(map[string]string, len(candidates))
	for _, candidate := range candidates {
		targetPath := filepath.Clean(candidate.TargetPath)
		if existingName, ok := targetPaths[targetPath]; ok {
			return fmt.Errorf("worktrees %q and %q share target path %q", existingName, candidate.Name, candidate.TargetPath)
		}
		targetPaths[targetPath] = candidate.Name

		// Allow target == current (already at managed path).
		if filepath.Clean(candidate.CurrentPath) == targetPath {
			continue
		}

		if _, err := os.Stat(candidate.TargetPath); err == nil {
			return fmt.Errorf("worktree directory %q already exists", candidate.TargetPath)
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("inspect worktree directory %q: %w", candidate.TargetPath, err)
		}
	}

	return nil
}

func migrateCandidateKey(candidate migrateCandidate) string {
	return candidate.Repo + "\x00" + candidate.Name
}

func migrateCandidateByKey(candidates []migrateCandidate, key string) (migrateCandidate, error) {
	for _, candidate := range candidates {
		if migrateCandidateKey(candidate) == key {
			return candidate, nil
		}
	}

	return migrateCandidate{}, fmt.Errorf("unknown worktree %q", key)
}

func moveLinkedWorktree(repository *Repository, candidate migrateCandidate) error {
	currentPath := filepath.Clean(candidate.CurrentPath)
	targetPath := filepath.Clean(candidate.TargetPath)
	if currentPath == targetPath {
		return nil
	}

	sourcePath := currentPath
	if pathIsWithin(currentPath, targetPath) {
		stagingPath, err := unusedTempPathIn(worktreeRoot(), "timber-migrate-")
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
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("inspect worktree directory %q: %w", targetPath, err)
	}

	if _, err := repository.git("worktree", "move", sourcePath, targetPath); err != nil {
		return err
	}
	return removeEmptySourceParents(currentPath)
}

func unusedTempPathIn(directory string, prefix string) (string, error) {
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return "", fmt.Errorf("create migration staging directory: %w", err)
	}
	file, err := os.CreateTemp(directory, prefix)
	if err != nil {
		return "", fmt.Errorf("create migration staging path: %w", err)
	}
	path := file.Name()
	if err := file.Close(); err != nil {
		return "", fmt.Errorf("close migration staging path: %w", err)
	}
	if err := os.Remove(path); err != nil {
		return "", fmt.Errorf("prepare migration staging path: %w", err)
	}
	return path, nil
}

// pathIsWithin reports whether child is the same as parent or nested under it.
func pathIsWithin(parent string, child string) bool {
	parent = canonicalPath(parent)
	child = canonicalPath(child)
	relativePath, err := filepath.Rel(parent, child)
	if err != nil {
		return false
	}
	return relativePath == "." || (relativePath != ".." && !strings.HasPrefix(relativePath, ".."+string(filepath.Separator)))
}

func canonicalPath(path string) string {
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return filepath.Clean(path)
	}
	return resolved
}
