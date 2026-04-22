package gitwt

import (
	"fmt"
	"io"

	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"
)

type pruneCommandOptions struct {
	prompt   bool
	runner   gitRunner
	prompter worktreePrompter
}

type huhWorktreePrompter struct{}

func NewPruneCommand() *cobra.Command {
	options := &pruneCommandOptions{
		runner:   gitRunner{},
		prompter: huhWorktreePrompter{},
	}

	command := &cobra.Command{
		Use:   "prune",
		Short: "Prune managed Git worktrees",
		Args:  cobra.NoArgs,
		RunE:  options.Execute,
	}

	command.Flags().BoolVarP(&options.prompt, "prompt", "p", false, "Prompt before pruning")

	return command
}

func (options *pruneCommandOptions) Execute(command *cobra.Command, args []string) error {
	worktrees, repoPath, err := managedWorktreesFromRepository(options.runner, ".")
	if err != nil {
		return err
	}

	enrichedWorktrees := make([]managedWorktree, 0, len(worktrees))
	for _, worktree := range worktrees {
		enrichedWorktree, err := enrichManagedWorktree(options.runner, repoPath, worktree)
		if err != nil {
			return err
		}
		enrichedWorktrees = append(enrichedWorktrees, enrichedWorktree)
	}

	selectedWorktrees := make([]managedWorktree, 0)
	if options.prompt {
		selectedWorktrees, err = options.prompter.Prompt(command.InOrStdin(), command.ErrOrStderr(), enrichedWorktrees)
		if err != nil {
			return err
		}
	} else {
		for _, worktree := range enrichedWorktrees {
			if worktree.Clean && worktree.Merged {
				selectedWorktrees = append(selectedWorktrees, worktree)
			}
		}
	}

	removeOptions := &removeCommandOptions{runner: options.runner}
	for _, worktree := range selectedWorktrees {
		if !options.prompt && (!worktree.Clean || !worktree.Merged) {
			continue
		}
		if err := removeOptions.removeWorktree(command, worktree.Name, true); err != nil {
			return err
		}
	}

	return nil
}

func (huhWorktreePrompter) Prompt(input io.Reader, output io.Writer, worktrees []managedWorktree) ([]managedWorktree, error) {
	selectedNames := make([]string, 0)
	options := make([]huh.Option[string], 0, len(worktrees))
	for _, worktree := range worktrees {
		label := fmt.Sprintf("%s (%s)", worktree.Name, worktree.DisplayPath)
		option := huh.NewOption(label, worktree.Name)
		if worktree.Clean && worktree.Merged {
			option = option.Selected(true)
		}
		options = append(options, option)
	}

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
