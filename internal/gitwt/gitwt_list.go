package gitwt

import (
	"fmt"
	"slices"
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
	command.Flags().BoolVar(&options.all, "all", false, "List worktrees from all registered repositories")
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

	worktrees := make([]managedWorktree, 0)
	for _, repo := range repos {
		repository, err := openBareRepository(repo.BarePath)
		if err != nil {
			return nil, err
		}

		repoWorktrees, err := managedWorktreesFromRepository(repository, repo.Name)
		if err != nil {
			return nil, err
		}

		for _, worktree := range repoWorktrees {
			enrichedWorktree, err := enrichManagedWorktree(repository, worktree)
			if err != nil {
				return nil, err
			}
			worktrees = append(worktrees, enrichedWorktree)
		}
	}

	slices.SortFunc(worktrees, compareManagedWorktrees)
	return worktrees, nil
}

func (x *listCommandOptions) reposToList() ([]registeredRepo, error) {
	if x.RepoFlag != "" {
		repo, err := registeredRepoByName(x.RepoFlag)
		if err != nil {
			return nil, err
		}
		return []registeredRepo{repo}, nil
	}

	if !x.all {
		if repo, _, err := x.tryResolveCurrent(); err == nil {
			return []registeredRepo{repo}, nil
		}
	}

	return listRegisteredRepos()
}
