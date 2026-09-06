package timber

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"github.com/spf13/cobra"
)

const repoRenamePathFileEnvVarName = "TIMBER_RENAME_PATH_FILE"

type (
	renamePathFunc      func(string, string) error
	repairWorktreesFunc func(string, []string) error
)

type repoRenameCommandOptions struct {
	runtime         Runtime
	renamePath      renamePathFunc
	repairWorktrees repairWorktreesFunc
}

type repositoryRenamePlan struct {
	runtime          Runtime
	sourceRepo       registeredRepo
	destinationRepo  registeredRepo
	worktreeMoves    []worktreePathMove
	linkedWorktrees  []linkedWorktreePath
	currentTargetDir string
}

type worktreePathMove struct {
	Source      string
	Destination string
}

type linkedWorktreePath struct {
	Original string
	Renamed  string
}

func NewRepoRenameCommand(runtime Runtime) *cobra.Command {
	options := &repoRenameCommandOptions{
		runtime:    runtime,
		renamePath: os.Rename,
		repairWorktrees: func(barePath string, worktreePaths []string) error {
			return repairWorktrees(runtime, barePath, worktreePaths)
		},
	}
	return &cobra.Command{
		Use:               "rename <old-name> <new-name>",
		Aliases:           []string{"mv"},
		Short:             "Rename a registered repository and its managed worktrees",
		Args:              cobra.ExactArgs(2),
		RunE:              options.Execute,
		ValidArgsFunction: runtime.completeRepoRenameArguments,
	}
}

func (x *repoRenameCommandOptions) Execute(command *cobra.Command, args []string) error {
	plan, err := x.runtime.buildRepositoryRenamePlan(args[0], args[1])
	if err != nil {
		return err
	}

	if err := x.runtime.reportRenamedCurrentPath(plan.currentTargetDir); err != nil {
		return err
	}
	if err := plan.apply(x.renamePath, x.repairWorktrees); err != nil {
		return err
	}

	message := fmt.Sprintf("renamed repository %s to %s", plan.sourceRepo.Name, plan.destinationRepo.Name)
	_, err = fmt.Fprintf(command.ErrOrStderr(), "%s\n", statusStyle.Render(message))
	return err
}

func (x repositoryRenamePlan) validateDestinationRepo() error {
	if _, err := os.Lstat(x.destinationRepo.BarePath); err == nil {
		return fmt.Errorf(
			"repository %q already exists at %s",
			x.destinationRepo.Name,
			x.destinationRepo.BarePath,
		)
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect repository path %q: %w", x.destinationRepo.BarePath, err)
	}
	return nil
}

func (x *repositoryRenamePlan) collectWorktrees() error {
	repository, err := openBareRepository(x.runtime, x.sourceRepo.BarePath)
	if err != nil {
		return err
	}
	worktrees, err := repository.listPorcelainWorktrees()
	if err != nil {
		return err
	}

	for _, worktree := range worktrees {
		sameAsBareRepo, err := samePath(worktree.Path, x.sourceRepo.BarePath)
		if err != nil {
			return err
		}
		if sameAsBareRepo {
			continue
		}
		if worktree.Prunable != "" {
			return fmt.Errorf("linked worktree at %q is prunable: %s", worktree.Path, worktree.Prunable)
		}
		if err := validateLinkedWorktree(x.runtime, worktree.Path, x.sourceRepo.BarePath); err != nil {
			return err
		}

		renamedPath := worktree.Path
		branchName := worktree.branchName()
		if branchName != "" {
			expectedSource := x.runtime.managedWorktreePath(x.sourceRepo.Name, branchName)
			managed, err := samePath(worktree.Path, expectedSource)
			if err != nil {
				return err
			}
			if managed {
				renamedPath = x.runtime.managedWorktreePath(x.destinationRepo.Name, branchName)
				x.worktreeMoves = append(x.worktreeMoves, worktreePathMove{
					Source:      worktree.Path,
					Destination: renamedPath,
				})
			}
		}

		x.linkedWorktrees = append(x.linkedWorktrees, linkedWorktreePath{
			Original: worktree.Path,
			Renamed:  renamedPath,
		})
	}
	return nil
}

func validateLinkedWorktree(runtime Runtime, worktreePath string, expectedBarePath string) error {
	repository, err := openRepository(runtime, worktreePath)
	if err != nil {
		return fmt.Errorf("open linked worktree %q: %w", worktreePath, err)
	}
	commonDir, err := repository.commonGitDir()
	if err != nil {
		return err
	}
	same, err := samePath(commonDir, expectedBarePath)
	if err != nil {
		return err
	}
	if !same {
		return fmt.Errorf("linked worktree %q uses unexpected common Git directory %q", worktreePath, commonDir)
	}
	return nil
}

func (x repositoryRenamePlan) validateWorktreeDestinations() error {
	for _, move := range x.worktreeMoves {
		if _, err := os.Lstat(move.Destination); err == nil {
			return fmt.Errorf("worktree directory %q already exists", move.Destination)
		} else if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("inspect worktree directory %q: %w", move.Destination, err)
		}
	}
	return nil
}

