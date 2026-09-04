package timber

import (
	"fmt"
	"path/filepath"

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
description = "create or open timber space"
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

func (x Runtime) herdrPluginInstallDirectory() string {
	return filepath.Join(x.xdgConfigHome(), "herdr", "plugins", herdrPluginDirectoryName)
}

func (x Runtime) herdrConfigFilePath() string {
	return filepath.Join(x.xdgConfigHome(), "herdr", "config.toml")
}

func (x Runtime) reportHerdrPluginInstall(command *cobra.Command, destination string) error {
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
		x.herdrConfigFilePath(),
		herdrKeybindingTOML,
	)
	return err
}
