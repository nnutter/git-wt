package gitwt

import (
	"fmt"
	"strconv"

	"github.com/spf13/cobra"
)

type listCommandOptions struct {
	repoSelection
	all bool
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
	command.Flags().BoolVarP(&options.all, "all", "a", false, "List worktrees from all registered repositories")
	command.MarkFlagsMutuallyExclusive("repo", "all")
	return command
}

func (x *listCommandOptions) Execute(command *cobra.Command, args []string) error {
	worktrees, err := x.collectWorktrees()
	if err != nil {
		return err
	}

	tableView := newOutputTable("Repo", "Name", "Status", "Commit", "Dirty")
	for _, worktree := range worktrees {
		tableView.Row(
			worktree.Repo,
			worktree.Name,
			worktree.Status,
			worktree.shortCommitHash(),
			strconv.FormatBool(!worktree.Clean),
		)
	}

	_, err = fmt.Fprintln(command.OutOrStdout(), tableView.String())
	return err
}

func (x *listCommandOptions) collectWorktrees() ([]managedWorktree, error) {
	repos, err := x.reposToList()
	if err != nil {
		return nil, err
	}
	return collectManagedWorktrees(repos)
}

func (x *listCommandOptions) reposToList() ([]registeredRepo, error) {
	if x.all {
		return listRegisteredRepos()
	}
	return x.reposToConsider()
}