func (x *repositoryRenamePlan) findCurrentTargetDirectory() error {
	currentDirectory := x.runtime.CurrentDirectory
	for _, move := range x.worktreeMoves {
		if !pathIsWithin(move.Source, currentDirectory) {
			continue
		}
		relativePath, err := filepath.Rel(move.Source, currentDirectory)
		if err != nil {
			return fmt.Errorf("resolve current worktree subdirectory: %w", err)
		}
		x.currentTargetDir = filepath.Join(move.Destination, relativePath)
		return nil
	}
	return nil
}

func (x repositoryRenamePlan) apply(renamePath renamePathFunc, repairWorktrees repairWorktreesFunc) error {
	completedMoves := make([]worktreePathMove, 0, len(x.worktreeMoves))
	for _, move := range x.worktreeMoves {
		if err := ensureDirectory(filepath.Dir(move.Destination)); err != nil {
			return errors.Join(err, x.rollback(renamePath, repairWorktrees, completedMoves, false))
		}
		if err := renamePath(move.Source, move.Destination); err != nil {
			return errors.Join(
				fmt.Errorf("rename worktree %q to %q: %w", move.Source, move.Destination, err),
				x.rollback(renamePath, repairWorktrees, completedMoves, false),
			)
		}
		completedMoves = append(completedMoves, move)
	}

	if err := renamePath(x.sourceRepo.BarePath, x.destinationRepo.BarePath); err != nil {
		return errors.Join(
			fmt.Errorf("rename bare repository %q to %q: %w", x.sourceRepo.BarePath, x.destinationRepo.BarePath, err),
			x.rollback(renamePath, repairWorktrees, completedMoves, false),
		)
	}

	if err := x.repairAndVerify(repairWorktrees, x.destinationRepo.BarePath, renamedLinkedPaths(x.linkedWorktrees)); err != nil {
		return errors.Join(err, x.rollback(renamePath, repairWorktrees, completedMoves, true))
	}
	for _, move := range x.worktreeMoves {
		if err := x.runtime.removeEmptySourceParents(move.Source); err != nil {
			return err
		}
	}
	return nil
}

func (x repositoryRenamePlan) rollback(
	renamePath renamePathFunc,
	repairWorktrees repairWorktreesFunc,
	completedMoves []worktreePathMove,
	bareRepoMoved bool,
) error {
	var rollbackErrors []error
	if bareRepoMoved {
		if err := renamePath(x.destinationRepo.BarePath, x.sourceRepo.BarePath); err != nil {
			rollbackErrors = append(rollbackErrors, fmt.Errorf("restore bare repository: %w", err))
			return errors.Join(rollbackErrors...)
		}
	}

	for _, move := range slices.Backward(completedMoves) {
		if err := ensureDirectory(filepath.Dir(move.Source)); err != nil {
			rollbackErrors = append(rollbackErrors, fmt.Errorf("restore worktree parent %q: %w", move.Source, err))
			continue
		}
		if err := renamePath(move.Destination, move.Source); err != nil {
			rollbackErrors = append(rollbackErrors, fmt.Errorf("restore worktree %q: %w", move.Source, err))
		}
	}
	if err := repairWorktrees(x.sourceRepo.BarePath, originalLinkedPaths(x.linkedWorktrees)); err != nil {
		rollbackErrors = append(rollbackErrors, fmt.Errorf("repair restored worktrees: %w", err))
	}
	return errors.Join(rollbackErrors...)
}

func (x repositoryRenamePlan) repairAndVerify(
	repairWorktrees repairWorktreesFunc,
	barePath string,
	worktreePaths []string,
) error {
	if err := repairWorktrees(barePath, worktreePaths); err != nil {
		return err
	}
	for _, worktreePath := range worktreePaths {
		if err := validateLinkedWorktree(x.runtime, worktreePath, barePath); err != nil {
			return fmt.Errorf("verify renamed repository: %w", err)
		}
	}
	return nil
}

func repairWorktrees(runtime Runtime, barePath string, worktreePaths []string) error {
	if len(worktreePaths) == 0 {
		return nil
	}
	repository, err := openBareRepository(runtime, barePath)
	if err != nil {
		return err
	}
	arguments := append([]string{"worktree", "repair"}, worktreePaths...)
	if _, err := repository.git(arguments...); err != nil {
		return fmt.Errorf("repair linked worktrees: %w", err)
	}
	return nil
}

func renamedLinkedPaths(worktrees []linkedWorktreePath) []string {
	paths := make([]string, 0, len(worktrees))
	for _, worktree := range worktrees {
		paths = append(paths, worktree.Renamed)
	}
	return paths
}

func originalLinkedPaths(worktrees []linkedWorktreePath) []string {
	paths := make([]string, 0, len(worktrees))
	for _, worktree := range worktrees {
		paths = append(paths, worktree.Original)
	}
	return paths
}
