package gitwt

import (
	"fmt"
	"os"

	"charm.land/lipgloss/v2"
	"github.com/spf13/cobra"
)

type removeOptions struct {
	force bool
}

func NewRemoveCmd() *cobra.Command {
	opts := &removeOptions{}

	cmd := &cobra.Command{
		Use:   "remove <name>",
		Short: "Remove a Git worktree managed by git-wt",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return opts.Execute(args)
		},
	}

	cmd.Flags().BoolVarP(&opts.force, "force", "f", false, "Force removal even if not clean or unmerged")

	return cmd
}

func (o *removeOptions) Execute(args []string) error {
	name := args[0]

	worktrees, err := getGitWtWorktrees()
	if err != nil {
		return fmt.Errorf("failed to get worktrees: %w", err)
	}

	var targetWT *Worktree
	for _, wt := range worktrees {
		if wt.Branch == name {
			targetWT = &wt
			break
		}
	}

	if targetWT == nil {
		return fmt.Errorf("git-wt worktree for branch '%s' not found", name)
	}

	// Check clean
	clean, err := isClean(targetWT.Path)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to check if worktree is clean: %w", err)
	}
	if !clean && !o.force {
		return fmt.Errorf("worktree is not clean. Use --force to override")
	}

	// Check merged
	upstream, err := getUpstreamOfBranch(name)
	if err != nil {
		// If there is no upstream, we might want to warn or require force, but let's assume if it fails it's unmerged.
		if !o.force {
			return fmt.Errorf("failed to get upstream branch for %s (use --force to override): %w", name, err)
		}
	} else {
		merged, err := isMerged(name, upstream)
		if err != nil {
			return fmt.Errorf("failed to check if merged: %w", err)
		}
		if !merged && !o.force {
			return fmt.Errorf("branch '%s' is not merged to upstream '%s'. Use --force to override", name, upstream)
		}
	}

	// Remove worktree
	_, err = execGit("worktree", "remove", "--force", targetWT.Path)
	if err != nil {
		return fmt.Errorf("failed to remove worktree: %w", err)
	}

	// Warn if directory still exists
	if _, err := os.Stat(targetWT.Path); !os.IsNotExist(err) {
		warnStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("208"))
		fmt.Fprintln(os.Stderr, warnStyle.Render(fmt.Sprintf("Warning: worktree directory '%s' was not removed completely.", targetWT.Path)))
	}

	// Delete branch
	branchFlag := "-d"
	if o.force {
		branchFlag = "-D"
	}
	out, err := execGit("branch", branchFlag, name)
	if err != nil {
		return fmt.Errorf("failed to delete branch '%s': %w", name, err)
	}
	fmt.Fprintln(os.Stdout, out)

	return nil
}
