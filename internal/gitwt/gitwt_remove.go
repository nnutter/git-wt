package gitwt

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

type removeCommandOptions struct {
	force bool
}

func NewRemoveCommand() *cobra.Command {
	options := &removeCommandOptions{}

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
	repository, err := PlainOpenWithOptions(".")
	if err != nil {
		return err
	}

	worktrees, _, err := managedWorktreesFromRepository(repository)
	if err != nil {
		return err
	}

	worktree, err := managedWorktreeByName(worktrees, name)
	if err != nil {
		return err
	}

	worktree, err = enrichManagedWorktree(repository, worktree)
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
	if _, err := repository.git(removeArguments...); err != nil {
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

	_, err = fmt.Fprintf(command.ErrOrStderr(), "%s\n", statusStyle.Render("removed "+name))
	return err
}
