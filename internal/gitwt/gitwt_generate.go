package gitwt

import (
	"github.com/spf13/cobra"
)

type generateCommandOptions struct{}

func NewGenerateCommand() *cobra.Command {
	options := &generateCommandOptions{}

	command := &cobra.Command{
		Use:   `generate`,
		Short: `Generate shell integration wrapping timber`,
		Args:  cobra.NoArgs,
		RunE:  options.Execute,
	}
	command.CompletionOptions.HiddenDefaultCmd = true

	command.AddCommand(NewZshCommand())

	return command
}

func (x *generateCommandOptions) Execute(command *cobra.Command, args []string) error {
	return command.Help()
}
