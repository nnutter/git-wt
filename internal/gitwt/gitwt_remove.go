package gitwt

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

type removeCommandOptions struct {
	force  bool
	runner gitRunner
}

func NewRemoveCommand() *cobra.Command {
	options := &removeCommandOptions{runner: gitRunner{}}

	command := &cobra.Command{
		Use:   "remove [-f|--force] <name>",
		Short: "Remove a managed Git worktree",
		Args:  cobra.ExactArgs(1),
		RunE:  options.Execute,
	}

	command.Flags().BoolVarP(&options.force, "force", "f", false, "Force removal")

	return command
}

func (options *removeCommandOptions) Execute(command *cobra.Command, args []string) error {
	return options.removeWorktree(command, args[0], options.force)
}

func (options *removeCommandOptions) removeWorktree(command *cobra.Command, name string, force bool) error {
	worktrees, repoPath, err := managedWorktreesFromRepository(options.runner, ".")
	if err != nil {
		return err
	}

	worktree, err := managedWorktreeByName(worktrees, name)
	if err != nil {
		return err
	}

	worktree, err = enrichManagedWorktree(options.runner, repoPath, worktree)
	if err != nil {
		return err
	}

	if !force && !worktree.Clean {
		return fmt.Errorf("worktree %q is not clean", name)
	}
	if !force && !worktree.Merged {
		return fmt.Errorf("branch %q is not merged to %s", name, worktree.UpstreamRef.Short())
	}

	if _, err := fmt.Fprintf(command.ErrOrStderr(), "%s\n", statusStyle.Render("removing "+name)); err != nil {
		return err
	}

	removeArguments := []string{"worktree", "remove"}
	if force {
		removeArguments = append(removeArguments, "--force")
	}
	removeArguments = append(removeArguments, worktree.Path)
	if _, err := options.runner.Run(repoPath, removeArguments...); err != nil {
		return err
	}

	repository, err := openRepository(repoPath)
	if err != nil {
		return err
	}

	branchExists, err := branchStillExists(repository, worktree.BranchReference)
	if err != nil {
		return err
	}
	if branchExists {
		if _, err := options.runner.Run(repoPath, "branch", branchDeleteFlag(force), name); err != nil {
			return err
		}
	}

	if _, err := os.Stat(worktree.Path); err == nil {
		if _, writeErr := fmt.Fprintf(command.ErrOrStderr(), "%s\n", warningStyle.Render("warning: worktree directory still exists: "+worktree.Path)); writeErr != nil {
			return writeErr
		}
	}

	_, err = fmt.Fprintf(command.ErrOrStderr(), "%s\n", statusStyle.Render("removed "+name))
	return err
}
