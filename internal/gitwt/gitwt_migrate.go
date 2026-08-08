package gitwt

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
	prompter migratePrompter
}

type huhMigratePrompter struct{}

func NewMigrateCommand() *cobra.Command {
	options := &migrateCommandOptions{prompter: huhMigratePrompter{}}

	command := &cobra.Command{
		Use:   "migrate",
		Short: "Register the current repository as bare and rehome worktrees",
		Args:  cobra.NoArgs,
		RunE:  options.Execute,
	}

	command.Flags().StringVar(&options.name, "name", "", "Repository name (default: derived from checkout)")
	command.Flags().BoolVarP(&options.prompt, "prompt", "p", false, "Prompt before migrating worktrees")

	return command
}

func (x *migrateCommandOptions) Execute(command *cobra.Command, args []string) error {
	sourceRepository, err := openRepository(".")
	if err != nil {
		return err
	}

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

	candidates, err := migrationCandidatesFromRepository(sourceRepository, repoName)
	if err != nil {
		return err
	}

	selectedCandidates := candidates
	if x.prompt {
		selectedCandidates, err = x.prompter.Prompt(command.InOrStdin(), command.ErrOrStderr(), candidates)
		if err != nil {
			return err
		}
	}

	if err := validateMigrationCandidates(selectedCandidates); err != nil {
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

	for _, candidate := range selectedCandidates {
		if err := applyMigrationCandidate(bareRepository, candidate); err != nil {
			return err
		}

		message := fmt.Sprintf("%sd %s to %s", candidate.Action, candidate.Name, candidate.TargetPath)
		if _, err := fmt.Fprintf(command.ErrOrStderr(), "%s\n", statusStyle.Render(message)); err != nil {
			return err
		}
	}

	return nil
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

	stagingDirectory, err := os.MkdirTemp("", "git-wt-migrate-")
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

func copyDirectoryContents(sourceDirectory string, destinationDirectory string, skipNames ...string) error {
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
		contents, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(destinationPath), 0o755); err != nil {
			return err
		}
		return os.WriteFile(destinationPath, contents, info.Mode().Perm())
	})
}

func (huhMigratePrompter) Prompt(input io.Reader, output io.Writer, candidates []migrateCandidate) ([]migrateCandidate, error) {
	selectedNames := make([]string, 0, len(candidates))
	options := lo.Map(candidates, func(candidate migrateCandidate, _ int) huh.Option[string] {
		label := candidate.Name + " (" + candidate.DisplayCurrentPath + " -> " + candidate.DisplayTargetPath + ")"
		return huh.NewOption(label, candidate.Name).Selected(true)
	})

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewMultiSelect[string]().
				Title("Select worktrees to migrate").
				Options(options...).
				Value(&selectedNames),
		),
	).WithInput(input).WithOutput(output)

	if err := form.Run(); err != nil {
		return nil, err
	}

	selectedCandidates := make([]migrateCandidate, 0, len(selectedNames))
	for _, selectedName := range selectedNames {
		candidate, err := migrateCandidateByName(candidates, selectedName)
		if err != nil {
			return nil, err
		}
		selectedCandidates = append(selectedCandidates, candidate)
	}

	return selectedCandidates, nil
}

func migrationCandidatesFromRepository(repository *Repository, repoName string) ([]migrateCandidate, error) {
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
			Name:               branchName,
			BranchName:         branchName,
			CurrentPath:        porcelainWorktree.Path,
			TargetPath:         targetPath,
			DisplayCurrentPath: currentRelativePath(currentDirectory, porcelainWorktree.Path),
			DisplayTargetPath:  currentRelativePath(currentDirectory, targetPath),
		})
	}

	slices.SortFunc(candidates, func(left, right migrateCandidate) int {
		return cmp.Compare(left.Name, right.Name)
	})

	return candidates, nil
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

func migrateCandidateByName(candidates []migrateCandidate, name string) (migrateCandidate, error) {
	for _, candidate := range candidates {
		if candidate.Name == name {
			return candidate, nil
		}
	}

	return migrateCandidate{}, fmt.Errorf("unknown worktree %q", name)
}

// pathIsWithin reports whether child is the same as parent or nested under it.
func pathIsWithin(parent string, child string) bool {
	relativePath, err := filepath.Rel(parent, child)
	if err != nil {
		return false
	}
	return relativePath == "." || (relativePath != ".." && !strings.HasPrefix(relativePath, ".."+string(filepath.Separator)))
}
