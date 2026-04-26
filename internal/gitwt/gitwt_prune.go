package gitwt

import (
	"fmt"
	"os"

	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"
)

type pruneOptions struct {
	prompt bool
}

func NewPruneCmd() *cobra.Command {
	opts := &pruneOptions{}

	cmd := &cobra.Command{
		Use:   "prune",
		Short: "Prune merged and clean git-wt worktrees",
		RunE: func(cmd *cobra.Command, args []string) error {
			return opts.Execute(args)
		},
	}

	cmd.Flags().BoolVarP(&opts.prompt, "prompt", "p", false, "Prompt before pruning to allow force removal of skipped worktrees")

	return cmd
}

func (o *pruneOptions) Execute(args []string) error {
	worktrees, err := getGitWtWorktrees()
	if err != nil {
		return fmt.Errorf("failed to get worktrees: %w", err)
	}

	if len(worktrees) == 0 {
		return nil
	}

	type wtStatus struct {
		WT       Worktree
		CanPrune bool
		Reason   string
	}

	var statuses []wtStatus

	for _, wt := range worktrees {
		clean, err := isClean(wt.Path)
		reason := ""
		canPrune := false

		if err != nil && !os.IsNotExist(err) {
			reason = "failed to check if clean"
		} else if !clean {
			reason = "not clean"
		} else {
			upstream, err := getUpstreamOfBranch(wt.Branch)
			if err != nil {
				reason = "no upstream"
			} else {
				merged, err := isMerged(wt.Branch, upstream)
				if err != nil {
					reason = "failed to check if merged"
				} else if !merged {
					reason = "not merged"
				} else {
					canPrune = true
				}
			}
		}
		statuses = append(statuses, wtStatus{WT: wt, CanPrune: canPrune, Reason: reason})
	}

	var toRemove []string

	if o.prompt {
		var options []huh.Option[string]
		var selected []string

		for _, st := range statuses {
			label := st.WT.Branch
			if !st.CanPrune {
				label += fmt.Sprintf(" (%s)", st.Reason)
			}
			opts := huh.NewOption(label, st.WT.Branch)
			if st.CanPrune {
				opts = opts.Selected(true)
			}
			options = append(options, opts)
		}

		form := huh.NewForm(
			huh.NewGroup(
				huh.NewMultiSelect[string]().
					Title("Select worktrees to remove").
					Options(options...).
					Value(&selected),
			),
		)

		err := form.Run()
		if err != nil {
			return fmt.Errorf("prompt cancelled: %w", err)
		}

		toRemove = selected
	} else {
		for _, st := range statuses {
			if st.CanPrune {
				toRemove = append(toRemove, st.WT.Branch)
			}
		}
	}

	for _, branch := range toRemove {
		removeOpts := removeOptions{force: true}
		fmt.Fprintf(os.Stdout, "Removing worktree %s...\n", branch)
		err := removeOpts.Execute([]string{branch})
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to remove %s: %v\n", branch, err)
		}
	}

	return nil
}
