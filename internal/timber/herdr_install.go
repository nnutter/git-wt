package timber

import (
	"github.com/spf13/cobra"

	"github.com/nnutter/timber/herdr"
)

const (
	herdrPluginID            = "nnutter.timber"
	herdrPluginDirectoryName = "timber"
	herdrKeybindingTOML      = `[[keys.command]]
key = "prefix+shift+s"
type = "plugin_action"
command = "nnutter.timber.open"
description = "open or create Timber Space"
`
)

type herdrInstallCommandOptions struct {
	runtime Runtime
}

func NewHerdrInstallCommand(runtime Runtime) *cobra.Command {
	options := &herdrInstallCommandOptions{runtime: runtime}

	return &cobra.Command{
		Use:   "install",
		Short: "Install the Herdr plugin and print keybinding instructions",
		Args:  cobra.NoArgs,
		RunE:  options.Execute,
	}
}

func (x *herdrInstallCommandOptions) Execute(command *cobra.Command, args []string) error {
	destination := x.runtime.herdrPluginInstallDirectory()
	if err := herdr.WritePlugin(destination); err != nil {
		return err
	}
	if _, err := x.runtime.runHerdr(command.Context(), "plugin", "link", destination, "--enabled"); err != nil {
		return err
	}
	return x.runtime.reportHerdrPluginInstall(command, destination)
}
