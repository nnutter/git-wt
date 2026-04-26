package gitwt

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

type RemoveCmd struct {
	Name  string
	Force bool
}

func (r *RemoveCmd) Execute(args []string) error {
	r.Name = args[0]

	dir, err := os.Getwd()
	if err != nil {
		return err
	}

	path := WorktreePath(dir, r.Name)

	if _, err := os.Stat(path); os.IsNotExist(err) {
		return fmt.Errorf("worktree directory %q does not exist", path)
	}

	if !r.Force {
		upstream, err := gitBranchUpstream(dir, r.Name)
		if err != nil {
			return fmt.Errorf("failed to resolve upstream for branch %q: %w", r.Name, err)
		}

		merged, err := gitMergeBaseIsAncestor(dir, r.Name, gitResolveLocalBranch(dir, upstream))
		if err != nil {
			return fmt.Errorf("failed to check merge status: %w", err)
		}
		if !merged {
			return fmt.Errorf("branch %q has not been merged to %q; use --force to override", r.Name, upstream)
		}

		clean, err := gitWorkdirClean(path)
		if err != nil {
			return fmt.Errorf("failed to check workdir status: %w", err)
		}
		if !clean {
			return fmt.Errorf("workdir is not clean; use --force to override")
		}
	}

	if err := gitWorktreeRemove(dir, path, r.Force); err != nil {
		return fmt.Errorf("failed to remove worktree: %w", err)
	}

	if _, err := os.Stat(path); err == nil {
		printWarn("directory %q still exists after remove", path)
	}

	exists, err := gitBranchExists(dir, r.Name)
	if err == nil && exists {
		if err := gitBranchDelete(dir, r.Name, r.Force); err != nil {
			printWarn("failed to delete branch %q: %v", r.Name, err)
		}
	}

	printInfo("Removed worktree %q", r.Name)
	return nil
}

func NewRemove() *cobra.Command {
	r := &RemoveCmd{}
	cmd := &cobra.Command{
		Use:   "remove <name>",
		Short: "Remove a worktree",
		Long:  "Remove a Git worktree and its associated branch.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return r.Execute(args)
		},
	}
	cmd.Flags().BoolVarP(&r.Force, "force", "f", false, "force removal even if unmerged or unclean")
	return cmd
}
