package gitwt

import (
"fmt"
"os"

"github.com/charmbracelet/huh"
"github.com/spf13/cobra"
)

type PruneCmd struct {
	flags struct {
		prompt bool
	}
}

func NewPruneCmd() *cobra.Command {
	cmd := &PruneCmd{}
	c := &cobra.Command{
		Use:   `prune [-p|--prompt]`,
		Short: `Prune Git worktrees with merged branches`,
		Args:  cobra.NoArgs,
		RunE:  cmd.Execute,
	}
	c.Flags().BoolVarP(&cmd.flags.prompt, `prompt`, `p`, false, `Prompt before removing worktrees`)
	return c
}

type pruneItem struct {
	name string
	path string
}

func (c *PruneCmd) Execute(cmd *cobra.Command, args []string) error {
	mainDir, err := mainWorktreeDir()
	if err != nil {
		return err
	}
	items, err := collectPruneCandidates(mainDir)
	if err != nil {
		return err
	}
	if len(items) == 0 {
		return nil
	}
	if c.flags.prompt {
		return c.promptRemove(items)
	}
	return c.silentRemove(items)
}

func collectPruneCandidates(mainDir string) ([]pruneItem, error) {
	worktrees, err := listWorktrees()
	if err != nil {
		return nil, err
	}
	var candidates []pruneItem
	for _, wt := range worktrees {
		if wt.wtPath == mainDir {
			continue
		}
		name := nameFromPath(mainDir, wt.wtPath)
		if name == `` {
			continue
		}
		branch := name
		upstream, err := getUpstream(branch)
		if err != nil {
			continue
		}
		merged, err := isAncestor(branch, upstream)
		if err != nil || !merged {
			continue
		}
		clean, err := isWorktreeClean(wt.wtPath)
		if err != nil || !clean {
			continue
		}
		candidates = append(candidates, pruneItem{name: name, path: wt.wtPath})
	}
	return candidates, nil
}

func (c *PruneCmd) silentRemove(items []pruneItem) error {
	for _, item := range items {
		branch := item.name
		if err := removeWorktree(item.path, false); err != nil {
			warningf(`failed to remove %s: %v`, item.name, err)
			continue
		}
		if err := deleteBranch(branch, false); err != nil {
			warningf(`failed to delete branch %s: %v`, branch, err)
		}
		fmt.Fprintf(os.Stderr, "Pruned %s\n", item.name)
	}
	return nil
}

func (c *PruneCmd) promptRemove(items []pruneItem) error {
	var options []huh.Option[pruneItem]
	for _, item := range items {
		options = append(options, huh.NewOption(item.name, item))
	}
	selected := make([]pruneItem, 0, len(items))
	for _, item := range items {
		selected = append(selected, item)
	}
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewMultiSelect[pruneItem]().
				Title(`Select worktrees to remove`).
				Options(options...).
				Value(&selected),
		),
	)
	if err := form.Run(); err != nil {
		return err
	}
	for _, item := range selected {
		branch := item.name
		if err := removeWorktree(item.path, true); err != nil {
			warningf(`failed to remove %s: %v`, item.name, err)
			continue
		}
		if err := deleteBranch(branch, true); err != nil {
			warningf(`failed to delete branch %s: %v`, branch, err)
		}
		fmt.Fprintf(os.Stderr, "Pruned %s\n", item.name)
	}
	return nil
}