package gitwt

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// RemoveCommand holds the flags and arguments for the remove subcommand.
type RemoveCommand struct {
	force bool
}

// Execute runs the remove subcommand logic.
// args[0] is the branch name identifying the worktree to remove.
//
// Clean Code rules applied:
//   - Single Responsibility: safety checks, removal, and branch deletion are separate methods.
//   - Fail fast on unmet preconditions (not merged / not clean).
func (r *RemoveCommand) Execute(args []string) error {
	branchName := args[0]

	repoPath, err := resolveRepoDotGitDir()
	if err != nil {
		return err
	}

	wt, err := findManagedWorktree(repoPath, branchName)
	if err != nil {
		return err
	}

	if !r.force {
		if err := r.assertSafeToRemove(wt); err != nil {
			return err
		}
	}

	if err := r.removeWorktree(repoPath, wt); err != nil {
		return err
	}

	if err := r.deleteBranch(repoPath, branchName); err != nil {
		return err
	}

	printStatus(os.Stderr, fmt.Sprintf("Removed worktree %q", branchName))
	return nil
}

// assertSafeToRemove checks that the worktree's branch has been merged to its
// upstream and that the workdir is clean before allowing removal.
func (r *RemoveCommand) assertSafeToRemove(wt ManagedWorktree) error {
	clean, err := isWorktreeClean(wt.Path)
	if err != nil {
		return fmt.Errorf("check clean status: %w", err)
	}
	if !clean {
		return fmt.Errorf("worktree %q has uncommitted changes; use --force to override", wt.Branch)
	}

	upstream, err := resolveBranchUpstream(wt.Path, wt.Branch)
	if err != nil {
		return fmt.Errorf("resolve upstream for %q: %w", wt.Branch, err)
	}

	merged, err := isBranchMergedToUpstream(wt.Path, upstream)
	if err != nil {
		return fmt.Errorf("check merge status: %w", err)
	}
	if !merged {
		return fmt.Errorf("branch %q has not been merged to %s; use --force to override", wt.Branch, upstream)
	}

	return nil
}

// removeWorktree shells out to `git worktree remove` and warns if the directory
// is not actually removed afterwards.
func (r *RemoveCommand) removeWorktree(repoPath string, wt ManagedWorktree) error {
	gitArgs := []string{"worktree", "remove"}
	if r.force {
		gitArgs = append(gitArgs, "--force")
	}
	gitArgs = append(gitArgs, wt.Path)

	if err := runGitCommand(repoPath, gitArgs...); err != nil {
		return fmt.Errorf("remove worktree: %w", err)
	}

	if pathExists(wt.Path) {
		printWarning(os.Stderr, fmt.Sprintf("worktree directory still exists after removal: %s", wt.Path))
	}
	return nil
}

// deleteBranch deletes the local branch after the worktree is removed.
// Uses -D (force) when the RemoveCommand was invoked with --force.
func (r *RemoveCommand) deleteBranch(repoPath, branchName string) error {
	deleteFlag := "-d"
	if r.force {
		deleteFlag = "-D"
	}
	if err := runGitCommand(repoPath, "branch", deleteFlag, branchName); err != nil {
		return fmt.Errorf("delete branch: %w", err)
	}
	return nil
}

// NewRemoveCommand constructs the cobra.Command for `git-wt remove`.
func NewRemoveCommand() *cobra.Command {
	r := &RemoveCommand{}
	cmd := &cobra.Command{
		Use:   "remove <name>",
		Short: "Remove a managed worktree",
		Long: `Remove a Git worktree managed by git-wt and delete its local branch.

Removal requires the branch to be merged to its upstream and the workdir to be
clean unless --force is specified.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return r.Execute(args)
		},
	}
	cmd.Flags().BoolVarP(&r.force, "force", "f", false, "force removal even if branch is unmerged or workdir is dirty")
	return cmd
}
