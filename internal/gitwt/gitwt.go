package gitwt

import "github.com/spf13/cobra"

var Command = NewRootCommand()

func NewRootCommand() *cobra.Command {
	rootCommand := &cobra.Command{
		Use:           "timber",
		Short:         "Manage Git worktrees",
		SilenceErrors: true,
		SilenceUsage:  true,
	}
	rootCommand.CompletionOptions.HiddenDefaultCmd = true

	rootCommand.AddCommand(NewCreateCommand())
	rootCommand.AddCommand(NewListCommand())
	rootCommand.AddCommand(NewMigrateCommand())
	rootCommand.AddCommand(NewPruneCommand())
	rootCommand.AddCommand(NewRemoveCommand())
	rootCommand.AddCommand(NewRepoCommand())
	rootCommand.AddCommand(NewSetupSpaceCommand())
	rootCommand.AddCommand(NewSwitchCommand())
	rootCommand.AddCommand(NewTUICommand())
	rootCommand.AddCommand(NewGenerateCommand())

	return rootCommand
}
