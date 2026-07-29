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
}

type migratePrompter interface {
	Prompt(io.Reader, io.Writer, []migrateCandidate) ([]migrateCandidate, error)
}

type migrateCommandOptions struct {
	prompt   bool
	prompter migratePrompter
}

type huhMigratePrompter struct{}

func NewMigrateCommand() *cobra.Command {
	options := &migrateCommandOptions{prompter: huhMigratePrompter{}}

	command := &cobra.Command{
		Use:   "migrate",
		Short: "Migrate existing Git worktrees to managed paths",
		Args:  cobra.NoArgs,
		RunE:  options.Execute,
	}

	command.Flags().BoolVarP(&options.prompt, "prompt", "p", false, "Prompt before migrating")

	return command
}

func (x *migrateCommandOptions) Execute(command *cobra.Command, args []string) error {
	repository, err := openRepository(".")
	if err != nil {
		return err
	}

	mainPath, err := repository.mainWorktreePath()
	if err != nil {
		return err
	}

	if mainNeedsLayoutMigration(mainPath) {
		targetMainPath := migratedMainPath(mainPath)
		if err := migrateMainWorktree(repository, command.ErrOrStderr(), mainPath, targetMainPath); err != nil {
			return err
		}
		// Re-open from the new main path (cwd may still point at the old location).
		repository, err = openRepository(targetMainPath)
		if err != nil {
			return err
		}
	}

	candidates, err := migrationCandidatesFromRepository(repository)
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

	for _, candidate := range selectedCandidates {
		if err := applyMigrationCandidate(repository, candidate); err != nil {
			return err
		}

		message := fmt.Sprintf("%sd %s to %s", candidate.Action, candidate.Name, candidate.TargetPath)
		if _, err := fmt.Fprintf(command.ErrOrStderr(), "%s\n", statusStyle.Render(message)); err != nil {
			return err
		}
	}

	return nil
}

// migrateMainWorktree moves main into <root>/main/<repo> via a temporary
// sibling path (a directory cannot be moved into a path under itself).
//
// Covers plain clone (<root> -> <root>/main/<repo>) and old layout
// (<root>/main -> <root>/main/<repo>).
//
// git worktree move refuses to move the main working tree, so this uses
// filesystem renames and git worktree repair to fix linked worktree gitdirs.
func migrateMainWorktree(repository *Repository, stderr io.Writer, mainPath string, targetPath string) error {
	if _, err := os.Stat(targetPath); err == nil {
		return fmt.Errorf("worktree directory %q already exists", targetPath)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("inspect worktree directory %q: %w", targetPath, err)
	}

	root := filepath.Dir(mainPath)
	temporaryPath := filepath.Join(root, ".git-wt-main-migrate")
	if _, err := os.Stat(temporaryPath); err == nil {
		return fmt.Errorf("temporary main migration path %q already exists", temporaryPath)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("inspect temporary main migration path %q: %w", temporaryPath, err)
	}

	if err := os.Rename(mainPath, temporaryPath); err != nil {
		return fmt.Errorf("move main worktree to temporary path: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		return fmt.Errorf("create main parent directory %q: %w", filepath.Dir(targetPath), err)
	}
	if err := os.Rename(temporaryPath, targetPath); err != nil {
		return fmt.Errorf("move main worktree to %q: %w", targetPath, err)
	}

	repository.WorkTree = targetPath
	repository.GitDir = filepath.Join(targetPath, ".git")
	if _, err := repository.git("worktree", "repair"); err != nil {
		return err
	}

	message := fmt.Sprintf("migrated main to %s", targetPath)
	_, err := fmt.Fprintf(stderr, "%s\n", statusStyle.Render(message))
	return err
}

func applyMigrationCandidate(repository *Repository, candidate migrateCandidate) error {
	if candidate.CurrentPath == "" {
		if err := ensureWorktreeDirectory(candidate.TargetPath); err != nil {
			return err
		}
		_, err := repository.git("worktree", "add", candidate.TargetPath, candidate.Name)
		return err
	}

	currentPath := filepath.Clean(candidate.CurrentPath)
	targetPath := filepath.Clean(candidate.TargetPath)
	if pathIsWithin(currentPath, targetPath) {
		return moveWorktreeViaTemporaryPath(repository, currentPath, targetPath)
	}

	parent := filepath.Dir(targetPath)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return fmt.Errorf("create worktree parent directory %q: %w", parent, err)
	}
	_, err := repository.git("worktree", "move", currentPath, targetPath)
	return err
}

