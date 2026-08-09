package gitwt

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/samber/lo"
	"github.com/spf13/cobra"
)

type removeCommandOptions struct {
	repoSelection
	force bool
}

func NewRemoveCommand() *cobra.Command {
	options := new(removeCommandOptions)

	command := &cobra.Command{
		Use:               "remove [-f|--force] [name]",
		Short:             "Remove a managed Git worktree",
		Args:              cobra.MaximumNArgs(1),
		RunE:              options.Execute,
		ValidArgsFunction: completeManagedWorktreeNames,
	}

	options.addFlags(command)
	command.Flags().BoolVarP(&options.force, "force", "f", false, "Force removal")

	return command
}

func completeManagedWorktreeNames(command *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) > 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	// Best-effort: use --repo or --current when provided.
	selection := repoSelection{
		RepoFlag:    flagValue(command, "repo"),
		CurrentFlag: flagBool(command, "current"),
	}
	if selection.RepoFlag == "" && !selection.CurrentFlag {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	repo, repository, err := selection.resolve()
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}

	worktrees, err := managedWorktreesFromRepository(repository, repo.Name)
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}

	worktreeNames := lo.FilterMap(worktrees, func(worktree managedWorktree, _ int) (string, bool) {
		return worktree.Name, strings.HasPrefix(worktree.Name, toComplete)
	})

	return worktreeNames, cobra.ShellCompDirectiveNoFileComp
}

func flagValue(command *cobra.Command, name string) string {
	value, err := command.Flags().GetString(name)
	if err != nil {
		return ""
	}
	return value
}

func flagBool(command *cobra.Command, name string) bool {
	value, err := command.Flags().GetBool(name)
	if err != nil {
		return false
	}
	return value
}

func (x *removeCommandOptions) Execute(command *cobra.Command, args []string) error {
	var name string
	if len(args) == 1 {
		name = args[0]
	}
	return x.removeWorktree(command, name, x.force)
}

func (x *removeCommandOptions) removeWorktree(command *cobra.Command, name string, force bool) error {
	repo, repository, err := x.resolve()
	if err != nil {
		return err
	}

	worktrees, err := managedWorktreesFromRepository(repository, repo.Name)
	if err != nil {
		return err
	}

	var worktree managedWorktree
	if name == "" {
		currentDirectory, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("get current directory: %w", err)
		}
		currentRepository, err := openRepository(currentDirectory)
		if err != nil {
			return fmt.Errorf("worktree name is required when not inside a managed worktree: %w", err)
		}
		worktree, err = managedWorktreeForPath(worktrees, currentRepository.WorkTree)
		if err != nil {
			return err
		}
	} else {
		worktree, err = managedWorktreeByName(worktrees, name)
		if err != nil {
			return err
		}
	}
	name = worktree.Name

	worktree, err = enrichManagedWorktree(repository, worktree)
	if err != nil {
		return err
	}

	if !force && !worktree.Clean {
		return fmt.Errorf("worktree %q is not clean", name)
	}
	if !force && !worktree.Merged {
		return fmt.Errorf("branch %q is not merged to %s", name, shortReference(worktree.UpstreamRef))
	}

	currentDirectory, err := os.Getwd()
	if err != nil {
		currentDirectory = ""
	}
	removingCurrentDirectory := currentDirectory != "" && pathIsWithin(worktree.Path, currentDirectory)

	removeArguments := []string{"worktree", "remove"}
	if force {
		removeArguments = append(removeArguments, "--force")
	}
	removeArguments = append(removeArguments, worktree.Path)
	if _, err := repository.git(removeArguments...); err != nil {
		return err
	}
	if err := removeEmptyParents(worktree.Path, worktreeRoot()); err != nil {
		return err
	}

	branchExists, err := repository.branchStillExists(worktree.BranchReference)
	if err != nil {
		return err
	}
	if branchExists {
		if _, err := repository.git("branch", branchDeleteFlag(force), name); err != nil {
			return err
		}
	}

	if _, err := os.Stat(worktree.Path); err == nil {
		if _, writeErr := fmt.Fprintf(command.ErrOrStderr(), "%s\n", warningStyle.Render("warning: worktree directory still exists: "+worktree.Path)); writeErr != nil {
			return writeErr
		}
	}

	message := fmt.Sprintf("removed %s at %s", name, worktree.shortCommitHash())
	if _, err := fmt.Fprintf(command.ErrOrStderr(), "%s\n", statusStyle.Render(message)); err != nil {
		return err
	}
	if removingCurrentDirectory {
		_, err = fmt.Fprintf(
			command.ErrOrStderr(),
			"%s\n",
			warningStyle.Render("current directory was removed"),
		)
		return err
	}
	return nil
}

// removeEmptyParents removes path and empty ancestor directories up to (but not
// including) stopPath.
func removeEmptyParents(path string, stopPath string) error {
	current := filepath.Clean(path)
	stopPath = filepath.Clean(stopPath)

	for {
		if current == stopPath || current == string(filepath.Separator) || current == "." {
			return nil
		}
		if !pathIsWithin(stopPath, current) {
			return nil
		}

		err := os.Remove(current)
		if err == nil {
			current = filepath.Dir(current)
			continue
		}
		if os.IsNotExist(err) {
			current = filepath.Dir(current)
			continue
		}
		if isNotEmptyError(err) {
			return nil
		}
		return fmt.Errorf("remove %q: %w", current, err)
	}
}
