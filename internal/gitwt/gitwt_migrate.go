package gitwt

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"

	"github.com/charmbracelet/huh"
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
	repository, err := PlainOpenWithOptions(".")
	if err != nil {
		return err
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
		if candidate.CurrentPath == "" {
			if _, err := repository.git("worktree", "add", candidate.TargetPath, candidate.Name); err != nil {
				return err
			}
		} else {
			if _, err := repository.git("worktree", "move", candidate.CurrentPath, candidate.TargetPath); err != nil {
				return err
			}
		}

		message := fmt.Sprintf("%sd %s to %s", candidate.Action, candidate.Name, candidate.TargetPath)
		if _, err := fmt.Fprintf(command.ErrOrStderr(), "%s\n", statusStyle.Render(message)); err != nil {
			return err
		}
	}

	return nil
}

func (huhMigratePrompter) Prompt(input io.Reader, output io.Writer, candidates []migrateCandidate) ([]migrateCandidate, error) {
	selectedNames := make([]string, 0, len(candidates))
	options := make([]huh.Option[string], 0, len(candidates))
	for _, candidate := range candidates {
		label := candidate.Name + " ("
		if candidate.CurrentPath == "" {
			label += "create " + candidate.DisplayTargetPath
		} else {
			label += candidate.DisplayCurrentPath + " -> " + candidate.DisplayTargetPath
		}
		label += ")"
		options = append(options, huh.NewOption(label, candidate.Name).Selected(true))
	}

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

	sort.Slice(candidates, func(leftIndex int, rightIndex int) bool {
		return candidates[leftIndex].Name < candidates[rightIndex].Name
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
