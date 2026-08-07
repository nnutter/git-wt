package gitwt

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/samber/lo"
	"github.com/spf13/cobra"
)

type removeCommandOptions struct {
	force bool
}

func NewRemoveCommand() *cobra.Command {
	options := &removeCommandOptions{}

	command := &cobra.Command{
		Use:               "remove [-f|--force] [name]",
		Short:             "Remove a managed Git worktree",
		Args:              cobra.MaximumNArgs(1),
		RunE:              options.Execute,
		ValidArgsFunction: completeManagedWorktreeNames,
	}

	command.Flags().BoolVarP(&options.force, "force", "f", false, "Force removal")

	return command
}

func completeManagedWorktreeNames(command *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) > 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	repository, err := openRepository(".")
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}

	worktrees, _, err := managedWorktreesFromRepository(repository)
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}

	worktreeNames := lo.FilterMap(worktrees, func(worktree managedWorktree, _ int) (string, bool) {
		return worktree.Name, !worktree.Main && strings.HasPrefix(worktree.Name, toComplete)
	})

	return worktreeNames, cobra.ShellCompDirectiveNoFileComp
}

func (x *removeCommandOptions) Execute(command *cobra.Command, args []string) error {
	var name string
	if len(args) == 1 {
		name = args[0]
	}
	return x.removeWorktree(command, name, x.force)
}

func (x *removeCommandOptions) removeWorktree(command *cobra.Command, name string, force bool) error {
	repository, err := openRepository(".")
	if err != nil {
		return err
	}
	currentWorkTree := repository.WorkTree

	worktrees, mainPath, err := managedWorktreesFromRepository(repository)
	if err != nil {
		return err
	}

	if filepath.Clean(currentWorkTree) != filepath.Clean(mainPath) {
		repository, err = openRepository(mainPath)
		if err != nil {
			return err
		}
	}

	var worktree managedWorktree
	if name == "" {
		if filepath.Clean(currentWorkTree) == filepath.Clean(mainPath) {
			return fmt.Errorf("cannot remove main worktree")
		}
		worktree, err = managedWorktreeForPath(worktrees, currentWorkTree)
	} else {
		mainBranch, mainBranchErr := repository.mainWorktreeBranch()
		if mainBranchErr != nil {
			return mainBranchErr
		}
		if name == mainBranch {
			return fmt.Errorf("cannot remove main worktree")
		}
		worktree, err = managedWorktreeByName(worktrees, name)
	}
	if err != nil {
		return err
	}
	if worktree.Main {
		return fmt.Errorf("cannot remove main worktree")
	}
	name = worktree.Name

	worktree, err = enrichManagedWorktree(repository, worktree)
	if err != nil {
		return err
	}

	if !force && !worktree.Clean {
		return fmt.Errorf("worktree %q is not clean", name)
	}
	if !force && !worktree.Merged {
		return fmt.Errorf("branch %q is not merged to %s", name, shortReference(worktree.UpstreamRef))
	}

	removeArguments := []string{"worktree", "remove"}
	if force {
		removeArguments = append(removeArguments, "--force")
	}
	removeArguments = append(removeArguments, worktree.Path)
	if _, err := repository.git(removeArguments...); err != nil {
		return err
	}
	if err := removeEmptyParents(worktree.Path, worktreeRoot(repository.WorkTree)); err != nil {
		return err
	}

	branchExists, err := repository.branchStillExists(worktree.BranchReference)
	if err != nil {
		return err
	}
	if branchExists {
		if _, err := repository.git("branch", branchDeleteFlag(force), name); err != nil {
			return err
		}
	}

	if _, err := os.Stat(worktree.Path); err == nil {
		if _, writeErr := fmt.Fprintf(command.ErrOrStderr(), "%s\n", warningStyle.Render("warning: worktree directory still exists: "+worktree.Path)); writeErr != nil {
			return writeErr
		}
	}

	message := fmt.Sprintf("removed %s at %s", name, worktree.shortCommitHash())
	_, err = fmt.Fprintf(command.ErrOrStderr(), "%s\n", statusStyle.Render(message))
	return err
}
