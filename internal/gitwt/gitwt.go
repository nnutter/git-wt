package gitwt

import (
	"github.com/spf13/cobra"
)

func NewRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "git-wt",
		Short: "Manage Git worktrees",
		Long:  "A command line tool to manage Git worktrees with sibling directory structure.",
	}

	cmd.AddCommand(NewCreateCmd())
	cmd.AddCommand(NewListCmd())
	cmd.AddCommand(NewRemoveCmd())
	cmd.AddCommand(NewPruneCmd())

	return cmd
}
