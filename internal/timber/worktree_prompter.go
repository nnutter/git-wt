package timber

import (
	"fmt"
	"io"
	"slices"

	"github.com/charmbracelet/huh"
	"github.com/samber/lo"
)

type worktreePrompter interface {
	Prompt(io.Reader, io.Writer, []managedWorktree) ([]managedWorktree, error)
}

type huhWorktreePrompter struct{}

func (huhWorktreePrompter) Prompt(input io.Reader, output io.Writer, worktrees []managedWorktree) ([]managedWorktree, error) {
	selectedKeys := make([]string, 0)
	options := lo.Map(worktrees, func(worktree managedWorktree, _ int) huh.Option[string] {
		label := fmt.Sprintf("%s %s (%s) %s", worktree.Repo, worktree.Name, worktree.DisplayPath, worktree.Status)
		if worktree.Clean {
			label += " (clean)"
		} else {
			label += " (dirty)"
		}
		option := huh.NewOption(label, pruneWorktreeKey(worktree))
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
				Value(&selectedKeys),
		),
	).WithInput(input).WithOutput(output)

	if err := form.Run(); err != nil {
		return nil, err
	}

	selectedWorktrees := make([]managedWorktree, 0, len(selectedKeys))
	for _, selectedKey := range selectedKeys {
		worktree, err := managedWorktreeByKey(worktrees, selectedKey)
		if err != nil {
			return nil, err
		}
		selectedWorktrees = append(selectedWorktrees, worktree)
	}

	return selectedWorktrees, nil
}

func pruneWorktreeKey(worktree managedWorktree) string {
	return worktree.Repo + "\x00" + worktree.Name
}

func managedWorktreeByKey(worktrees []managedWorktree, key string) (managedWorktree, error) {
	index := slices.IndexFunc(worktrees, func(worktree managedWorktree) bool {
		return pruneWorktreeKey(worktree) == key
	})
	if index >= 0 {
		return worktrees[index], nil
	}
	return managedWorktree{}, fmt.Errorf("unknown worktree %q", key)
}
