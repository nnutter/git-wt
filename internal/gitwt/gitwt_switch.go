package gitwt

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

type switchCommandOptions struct {
	repoSelection
}

func NewSwitchCommand() *cobra.Command {
	options := new(switchCommandOptions)

	command := &cobra.Command{
		Use:               "switch [name]",
		Short:             "Resolve a managed worktree path",
		Args:              cobra.ExactArgs(1),
		RunE:              options.Execute,
		Hidden:            true,
		ValidArgsFunction: completeManagedWorktreeNames,
	}
	options.addRepoFlag(command)
	return command
}

const switchPathFileEnvVarName = "GIT_WT_SWITCH_PATH_FILE"

func (x *switchCommandOptions) Execute(command *cobra.Command, args []string) error {
	name := args[0]
	repoName, err := x.resolveSwitchRepoName()
	if err != nil {
		return err
	}

	worktreePath := managedWorktreePath(repoName, name)
	if _, err := os.Stat(worktreePath); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("Worktree %s not found at %s", name, worktreePath)
		}
		return fmt.Errorf("inspect worktree directory %q: %w", worktreePath, err)
	}

	if err := reportAlreadyInWorktree(command, name, worktreePath); err != nil {
		return err
	}
	return reportSwitchWorktreePath(command, worktreePath)
}

func (x *switchCommandOptions) resolveSwitchRepoName() (string, error) {
	if x.RepoFlag != "" {
		return x.RepoFlag, nil
	}
	repoName := repoNameFromCurrentGitCommonDir()
	if repoName == "" {
		return "", fmt.Errorf("Not inside a registered repository worktree; pass --repo")
	}
	return repoName, nil
}

func reportAlreadyInWorktree(command *cobra.Command, name string, worktreePath string) error {
	currentDirectory, err := os.Getwd()
	if err != nil {
		return nil
	}
	same, err := samePath(currentDirectory, worktreePath)
	if err != nil || !same {
		return err
	}
	_, err = fmt.Fprintf(command.ErrOrStderr(), "Already in %s\n", name)
	return err
}

func reportSwitchWorktreePath(command *cobra.Command, worktreePath string) error {
	if pathFile := os.Getenv(switchPathFileEnvVarName); pathFile != "" {
		if err := os.WriteFile(pathFile, []byte(worktreePath+"\n"), 0o600); err != nil {
			return fmt.Errorf("write switch worktree path file: %w", err)
		}
		return nil
	}

	_, err := fmt.Fprintln(command.OutOrStdout(), worktreePath)
	return err
}
