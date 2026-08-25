package timber

import "github.com/spf13/cobra"

func NewTUICommand() *cobra.Command {
	command := &cobra.Command{
		Use:   "tui",
		Short: "Interactive prompts for timber",
	}
	command.AddCommand(NewTUICreateCommand())
	return command
}
