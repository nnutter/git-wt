package gitwt

import (
	"fmt"
	"io"

	"github.com/charmbracelet/huh"
	"github.com/samber/lo"
	"github.com/spf13/cobra"
)

type worktreePrompter interface {
	Prompt(io.Reader, io.Writer, []managedWorktree) ([]managedWorktree, error)
}

type pruneCommandOptions struct {
	repoSelection
	prompt   bool
	dryRun   bool
	prompter worktreePrompter
}

type huhWorktreePrompter struct{}

func NewPruneCommand() *cobra.Command {
	options := &pruneCommandOptions{
		prompter: huhWorktreePrompter{},
	}

	command := &cobra.Command{
		Use:   "prune",
		Short: "Prune managed Git worktrees",
		Args:  cobra.NoArgs,
		RunE:  options.Execute,
	}

	options.addRepoFlag(command)
	command.Flags().BoolVarP(&options.prompt, "prompt", "p", false, "Prompt before pruning")
	command.Flags().BoolVarP(&options.dryRun, "dry-run", "n", false, "List worktrees that would be pruned")

	return command
}

func (x *pruneCommandOptions) Execute(command *cobra.Command, args []string) error {
	repo, repository, err := x.resolve()
	if err != nil {
		return err
	}

	worktrees, err := managedWorktreesFromRepository(repository, repo.Name)
	if err != nil {
		return err
	}

	enrichedWorktrees := make([]managedWorktree, 0, len(worktrees))
	for _, worktree := range worktrees {
		enrichedWorktree, err := enrichManagedWorktree(repository, worktree)
		if err != nil {
			return err
		}
		enrichedWorktrees = append(enrichedWorktrees, enrichedWorktree)
	}

	selectedWorktrees := make([]managedWorktree, 0)
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
		return reportPruneDryRun(command, selectedWorktrees)
	}

	removeOptions := &removeCommandOptions{repoSelection: x.repoSelection}
	for _, worktree := range selectedWorktrees {
		if !x.prompt && (!worktree.Clean || !worktree.Merged) {
			continue
		}
		if err := removeOptions.removeWorktree(command, worktree.Name, true); err != nil {
			return err
		}
	}

	return nil
}

func reportPruneDryRun(command *cobra.Command, worktrees []managedWorktree) error {
	for _, worktree := range worktrees {
		message := fmt.Sprintf(
			"would prune %s (%s) at %s",
			worktree.Name,
			worktree.Repo,
			displayHomePath(worktree.Path),
		)
		if _, err := fmt.Fprintf(command.ErrOrStderr(), "%s\n", statusStyle.Render(message)); err != nil {
			return err
		}
	}
	return nil
}

func (huhWorktreePrompter) Prompt(input io.Reader, output io.Writer, worktrees []managedWorktree) ([]managedWorktree, error) {
	selectedNames := make([]string, 0)
	options := lo.Map(worktrees, func(worktree managedWorktree, _ int) huh.Option[string] {
		label := fmt.Sprintf("%s (%s) %s", worktree.Name, worktree.DisplayPath, worktree.Status)
		if worktree.Clean {
			label += " (clean)"
		} else {
			label += " (dirty)"
		}
		option := huh.NewOption(label, worktree.Name)
		if worktree.Clean && worktree.Merged {
			option = option.Selected(true)
		}
		return option
	})

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewMultiSelect[string]().
				Title("Select worktrees to prune").
				Options(options...).
				Value(&selectedNames),
		),
	).WithInput(input).WithOutput(output)

	if err := form.Run(); err != nil {
		return nil, err
	}

	selectedWorktrees := make([]managedWorktree, 0, len(selectedNames))
	for _, selectedName := range selectedNames {
		worktree, err := managedWorktreeByName(worktrees, selectedName)
		if err != nil {
			return nil, err
		}
		selectedWorktrees = append(selectedWorktrees, worktree)
	}

	return selectedWorktrees, nil
}
