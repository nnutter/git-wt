package gitwt

import (
	"github.com/spf13/cobra"
)

var RootCmd = &cobra.Command{
	Use:   "git-wt",
	Short: "Manage Git worktrees",
	Long:  `A command line tool that uses 'git worktree' under the hood to manage Git worktrees.`,
}

func init() {
	RootCmd.AddCommand(NewCreate())
	RootCmd.AddCommand(NewList())
	RootCmd.AddCommand(NewRemove())
	RootCmd.AddCommand(NewPrune())
}