// pathIsWithin reports whether child is the same as parent or nested under it.
func pathIsWithin(parent string, child string) bool {
	relativePath, err := filepath.Rel(parent, child)
	if err != nil {
		return false
	}
	return relativePath == "." || (relativePath != ".." && !strings.HasPrefix(relativePath, ".."+string(filepath.Separator)))
}

// moveWorktreeViaTemporaryPath moves a worktree to a path nested under its
// current location (e.g. <root>/feature -> <root>/feature/repo).
func moveWorktreeViaTemporaryPath(repository *Repository, currentPath string, targetPath string) error {
	root := worktreeRoot(repository.WorkTree)
	temporaryPath := filepath.Join(root, ".git-wt-migrate-"+filepath.Base(currentPath))
	// Disambiguate when Base collides (nested branch names share final segment).
	if filepath.Clean(temporaryPath) == currentPath || filepath.Clean(temporaryPath) == targetPath {
		temporaryPath = filepath.Join(root, ".git-wt-migrate-tmp")
	}
	if _, err := os.Stat(temporaryPath); err == nil {
		return fmt.Errorf("temporary migration path %q already exists", temporaryPath)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("inspect temporary migration path %q: %w", temporaryPath, err)
	}

	if _, err := repository.git("worktree", "move", currentPath, temporaryPath); err != nil {
		return err
	}
	parent := filepath.Dir(targetPath)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return fmt.Errorf("create worktree parent directory %q: %w", parent, err)
	}
	_, err := repository.git("worktree", "move", temporaryPath, targetPath)
	return err
}

func (huhMigratePrompter) Prompt(input io.Reader, output io.Writer, candidates []migrateCandidate) ([]migrateCandidate, error) {
	selectedNames := make([]string, 0, len(candidates))
	options := lo.Map(candidates, func(candidate migrateCandidate, _ int) huh.Option[string] {
		label := candidate.Name + " ("
		if candidate.CurrentPath == "" {
			label += "create " + candidate.DisplayTargetPath
		} else {
			label += candidate.DisplayCurrentPath + " -> " + candidate.DisplayTargetPath
		}
		label += ")"
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

func migrationCandidatesFromRepository(repository *Repository) ([]migrateCandidate, error) {
	porcelainWorktrees, err := repository.listPorcelainWorktrees()
	if err != nil {
		return nil, err
	}

	mainPath, err := repository.mainWorktreePath()
	if err != nil {
		return nil, err
	}

	currentDirectory, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("get current directory: %w", err)
	}

	branchesByWorktree := make(map[string]string, len(porcelainWorktrees))
	candidates := make([]migrateCandidate, 0)
	for _, porcelainWorktree := range porcelainWorktrees {
		if porcelainWorktree.BranchRef == "" {
			continue
		}

		branchName := porcelainWorktree.branchName()
		if branchName == "" {
			continue
		}

		branchesByWorktree[branchName] = porcelainWorktree.Path
		if filepath.Clean(porcelainWorktree.Path) == filepath.Clean(mainPath) {
			continue
		}

		targetPath := managedWorktreePath(mainPath, branchName)
		if filepath.Clean(porcelainWorktree.Path) == filepath.Clean(targetPath) {
			continue
		}

		candidates = append(candidates, migrateCandidate{
			Action:             "migrate",
			Name:               branchName,
			CurrentPath:        porcelainWorktree.Path,
			TargetPath:         targetPath,
			DisplayCurrentPath: currentRelativePath(currentDirectory, porcelainWorktree.Path),
			DisplayTargetPath:  currentRelativePath(currentDirectory, targetPath),
		})
	}

	branches, err := repository.localBranches()
	if err != nil {
		return nil, err
	}

	for _, branchName := range branches {
		if _, ok := branchesByWorktree[branchName]; ok {
			continue
		}

		targetPath := managedWorktreePath(mainPath, branchName)
		candidates = append(candidates, migrateCandidate{
			Action:            "create",
			Name:              branchName,
			TargetPath:        targetPath,
			DisplayTargetPath: currentRelativePath(currentDirectory, targetPath),
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
