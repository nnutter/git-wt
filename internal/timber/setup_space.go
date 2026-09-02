package timber

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"
)

type setupSpaceCommandOptions struct {
	repoSelection
	newSpace bool
}

func NewHerdrCommand() *cobra.Command {
	command := &cobra.Command{
		Use:   "herdr",
		Short: "Manage the Herdr plugin and spaces",
	}
	command.AddCommand(NewHerdrInstallCommand())
	command.AddCommand(NewHerdrSpaceCommand())
	return command
}

func NewHerdrSpaceCommand() *cobra.Command {
	options := new(setupSpaceCommandOptions)

	command := &cobra.Command{
		Use:               "space [name[@repo]]",
		Short:             "Set up a Herdr space for a managed Git worktree",
		Args:              cobra.MaximumNArgs(1),
		RunE:              options.Execute,
		ValidArgsFunction: completeQualifiedWorktreeNames,
	}
	command.Flags().BoolVarP(&options.newSpace, "new", "n", false, "Open a new Herdr workspace")
	return command
}

func (x *setupSpaceCommandOptions) Execute(command *cobra.Command, args []string) error {
	worktree, err := x.resolveWorktree(command.InOrStdin(), args)
	if err != nil {
		return err
	}
	if x.newSpace {
		if err := openHerdrSpace(command.Context(), worktree); err != nil {
			return err
		}
		return reportOpenedHerdrSpace(command, worktree.Name)
	}
	if err := defineCurrentHerdrSpace(command.Context(), worktree); err != nil {
		return err
	}
	return reportDefinedCurrentHerdrSpace(command, worktree.Name)
}

func (x *setupSpaceCommandOptions) resolveWorktree(input io.Reader, args []string) (managedWorktree, error) {
	var raw string
	if len(args) == 1 {
		raw = args[0]
	}
	qualified, err := parseQualifiedName(raw)
	if err != nil {
		return managedWorktree{}, err
	}
	if qualified.Repo != "" {
		x.RepoName = qualified.Repo
	}

	repo, repository, err := x.resolveForWorktree(qualified.Name, input)
	if err != nil {
		return managedWorktree{}, err
	}

	worktrees, err := managedWorktreesFromRepository(repository, repo.Name)
	if err != nil {
		return managedWorktree{}, err
	}

	return selectManagedWorktree(worktrees, qualified.Name)
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
