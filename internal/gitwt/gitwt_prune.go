package gitwt

import (
	"fmt"
	"os"

	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"
)

// PruneCommand holds the flags and arguments for the prune subcommand.
type PruneCommand struct {
	prompt bool
}

// worktreeRemovalCandidate bundles a ManagedWorktree with its safety status.
type worktreeRemovalCandidate struct {
	worktree ManagedWorktree
	merged   bool
	clean    bool
}

func (w *worktreeRemovalCandidate) safeToAutoRemove() bool {
	return w.merged && w.clean
}

// Execute runs the prune subcommand logic.
//
// Clean Code rules applied:
//   - Single Responsibility: candidate evaluation, selection, and removal are separate steps.
//   - Intention-revealing names throughout.
func (p *PruneCommand) Execute(args []string) error {
	repoPath, err := resolveRepoDotGitDir()
	if err != nil {
		return err
	}

	candidates, err := p.evaluateCandidates(repoPath)
	if err != nil {
		return err
	}

	if len(candidates) == 0 {
		printStatus(os.Stderr, "No managed worktrees found to prune.")
		return nil
	}

	toRemove, err := p.selectWorktreesToRemove(candidates)
	if err != nil {
		return err
	}

	return p.removeCandidates(repoPath, toRemove)
}

// evaluateCandidates builds the list of worktrees with their merged/clean status.
func (p *PruneCommand) evaluateCandidates(repoPath string) ([]worktreeRemovalCandidate, error) {
	worktrees, err := listManagedWorktrees(repoPath)
	if err != nil {
		return nil, err
	}

	candidates := make([]worktreeRemovalCandidate, 0, len(worktrees))
	for _, wt := range worktrees {
		candidate, err := p.evaluateCandidate(wt)
		if err != nil {
			printWarning(os.Stderr, fmt.Sprintf("skipping %q: %v", wt.Branch, err))
			continue
		}
		candidates = append(candidates, candidate)
	}
	return candidates, nil
}

// evaluateCandidate checks the merged and clean status of a single worktree.
func (p *PruneCommand) evaluateCandidate(wt ManagedWorktree) (worktreeRemovalCandidate, error) {
	upstream, err := resolveBranchUpstream(wt.Path, wt.Branch)
	if err != nil {
		return worktreeRemovalCandidate{}, err
	}

	merged, err := isBranchMergedToUpstream(wt.Path, upstream)
	if err != nil {
		return worktreeRemovalCandidate{}, err
	}

	clean, err := isWorktreeClean(wt.Path)
	if err != nil {
		return worktreeRemovalCandidate{}, err
	}

	return worktreeRemovalCandidate{worktree: wt, merged: merged, clean: clean}, nil
}

// selectWorktreesToRemove decides which worktrees will be removed.
// Merged+clean ones are auto-selected; unmerged/unclean ones are included only
// if --prompt is set and the user selects them.
func (p *PruneCommand) selectWorktreesToRemove(candidates []worktreeRemovalCandidate) ([]worktreeRemovalCandidate, error) {
	if !p.prompt {
		return filterAutoRemovable(candidates), nil
	}
	return p.promptUserForSelection(candidates)
}

// filterAutoRemovable returns only candidates that are both merged and clean.
func filterAutoRemovable(candidates []worktreeRemovalCandidate) []worktreeRemovalCandidate {
	var result []worktreeRemovalCandidate
	for _, c := range candidates {
		if c.safeToAutoRemove() {
			result = append(result, c)
		}
	}
	return result
}

// promptUserForSelection presents a multi-select form with merged/clean entries
// pre-selected so the user can confirm or add unmerged/unclean entries.
func (p *PruneCommand) promptUserForSelection(candidates []worktreeRemovalCandidate) ([]worktreeRemovalCandidate, error) {
	options := make([]huh.Option[string], len(candidates))
	for i, c := range candidates {
		label := formatCandidateLabel(c)
		options[i] = huh.NewOption(label, c.worktree.Branch).Selected(c.safeToAutoRemove())
	}

	var selectedBranches []string
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewMultiSelect[string]().
				Title("Select worktrees to prune").
				Description("Merged+clean entries are pre-selected. Unmerged/unclean entries require manual selection.").
				Options(options...).
				Value(&selectedBranches),
		),
	)

	if err := form.Run(); err != nil {
		return nil, fmt.Errorf("prompt: %w", err)
	}

	return filterByBranchNames(candidates, selectedBranches), nil
}

// formatCandidateLabel builds a human-readable label for a worktree candidate.
func formatCandidateLabel(c worktreeRemovalCandidate) string {
	status := ""
	if !c.merged {
		status += " [unmerged]"
	}
	if !c.clean {
		status += " [dirty]"
	}
	return c.worktree.Branch + status
}

// filterByBranchNames returns candidates whose branch name is in selectedBranches.
func filterByBranchNames(candidates []worktreeRemovalCandidate, selectedBranches []string) []worktreeRemovalCandidate {
	selected := make(map[string]bool, len(selectedBranches))
	for _, b := range selectedBranches {
		selected[b] = true
	}
	var result []worktreeRemovalCandidate
	for _, c := range candidates {
		if selected[c.worktree.Branch] {
			result = append(result, c)
		}
	}
	return result
}

// removeCandidates removes each selected candidate worktree with --force since
// the user has already confirmed their selection.
func (p *PruneCommand) removeCandidates(repoPath string, candidates []worktreeRemovalCandidate) error {
	if len(candidates) == 0 {
		printStatus(os.Stderr, "Nothing to prune.")
		return nil
	}

	remover := &RemoveCommand{force: true}
	for _, c := range candidates {
		wt := c.worktree
		printStatus(os.Stderr, fmt.Sprintf("Pruning %q...", wt.Branch))
		if err := remover.removeWorktree(repoPath, wt); err != nil {
			printError(os.Stderr, fmt.Sprintf("failed to remove worktree %q: %v", wt.Branch, err))
			continue
		}
		if err := remover.deleteBranch(repoPath, wt.Branch); err != nil {
			printError(os.Stderr, fmt.Sprintf("failed to delete branch %q: %v", wt.Branch, err))
		}
	}
	return nil
}

// NewPruneCommand constructs the cobra.Command for `git-wt prune`.
func NewPruneCommand() *cobra.Command {
	p := &PruneCommand{}
	cmd := &cobra.Command{
		Use:   "prune",
		Short: "Prune merged and clean worktrees",
		Long: `Remove managed worktrees whose branches have been merged to upstream and whose
workdirs are clean.

Without --prompt, only merged+clean worktrees are removed silently.
With --prompt, a selection prompt is shown including unmerged/unclean worktrees.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return p.Execute(args)
		},
	}
	cmd.Flags().BoolVarP(&p.prompt, "prompt", "p", false, "interactively select worktrees to prune including unmerged/unclean ones")
	return cmd
}
