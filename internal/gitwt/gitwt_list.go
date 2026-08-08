package gitwt

import (
	"fmt"
	"strconv"

	"charm.land/lipgloss/v2"
	"charm.land/lipgloss/v2/table"
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
	options.addFlags(command)
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

	tableView := table.New().
		Headers("Name", "Status", "Commit", "Dirty").
		Border(lipgloss.NormalBorder()).
		BorderHeader(true).
		StyleFunc(func(row int, column int) lipgloss.Style {
			if row == table.HeaderRow {
				return lipgloss.NewStyle().Bold(true).PaddingLeft(1).PaddingRight(1)
			}
			return lipgloss.NewStyle().PaddingLeft(1).PaddingRight(1)
		})

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
