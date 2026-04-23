package gitwt

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

type createCommandOptions struct {
	upstream string
}

func NewCreateCommand() *cobra.Command {
	options := &createCommandOptions{}

	command := &cobra.Command{
		Use:   "create <name>",
		Short: "Create a managed Git worktree",
		Args:  cobra.ExactArgs(1),
		RunE:  options.Execute,
	}

	command.Flags().StringVarP(&options.upstream, "upstream", "u", "", "Upstream branch")

	return command
}

func (options *createCommandOptions) Execute(command *cobra.Command, args []string) error {
	branchName := args[0]
	repository, err := PlainOpenWithOptions(".")
	if err != nil {
		return err
	}

	branchAlreadyExists, err := repository.branchExists(branchName)
	if err != nil {
		return err
	}
	if branchAlreadyExists {
		return fmt.Errorf("branch %q already exists", branchName)
	}

	_, mainPath, err := managedWorktreesFromRepository(repository)
	if err != nil {
		return err
	}

	worktreePath := managedWorktreePath(mainPath, branchName)
	if _, err := os.Stat(worktreePath); err == nil {
		return fmt.Errorf("worktree directory %q already exists", worktreePath)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("inspect worktree directory %q: %w", worktreePath, err)
	}

	upstreamBranch := options.upstream
	if upstreamBranch == "" {
		resolvedUpstream, _, err := repository.remoteHeadBranch()
		if err != nil {
			return err
		}
		upstreamBranch = resolvedUpstream
	}

	if _, err := fmt.Fprintf(command.ErrOrStderr(), "%s\n", statusStyle.Render("creating "+filepath.Base(worktreePath))); err != nil {
		return err
	}

	if _, err := repository.git("worktree", "add", "-b", branchName, worktreePath, upstreamBranch); err != nil {
		return err
	}

	if err := repository.addBranchConfig(branchName, upstreamBranch); err != nil {
		return err
	}

	if _, err := repository.git("branch", "--set-upstream-to", upstreamBranch, branchName); err != nil {
		return err
	}

	_, err = fmt.Fprintf(command.ErrOrStderr(), "%s\n", statusStyle.Render("created "+worktreePath))
	return err
}
