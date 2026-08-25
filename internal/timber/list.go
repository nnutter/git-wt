package timber

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
		Use:               "list [@repo]",
		Short:             "List managed Git worktrees",
		Args:              cobra.MaximumNArgs(1),
		RunE:              options.Execute,
		ValidArgsFunction: completeRepoQualifiers,
	}
	return command
}

func (x *listCommandOptions) Execute(command *cobra.Command, args []string) error {
	if err := x.applyRepoArg(args); err != nil {
		return err
	}

	worktrees, err := x.collectWorktrees()
	if err != nil {
		return err
	}

	tableView := newOutputTable("Name", "Repo", "Status", "Commit", "Dirty")
	for _, worktree := range worktrees {
		tableView.Row(
			worktree.Name,
			worktree.Repo,
			worktree.Status,
			worktree.shortCommitHash(),
			strconv.FormatBool(!worktree.Clean),
		)
	}

	_, err = fmt.Fprintln(command.OutOrStdout(), tableView.String())
	return err
}

func (x *listCommandOptions) collectWorktrees() ([]managedWorktree, error) {
	repos, err := x.reposToConsider()
	if err != nil {
		return nil, err
	}
	return collectListedWorktrees(repos)
}

func (x *listCommandOptions) applyRepoArg(args []string) error {
	if len(args) == 0 {
		return nil
	}
	repo, err := parseRepoOnlyArg(args[0])
	if err != nil {
		return err
	}
	x.RepoName = repo
	return nil
}
