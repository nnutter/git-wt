package gitwt

import (
	"fmt"
	"strconv"

	"github.com/spf13/cobra"
)

type listCommandOptions struct {
	repoSelection
}

func NewListCommand() *cobra.Command {
	options := new(listCommandOptions)

	command := &cobra.Command{
		Use:   "list",
		Short: "List managed Git worktrees",
		Args:  cobra.NoArgs,
		RunE:  options.Execute,
	}
	options.addRepoFlag(command)
	return command
}

func (x *listCommandOptions) Execute(command *cobra.Command, args []string) error {
	repo, repository, err := x.resolve()
	if err != nil {
		return err
	}

	worktrees, err := managedWorktreesFromRepository(repository, repo.Name)
	if err != nil {
		return err
	}

	enrichedWorktrees := make([]managedWorktree, 0, len(worktrees))
	for _, worktree := range worktrees {
		enrichedWorktree, err := enrichManagedWorktree(repository, worktree)
		if err != nil {
			return err
		}
		enrichedWorktrees = append(enrichedWorktrees, enrichedWorktree)
	}

	tableView := newOutputTable("Name", "Status", "Commit", "Dirty")
	for _, worktree := range enrichedWorktrees {
		tableView.Row(
			worktree.Name,
			worktree.Status,
			worktree.shortCommitHash(),
			strconv.FormatBool(!worktree.Clean),
		)
	}

	_, err = fmt.Fprintln(command.OutOrStdout(), tableView.String())
	return err
}
