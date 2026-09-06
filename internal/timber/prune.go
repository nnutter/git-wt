package timber

import (
	"github.com/samber/lo"
	"github.com/spf13/cobra"
)

type pruneCommandOptions struct {
	repoSelection
	prompt   bool
	dryRun   bool
	prompter worktreePrompter
}

func NewPruneCommand(runtime Runtime) *cobra.Command {
	options := &pruneCommandOptions{
		runtime:  runtime,
		prompter: huhWorktreePrompter{},
	}

	command := &cobra.Command{
		Use:               "prune [@repo]",
		Aliases:           []string{"clean"},
		Short:             "Prune managed Git worktrees",
		Args:              cobra.MaximumNArgs(1),
		RunE:              options.Execute,
		ValidArgsFunction: runtime.completeRepoQualifiers,
	}

	command.Flags().BoolVarP(&options.prompt, "prompt", "p", false, "Prompt before pruning")
	command.Flags().BoolVarP(&options.dryRun, "dry-run", "n", false, "List worktrees that would be pruned")

	return command
}

func (x *pruneCommandOptions) Execute(command *cobra.Command, args []string) error {
	if len(args) == 1 {
		repo, err := x.runtime.parseRepoOnlyArg(args[0])
		if err != nil {
			return err
		}
		x.RepoName = repo
	}

	repos, err := x.reposToConsider()
	if err != nil {
		return err
	}

	enrichedWorktrees, err := x.runtime.collectManagedWorktrees(repos)
	if err != nil {
		return err
	}

	var selectedWorktrees []managedWorktree
	if x.prompt {
		selectedWorktrees, err = x.prompter.Prompt(command.InOrStdin(), command.ErrOrStderr(), enrichedWorktrees)
		if err != nil {
			return err
		}
	} else {
		selectedWorktrees = lo.Filter(enrichedWorktrees, func(worktree managedWorktree, _ int) bool {
			return worktree.Clean && worktree.Merged
		})
	}

	if x.dryRun {
		return x.runtime.reportPruneDryRun(command, selectedWorktrees)
	}

	removeOptions := &removeCommandOptions{repoSelection: x.repoSelection}
	for _, worktree := range selectedWorktrees {
		if !x.prompt && (!worktree.Clean || !worktree.Merged) {
			continue
		}
		if worktree.Repo != "" {
			removeOptions.RepoName = worktree.Repo
		}
		if err := removeOptions.removeWorktree(command, worktree.Name, true); err != nil {
			return err
		}
	}

	return nil
}
