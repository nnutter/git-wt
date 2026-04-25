package gitwt

import (
	"github.com/spf13/cobra"
)

type shellCommandOptions struct{}

func NewShellCommand() *cobra.Command {
	options := &shellCommandOptions{}

	command := &cobra.Command{
		Use:   `shell`,
		Short: `Generate shell integration for worktrees`,
		Args:  cobra.NoArgs,
		RunE:  options.Execute,
	}
	command.CompletionOptions.HiddenDefaultCmd = true

	command.AddCommand(NewZshCommand())

	return command
}

func (x *shellCommandOptions) Execute(command *cobra.Command, args []string) error {
	return command.Help()
}
