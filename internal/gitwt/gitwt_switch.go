package gitwt

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

func NewSwitchCommand() *cobra.Command {
	options := new(switchCommandOptions)

	command := &cobra.Command{
		Use:               "switch [name[@repo]]",
		Short:             "Resolve a managed worktree path",
		Args:              cobra.ExactArgs(1),
		RunE:              options.Execute,
		Hidden:            true,
		ValidArgsFunction: completeQualifiedWorktreeNames,
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
	qualified, err := parseQualifiedName(args[0])
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

	worktreePath := managedWorktreePath(repoName, name)
	if _, err := os.Stat(worktreePath); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("worktree %s not found at %s", name, worktreePath)
		}
		return fmt.Errorf("inspect worktree directory %q: %w", worktreePath, err)
	}

	if err := reportAlreadyInWorktree(command, name, worktreePath); err != nil {
		return err
	}
	return reportSwitchWorktreePath(command, worktreePath)
}

func (x *switchCommandOptions) createAndReport(command *cobra.Command, args []string) error {
	createOptions := new(createCommandOptions)
	createOptions.repoSelection = x.repoSelection
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
	return reportSwitchWorktreePath(command, worktreePath)
}

func (x *switchCommandOptions) resolveSwitchRepoName(worktreeName string) (string, error) {
	if x.RepoName != "" {
		return x.RepoName, nil
	}
	return inferUniqueRepoForWorktree(worktreeName)
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
