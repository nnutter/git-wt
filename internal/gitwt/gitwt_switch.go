package gitwt

import (
	"fmt"
	"os"
	"slices"
	"strings"

	"github.com/spf13/cobra"
)

type switchCommandOptions struct {
	repoSelection
	create   bool
	noCd     bool
	all      bool
	upstream string
	herdr    bool
	noHerdr  bool
}

func NewSwitchCommand() *cobra.Command {
	options := new(switchCommandOptions)

	command := &cobra.Command{
		Use:               "switch [name]",
		Short:             "Resolve a managed worktree path",
		Args:              cobra.ExactArgs(1),
		RunE:              options.Execute,
		Hidden:            true,
		ValidArgsFunction: completeSwitchWorktreeNames,
	}
	options.addRepoFlag(command)
	command.Flags().BoolVarP(&options.create, "create", "c", false, "Create the worktree if it does not exist")
	command.Flags().BoolVar(&options.noCd, "no-cd", false, "Create without reporting a path to change to")
	command.Flags().BoolVarP(&options.all, "all", "a", false, "Ignore the current worktree repository")
	command.Flags().StringVarP(&options.upstream, "upstream", "u", "", "Upstream branch")
	command.Flags().BoolVar(&options.herdr, "herdr", false, "Also create a Herdr workspace for the new worktree")
	command.Flags().BoolVar(&options.noHerdr, "no-herdr", false, "Do not create a Herdr workspace")
	command.MarkFlagsMutuallyExclusive("herdr", "no-herdr")
	command.MarkFlagsMutuallyExclusive("create", "all")
	return command
}

const switchPathFileEnvVarName = "GIT_WT_SWITCH_PATH_FILE"

func (x *switchCommandOptions) Execute(command *cobra.Command, args []string) error {
	if x.create {
		return x.createAndReport(command, args)
	}

	name := args[0]
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

func completeSwitchWorktreeNames(command *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) > 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	if flagValue(command, "repo") != "" {
		return completeManagedWorktreeNames(command, args, toComplete)
	}

	all, err := command.Flags().GetBool("all")
	if err != nil || !all {
		return completeManagedWorktreeNames(command, args, toComplete)
	}

	return worktreeNamesAcrossRepos(toComplete), cobra.ShellCompDirectiveNoFileComp
}

func worktreeNamesAcrossRepos(toComplete string) []string {
	repos, err := listRegisteredRepos()
	if err != nil {
		return nil
	}

	var names []string
	seen := make(map[string]struct{})
	for _, repo := range repos {
		for _, name := range managedWorktreeNamesOnDisk(repo.Name, toComplete) {
			if _, exists := seen[name]; exists {
				continue
			}
			seen[name] = struct{}{}
			names = append(names, name)
		}
	}
	slices.Sort(names)
	return names
}

func (x *switchCommandOptions) resolveSwitchRepoName(worktreeName string) (string, error) {
	if x.RepoFlag != "" {
		return x.RepoFlag, nil
	}
	if !x.all {
		if repoName := repoNameFromCurrentGitCommonDir(); repoName != "" {
			return repoName, nil
		}
	}
	return inferUniqueRepoForWorktree(worktreeName)
}

func inferUniqueRepoForWorktree(worktreeName string) (string, error) {
	repos, err := listRegisteredRepos()
	if err != nil {
		return "", err
	}

	var matches []string
	for _, repo := range repos {
		worktreePath := managedWorktreePath(repo.Name, worktreeName)
		_, err := os.Stat(worktreePath)
		if err == nil {
			matches = append(matches, repo.Name)
			continue
		}
		if !os.IsNotExist(err) {
			return "", fmt.Errorf("inspect worktree directory %q: %w", worktreePath, err)
		}
	}

	switch len(matches) {
	case 1:
		return matches[0], nil
	case 0:
		return "", fmt.Errorf("worktree %s not found", worktreeName)
	default:
		slices.Sort(matches)
		return "", fmt.Errorf(
			"worktree %q exists in multiple repositories; pass --repo (%s)",
			worktreeName,
			strings.Join(matches, ", "),
		)
	}
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
