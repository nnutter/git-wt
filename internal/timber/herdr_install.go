package timber

import (
	"fmt"
	"path/filepath"

	"github.com/nnutter/timber/herdr"
	"github.com/spf13/cobra"
)

const (
	herdrPluginID            = "nnutter.timber"
	herdrPluginDirectoryName = "timber"
	herdrKeybindingTOML      = `[[keys.command]]
key = "prefix+shift+s"
type = "plugin_action"
command = "nnutter.timber.open"
description = "create or open timber space"
`
)

type herdrInstallCommandOptions struct{}

func NewHerdrInstallCommand() *cobra.Command {
	options := new(herdrInstallCommandOptions)

	return &cobra.Command{
		Use:   "install",
		Short: "Install the Herdr plugin and print keybinding instructions",
		Args:  cobra.NoArgs,
		RunE:  options.Execute,
	}
}

func (x *herdrInstallCommandOptions) Execute(command *cobra.Command, args []string) error {
	destination := herdrPluginInstallDirectory()
	if err := herdr.WritePlugin(destination); err != nil {
		return err
	}
	if _, err := runHerdr(command.Context(), "plugin", "link", destination, "--enabled"); err != nil {
		return err
	}
	return reportHerdrPluginInstall(command, destination)
}

func herdrPluginInstallDirectory() string {
	return filepath.Join(xdgConfigHome(), "herdr", "plugins", herdrPluginDirectoryName)
}

func herdrConfigFilePath() string {
	return filepath.Join(xdgConfigHome(), "herdr", "config.toml")
}

func reportHerdrPluginInstall(command *cobra.Command, destination string) error {
	status := command.ErrOrStderr()
	if _, err := fmt.Fprintf(status, "%s\n", statusStyle.Render("installed herdr plugin to "+destination)); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(status, "%s\n", statusStyle.Render("linked herdr plugin "+herdrPluginID)); err != nil {
		return err
	}

	_, err := fmt.Fprintf(
		command.OutOrStdout(),
		"Add this keybinding to %s:\n\n%s",
		herdrConfigFilePath(),
		herdrKeybindingTOML,
	)
	return err
}
