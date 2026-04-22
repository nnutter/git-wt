package gitwt

import "github.com/spf13/cobra"

var Command = NewRootCommand()

func NewRootCommand() *cobra.Command {
	rootCommand := &cobra.Command{
		Use:           "git-wt",
		Short:         "Manage Git worktrees",
		SilenceErrors: true,
		SilenceUsage:  true,
	}

	rootCommand.AddCommand(NewCreateCommand())
	rootCommand.AddCommand(NewListCommand())
	rootCommand.AddCommand(NewPruneCommand())
	rootCommand.AddCommand(NewRemoveCommand())

	return rootCommand
}
