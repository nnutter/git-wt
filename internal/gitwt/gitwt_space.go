package gitwt

import (
	"fmt"

	"github.com/spf13/cobra"
)

type spaceCommandOptions struct {
	repoSelection
	currentSpace bool
}

func NewSpaceCommand() *cobra.Command {
	options := new(spaceCommandOptions)

	command := &cobra.Command{
		Use:               "space [name]",
		Short:             "Open a managed Git worktree in Herdr",
		Args:              cobra.MaximumNArgs(1),
		RunE:              options.Execute,
		ValidArgsFunction: completeManagedWorktreeNames,
	}
	options.addRepoFlag(command)
	command.Flags().BoolVar(&options.currentSpace, "current", false, "Define tabs in the current Herdr workspace")
	return command
}

func (x *spaceCommandOptions) Execute(command *cobra.Command, args []string) error {
	worktree, err := x.resolveWorktree(args)
	if err != nil {
		return err
	}
	if x.currentSpace {
		if err := defineCurrentHerdrSpace(command.Context(), worktree); err != nil {
			return err
		}
		return reportDefinedCurrentHerdrSpace(command, worktree.Name)
	}
	if err := openHerdrSpace(command.Context(), worktree); err != nil {
		return err
	}
	return reportOpenedHerdrSpace(command, worktree.Name)
}

func (x *spaceCommandOptions) resolveWorktree(args []string) (managedWorktree, error) {
	repo, repository, err := x.resolve()
	if err != nil {
		return managedWorktree{}, err
	}

	worktrees, err := managedWorktreesFromRepository(repository, repo.Name)
	if err != nil {
		return managedWorktree{}, err
	}

	var name string
	if len(args) == 1 {
		name = args[0]
	}
	return selectManagedWorktree(worktrees, name)
}

func reportOpenedHerdrSpace(command *cobra.Command, worktreeName string) error {
	message := fmt.Sprintf("opened herdr space for %s", worktreeName)
	_, err := fmt.Fprintf(command.ErrOrStderr(), "%s\n", statusStyle.Render(message))
	return err
}

func reportDefinedCurrentHerdrSpace(command *cobra.Command, worktreeName string) error {
	message := fmt.Sprintf("defined herdr tabs in current space for %s", worktreeName)
	_, err := fmt.Fprintf(command.ErrOrStderr(), "%s\n", statusStyle.Render(message))
	return err
}
