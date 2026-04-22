package gitwt

import (
	"fmt"

	"charm.land/lipgloss/v2"
	"charm.land/lipgloss/v2/table"
	"github.com/spf13/cobra"
)

type listCommandOptions struct {
	runner gitRunner
}

func NewListCommand() *cobra.Command {
	options := &listCommandOptions{runner: gitRunner{}}

	return &cobra.Command{
		Use:   "list",
		Short: "List managed Git worktrees",
		Args:  cobra.NoArgs,
		RunE:  options.Execute,
	}
}

func (options *listCommandOptions) Execute(command *cobra.Command, args []string) error {
	worktrees, _, err := managedWorktreesFromRepository(options.runner, ".")
	if err != nil {
		return err
	}

	tableView := table.New().
		Headers("Name", "Path").
		Border(lipgloss.NormalBorder()).
		BorderHeader(true).
		StyleFunc(func(row int, column int) lipgloss.Style {
			if row == table.HeaderRow {
				return lipgloss.NewStyle().Bold(true)
			}
			return lipgloss.NewStyle()
		})

	for _, worktree := range worktrees {
		tableView.Row(worktree.Name, worktree.DisplayPath)
	}

	_, err = fmt.Fprintln(command.OutOrStdout(), tableView.String())
	return err
}
