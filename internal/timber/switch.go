package timber

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

type switchCommandOptions struct {
	repoSelection
	create   bool
	noCd     bool
	upstream string
	herdr    bool
	noHerdr  bool
}

func NewSwitchCommand(runtime Runtime) *cobra.Command {
	options := &switchCommandOptions{runtime: runtime}

	command := &cobra.Command{
		Use:               "switch [name[@repo]]",
		Aliases:           []string{"sw"},
		Short:             "Resolve a managed worktree path",
		Args:              cobra.ExactArgs(1),
		RunE:              options.Execute,
		Hidden:            true,
		ValidArgsFunction: runtime.completeQualifiedWorktreeNames,
	}
	command.Flags().BoolVarP(&options.create, "create", "c", false, "Create the worktree if it does not exist")
	command.Flags().BoolVar(&options.noCd, "no-cd", false, "Create without reporting a path to change to")
	command.Flags().StringVarP(&options.upstream, "upstream", "u", "", "Upstream branch")
	command.Flags().BoolVar(&options.herdr, "herdr", false, "Also create a Herdr workspace for the new worktree")
	command.Flags().BoolVar(&options.noHerdr, "no-herdr", false, "Do not create a Herdr workspace")
	command.MarkFlagsMutuallyExclusive("herdr", "no-herdr")
	return command
}

const switchPathFileEnvVarName = "TIMBER_SWITCH_PATH_FILE"

func (x *switchCommandOptions) Execute(command *cobra.Command, args []string) error {
	qualified, err := x.runtime.parseQualifiedName(args[0])
	if err != nil {
		return err
	}
	if qualified.Repo != "" {
		x.RepoName = qualified.Repo
	}

	if x.create {
		return x.createAndReport(command, nameArgs(qualified.Name))
	}

	name := qualified.Name
	if name == "" {
		return fmt.Errorf("worktree name is required")
	}
	repoName, err := x.resolveSwitchRepoName(name)
	if err != nil {
		return err
	}

	worktreePath := x.runtime.managedWorktreePath(repoName, name)
	if _, err := os.Stat(worktreePath); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("worktree %s not found at %s", name, worktreePath)
		}
		return fmt.Errorf("inspect worktree directory %q: %w", worktreePath, err)
	}

	if err := x.runtime.reportAlreadyInWorktree(command, name, worktreePath); err != nil {
		return err
	}
	return x.runtime.reportSwitchWorktreePath(command, worktreePath)
}

func (x *switchCommandOptions) createAndReport(command *cobra.Command, args []string) error {
	createOptions := &createCommandOptions{repoSelection: x.repoSelection}
	createOptions.upstream = x.upstream
	createOptions.herdr = x.herdr
	createOptions.noHerdr = x.noHerdr

	worktreePath, err := createOptions.createWorktree(command, args)
	if err != nil {
		return err
	}
	if x.noCd || createOptions.shouldCreateHerdrWorkspace() {
		return nil
	}
	return x.runtime.reportSwitchWorktreePath(command, worktreePath)
}

func (x *switchCommandOptions) resolveSwitchRepoName(worktreeName string) (string, error) {
	if x.RepoName != "" {
		return x.RepoName, nil
	}
	return x.runtime.inferUniqueRepoForWorktree(worktreeName)
}

func (x Runtime) reportAlreadyInWorktree(command *cobra.Command, name string, worktreePath string) error {
	currentDirectory := x.CurrentDirectory
	same, err := samePath(currentDirectory, worktreePath)
	if err != nil || !same {
		return err
	}
	_, err = fmt.Fprintf(command.ErrOrStderr(), "Already in %s\n", name)
	return err
}

func (x Runtime) reportSwitchWorktreePath(command *cobra.Command, worktreePath string) error {
	if pathFile := x.SwitchPathFile; pathFile != "" {
		if err := x.writePathFile(pathFile, worktreePath); err != nil {
			return fmt.Errorf("write switch worktree path file: %w", err)
		}
		return nil
	}

	_, err := fmt.Fprintln(command.OutOrStdout(), worktreePath)
	return err
}
