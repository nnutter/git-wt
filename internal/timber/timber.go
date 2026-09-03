package timber

import "github.com/spf13/cobra"

func NewRootCommand(runtime Runtime) *cobra.Command {
	rootCommand := &cobra.Command{
		Use:           "timber",
		Short:         "Manage Git worktrees",
		SilenceErrors: true,
		SilenceUsage:  true,
	}
	rootCommand.CompletionOptions.HiddenDefaultCmd = true

	rootCommand.AddCommand(NewCreateCommand(runtime))
	rootCommand.AddCommand(NewListCommand(runtime))
	rootCommand.AddCommand(NewMigrateCommand(runtime))
	rootCommand.AddCommand(NewPruneCommand(runtime))
	rootCommand.AddCommand(NewRemoveCommand(runtime))
	rootCommand.AddCommand(NewRepoCommand(runtime))
	rootCommand.AddCommand(NewHerdrCommand(runtime))
	rootCommand.AddCommand(NewSwitchCommand(runtime))
	rootCommand.AddCommand(NewTUICommand(runtime))
	rootCommand.AddCommand(NewGenerateCommand(runtime))

	return rootCommand
}
